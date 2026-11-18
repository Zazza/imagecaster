package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

var _ ShootQueue = (*queue.Store)(nil)

type sqEnv struct {
	s       *Server
	store   *queue.Store
	outDir  string
	people  *fakePeople
	shooter *fakeShooter
	sess    *fakeSessions
	refiner *fakeRefiner
}

func sqNewStore(t *testing.T) *queue.Store {
	t.Helper()
	pdb, err := preset.Open(":memory:")
	require.NoError(t, err)
	return queue.NewStore(pdb.DB())
}

func sqNewEnv(t *testing.T, eng StylistEngine, cfg *config.Config) *sqEnv {
	t.Helper()
	if eng == nil {
		eng = &sgEngine{}
	}
	env := &sqEnv{
		store:   sqNewStore(t),
		outDir:  t.TempDir(),
		people:  &fakePeople{people: shootPeopleAll()},
		shooter: &fakeShooter{},
		sess:    &fakeSessions{},
		refiner: &fakeRefiner{size: 256},
	}
	env.s = NewServer(Deps{
		OutDir:   env.outDir,
		People:   env.people,
		Shooter:  env.shooter,
		Sessions: env.sess,
		Stylist:  eng,
		Refiner:  env.refiner,
		Queue:    env.store,
		Cfg:      cfg,
	})
	RegisterTools(env.s)
	return env
}

func (e *sqEnv) activeJobs(t *testing.T) int {
	t.Helper()
	n, err := e.store.ActiveCount()
	require.NoError(t, err)
	return n
}

func sqJobIDs(t *testing.T, m map[string]any) []int64 {
	t.Helper()
	raw, ok := m["jobs"].([]any)
	require.True(t, ok, "jobs must be an array: %v", m)
	out := make([]int64, 0, len(raw))
	for _, v := range raw {
		f, ok := v.(float64)
		require.True(t, ok, "job id must be a number: %v", v)
		out = append(out, int64(f))
	}
	return out
}

func sqJob(t *testing.T, store *queue.Store, id int64) (*queue.Job, ShootFrameParams) {
	t.Helper()
	job, err := store.Get(id)
	require.NoError(t, err)
	require.NotNil(t, job, "job %d", id)
	var p ShootFrameParams
	require.NoError(t, json.Unmarshal([]byte(job.Params), &p), job.Params)
	return job, p
}

func sqLabel(t *testing.T, job *queue.Job) string {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(job.ProgressDetail), &m), job.ProgressDetail)
	label, _ := m["label"].(string)
	return label
}

func sqIDsArg(ids ...int64) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, float64(id))
	}
	return out
}

func sqFrames(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["frames"].([]any)
	require.True(t, ok, "frames must be an array: %v", m)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		obj, ok := v.(map[string]any)
		require.True(t, ok)
		out = append(out, obj)
	}
	return out
}

func sqNoSyncWork(t *testing.T, env *sqEnv, eng *sgEngine) {
	t.Helper()
	assert.Empty(t, env.shooter.genCalls(), "stylist_shoot must not generate itself")
	assert.Empty(t, env.sess.namedCalls(), "stylist_shoot must not save frames itself")
	assert.Empty(t, env.refiner.refineCalls(), "stylist_shoot must not run the face pass itself")
	if eng != nil {
		assert.Empty(t, eng.calls(), "stylist_shoot must not run the try-on itself")
	}
}

type sqFailQueue struct {
	mu    sync.Mutex
	calls int
}

func (q *sqFailQueue) CreateLabeled(queue.JobType, string, string, string) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls++
	return 0, errors.New("database is locked")
}

func (q *sqFailQueue) Get(int64) (*queue.Job, error) {
	return nil, nil
}

type sqErrShooter struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (s *sqErrShooter) TestGenerate(generation.TestGenerateParams) ([]generation.TestGenerateResultItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return nil, s.err
}

type sqEmitter struct{}

func (sqEmitter) Emit(string, ...any) {}

func TestStylistShootQueue_OneJobPerShot(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sqNewEnv(t, eng, nil)
	shots := []string{"front", "side", "back"}

	m, out := styCall(t, env.s, "stylist_shoot", shootArgs("anna", shots...))

	sqNoSyncWork(t, env, eng)
	ids := sqJobIDs(t, m)
	require.Len(t, ids, 3, out)
	assert.Equal(t, 3, env.activeJobs(t))
	var prompts []string
	for i, id := range ids {
		if i > 0 {
			assert.Greater(t, id, ids[i-1], "jobs must be created in shot order")
		}
		job, p := sqJob(t, env.store, id)
		assert.Equal(t, queue.JobStylistFrame, job.Type, "job %d", i)
		assert.Equal(t, queue.StatusPending, job.Status, "job %d", i)
		assert.Equal(t, "stylist", job.Source, "job %d", i)
		assert.Equal(t, fmt.Sprintf("Стилист: anna, %s (%d из 3)", shots[i], i+1), sqLabel(t, job), "job %d", i)

		assert.Equal(t, "anna", p.Person, "job %d", i)
		assert.Equal(t, "Z-Image", p.Base, "job %d", i)
		assert.Equal(t, "z-image-turbo-fp8-aio", p.Model, "job %d", i)
		assert.Equal(t, shots[i], p.Shot, "job %d", i)
		assert.Equal(t, i+1, p.Index, "job %d", i)
		assert.Equal(t, 3, p.Total, "job %d", i)
		assert.Nil(t, p.Seed, "job %d", i)
		assert.True(t, p.FacePass, "job %d", i)
		assert.Equal(t, "a red silk midi dress with puff sleeves", p.Outfit, "job %d", i)
		assert.Empty(t, p.GarmentBase64, "job %d", i)
		assert.Contains(t, p.Prompt, "slim woman, fair skin, shoulder-length wavy auburn hair", "job %d", i)
		assert.Contains(t, p.Prompt, "a red silk midi dress with puff sleeves", "job %d", i)
		assert.Contains(t, p.Prompt, shots[i], "job %d", i)
		assert.NotContains(t, p.Prompt, "anna_trig", "job %d", i)
		assert.True(t, strings.HasSuffix(strings.TrimSpace(p.Prompt), "<lora:anna_lora_v1:1>"), "job %d prompt: %q", i, p.Prompt)
		prompts = append(prompts, p.Prompt)
	}
	assert.Len(t, map[string]bool{prompts[0]: true, prompts[1]: true, prompts[2]: true}, 3)
}

func TestStylistShootQueue_Response(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sqNewEnv(t, eng, nil)
	garment, gdata := garmentWrite(t)

	m, out := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front", "side"))

	assert.Len(t, sqJobIDs(t, m), 2)
	assert.Equal(t, "anna", m["person"])
	assert.Equal(t, "Z-Image", m["base"])
	assert.Equal(t, "z-image-turbo-fp8-aio", m["model"])
	hint, _ := m["hint"].(string)
	require.NotEmpty(t, strings.TrimSpace(hint), out)
	assert.Contains(t, strings.ToLower(hint), "приложени")
	assert.NotContains(t, m, "variants")
	lower := strings.ToLower(out)
	for _, f := range []string{".png", ".jpg", "iVBORw0KGgo", "image_base64", "garment_base64", env.outDir, garment, filepath.Dir(garment), base64.StdEncoding.EncodeToString(gdata)} {
		assert.NotContains(t, lower, strings.ToLower(f))
	}
}

func TestStylistShootQueue_ParamsByBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		person string
		base   string
		model  string
	}{
		{person: "anna", base: "Z-Image", model: "z-image-turbo-fp8-aio"},
		{person: "boris", base: "Moss-Image", model: "moss_image_sft_bf16"},
		{person: "vera", base: "SDXL", model: "juggernautXL_ragnarokBy"},
		{person: "gleb", base: "Flux.2", model: "flux-2-klein-4b"},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			t.Parallel()
			env := sqNewEnv(t, nil, nil)

			m, _ := styCall(t, env.s, "stylist_shoot", shootArgs(tt.person, "front"))

			ids := sqJobIDs(t, m)
			require.Len(t, ids, 1)
			job, p := sqJob(t, env.store, ids[0])
			assert.Equal(t, tt.person, p.Person)
			assert.Equal(t, tt.base, p.Base)
			assert.Equal(t, tt.model, p.Model)
			assert.Equal(t, fmt.Sprintf("Стилист: %s, front (1 из 1)", tt.person), sqLabel(t, job))
			assert.Equal(t, tt.base, m["base"])
			assert.Equal(t, tt.model, m["model"])
			assert.Empty(t, env.shooter.genCalls())
		})
	}
}

func TestStylistShootQueue_Seed(t *testing.T) {
	t.Parallel()
	env := sqNewEnv(t, nil, nil)
	args := shootArgs("anna", "a", "b", "c")
	args["seed"] = float64(1000)

	m, _ := styCall(t, env.s, "stylist_shoot", args)

	ids := sqJobIDs(t, m)
	require.Len(t, ids, 3)
	for i, id := range ids {
		_, p := sqJob(t, env.store, id)
		require.NotNil(t, p.Seed, "job %d seed", i)
		assert.Equal(t, int64(1000+i), *p.Seed, "job %d seed", i)
	}
}

func TestStylistShootQueue_DefaultShots(t *testing.T) {
	t.Parallel()
	env := sqNewEnv(t, nil, nil)

	m, _ := styCall(t, env.s, "stylist_shoot", shootArgs("anna"))

	ids := sqJobIDs(t, m)
	require.Len(t, ids, 4)
	shots := map[string]bool{}
	for i, id := range ids {
		_, p := sqJob(t, env.store, id)
		assert.Equal(t, i+1, p.Index)
		assert.Equal(t, 4, p.Total)
		require.NotEmpty(t, p.Shot)
		shots[p.Shot] = true
	}
	assert.Len(t, shots, 4)
	assert.Empty(t, env.shooter.genCalls())
}

func TestStylistShootQueue_FacePassFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		set  any
		want bool
	}{
		{name: "on", set: true, want: true},
		{name: "off", set: false, want: false},
		{name: "default off", set: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := sqNewEnv(t, nil, nil)
			args := shootArgs("anna", "front")
			delete(args, "face_pass")
			if tt.set != nil {
				args["face_pass"] = tt.set
			}

			m, _ := styCall(t, env.s, "stylist_shoot", args)

			ids := sqJobIDs(t, m)
			require.Len(t, ids, 1)
			_, p := sqJob(t, env.store, ids[0])
			assert.Equal(t, tt.want, p.FacePass)
		})
	}
}

func TestStylistShootQueue_Garment(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sqNewEnv(t, eng, nil)
	garment, gdata := garmentWrite(t)

	m, _ := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front", "side"))

	sqNoSyncWork(t, env, eng)
	ids := sqJobIDs(t, m)
	require.Len(t, ids, 2)
	for i, id := range ids {
		_, p := sqJob(t, env.store, id)
		require.NotEmpty(t, p.GarmentBase64, "job %d must carry the garment photo", i)
		assert.Equal(t, gdata, styDecodeB64(t, p.GarmentBase64), "job %d", i)
		assert.Equal(t, "a red silk midi dress with puff sleeves", p.Outfit, "job %d", i)
	}
}

func TestStylistShootQueue_ValidationBeforeEnqueue(t *testing.T) {
	t.Parallel()
	seven := []any{"a", "b", "c", "d", "e", "f", "g"}
	tests := []struct {
		name   string
		eng    StylistEngine
		cfg    *config.Config
		mutate func(t *testing.T, args map[string]any)
	}{
		{name: "person missing", mutate: func(_ *testing.T, a map[string]any) { delete(a, "person") }},
		{name: "unknown person", mutate: func(_ *testing.T, a map[string]any) { a["person"] = "nobody" }},
		{name: "person with unsupported base", mutate: func(_ *testing.T, a map[string]any) { a["person"] = "dina" }},
		{name: "appearance blank", mutate: func(_ *testing.T, a map[string]any) { a["appearance"] = "   " }},
		{name: "appearance missing", mutate: func(_ *testing.T, a map[string]any) { delete(a, "appearance") }},
		{name: "seven shots", mutate: func(_ *testing.T, a map[string]any) { a["shots"] = seven }},
		{name: "garment file missing", mutate: func(t *testing.T, a map[string]any) {
			a["garment_path"] = filepath.Join(t.TempDir(), "nope.png")
		}},
		{name: "garment is a url", mutate: func(_ *testing.T, a map[string]any) { a["garment_path"] = "http://127.0.0.1:1/garment.png" }},
		{name: "engine without try-on", eng: &stubFaceEng{}, mutate: func(t *testing.T, a map[string]any) {
			g, _ := garmentWrite(t)
			a["garment_path"] = g
		}},
		{name: "cloud LLM", cfg: &config.Config{LLMUrl: "https://api.openai.com/v1", SDUrl: "http://127.0.0.1:7860"}, mutate: func(*testing.T, map[string]any) {}},
		{name: "cloud SD", cfg: &config.Config{LLMUrl: "http://localhost:1234", SDUrl: "http://8.8.8.8:7860"}, mutate: func(*testing.T, map[string]any) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := sqNewEnv(t, tt.eng, tt.cfg)
			args := shootArgs("anna", "front")
			tt.mutate(t, args)

			out, err := styExec(env.s, "stylist_shoot", args)

			require.Error(t, err, out)
			assert.Zero(t, env.activeJobs(t), "no job may be created on validation error")
			assert.Empty(t, env.shooter.genCalls())
			assert.Empty(t, env.sess.namedCalls())
		})
	}
}

func TestStylistShootQueue_PeopleServerError(t *testing.T) {
	t.Parallel()
	env := sqNewEnv(t, nil, nil)
	env.people.err = errors.New("connection refused")

	_, err := styExec(env.s, "stylist_shoot", shootArgs("anna", "front"))

	require.Error(t, err)
	assert.Zero(t, env.activeJobs(t))
}

func TestStylistShootQueue_CreateError(t *testing.T) {
	t.Parallel()
	q := &sqFailQueue{}
	shooter := &fakeShooter{}
	sess := &fakeSessions{}
	s := NewServer(Deps{OutDir: t.TempDir(), People: &fakePeople{people: shootPeopleAll()}, Shooter: shooter, Sessions: sess, Stylist: &sgEngine{}, Queue: q})
	RegisterTools(s)

	out, err := styExec(s, "stylist_shoot", shootArgs("anna", "front", "side"))

	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "database is locked")
	assert.Positive(t, q.calls)
	assert.Empty(t, shooter.genCalls())
	assert.Empty(t, sess.namedCalls())
}

func TestStylistTry_UnchangedWithQueue(t *testing.T) {
	t.Parallel()
	store := sqNewStore(t)
	eng := &fakeStylist{}
	sess := &fakeSessions{}
	s := NewServer(Deps{OutDir: t.TempDir(), Stylist: eng, Sessions: sess, Queue: store})
	RegisterTools(s)
	photo, _ := styWritePhoto(t)

	m, _ := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(2)})

	assert.Len(t, eng.editCalls(), 2)
	assert.Len(t, sess.namedCalls(), 2)
	assert.Len(t, styVariants(t, m), 2)
	n, err := store.ActiveCount()
	require.NoError(t, err)
	assert.Zero(t, n, "stylist_try must stay synchronous")
}

func sqParamsJSON(t *testing.T, p ShootFrameParams) string {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	return string(raw)
}

func TestStylistShootStatus_Frames(t *testing.T) {
	t.Parallel()
	env := sqNewEnv(t, nil, nil)
	gb64 := base64.StdEncoding.EncodeToString(styPNG(5))
	mk := func(shot string, index int) int64 {
		p := ShootFrameParams{Person: "anna", Base: "Z-Image", Model: "z-image-turbo-fp8-aio", Shot: shot, Prompt: "p " + shot, Index: index, Total: 4, GarmentBase64: gb64, FacePass: true}
		id, err := env.store.CreateLabeled(queue.JobStylistFrame, sqParamsJSON(t, p), "stylist", "Стилист: anna, "+shot)
		require.NoError(t, err)
		return id
	}
	done := mk("front", 1)
	failed := mk("side", 2)
	running := mk("back", 3)
	pending := mk("walk", 4)
	require.NoError(t, env.store.UpdateCompleted(done, `{"session_item_id":101,"frame":{"id":101,"shot":"front","prompt":"p front","face_pass":"done","garment":"done"}}`))
	require.NoError(t, env.store.UpdateStatus(failed, queue.StatusFailed, "generation failed: out of memory"))
	require.NoError(t, env.store.UpdateStatus(running, queue.StatusRunning, ""))
	other, err := env.store.Create(queue.JobTxt2Img, `{"prompt":"a cat"}`, "mcp")
	require.NoError(t, err)

	m, out := styCall(t, env.s, "stylist_shoot_status", map[string]any{"job_ids": sqIDsArg(done, failed, running, pending, other, 99999)})

	frames := sqFrames(t, m)
	require.Len(t, frames, 6, out)
	wantIDs := []int64{done, failed, running, pending, other, 99999}
	for i, f := range frames {
		assert.Equal(t, float64(wantIDs[i]), f["job_id"], "frame %d", i)
	}

	assert.Equal(t, "completed", frames[0]["status"])
	assert.Equal(t, float64(101), frames[0]["id"])
	assert.Equal(t, "front", frames[0]["shot"])
	assert.Equal(t, float64(1), frames[0]["index"])
	assert.Equal(t, float64(4), frames[0]["total"])
	assert.Equal(t, "done", frames[0]["face_pass"])
	assert.Equal(t, "done", frames[0]["garment"])

	assert.Equal(t, "failed", frames[1]["status"])
	errText, _ := frames[1]["error"].(string)
	assert.Contains(t, errText, "out of memory")
	assert.Equal(t, "side", frames[1]["shot"])
	assert.Equal(t, float64(2), frames[1]["index"])
	assert.NotContains(t, frames[1], "id")

	assert.Equal(t, "running", frames[2]["status"])
	assert.Equal(t, "back", frames[2]["shot"])
	assert.NotContains(t, frames[2], "id")

	assert.Equal(t, "pending", frames[3]["status"])
	assert.Equal(t, "walk", frames[3]["shot"])
	assert.Equal(t, float64(4), frames[3]["index"])
	assert.Equal(t, float64(4), frames[3]["total"])
	assert.NotContains(t, frames[3], "id")

	assert.Equal(t, "not_found", frames[4]["status"], "a non-stylist job is not a frame")
	assert.Equal(t, "not_found", frames[5]["status"])

	for _, f := range []string{"iVBORw0KGgo", gb64, "garment_base64", "image_base64", ".png", env.outDir, "a cat"} {
		assert.NotContains(t, out, f)
	}
}

func TestStylistShootStatus_GarmentSkipped(t *testing.T) {
	t.Parallel()
	env := sqNewEnv(t, nil, nil)
	p := ShootFrameParams{Person: "anna", Base: "Z-Image", Model: "z-image-turbo-fp8-aio", Shot: "front", Prompt: "p", Index: 1, Total: 1}
	id, err := env.store.CreateLabeled(queue.JobStylistFrame, sqParamsJSON(t, p), "stylist", "Стилист: anna, front (1 из 1)")
	require.NoError(t, err)
	require.NoError(t, env.store.UpdateCompleted(id, `{"session_item_id":7,"frame":{"id":7,"shot":"front","prompt":"p","face_pass":"off"}}`))

	m, out := styCall(t, env.s, "stylist_shoot_status", map[string]any{"job_ids": sqIDsArg(id)})

	frames := sqFrames(t, m)
	require.Len(t, frames, 1, out)
	assert.Equal(t, "completed", frames[0]["status"])
	assert.Equal(t, float64(7), frames[0]["id"])
	assert.Equal(t, "off", frames[0]["face_pass"])
	assert.NotContains(t, frames[0], "garment", "garment is reported only when the frame had one")
}

func TestStylistShootStatus_BadArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "job_ids missing", args: map[string]any{}},
		{name: "job_ids empty", args: map[string]any{"job_ids": []any{}}},
		{name: "job_ids not an array", args: map[string]any{"job_ids": "1,2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := sqNewEnv(t, nil, nil)

			out, err := styExec(env.s, "stylist_shoot_status", tt.args)

			require.Error(t, err, out)
		})
	}
}

func TestStylistShootStatus_NoQueue(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)

	out, err := styExec(env.s, "stylist_shoot_status", map[string]any{"job_ids": sqIDsArg(1)})

	require.Error(t, err, out)
}

func sqFrameParams() ShootFrameParams {
	return ShootFrameParams{
		Person: "anna",
		Base:   "Z-Image",
		Model:  "z-image-turbo-fp8-aio",
		Shot:   "front full body",
		Prompt: "slim woman, front full body, studio <lora:anna_lora_v1:1>",
		Index:  1,
		Total:  2,
		Outfit: "a red silk midi dress with puff sleeves",
	}
}

func TestRunShootFrame_ParamsByBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		base    string
		model   string
		sampler string
		steps   int
		cfg     float64
	}{
		{base: "Z-Image", model: "z-image-turbo-fp8-aio", sampler: "euler", steps: 8, cfg: 1},
		{base: "Moss-Image", model: "moss_image_sft_bf16", sampler: "euler", steps: 30, cfg: 3},
		{base: "SDXL", model: "juggernautXL_ragnarokBy", sampler: "dpm++2m_sde", steps: 30, cfg: 6},
		{base: "Flux.2", model: "flux-2-klein-4b", sampler: "euler", steps: 4, cfg: 1},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			t.Parallel()
			shooter := &fakeShooter{}
			sess := &fakeSessions{}
			p := sqFrameParams()
			p.Base = tt.base
			p.Model = tt.model

			res, err := RunShootFrame(shooter, sess, &stubFaceEng{}, &fakeRefiner{size: 256}, p)

			require.NoError(t, err)
			calls := shooter.genCalls()
			require.Len(t, calls, 1)
			c := calls[0]
			assert.Equal(t, []string{tt.model}, c.SelectedModels)
			assert.Equal(t, tt.sampler, c.Sampler)
			assert.Equal(t, tt.steps, c.Steps)
			assert.InDelta(t, tt.cfg, c.CfgScale, 1e-9)
			assert.Equal(t, p.Prompt, c.Prompt)
			assert.Nil(t, c.Seed)
			require.NotNil(t, res)
			assert.Equal(t, "front full body", res.Shot)
			assert.Equal(t, p.Prompt, res.Prompt)
		})
	}
}

func TestRunShootFrame_SavesToStylistSession(t *testing.T) {
	t.Parallel()
	shooter := &fakeShooter{}
	sess := &fakeSessions{}
	eng := &sgEngine{}

	res, err := RunShootFrame(shooter, sess, eng, &fakeRefiner{size: 256}, sqFrameParams())

	require.NoError(t, err)
	require.NotNil(t, res)
	named := sess.namedCalls()
	require.Len(t, named, 1)
	assert.Equal(t, StylistSessionName, named[0].name)
	assert.Equal(t, "stylist", named[0].source)
	assert.Equal(t, shooter.genImages()[0], named[0].image)
	assert.Equal(t, named[0].returned, res.ID)
	assert.Equal(t, "off", res.FacePass)
	assert.Empty(t, res.Garment)
	assert.Empty(t, eng.calls(), "no try-on without a garment")
}

func TestRunShootFrame_Seed(t *testing.T) {
	t.Parallel()
	shooter := &fakeShooter{}
	p := sqFrameParams()
	seed := int64(1234)
	p.Seed = &seed

	_, err := RunShootFrame(shooter, &fakeSessions{}, &stubFaceEng{}, nil, p)

	require.NoError(t, err)
	calls := shooter.genCalls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].Seed)
	assert.Equal(t, int64(1234), *calls[0].Seed)
}

func TestRunShootFrame_Garment(t *testing.T) {
	t.Parallel()
	shooter := &fakeShooter{}
	sess := &fakeSessions{}
	eng := &sgEngine{}
	gdata := styPNG(5)
	p := sqFrameParams()
	p.GarmentBase64 = base64.StdEncoding.EncodeToString(gdata)

	res, err := RunShootFrame(shooter, sess, eng, nil, p)

	require.NoError(t, err)
	tries := eng.calls()
	require.Len(t, tries, 1)
	raw := shooter.genImages()
	require.Len(t, raw, 1)
	assert.Equal(t, raw[0], tries[0].image, "try-on dresses the generated frame")
	assert.Equal(t, gdata, styDecodeB64(t, tries[0].garment))
	assert.Contains(t, tries[0].instruction, "a red silk midi dress with puff sleeves")
	named := sess.namedCalls()
	require.Len(t, named, 1)
	assert.Equal(t, eng.tryOnResults()[0], named[0].image, "the try-on result is saved")
	assert.Equal(t, "done", res.Garment)
}

func TestRunShootFrame_GarmentTryOnFails(t *testing.T) {
	t.Parallel()
	shooter := &fakeShooter{}
	sess := &fakeSessions{}
	eng := &sgEngine{errOn: map[int]error{1: errors.New("klein try-on failed: out of memory")}}
	p := sqFrameParams()
	p.GarmentBase64 = base64.StdEncoding.EncodeToString(styPNG(5))

	res, err := RunShootFrame(shooter, sess, eng, nil, p)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, strings.HasPrefix(res.Garment, "skipped:"), "garment: %q", res.Garment)
	assert.NotEmpty(t, strings.TrimSpace(strings.TrimPrefix(res.Garment, "skipped:")))
	named := sess.namedCalls()
	require.Len(t, named, 1)
	assert.Equal(t, shooter.genImages()[0], named[0].image, "frame without the garment is saved as generated")
}

func TestRunShootFrame_GarmentWithoutTryOnEngine(t *testing.T) {
	t.Parallel()
	sess := &fakeSessions{}
	p := sqFrameParams()
	p.GarmentBase64 = base64.StdEncoding.EncodeToString(styPNG(5))

	_, err := RunShootFrame(&fakeShooter{}, sess, &stubFaceEng{}, nil, p)

	require.Error(t, err)
	assert.Empty(t, sess.namedCalls())
}

func TestRunShootFrame_FacePass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		eng      StylistEngine
		refiner  FaceRefiner
		want     string
		refined  bool
		keepsRaw bool
	}{
		{name: "done", eng: &stubFaceEng{}, refiner: &fakeRefiner{size: 256}, want: shootFaceDone, refined: true},
		{name: "no refiner", eng: &stubFaceEng{}, refiner: nil, want: shootFacePassMissing, keepsRaw: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shooter := &fakeShooter{}
			sess := &fakeSessions{}
			p := sqFrameParams()
			p.FacePass = true

			res, err := RunShootFrame(shooter, sess, tt.eng, tt.refiner, p)

			require.NoError(t, err)
			assert.Equal(t, tt.want, res.FacePass)
			named := sess.namedCalls()
			require.Len(t, named, 1)
			raw := shooter.genImages()[0]
			if tt.keepsRaw {
				assert.Equal(t, raw, named[0].image)
			} else {
				assert.NotEqual(t, raw, named[0].image, "face pass result is saved")
			}
			if fr, ok := tt.refiner.(*fakeRefiner); ok && tt.refined {
				calls := fr.refineCalls()
				require.Len(t, calls, 1)
				assert.Equal(t, p.Prompt, calls[0].Prompt)
			}
		})
	}
}

func TestRunShootFrame_FacePassOffSkipsRefiner(t *testing.T) {
	t.Parallel()
	refiner := &fakeRefiner{size: 256}
	sess := &fakeSessions{}

	res, err := RunShootFrame(&fakeShooter{}, sess, &stubFaceEng{}, refiner, sqFrameParams())

	require.NoError(t, err)
	assert.Equal(t, "off", res.FacePass)
	assert.Empty(t, refiner.refineCalls())
	assert.Len(t, sess.namedCalls(), 1)
}

func TestRunShootFrame_InterruptedNotSaved(t *testing.T) {
	t.Parallel()
	gb64 := base64.StdEncoding.EncodeToString(styPNG(5))
	tests := []struct {
		name     string
		shooter  StylistShooter
		eng      StylistEngine
		refiner  FaceRefiner
		garment  string
		facePass bool
	}{
		{name: "generation interrupted", shooter: &sqErrShooter{err: generation.ErrInterrupted}, eng: &stubFaceEng{}},
		{name: "generation wrapped interrupted", shooter: &sqErrShooter{err: fmt.Errorf("sd job: %w", generation.ErrInterrupted)}, eng: &stubFaceEng{}},
		{name: "try-on interrupted", shooter: &fakeShooter{}, eng: &sgEngine{errOn: map[int]error{1: generation.ErrInterrupted}}, garment: gb64},
		{name: "face pass interrupted", shooter: &fakeShooter{}, eng: &stubFaceEng{}, refiner: &fakeRefiner{err: generation.ErrInterrupted}, facePass: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sess := &fakeSessions{}
			p := sqFrameParams()
			p.GarmentBase64 = tt.garment
			p.FacePass = tt.facePass

			_, err := RunShootFrame(tt.shooter, sess, tt.eng, tt.refiner, p)

			require.Error(t, err)
			assert.True(t, generation.IsInterrupted(err), "error must be an interruption: %v", err)
			assert.Empty(t, sess.namedCalls(), "cancelled frame must not reach the session")
		})
	}
}

func TestRunShootFrame_GenerationFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		shooter StylistShooter
	}{
		{name: "generate returns error", shooter: &sqErrShooter{err: errors.New("dial tcp: connection refused")}},
		{name: "item has error", shooter: &fakeShooter{itemErrOn: map[int]bool{1: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sess := &fakeSessions{}

			_, err := RunShootFrame(tt.shooter, sess, &stubFaceEng{}, nil, sqFrameParams())

			require.Error(t, err)
			assert.False(t, generation.IsInterrupted(err))
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestRunShootFrame_UnknownBase(t *testing.T) {
	t.Parallel()
	shooter := &fakeShooter{}
	sess := &fakeSessions{}
	p := sqFrameParams()
	p.Base = "Chroma"

	_, err := RunShootFrame(shooter, sess, &stubFaceEng{}, nil, p)

	require.Error(t, err)
	assert.Empty(t, sess.namedCalls())
}

func TestNewShootFrameHandler(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, NewShootFrameHandler(&fakeSessions{}))
}

func TestStylistShootQueue_EndToEnd(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sqNewEnv(t, eng, nil)
	garment, _ := garmentWrite(t)
	runShooter := &fakeShooter{}
	runSess := &fakeSessions{}
	runRefiner := &fakeRefiner{size: 256}

	m, _ := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front", "side"))
	ids := sqJobIDs(t, m)
	require.Len(t, ids, 2)

	svc := queue.NewService(env.store, queue.NewProcessor(nil, nil, env.store, t.TempDir(), sqEmitter{}), sqEmitter{})
	svc.Handle(queue.JobStylistFrame, func(_ *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		var p ShootFrameParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, err
		}
		res, err := RunShootFrame(runShooter, runSess, eng, runRefiner, p)
		if err != nil {
			return nil, err
		}
		frame, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return &queue.JobResult{SessionItemID: res.ID, Frame: frame}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		svc.Stop(2 * time.Second)
	})
	svc.Start(ctx)
	require.Eventually(t, func() bool {
		for _, id := range ids {
			job, err := env.store.Get(id)
			if err != nil || job == nil || job.Status != queue.StatusCompleted {
				return false
			}
		}
		return true
	}, 5*time.Second, 20*time.Millisecond)

	sm, out := styCall(t, env.s, "stylist_shoot_status", map[string]any{"job_ids": sqIDsArg(ids...)})

	frames := sqFrames(t, sm)
	require.Len(t, frames, 2, out)
	named := runSess.namedCalls()
	require.Len(t, named, 2)
	for i, f := range frames {
		assert.Equal(t, "completed", f["status"], "frame %d", i)
		assert.Equal(t, float64(named[i].returned), f["id"], "frame %d", i)
		assert.Equal(t, StylistSessionName, named[i].name)
		assert.Equal(t, "done", f["garment"], "frame %d", i)
		assert.Equal(t, shootFaceDone, f["face_pass"], "frame %d", i)
		assert.Equal(t, float64(i+1), f["index"], "frame %d", i)
		assert.Equal(t, float64(2), f["total"], "frame %d", i)
		job, err := env.store.Get(ids[i])
		require.NoError(t, err)
		assert.NotContains(t, job.Params, "garment_base64", "garment photo must be removed after the frame is done")
	}
	assert.NotContains(t, out, "iVBORw0KGgo")
}
