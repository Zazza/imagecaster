package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

type fakePeople struct {
	mu     sync.Mutex
	people []serverclient.Person
	err    error
	calls  int
}

func (f *fakePeople) People(ctx context.Context) ([]serverclient.Person, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]serverclient.Person(nil), f.people...), nil
}

func (f *fakePeople) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeShooter struct {
	mu        sync.Mutex
	calls     []generation.TestGenerateParams
	images    []string
	failOn    map[int]bool
	itemErrOn map[int]bool
}

func (f *fakeShooter) TestGenerate(p generation.TestGenerateParams) ([]generation.TestGenerateResultItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := p
	cp.SelectedModels = append([]string(nil), p.SelectedModels...)
	if p.Seed != nil {
		v := *p.Seed
		cp.Seed = &v
	}
	f.calls = append(f.calls, cp)
	n := len(f.calls)
	if f.failOn[n] {
		return nil, fmt.Errorf("fake generate failure on call %d", n)
	}
	model := ""
	if len(p.SelectedModels) > 0 {
		model = p.SelectedModels[0]
	}
	if f.itemErrOn[n] {
		return []generation.TestGenerateResultItem{{Name: model, ModelName: model, Error: fmt.Sprintf("fake item failure %d", n)}}, nil
	}
	img := base64.StdEncoding.EncodeToString(styPNG(n))
	f.images = append(f.images, img)
	return []generation.TestGenerateResultItem{{Name: model, ModelName: model, Image: img, Seed: int64(n)}}, nil
}

func (f *fakeShooter) genCalls() []generation.TestGenerateParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]generation.TestGenerateParams(nil), f.calls...)
}

func (f *fakeShooter) genImages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.images...)
}

func shootTime() *time.Time {
	t := time.Date(2026, 11, 16, 11, 0, 0, 0, time.UTC)
	return &t
}

func shootPerson(name, base string) serverclient.Person {
	return serverclient.Person{
		Name:      name,
		Trigger:   name + "_trig",
		Gender:    "woman",
		Base:      base,
		Lora:      name + "_lora_v1",
		Steps:     2000,
		CreatedAt: shootTime(),
	}
}

func shootPeopleAll() []serverclient.Person {
	return []serverclient.Person{
		shootPerson("anna", "Z-Image"),
		shootPerson("boris", "Moss-Image"),
		shootPerson("vera", "SDXL"),
		shootPerson("gleb", "Flux.2"),
		shootPerson("dina", "Chroma"),
	}
}

type shootEnv struct {
	s       *Server
	outDir  string
	people  *fakePeople
	shooter *fakeShooter
	sess    *fakeSessions
	eng     *fakeStylist
}

func shootNewEnv(t *testing.T, people *fakePeople, shooter *fakeShooter, cfg *config.Config) *shootEnv {
	t.Helper()
	env := &shootEnv{outDir: t.TempDir(), people: people, shooter: shooter, sess: &fakeSessions{}, eng: &fakeStylist{}}
	deps := Deps{OutDir: env.outDir, Stylist: env.eng, Sessions: env.sess, Cfg: cfg}
	if people != nil {
		deps.People = people
	}
	if shooter != nil {
		deps.Shooter = shooter
	}
	env.s = NewServer(deps)
	RegisterTools(env.s)
	return env
}

func shootDefaultEnv(t *testing.T) *shootEnv {
	t.Helper()
	return shootNewEnv(t, &fakePeople{people: shootPeopleAll()}, &fakeShooter{}, nil)
}

func shootArgs(person string, shots ...string) map[string]any {
	args := map[string]any{
		"person":     person,
		"appearance": "slim woman, fair skin, shoulder-length wavy auburn hair",
		"look":       map[string]any{"outfit": "a red silk midi dress with puff sleeves"},
		"face_pass":  true,
	}
	if len(shots) > 0 {
		list := make([]any, 0, len(shots))
		for _, s := range shots {
			list = append(list, s)
		}
		args["shots"] = list
	}
	return args
}

func shootPeopleList(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["people"].([]any)
	require.True(t, ok, "people must be an array: %v", m)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		obj, ok := v.(map[string]any)
		require.True(t, ok)
		out = append(out, obj)
	}
	return out
}

func TestStylistPeople_ListsSupportedBases(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)

	m, out := styCall(t, env.s, "stylist_people", map[string]any{})

	list := shootPeopleList(t, m)
	require.Len(t, list, 4, out)
	byName := map[string]map[string]any{}
	for _, p := range list {
		name, _ := p["name"].(string)
		byName[name] = p
	}
	want := map[string]string{"anna": "Z-Image", "boris": "Moss-Image", "vera": "SDXL", "gleb": "Flux.2"}
	for name, base := range want {
		p, ok := byName[name]
		require.True(t, ok, "person %s missing: %s", name, out)
		assert.Equal(t, base, p["base"])
		assert.Equal(t, "woman", p["gender"])
		assert.NotEmpty(t, p["created_at"], "created_at of %s", name)
	}
	assert.NotContains(t, byName, "dina")
	assert.NotContains(t, out, "Chroma")
	assert.NotContains(t, out, "image_base64")
}

func TestStylistPeople_Empty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		people []serverclient.Person
	}{
		{name: "no people on server", people: nil},
		{name: "only unsupported bases", people: []serverclient.Person{shootPerson("dina", "Chroma")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootNewEnv(t, &fakePeople{people: tt.people}, &fakeShooter{}, nil)

			m, out := styCall(t, env.s, "stylist_people", map[string]any{})

			list, ok := m["people"].([]any)
			require.True(t, ok, "people must be an empty array, not null: %s", out)
			assert.Empty(t, list)
			assert.Contains(t, out, "stylist_try")
		})
	}
}

func TestStylistPeople_ServerError(t *testing.T) {
	t.Parallel()
	env := shootNewEnv(t, &fakePeople{err: errors.New("dial tcp 192.168.1.184:8080: connection refused")}, &fakeShooter{}, nil)

	out, err := styExec(env.s, "stylist_people", map[string]any{})

	require.Error(t, err, "server failure must not become an empty list: %s", out)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestStylistPeople_NoPeopleDep(t *testing.T) {
	t.Parallel()
	env := shootNewEnv(t, nil, &fakeShooter{}, nil)

	_, err := styExec(env.s, "stylist_people", map[string]any{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "недоступно")
}

func TestStylistShoot_ParamsByBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		person  string
		base    string
		model   string
		sampler string
		steps   int
		cfg     float64
	}{
		{person: "anna", base: "Z-Image", model: "z-image-turbo-fp8-aio", sampler: "euler", steps: 8, cfg: 1},
		{person: "boris", base: "Moss-Image", model: "moss_image_sft_bf16", sampler: "euler", steps: 30, cfg: 3},
		{person: "vera", base: "SDXL", model: "juggernautXL_ragnarokBy", sampler: "dpm++2m_sde", steps: 30, cfg: 6},
		{person: "gleb", base: "Flux.2", model: "flux-2-klein-4b", sampler: "euler", steps: 4, cfg: 1},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			t.Parallel()
			env := shootDefaultEnv(t)

			m, _ := styCall(t, env.s, "stylist_shoot", shootArgs(tt.person, "front full body", "side view"))

			calls := env.shooter.genCalls()
			require.Len(t, calls, 2)
			for i, c := range calls {
				assert.Equal(t, "models", c.Mode, "call %d", i)
				assert.Equal(t, []string{tt.model}, c.SelectedModels, "call %d", i)
				assert.Equal(t, tt.sampler, c.Sampler, "call %d", i)
				assert.Equal(t, tt.steps, c.Steps, "call %d", i)
				assert.InDelta(t, tt.cfg, c.CfgScale, 1e-9, "call %d", i)
				assert.Equal(t, shootWidth, c.Width, "call %d", i)
				assert.Equal(t, shootHeight, c.Height, "call %d", i)
			}
			assert.Equal(t, tt.person, m["person"])
			assert.Equal(t, tt.base, m["base"])
			assert.Equal(t, tt.model, m["model"])
		})
	}
}

func TestStylistShoot_ShotsCount(t *testing.T) {
	t.Parallel()
	six := []any{"a", "b", "c", "d", "e", "f"}
	seven := append(append([]any(nil), six...), "g")
	tests := []struct {
		name      string
		shots     any
		wantErr   bool
		wantCount int
	}{
		{name: "missing gives 4 defaults", shots: nil, wantCount: 4},
		{name: "empty gives 4 defaults", shots: []any{}, wantCount: 4},
		{name: "one", shots: []any{"front full body"}, wantCount: 1},
		{name: "six allowed", shots: six, wantCount: 6},
		{name: "seven rejected", shots: seven, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootDefaultEnv(t)
			args := shootArgs("anna")
			if tt.shots != nil {
				args["shots"] = tt.shots
			}

			out, err := styExec(env.s, "stylist_shoot", args)

			if tt.wantErr {
				require.Error(t, err, out)
				assert.Empty(t, env.shooter.genCalls())
				assert.Empty(t, env.sess.namedCalls())
				return
			}
			require.NoError(t, err)
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &m), out)
			assert.Len(t, styVariants(t, m), tt.wantCount)
			assert.Len(t, env.shooter.genCalls(), tt.wantCount)
			assert.Len(t, env.sess.namedCalls(), tt.wantCount)
		})
	}
}

func TestStylistShoot_DefaultShotsAreDistinct(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)

	m, _ := styCall(t, env.s, "stylist_shoot", shootArgs("anna"))

	variants := styVariants(t, m)
	require.Len(t, variants, 4)
	shots := map[string]bool{}
	prompts := map[string]bool{}
	for _, v := range variants {
		shot, _ := v["shot"].(string)
		require.NotEmpty(t, shot)
		shots[shot] = true
		prompt, _ := v["prompt"].(string)
		assert.Contains(t, prompt, shot)
		prompts[prompt] = true
	}
	assert.Len(t, shots, 4)
	assert.Len(t, prompts, 4)
}

func TestStylistShoot_Prompt(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)
	look := map[string]any{
		"hair_cut":   "chin-length bob",
		"hair_color": "platinum blonde",
		"outfit":     "a red silk midi dress with puff sleeves",
		"headwear":   "a black beret",
		"glasses":    "round tortoiseshell glasses",
		"makeup":     "red lipstick",
		"free":       "silver hoop earrings",
	}
	shots := []string{"front full body, hands on hips", "three-quarter view walking", "back view looking over shoulder"}
	args := map[string]any{
		"person":     "anna",
		"appearance": "slim woman, fair skin, shoulder-length wavy auburn hair",
		"look":       look,
		"shots":      []any{shots[0], shots[1], shots[2]},
		"background": "sunlit city street",
	}

	m, _ := styCall(t, env.s, "stylist_shoot", args)

	calls := env.shooter.genCalls()
	require.Len(t, calls, 3)
	variants := styVariants(t, m)
	require.Len(t, variants, 3)
	var common []string
	for i, c := range calls {
		p := c.Prompt
		assert.NotContains(t, p, "anna_trig", "call %d trigger", i)
		assert.Contains(t, p, "slim woman, fair skin, shoulder-length wavy auburn hair", "call %d appearance", i)
		for k, v := range look {
			assert.Contains(t, p, v.(string), "call %d look.%s", i, k)
		}
		assert.Contains(t, p, shots[i], "call %d shot", i)
		assert.Contains(t, p, "sunlit city street", "call %d background", i)
		assert.True(t, strings.HasSuffix(strings.TrimSpace(p), "<lora:anna_lora_v1:1>"), "call %d must end with lora tag: %q", i, p)
		assert.Equal(t, 1, strings.Count(p, shots[i]), "call %d shot once", i)
		common = append(common, strings.Replace(p, shots[i], "", 1))

		assert.Equal(t, shots[i], variants[i]["shot"], "variant %d shot", i)
		assert.Equal(t, p, variants[i]["prompt"], "variant %d prompt", i)
	}
	for i := 1; i < len(common); i++ {
		assert.Equal(t, common[0], common[i], "prompts must differ only by shot")
	}
}

func TestStylistShoot_DefaultBackgroundIsStudio(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)

	styCall(t, env.s, "stylist_shoot", shootArgs("anna", "front full body"))

	calls := env.shooter.genCalls()
	require.Len(t, calls, 1)
	assert.Contains(t, strings.ToLower(calls[0].Prompt), "studio")
}

func TestStylistShoot_EmptyLookAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		look any
	}{
		{name: "look missing", look: nil},
		{name: "look empty object", look: map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootDefaultEnv(t)
			args := shootArgs("anna", "front full body")
			delete(args, "look")
			if tt.look != nil {
				args["look"] = tt.look
			}

			m, _ := styCall(t, env.s, "stylist_shoot", args)

			calls := env.shooter.genCalls()
			require.Len(t, calls, 1)
			assert.Contains(t, calls[0].Prompt, "slim woman, fair skin, shoulder-length wavy auburn hair")
			assert.NotContains(t, calls[0].Prompt, "anna_trig")
			assert.Len(t, styVariants(t, m), 1)
		})
	}
}

func TestStylistShoot_Seed(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)
	args := shootArgs("anna", "a", "b", "c")
	args["seed"] = float64(1000)

	styCall(t, env.s, "stylist_shoot", args)

	calls := env.shooter.genCalls()
	require.Len(t, calls, 3)
	for i, c := range calls {
		require.NotNil(t, c.Seed, "call %d seed", i)
		assert.Equal(t, int64(1000+i), *c.Seed, "call %d seed", i)
	}
}

func TestStylistShoot_SavesToStylistSession(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)

	m, out := styCall(t, env.s, "stylist_shoot", shootArgs("anna", "front", "side"))

	named := env.sess.namedCalls()
	require.Len(t, named, 2)
	images := env.shooter.genImages()
	require.Len(t, images, 2)
	var returned []int64
	for i, c := range named {
		assert.Equal(t, "Стилист", c.name)
		assert.Equal(t, images[i], c.image, "saved image %d must be the generated one", i)
		returned = append(returned, c.returned)
	}
	variants := styVariants(t, m)
	require.Len(t, variants, 2)
	assert.Equal(t, returned, styIDs(variants))

	assert.NotContains(t, m, "image_base64")
	for _, v := range variants {
		assert.NotContains(t, v, "image_base64")
		assert.NotContains(t, v, "image")
		assert.NotContains(t, v, "path")
	}
	lower := strings.ToLower(out)
	for _, f := range append([]string{".png", ".jpg", "iVBORw0KGgo", env.outDir}, images...) {
		assert.NotContains(t, lower, strings.ToLower(f))
	}
}

func TestStylistShoot_BadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mutate   func(args map[string]any)
		wantText []string
	}{
		{name: "unknown person lists names", mutate: func(a map[string]any) { a["person"] = "nobody" }, wantText: []string{"anna", "boris", "vera", "gleb"}},
		{name: "person missing", mutate: func(a map[string]any) { delete(a, "person") }},
		{name: "person with unsupported base", mutate: func(a map[string]any) { a["person"] = "dina" }},
		{name: "appearance empty", mutate: func(a map[string]any) { a["appearance"] = "" }},
		{name: "appearance blank", mutate: func(a map[string]any) { a["appearance"] = "   " }},
		{name: "appearance missing", mutate: func(a map[string]any) { delete(a, "appearance") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootDefaultEnv(t)
			args := shootArgs("anna", "front full body")
			tt.mutate(args)

			out, err := styExec(env.s, "stylist_shoot", args)

			require.Error(t, err, out)
			for _, w := range tt.wantText {
				assert.Contains(t, err.Error(), w)
			}
			assert.Empty(t, env.shooter.genCalls())
			assert.Empty(t, env.sess.namedCalls())
		})
	}
}

func TestStylistShoot_PeopleServerError(t *testing.T) {
	t.Parallel()
	env := shootNewEnv(t, &fakePeople{err: errors.New("connection refused")}, &fakeShooter{}, nil)

	_, err := styExec(env.s, "stylist_shoot", shootArgs("anna", "front"))

	require.Error(t, err)
	assert.Empty(t, env.shooter.genCalls())
	assert.Empty(t, env.sess.namedCalls())
}

func TestStylistShoot_FrameFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		shooter *fakeShooter
	}{
		{name: "generate returns error", shooter: &fakeShooter{failOn: map[int]bool{2: true}}},
		{name: "item has error", shooter: &fakeShooter{itemErrOn: map[int]bool{2: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootNewEnv(t, &fakePeople{people: shootPeopleAll()}, tt.shooter, nil)

			out, err := styExec(env.s, "stylist_shoot", shootArgs("anna", "front", "side", "back"))

			require.Error(t, err, out)
			assert.Contains(t, err.Error(), "кадр 2 из 3")
			named := env.sess.namedCalls()
			require.Len(t, named, 1, "first frame must stay in the session")
			assert.Equal(t, "Стилист", named[0].name)
			assert.Contains(t, err.Error(), strconv.FormatInt(named[0].returned, 10), "saved frame id must be listed")
			assert.NotContains(t, err.Error(), "iVBORw0KGgo")
		})
	}
}

func TestStylistShoot_MissingDeps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		people  *fakePeople
		shooter *fakeShooter
	}{
		{name: "no people", people: nil, shooter: &fakeShooter{}},
		{name: "no shooter", people: &fakePeople{people: shootPeopleAll()}, shooter: nil},
		{name: "neither", people: nil, shooter: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootNewEnv(t, tt.people, tt.shooter, nil)

			_, err := styExec(env.s, "stylist_shoot", shootArgs("anna", "front"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "недоступно")
			if tt.shooter != nil {
				assert.Empty(t, tt.shooter.genCalls())
			}
			assert.Empty(t, env.sess.namedCalls())
		})
	}
}

func TestStylistShoot_NoSessions(t *testing.T) {
	t.Parallel()
	shooter := &fakeShooter{}
	s := NewServer(Deps{OutDir: t.TempDir(), Stylist: &fakeStylist{}, People: &fakePeople{people: shootPeopleAll()}, Shooter: shooter})
	RegisterTools(s)

	_, err := styExec(s, "stylist_shoot", shootArgs("anna", "front"))

	require.Error(t, err)
	assert.Empty(t, shooter.genCalls())
}

func TestStylistShootPeople_RequireLocalBackends(t *testing.T) {
	t.Parallel()
	const localLLM = "http://localhost:1234"
	const localSD = "http://127.0.0.1:7860"
	tests := []struct {
		name    string
		llmURL  string
		sdURL   string
		wantErr bool
	}{
		{name: "cloud LLM", llmURL: "https://api.openai.com/v1", sdURL: localSD, wantErr: true},
		{name: "cloud SD", llmURL: localLLM, sdURL: "http://8.8.8.8:7860", wantErr: true},
		{name: "both local", llmURL: localLLM, sdURL: localSD, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootNewEnv(t, &fakePeople{people: shootPeopleAll()}, &fakeShooter{}, &config.Config{LLMUrl: tt.llmURL, SDUrl: tt.sdURL})

			_, errPeople := styExec(env.s, "stylist_people", map[string]any{})
			_, errShoot := styExec(env.s, "stylist_shoot", shootArgs("anna", "front"))

			if tt.wantErr {
				assert.Error(t, errPeople)
				assert.Error(t, errShoot)
				assert.Empty(t, env.shooter.genCalls())
				assert.Empty(t, env.sess.namedCalls())
				return
			}
			assert.NoError(t, errPeople)
			assert.NoError(t, errShoot)
			assert.Len(t, env.shooter.genCalls(), 1)
			assert.Positive(t, env.people.callCount())
		})
	}
}

func shootContainsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func TestStylistDescribe_DetailedPrompt(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	s, _ := styNewServer(t, eng, &fakeSessions{})
	photo, _ := styWritePhoto(t)

	styCall(t, s, "stylist_describe", map[string]any{"photo_path": photo})

	calls := eng.describeCalls()
	require.Len(t, calls, 1)
	p := strings.ToLower(calls[0].prompt)
	checks := []struct {
		what  string
		words []string
	}{
		{what: "body build", words: []string{"body", "build", "physique", "figure", "телосложени"}},
		{what: "proportions", words: []string{"proportion", "пропорци"}},
		{what: "skin color", words: []string{"skin"}},
		{what: "skin as in the photo", words: []string{"as it appears", "as seen", "as in the photo", "in this photo", "как на снимке", "как на фото"}},
		{what: "lighting caveat", words: []string{"light", "свет"}},
		{what: "hairstyle", words: []string{"hair"}},
		{what: "silhouette or fit", words: []string{"silhouette", "fit", "силуэт", "посадк"}},
		{what: "length", words: []string{"length", "длин"}},
		{what: "neckline", words: []string{"neckline", "вырез"}},
		{what: "sleeves", words: []string{"sleeve", "рукав"}},
		{what: "fabric", words: []string{"fabric", "material", "ткан"}},
		{what: "texture", words: []string{"texture", "фактур"}},
		{what: "colors", words: []string{"color", "colour", "цвет"}},
		{what: "print", words: []string{"print", "pattern", "принт"}},
		{what: "print scale", words: []string{"scale", "масштаб"}},
		{what: "fastenings", words: []string{"fastening", "fastener", "closure", "button", "zip", "застёжк", "застежк"}},
		{what: "garment without a person", words: []string{"no person", "without a person", "only a garment", "only the garment", "only clothing", "garment alone", "no one", "without a model", "без человека", "только вещь"}},
	}
	for _, c := range checks {
		assert.True(t, shootContainsAny(p, c.words...), "describe prompt must ask about %s: %q", c.what, calls[0].prompt)
	}
}

func TestPrompts_StylistMentionsShoot(t *testing.T) {
	t.Parallel()
	s, _ := styNewServer(t, nil, nil)

	m := styRPC(t, s, "prompts/get", map[string]any{"name": "stylist", "arguments": map[string]any{"photo": "active"}})

	require.Nil(t, m["error"], "%v", m)
	res, ok := m["result"].(map[string]any)
	require.True(t, ok, "%v", m)
	msgs, ok := res["messages"].([]any)
	require.True(t, ok, "%v", res)
	require.NotEmpty(t, msgs)
	msg, ok := msgs[0].(map[string]any)
	require.True(t, ok)
	content, ok := msg["content"].(map[string]any)
	require.True(t, ok, "%v", msg)
	text, _ := content["text"].(string)
	assert.Contains(t, text, "stylist_people")
	assert.Contains(t, text, "stylist_shoot")
}

func TestStylistShoot_PromptTailByBackground(t *testing.T) {
	t.Parallel()
	p := shootPerson("anna", "Z-Image")
	look := Look{Outfit: "a red dress"}

	candid := shootPrompt(p, "a woman", look, "standing, waist-up", "an evening old city street, street lamps")
	assert.Contains(t, candid, "candid photograph")
	assert.NotContains(t, candid, "professional studio portrait")

	studio := shootPrompt(p, "a woman", look, "head-and-shoulders portrait", "a dark grey Studio backdrop, soft light")
	assert.Contains(t, studio, "professional studio portrait")
	assert.NotContains(t, studio, "candid photograph")
	assert.Contains(t, studio, "<lora:anna_lora_v1:1>")
}

func TestStylistShoot_LandscapeOrientation(t *testing.T) {
	t.Parallel()
	env := shootDefaultEnv(t)

	args := shootArgs("anna", "front full body")
	plan, err := planShoot(env.people, env.eng, args)
	require.NoError(t, err)
	assert.Equal(t, shootWidth, plan.w)
	assert.Equal(t, shootHeight, plan.h)

	args["orientation"] = "landscape"
	plan, err = planShoot(env.people, env.eng, args)
	require.NoError(t, err)
	assert.Equal(t, shootHeight, plan.w, "landscape width")
	assert.Equal(t, shootWidth, plan.h, "landscape height")
}
