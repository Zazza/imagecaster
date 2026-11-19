package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
)

type spPeople struct {
	list []serverclient.Person
	err  error
}

func (p spPeople) List(context.Context) ([]serverclient.Person, error) { return p.list, p.err }

type spJob struct {
	jobType queue.JobType
	params  mcp.ShootFrameParams
	source  string
	label   string
}

type spQueue struct {
	jobs   []spJob
	failAt int
}

func (q *spQueue) EnqueueLabeled(jobType queue.JobType, params any, source, label string) (int64, error) {
	if q.failAt > 0 && len(q.jobs)+1 == q.failAt {
		return 0, errors.New("db locked")
	}
	p, _ := params.(mcp.ShootFrameParams)
	q.jobs = append(q.jobs, spJob{jobType: jobType, params: p, source: source, label: label})
	return int64(100 + len(q.jobs)), nil
}

type spSettings struct {
	m   map[string]string
	err error
}

func (s *spSettings) GetSetting(key string) (string, error) { return s.m[key], s.err }
func (s *spSettings) SetSetting(key, value string) error {
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

func spPeopleList() spPeople {
	return spPeople{list: []serverclient.Person{
		{Name: "anna", Base: "Z-Image", Lora: "anna_z", Gender: "woman", Identity: true},
		{Name: "ivan", Base: "SDXL", Lora: "ivan_x", Gender: "man"},
		{Name: "old", Base: "SD1.5", Lora: "old_l", Gender: "woman"},
	}}
}

func spSeries(shots ...stylist.Shot) stylist.Series {
	return stylist.Series{Outfit: "a red dress", Hair: "low bun", Makeup: "red lips", Background: "studio backdrop", Place: "studio", Shots: shots}
}

func TestStylistShoot_EnqueuesFrames(t *testing.T) {
	seed := int64(10)
	req := StylistShootRequest{
		Person: "anna", Appearance: "a woman, 170 cm", FaceSwap: true, Seed: &seed,
		Series: []stylist.Series{
			spSeries(stylist.Shot{Pose: "Анфас", Prompt: "facing camera"}, stylist.Shot{Pose: "Лёжа", Prompt: "lying", Landscape: true}),
			spSeries(stylist.Shot{Pose: "Сбоку", Prompt: "side view"}),
		},
	}
	q := &spQueue{}
	ids, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, req)
	require.NoError(t, err)
	assert.Equal(t, []int64{101, 102, 103}, ids)
	require.Len(t, q.jobs, 3)
	for i, j := range q.jobs {
		assert.Equal(t, queue.JobStylistFrame, j.jobType)
		assert.Equal(t, "stylist", j.source)
		assert.Equal(t, "anna", j.params.Person)
		assert.Equal(t, "Z-Image", j.params.Base)
		assert.Equal(t, stylist.Models["Z-Image"].Model, j.params.Model)
		assert.Equal(t, i+1, j.params.Index)
		assert.Equal(t, 3, j.params.Total)
		assert.True(t, j.params.FaceSwap)
		assert.False(t, j.params.FacePass)
		require.NotNil(t, j.params.Seed)
		assert.Equal(t, seed+int64(i), *j.params.Seed)
		assert.True(t, strings.Contains(j.params.Prompt, "of a woman, 170 cm, "), j.params.Prompt)
		assert.True(t, strings.Contains(j.params.Prompt, ". Wearing a red dress. Hair: low bun. Makeup: red lips. "), j.params.Prompt)
		assert.NotContains(t, j.params.Prompt, "BREAK")
		assert.True(t, strings.HasSuffix(j.params.Prompt, "<lora:anna_z:1>"))
	}
	assert.Equal(t, [2]int{1024, 1536}, [2]int{q.jobs[0].params.Width, q.jobs[0].params.Height})
	assert.Equal(t, [2]int{1536, 1024}, [2]int{q.jobs[1].params.Width, q.jobs[1].params.Height})
	assert.Equal(t, "Стилист: anna, Анфас (1 из 3)", q.jobs[0].label)
	raw, err := json.Marshal(q.jobs[0].params)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"face_pass":false`)
}

func TestStylistShoot_FaceSwapOnlyWithIdentity(t *testing.T) {
	tests := []struct {
		name     string
		person   string
		faceSwap bool
		want     bool
	}{
		{"identity and checkbox", "anna", true, true},
		{"identity without checkbox", "anna", false, false},
		{"no identity", "ivan", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			_, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, StylistShootRequest{
				Person: tt.person, FaceSwap: tt.faceSwap, Series: []stylist.Series{spSeries(stylist.Shot{Prompt: "front"})},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, q.jobs[0].params.FaceSwap)
		})
	}
}

func TestStylistShoot_Refuses(t *testing.T) {
	many := make([]stylist.Shot, 25)
	tests := []struct {
		name     string
		notLocal bool
		people   spPeople
		req      StylistShootRequest
		want     string
	}{
		{"non-local sd", true, spPeopleList(), StylistShootRequest{Person: "anna", Series: []stylist.Series{spSeries(stylist.Shot{})}}, "локальной"},
		{"more than 24", false, spPeopleList(), StylistShootRequest{Person: "anna", Series: []stylist.Series{spSeries(many...)}}, "24"},
		{"no frames", false, spPeopleList(), StylistShootRequest{Person: "anna"}, "нет кадров"},
		{"unknown person", false, spPeopleList(), StylistShootRequest{Person: "zoe", Series: []stylist.Series{spSeries(stylist.Shot{})}}, "не найден"},
		{"unsupported base", false, spPeopleList(), StylistShootRequest{Person: "old", Series: []stylist.Series{spSeries(stylist.Shot{})}}, "SD1.5"},
		{"people error", false, spPeople{err: errors.New("offline")}, StylistShootRequest{Person: "anna", Series: []stylist.Series{spSeries(stylist.Shot{})}}, "offline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			_, err := stylistShoot(context.Background(), tt.people, nil, q, tt.notLocal, tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, q.jobs, "nothing is enqueued")
		})
	}
}

func spImage(t *testing.T, w, h int, asPNG bool) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = byte(i % 251)
	}
	var buf bytes.Buffer
	if asPNG {
		require.NoError(t, png.Encode(&buf, img))
	} else {
		require.NoError(t, jpeg.Encode(&buf, img, nil))
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func spSize(t *testing.T, b64 string) (int, int, string) {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	return cfg.Width, cfg.Height, format
}

func TestStylistShoot_DefaultAppearanceAndGarment(t *testing.T) {
	small := spImage(t, 64, 96, true)
	tests := []struct {
		name       string
		outfit     string
		wantPrompt string
		wantOutfit string
	}{
		{"structure named", " a top and a skirt ", "a man, a top and a skirt, low bun", "a top and a skirt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			_, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, StylistShootRequest{
				Person: "ivan", Garment: small, GarmentOutfit: tt.outfit, Series: []stylist.Series{spSeries(stylist.Shot{Prompt: "front"})},
			})
			require.NoError(t, err)
			p := q.jobs[0].params
			assert.True(t, strings.HasPrefix(p.Prompt, "front, "+tt.wantPrompt), p.Prompt)
			assert.NotContains(t, p.Prompt, "a red dress")
			assert.Equal(t, small, p.GarmentBase64, "small photo is sent as is")
			assert.Equal(t, tt.wantOutfit, p.Outfit)
		})
	}
}

func TestShrinkGarment(t *testing.T) {
	tests := []struct {
		name       string
		w, h       int
		png        bool
		wantW      int
		wantH      int
		wantFormat string
	}{
		{"big jpeg", 3000, 1500, false, 2048, 1024, "jpeg"},
		{"big png tall", 1000, 4096, true, 500, 2048, "png"},
		{"small kept", 800, 600, false, 800, 600, "jpeg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shrinkGarment(spImage(t, tt.w, tt.h, tt.png))
			require.NoError(t, err)
			w, h, f := spSize(t, got)
			assert.Equal(t, [2]int{tt.wantW, tt.wantH}, [2]int{w, h})
			assert.Equal(t, tt.wantFormat, f)
		})
	}
	got, err := shrinkGarment("aGVsbG8=")
	require.NoError(t, err, "a format we cannot read goes to the server as is, like MCP")
	assert.Equal(t, "aGVsbG8=", got)
	_, err = shrinkGarment("not base64!")
	assert.Error(t, err)
	q := &spQueue{}
	_, err = stylistShoot(context.Background(), spPeopleList(), nil, q, false, StylistShootRequest{
		Person: "anna", Garment: spImage(t, 3000, 1500, false), GarmentOutfit: "a dress", Series: []stylist.Series{spSeries(stylist.Shot{}, stylist.Shot{})},
	})
	require.NoError(t, err)
	for _, j := range q.jobs {
		w, _, _ := spSize(t, j.params.GarmentBase64)
		assert.Equal(t, 2048, w)
	}
	assert.Equal(t, q.jobs[0].params.GarmentBase64, q.jobs[1].params.GarmentBase64, "shrunk once, same for every frame")
}

func TestStylistShoot_PartialEnqueueReportsIDs(t *testing.T) {
	q := &spQueue{failAt: 2}
	ids, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, StylistShootRequest{
		Person: "anna", Series: []stylist.Series{spSeries(stylist.Shot{}, stylist.Shot{})},
	})
	require.Error(t, err)
	assert.Equal(t, []int64{101}, ids)
	assert.Contains(t, err.Error(), "101")
}

func TestDefaultStylistAppearance(t *testing.T) {
	tests := []struct{ gender, want string }{
		{"woman", "a woman"}, {"man", "a man"}, {"person", "a person"}, {"", "a woman"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, defaultStylistAppearance(tt.gender), tt.gender)
	}
}

func TestStylistAppearance_RoundTrip(t *testing.T) {
	st := &spSettings{}
	require.NoError(t, setStylistAppearance(st, "anna", " a woman, 165 cm ", nil))
	require.NoError(t, setStylistAppearance(st, "ivan", "a man", nil))
	got, err := getStylistAppearance(st, "anna")
	require.NoError(t, err)
	assert.Equal(t, "a woman, 165 cm", got)
	require.NoError(t, setStylistAppearance(st, "anna", "", nil))
	got, err = getStylistAppearance(st, "anna")
	require.NoError(t, err)
	assert.Empty(t, got)
	got, _ = getStylistAppearance(st, "ivan")
	assert.Equal(t, "a man", got)
	assert.Error(t, setStylistAppearance(st, " ", "x", nil))
	assert.Error(t, setStylistAppearance(st, "anna", strings.Repeat("я", stylistAppearanceMax+1), nil))

	broken := &spSettings{m: map[string]string{stylistAppearanceKey: "{"}}
	_, err = getStylistAppearance(broken, "anna")
	assert.ErrorIs(t, err, errStylistAppearanceCorrupt)
	var logged error
	require.NoError(t, setStylistAppearance(broken, "anna", "a woman", func(err error) { logged = err }))
	assert.ErrorIs(t, logged, errStylistAppearanceCorrupt)
	got, err = getStylistAppearance(broken, "anna")
	require.NoError(t, err)
	assert.Equal(t, "a woman", got)
}

type spChat struct {
	reply string
	err   error
}

func (c spChat) Chat(string, string, string, float64, int) (string, error) { return c.reply, c.err }

func TestPlanStylist_LLMFallback(t *testing.T) {
	base, err := stylist.Load()
	require.NoError(t, err)
	tests := []struct {
		name    string
		chat    stylist.Ideas
		model   string
		usedLLM bool
		logged  bool
	}{
		{"llm ok", spChat{reply: `{"series":[{"outfit":"slim blue denim jeans, white cotton t-shirt, sneakers","hair":"bun","makeup":"soft","background":"park","place":"urban"},{"outfit":"long camel wool coat, black turtleneck, black trousers","hair":"braid","makeup":"none","background":"street","place":"urban"}]}`}, "m", true, false},
		{"llm error", spChat{err: errors.New("down")}, "m", false, true},
		{"no model", spChat{}, "", false, true},
		{"garbage", spChat{reply: "nope"}, "m", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logged := false
			res, err := planStylist(tt.chat, tt.model, stylist.PlanRequest{Frames: 6, Seed: 1}, base, func(error) { logged = true })
			require.NoError(t, err)
			assert.Len(t, res.Series, 2)
			assert.Equal(t, tt.usedLLM, res.UsedLLM)
			assert.Equal(t, tt.logged, logged)
			if !tt.usedLLM {
				assert.NotEmpty(t, res.Warnings)
				assert.NotEmpty(t, res.Series[0].Outfit)
			} else {
				assert.Equal(t, "slim blue denim jeans, white cotton t-shirt, sneakers", res.Series[0].Outfit)
			}
		})
	}
	_, err = planStylist(spChat{}, "m", stylist.PlanRequest{Frames: 25}, base, nil)
	assert.Error(t, err)
}

func TestRegenerateStylistSeries(t *testing.T) {
	base, err := stylist.Load()
	require.NoError(t, err)
	res, err := regenerateStylistSeries(spChat{reply: `{"series":[{"outfit":"long camel wool coat, black turtleneck, black trousers","hair":"braid","makeup":"none","background":"street","place":"urban"}]}`}, "m", stylist.PlanRequest{Frames: 6, Seed: 2}, base, 1, nil)
	require.NoError(t, err)
	assert.True(t, res.UsedLLM)
	assert.Equal(t, "long camel wool coat, black turtleneck, black trousers", res.Series.Outfit)
	assert.Len(t, res.Series.Shots, 2)
}

type spSeqChat struct {
	replies []string
	systems []string
	users   []string
}

func (c *spSeqChat) Chat(_ string, system, user string, _ float64, _ int) (string, error) {
	c.systems = append(c.systems, system)
	c.users = append(c.users, user)
	if len(c.replies) == 0 {
		return "", errors.New("no reply")
	}
	r := c.replies[0]
	c.replies = c.replies[1:]
	return r, nil
}

func TestPlanStylist_RussianTranslation(t *testing.T) {
	base, err := stylist.Load()
	require.NoError(t, err)
	const ideas = `{"series":[{"outfit":"long camel wool coat, black turtleneck, black trousers","hair":"braid","background":"street","place":"urban"}]}`
	aspects := map[string]stylist.AspectChoice{"makeup": {Mode: stylist.ModePick, IDs: []string{base.Makeup[0].ID}}}
	tests := []struct {
		name     string
		locale   string
		replies  []string
		calls    int
		wantRu   [3]string
		wantAI   []string
		warnNoRu bool
	}{
		{name: "ru: one call with only LLM fields", locale: "ru", replies: []string{ideas, `{"texts":["длинное пальто цвета кэмел","коса","улица"]}`}, calls: 2, wantRu: [3]string{"длинное пальто цвета кэмел", "коса", "улица"}, wantAI: []string{"outfit", "hair", "background"}},
		{name: "ru: failed translation warns, English shown", locale: "ru", replies: []string{ideas, `{"texts":["пальто"]}`}, calls: 2, warnNoRu: true},
		{name: "en: no call", locale: "en", replies: []string{ideas}, calls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat := &spSeqChat{replies: tt.replies}
			res, err := planStylist(chat, "m", stylist.PlanRequest{Frames: 2, Seed: 1, Aspects: aspects, Locale: tt.locale}, base, nil)
			require.NoError(t, err)
			require.Len(t, chat.systems, tt.calls)
			s := res.Series[0]
			assert.Equal(t, tt.wantRu, [3]string{s.OutfitRu, s.HairRu, s.BackgroundRu})
			assert.Equal(t, tt.wantAI, s.RuAI)
			assert.Equal(t, base.Makeup[0].Name, s.MakeupRu)
			assert.Equal(t, "long camel wool coat, black turtleneck, black trousers", s.Outfit)
			assert.Equal(t, tt.warnNoRu, slices.Contains(res.Warnings, stylistWarnNoRu), res.Warnings)
			if tt.calls == 2 {
				assert.Equal(t, `{"texts":["long camel wool coat, black turtleneck, black trousers","braid","street"]}`, chat.users[1])
			}
		})
	}
}

func TestRegenerateStylistSeries_Russian(t *testing.T) {
	base, err := stylist.Load()
	require.NoError(t, err)
	chat := &spSeqChat{replies: []string{`{"series":[{"outfit":"long camel wool coat, black turtleneck, black trousers","hair":"braid","makeup":"none","background":"street","place":"urban"}]}`, `{"texts":["пальто","коса","без макияжа","улица"]}`}}
	res, err := regenerateStylistSeries(chat, "m", stylist.PlanRequest{Frames: 6, Seed: 2, Locale: "ru"}, base, 1, nil)
	require.NoError(t, err)
	assert.Len(t, chat.systems, 2)
	assert.Equal(t, "пальто", res.Series.OutfitRu)
	assert.Equal(t, []string{"outfit", "hair", "makeup", "background"}, res.Series.RuAI)
}

func spOrientedJPEG(t *testing.T, w, h int, orientation uint16) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	raw := buf.Bytes()
	var tiff bytes.Buffer
	be := binary.BigEndian
	tiff.WriteString("MM\x00*")
	for _, v := range []any{uint32(8), uint16(1), uint16(0x0112), uint16(3), uint32(1), orientation, uint16(0), uint32(0)} {
		require.NoError(t, binary.Write(&tiff, be, v))
	}
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	be.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{}, raw[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	out = append(out, raw[2:]...)
	return base64.StdEncoding.EncodeToString(out)
}

func TestShrinkGarment_EXIFOrientation(t *testing.T) {
	tests := []struct {
		name         string
		orientation  uint16
		wantW, wantH int
	}{
		{"rotated 90 (phone portrait)", 6, 1024, 2048},
		{"rotated 270", 8, 1024, 2048},
		{"normal", 1, 2048, 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shrinkGarment(spOrientedJPEG(t, 3000, 1500, tt.orientation))
			require.NoError(t, err)
			w, h, _ := spSize(t, got)
			assert.Equal(t, [2]int{tt.wantW, tt.wantH}, [2]int{w, h})
		})
	}
}

func TestStylistFrames(t *testing.T) {
	pdb, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { pdb.Close() })
	store := queue.NewStore(pdb.DB())

	done, err := store.CreateLabeled(queue.JobStylistFrame, `{"garment_base64":"abc"}`, "stylist", "Стилист: anna (1 из 4)")
	require.NoError(t, err)
	raw, err := json.Marshal(mcp.ShootFrameResult{ID: 300, Shot: "Анфас", FacePass: "off", FaceSwap: "done"})
	require.NoError(t, err)
	stored, err := json.Marshal(queue.JobResult{Frame: raw})
	require.NoError(t, err)
	require.NoError(t, store.UpdateCompleted(done, string(stored)))

	failed, err := store.CreateLabeled(queue.JobStylistFrame, `{}`, "stylist", "b")
	require.NoError(t, err)
	require.NoError(t, store.UpdateStatus(failed, queue.StatusFailed, "sd: out of memory"))

	pending, err := store.CreateLabeled(queue.JobStylistFrame, `{}`, "stylist", "c")
	require.NoError(t, err)

	other, err := store.Create(queue.JobTxt2Img, `{}`, "generate")
	require.NoError(t, err)
	require.NoError(t, store.UpdateCompleted(other, `{"session_item_id":5}`))

	broken, err := store.CreateLabeled(queue.JobStylistFrame, `{}`, "stylist", "d")
	require.NoError(t, err)
	require.NoError(t, store.UpdateCompleted(broken, `not json`))

	panel, err := store.ListForPanel(100)
	require.NoError(t, err)
	for _, j := range panel {
		require.Empty(t, j.Result, "panel list must stay without result")
	}

	tests := []struct {
		name string
		ids  []int64
		want []StylistFrame
	}{
		{"empty", nil, []StylistFrame{}},
		{"completed frame from the stored result", []int64{done}, []StylistFrame{
			{JobID: done, Status: "completed", ItemID: 300, Shot: "Анфас", FaceSwap: "done"},
		}},
		{"order of ids, failed with error, pending, missing, foreign type, broken result", []int64{pending, failed, 9999, other, broken, done}, []StylistFrame{
			{JobID: pending, Status: "pending"},
			{JobID: failed, Status: "failed", Error: "sd: out of memory"},
			{JobID: 9999, Status: "gone"},
			{JobID: other, Status: "gone"},
			{JobID: broken, Status: "completed"},
			{JobID: done, Status: "completed", ItemID: 300, Shot: "Анфас", FaceSwap: "done"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := stylistFrames(store, tc.ids)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("keeps only the last ids over the limit", func(t *testing.T) {
		ids := make([]int64, stylistFramesMax+5)
		for i := range ids {
			ids[i] = int64(100000 + i)
		}
		ids[len(ids)-1] = done
		got, err := stylistFrames(store, ids)
		require.NoError(t, err)
		require.Len(t, got, stylistFramesMax)
		assert.Equal(t, int64(100005), got[0].JobID)
		assert.Equal(t, int64(300), got[len(got)-1].ItemID)
	})
}

func TestStylistShoot_EditedShotHasNoFramingLead(t *testing.T) {
	var req StylistShootRequest
	require.NoError(t, json.Unmarshal([]byte(`{"person":"anna","series":[{"outfit":"a red dress","shots":[
		{"pose":"Анфас","prompt":"facing camera","framing":"close"},
		{"pose":"Анфас","prompt":"sitting on the stairs, full length","prompt_ru":"сидит на лестнице, в полный рост"}
	]}]}`), &req))
	q := &spQueue{}
	_, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, req)
	require.NoError(t, err)
	require.Len(t, q.jobs, 2)
	assert.True(t, strings.HasPrefix(q.jobs[0].params.Prompt, "A close-up head-and-shoulders photograph of a woman, facing camera."), q.jobs[0].params.Prompt)
	assert.True(t, strings.HasPrefix(q.jobs[1].params.Prompt, "A photograph of a woman, sitting on the stairs, full length."), q.jobs[1].params.Prompt)
	assert.NotContains(t, q.jobs[1].params.Prompt, "close-up")
	assert.NotContains(t, q.jobs[1].params.Prompt, "лестнице")
}

func TestShrinkGarment_WebP(t *testing.T) {
	data, err := os.ReadFile("testdata/stylist-garment-3000x1500.webp")
	require.NoError(t, err)
	got, err := shrinkGarment(base64.StdEncoding.EncodeToString(data))
	require.NoError(t, err)
	w, h, f := spSize(t, got)
	assert.Equal(t, [2]int{2048, 1024}, [2]int{w, h})
	assert.Equal(t, "jpeg", f)
}

func TestStylistShoot_GarmentNeedsItemNames(t *testing.T) {
	small := spImage(t, 64, 96, true)
	for _, outfit := range []string{"", "   "} {
		q := &spQueue{}
		_, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, StylistShootRequest{
			Person: "ivan", Garment: small, GarmentOutfit: outfit, Series: []stylist.Series{spSeries(stylist.Shot{Prompt: "front"})},
		})
		require.ErrorIs(t, err, errStylistGarmentUnnamed)
		assert.Empty(t, q.jobs, "nothing is enqueued")
	}
	q := &spQueue{}
	_, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, StylistShootRequest{
		Person: "ivan", Series: []stylist.Series{spSeries(stylist.Shot{Prompt: "front"})},
	})
	require.NoError(t, err, "without a garment photo the field is not needed")
}

type fakeDescriber struct {
	answer string
	err    error
	calls  int
	prompt string
}

func (f *fakeDescriber) DescribeImage(_, prompt string) (string, error) {
	f.calls++
	f.prompt = prompt
	return f.answer, f.err
}

func TestStylistDescribeGarment(t *testing.T) {
	small := spImage(t, 64, 96, true)
	tests := []struct {
		name     string
		garment  string
		notLocal bool
		answer   string
		err      error
		want     string
		wantErr  error
		wantCall int
	}{
		{"names the items", small, false, "a cropped top and a midi skirt", nil, "a cropped top and a midi skirt", nil, 1},
		{"quotes, dot and extra lines dropped", small, false, "\"a blazer and wide trousers.\"\nThe person is standing.", nil, "a blazer and wide trousers", nil, 1},
		{"remote model: photo not sent", small, true, "x", nil, "", errStylistVisionNotLocal, 0},
		{"model error", small, false, "", errors.New("timeout"), "", nil, 1},
		{"empty answer", small, false, "  ", nil, "", nil, 1},
		{"no photo", "", false, "x", nil, "", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &fakeDescriber{answer: tt.answer, err: tt.err}
			got, err := stylistDescribeGarment(d, tt.notLocal, tt.garment)
			assert.Equal(t, tt.wantCall, d.calls)
			if tt.want == "" {
				require.Error(t, err)
				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, d.prompt, "Do not mention colours")
		})
	}
}
