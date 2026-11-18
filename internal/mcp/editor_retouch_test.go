package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

var erGray = color.NRGBA{R: 128, G: 128, B: 128, A: 255}

type fakeRetoucher struct {
	mu      sync.Mutex
	calls   []generation.DetailParams
	faces   []string
	outs    []string
	err     error
	faceErr error
	resized bool
}

func (f *fakeRetoucher) UpscaleDetail(p generation.DetailParams) (*generation.DetailResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, p)
	if f.err != nil {
		return nil, f.err
	}
	raw, err := base64.StdEncoding.DecodeString(p.ImageBase64)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	upscaled := p.TargetLong > 0 && max(w, h) < p.TargetLong
	if upscaled {
		w, h = w*2, h*2
	}
	out := base64.StdEncoding.EncodeToString(edSolidPNG(w, h, erGray))
	f.outs = append(f.outs, out)
	res := &generation.DetailResult{Image: out, Width: w, Height: h, Upscaled: upscaled, Resized: upscaled && f.resized, Detailed: p.Detail > 0}
	if res.Detailed {
		res.Tiles = 1
	}
	return res, nil
}

func (f *fakeRetoucher) FaceLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faces = append(f.faces, imageBase64)
	if f.faceErr != nil {
		return nil, f.faceErr
	}
	return styFace(), nil
}

func (f *fakeRetoucher) detailCalls() []generation.DetailParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]generation.DetailParams(nil), f.calls...)
}

func (f *fakeRetoucher) faceCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.faces...)
}

func (f *fakeRetoucher) outputs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.outs...)
}

const (
	erGenItem     = int64(42)
	erPhotoItem   = int64(43)
	erNoPrompt    = int64(44)
	erShootFrame  = int64(45)
	erTryVariant  = int64(46)
	erNoModel     = int64(47)
	erSession     = int64(7)
	erOtherSess   = int64(9)
	erGenModel    = "juggernautXL_ragnarokBy"
	erGenPrompt   = "portrait of anna, studio light <lora:anna_lora_v1:1>"
	erGenNegative = "lowres, blurry"
)

func erItemSess() *fakeItemSessions {
	sess := newFakeItemSessions()
	seed := int64(12345)
	sess.put(&preset.SessionItem{
		ID: erGenItem, SessionID: erSession, Source: "sd", Width: 200, Height: 300,
		Prompt: erGenPrompt, NegativePrompt: erGenNegative, Sampler: "euler_a", Steps: 30, CfgScale: 5.5, Seed: &seed,
		Meta: &preset.GenerationMeta{Model: erGenModel, Label: "Портрет в студии"},
	}, styPhotoPNG(200, 300, erGray))
	sess.put(&preset.SessionItem{ID: erPhotoItem, SessionID: erSession, Source: "import", Width: 120, Height: 80}, styPhotoPNG(120, 80, erGray))
	sess.put(&preset.SessionItem{
		ID: erNoPrompt, SessionID: erSession, Source: "sd", Width: 100, Height: 100,
		Meta: &preset.GenerationMeta{Model: erGenModel, Label: "Без промпта"},
	}, styPhotoPNG(100, 100, erGray))
	sess.put(&preset.SessionItem{
		ID: erShootFrame, SessionID: erOtherSess, Source: "stylist", Width: 96, Height: 144,
		Prompt: "slim woman, front full body <lora:anna_lora_v1:1>",
		Meta:   &preset.GenerationMeta{Model: "z-image-turbo-fp8-aio", Label: "Стилист: anna, front"},
	}, styPhotoPNG(96, 144, erGray))
	sess.put(&preset.SessionItem{
		ID: erTryVariant, SessionID: erSession, Source: "stylist", Width: 200, Height: 300,
		Meta: &preset.GenerationMeta{Label: "Стилист: образ", SourceItem: erGenItem},
	}, styPhotoPNG(200, 300, erGray))
	sess.put(&preset.SessionItem{
		ID: erNoModel, SessionID: erSession, Source: "sd", Width: 100, Height: 100, Prompt: "a cat",
		Meta: &preset.GenerationMeta{Label: "Кот"},
	}, styPhotoPNG(100, 100, erGray))
	sess.active = erGenItem
	return sess
}

func erRun(t *testing.T, eng RetouchEngine, sess AppSessions, p RetouchJobParams) *RetouchResult {
	t.Helper()
	res, err := RunRetouch(eng, sess, p, func() bool { return false })
	require.NoError(t, err)
	require.NotNil(t, res)
	return res
}

func erMeanR(t *testing.T, b64 string) float64 {
	t.Helper()
	img := edDecode(t, b64)
	b := img.Bounds()
	var sum float64
	var n int
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			sum += float64(c.R)
			n++
		}
	}
	return sum / float64(n)
}

func TestRunRetouch_ItemWithGenerationData(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25, Label: "Ретушь: Портрет в студии (1 из 1)", Index: 1, Total: 1})

	calls := eng.detailCalls()
	require.Len(t, calls, 1)
	p := calls[0]
	assert.Equal(t, 3840, p.TargetLong)
	assert.InDelta(t, 0.25, p.Detail, 1e-9)
	assert.Equal(t, erGenModel, p.Model)
	assert.Equal(t, erGenPrompt, p.Prompt, "same prompt with the LoRA as the source")
	assert.Equal(t, erGenNegative, p.NegativePrompt)
	assert.Equal(t, "euler_a", p.Sampler)
	assert.Equal(t, 30, p.Steps)
	assert.InDelta(t, 5.5, p.CfgScale, 1e-9)
	require.NotNil(t, p.Seed)
	assert.Equal(t, int64(12345), *p.Seed)
	srcImg, err := sess.GetSessionImage(erGenItem)
	require.NoError(t, err)
	assert.Equal(t, srcImg, p.ImageBase64)
	assert.Equal(t, "done", res.Upscale)
	assert.Equal(t, "done", res.Detail)
	assert.False(t, res.Adjust)
	assert.Empty(t, res.Retouch)
	assert.Equal(t, 400, res.Width)
	assert.Equal(t, 600, res.Height)
}

func TestRunRetouch_ItemSavedToItsSession(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erShootFrame, Size: "4k", Detail: 0.25, Index: 1, Total: 1})

	assert.Empty(t, sess.namedCalls(), "item result goes to the item's session, not a named one")
	adds := sess.idCalls()
	require.Len(t, adds, 1, "one new item; the source stays as is")
	assert.Equal(t, erOtherSess, adds[0].sessionID)
	assert.Equal(t, "editor", adds[0].source)
	require.NotNil(t, adds[0].meta)
	assert.Equal(t, erShootFrame, adds[0].meta.SourceItem)
	assert.Contains(t, adds[0].meta.Label, "Ретушь")
	assert.Equal(t, adds[0].returned, res.ID)
	assert.Positive(t, res.ID)
	saved := edDecode(t, adds[0].image)
	assert.Equal(t, res.Width, saved.Bounds().Dx())
	assert.Equal(t, res.Height, saved.Bounds().Dy())
	assert.Equal(t, 192, res.Width)
	assert.Equal(t, 288, res.Height)
}

func TestRunRetouch_PhotoSavedToEditorSession(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := &fakeSessions{}
	src := base64.StdEncoding.EncodeToString(styPhotoPNG(150, 100, erGray))

	res := erRun(t, eng, sess, RetouchJobParams{ImageBase64: src, Size: "4k", Detail: 0, Index: 1, Total: 1})

	calls := sess.namedCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, EditorSessionName, calls[0].name)
	assert.Equal(t, "editor", calls[0].source)
	require.NotNil(t, calls[0].meta)
	assert.Contains(t, calls[0].meta.Label, "Ретушь")
	assert.Equal(t, calls[0].returned, res.ID)
	saved := edDecode(t, calls[0].image)
	assert.Equal(t, 300, saved.Bounds().Dx())
	assert.Equal(t, 200, saved.Bounds().Dy())
	assert.Equal(t, "off", res.Detail)
}

func TestRunRetouch_NoGenerationDataSkipsDetail(t *testing.T) {
	t.Parallel()
	photo := base64.StdEncoding.EncodeToString(styPhotoPNG(150, 100, erGray))
	tests := []struct {
		name string
		p    RetouchJobParams
	}{
		{name: "camera photo by path", p: RetouchJobParams{ImageBase64: photo, Size: "4k", Detail: 0.25}},
		{name: "item without generation data", p: RetouchJobParams{ItemID: erPhotoItem, Size: "4k", Detail: 0.25}},
		{name: "item without prompt", p: RetouchJobParams{ItemID: erNoPrompt, Size: "4k", Detail: 0.25}},
		{name: "item without model", p: RetouchJobParams{ItemID: erNoModel, Size: "4k", Detail: 0.25}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeRetoucher{}
			sess := erItemSess()

			res := erRun(t, eng, sess, tt.p)

			for i, c := range eng.detailCalls() {
				assert.Zero(t, c.Detail, "call %d: no redraw without generation data", i)
				assert.Equal(t, 3840, c.TargetLong, "call %d: upscale still done", i)
			}
			require.NotEmpty(t, eng.detailCalls(), "upscale is still done")
			assert.Equal(t, "done", res.Upscale)
			require.True(t, strings.HasPrefix(res.Detail, "skipped:"), "detail: %q", res.Detail)
			assert.NotEmpty(t, strings.TrimSpace(strings.TrimPrefix(res.Detail, "skipped:")), "skip reason must be given")
			assert.Positive(t, res.ID)
		})
	}
}

func TestRunRetouch_Size(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		size        string
		wantTarget  int
		wantUpscale string
	}{
		{name: "4k", size: "4k", wantTarget: 3840, wantUpscale: "done"},
		{name: "2x of long side 300", size: "2x", wantTarget: 600, wantUpscale: "done"},
		{name: "off", size: "off", wantTarget: 0, wantUpscale: "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeRetoucher{}
			sess := erItemSess()

			res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: tt.size, Detail: 0.25})

			calls := eng.detailCalls()
			require.NotEmpty(t, calls)
			for i, c := range calls {
				assert.Equal(t, tt.wantTarget, c.TargetLong, "call %d", i)
			}
			assert.Equal(t, tt.wantUpscale, res.Upscale)
		})
	}
}

func TestRunRetouch_AlreadyAboveTargetNotShrunk(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := &fakeSessions{}
	src := base64.StdEncoding.EncodeToString(edSolidPNG(40, 4000, erGray))

	res := erRun(t, eng, sess, RetouchJobParams{ImageBase64: src, Size: "4k", Retouch: &faceguard.Retouch{Light: true}})

	calls := sess.namedCalls()
	require.Len(t, calls, 1)
	saved := edDecode(t, calls[0].image)
	assert.Equal(t, 40, saved.Bounds().Dx())
	assert.Equal(t, 4000, saved.Bounds().Dy())
	assert.NotEqual(t, "done", res.Upscale)
	assert.True(t, strings.HasPrefix(res.Upscale, "skipped:"), "upscale: %q", res.Upscale)
}

func TestRunRetouch_DetailZeroOff(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0})

	for i, c := range eng.detailCalls() {
		assert.Zero(t, c.Detail, "call %d", i)
	}
	assert.Equal(t, "off", res.Detail)
	assert.Equal(t, "done", res.Upscale)
}

func TestRunRetouch_Adjust(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		adj      *adjust.Params
		wantAdj  bool
		brighter bool
	}{
		{name: "brightness applied", adj: &adjust.Params{Brightness: 40}, wantAdj: true, brighter: true},
		{name: "neutral adjust is not applied", adj: &adjust.Params{}, wantAdj: false},
		{name: "no adjust", adj: nil, wantAdj: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeRetoucher{}
			sess := erItemSess()

			res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25, Adjust: tt.adj})

			assert.Equal(t, tt.wantAdj, res.Adjust)
			adds := sess.idCalls()
			require.Len(t, adds, 1)
			outs := eng.outputs()
			require.NotEmpty(t, outs)
			got := erMeanR(t, adds[0].image)
			base := erMeanR(t, outs[len(outs)-1])
			if tt.brighter {
				assert.Greater(t, got, base+10, "adjust is applied after upscale and detail")
				return
			}
			assert.InDelta(t, base, got, 1.0)
		})
	}
}

func TestRunRetouch_RetouchLandmarksFromSource(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25, Retouch: &faceguard.Retouch{UnderEyes: true, EyeWhites: true}})

	src, err := sess.GetSessionImage(erGenItem)
	require.NoError(t, err)
	faces := eng.faceCalls()
	require.NotEmpty(t, faces, "retouch needs face landmarks")
	for i, f := range faces {
		assert.Equal(t, src, f, "face call %d must use the source image", i)
	}
	for _, out := range eng.outputs() {
		assert.NotContains(t, faces, out, "landmarks are not taken on the upscaled image")
	}
	assert.Equal(t, "done", res.Retouch)
	adds := sess.idCalls()
	require.Len(t, adds, 1)
	saved := edDecode(t, adds[0].image)
	assert.Equal(t, 400, saved.Bounds().Dx(), "retouch is applied to the upscaled result")
	assert.Equal(t, 600, saved.Bounds().Dy())
}

func TestRunRetouch_RetouchFaceNotFound(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{faceErr: errors.New("no face found")}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25, Retouch: &faceguard.Retouch{UnderEyes: true}})

	assert.NotEqual(t, "done", res.Retouch)
	assert.NotEmpty(t, strings.TrimSpace(res.Retouch), "a note explains why retouch was not done")
	assert.Len(t, sess.idCalls(), 1, "the upscaled result is still saved")
}

func TestRunRetouch_ZeroRetouchNotApplied(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25, Retouch: &faceguard.Retouch{}})

	assert.Empty(t, res.Retouch)
	assert.Empty(t, eng.faceCalls())
}

func TestRunRetouch_StoppedNotSaved(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := erItemSess()

	_, err := RunRetouch(eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25}, func() bool { return true })

	require.Error(t, err)
	assert.True(t, generation.IsInterrupted(err), "got %v", err)
	assert.Empty(t, sess.idCalls())
	assert.Empty(t, sess.namedCalls())
}

func TestRunRetouch_Errors(t *testing.T) {
	t.Parallel()
	urlErr := &url.Error{Op: "Post", URL: "http://192.168.1.184:8080/api/sd/jobs", Err: errors.New("connection refused")}
	tests := []struct {
		name        string
		engErr      error
		p           RetouchJobParams
		interrupted bool
		contains    string
	}{
		{name: "interrupted upscale", engErr: fmt.Errorf("tile 3: %w", generation.ErrInterrupted), p: RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25}, interrupted: true},
		{name: "server unreachable", engErr: urlErr, p: RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25}, contains: "connection refused"},
		{name: "server unreachable for photo", engErr: urlErr, p: RetouchJobParams{ImageBase64: base64.StdEncoding.EncodeToString(styPhotoPNG(60, 60, erGray)), Size: "4k"}, contains: "connection refused"},
		{name: "unknown item", p: RetouchJobParams{ItemID: 9999, Size: "4k", Detail: 0.25}},
		{name: "no image at all", p: RetouchJobParams{Size: "4k", Detail: 0.25}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeRetoucher{err: tt.engErr}
			sess := erItemSess()

			_, err := RunRetouch(eng, sess, tt.p, func() bool { return false })

			require.Error(t, err)
			assert.Equal(t, tt.interrupted, generation.IsInterrupted(err), "got %v", err)
			assert.NotContains(t, err.Error(), "192.168")
			if tt.contains != "" {
				assert.Contains(t, err.Error(), tt.contains)
			}
			assert.Empty(t, sess.idCalls(), "nothing is saved on error")
			assert.Empty(t, sess.namedCalls(), "nothing is saved on error")
		})
	}
}

func TestNewRetouchHandler(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, NewRetouchHandler(&fakeSessions{}))
	assert.Equal(t, queue.JobType("retouch"), queue.JobRetouch)
}

type erEnv struct {
	s     *Server
	store *queue.Store
	sess  *fakeItemSessions
	eng   *fakeRetoucher
	out   string
}

func erNewEnv(t *testing.T, cfg *config.Config) *erEnv {
	t.Helper()
	env := &erEnv{store: sqNewStore(t), sess: erItemSess(), eng: &fakeRetoucher{}, out: t.TempDir()}
	env.s = NewServer(Deps{OutDir: env.out, Sessions: env.sess, Retoucher: env.eng, Queue: env.store, Cfg: cfg})
	RegisterTools(env.s)
	return env
}

func (e *erEnv) jobs(t *testing.T) int {
	t.Helper()
	n, err := e.store.ActiveCount()
	require.NoError(t, err)
	return n
}

func erJob(t *testing.T, store *queue.Store, id int64) (*queue.Job, RetouchJobParams) {
	t.Helper()
	job, err := store.Get(id)
	require.NoError(t, err)
	require.NotNil(t, job, "job %d", id)
	var p RetouchJobParams
	require.NoError(t, json.Unmarshal([]byte(job.Params), &p), job.Params)
	return job, p
}

func erNoSyncWork(t *testing.T, env *erEnv) {
	t.Helper()
	assert.Empty(t, env.eng.detailCalls(), "editor_retouch must not upscale itself")
	assert.Empty(t, env.eng.faceCalls(), "editor_retouch must not retouch itself")
	assert.Empty(t, env.sess.idCalls(), "editor_retouch must not save itself")
	assert.Empty(t, env.sess.namedCalls(), "editor_retouch must not save itself")
}

func erAssertNoLeak(t *testing.T, out string, extra ...string) {
	t.Helper()
	lower := strings.ToLower(out)
	for _, f := range append([]string{"ivborw0kggo", "image_base64", "base64", ".png", ".jpg"}, extra...) {
		assert.NotContains(t, lower, strings.ToLower(f))
	}
}

func TestEditorRetouch_ItemsOneJobEach(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)
	items := []int64{erGenItem, erShootFrame, erPhotoItem}

	m, out := styCall(t, env.s, "editor_retouch", map[string]any{"items": sqIDsArg(items...)})

	erNoSyncWork(t, env)
	ids := sqJobIDs(t, m)
	require.Len(t, ids, 3, out)
	assert.Equal(t, 3, env.jobs(t))
	for i, id := range ids {
		if i > 0 {
			assert.Greater(t, id, ids[i-1], "jobs are created in order")
		}
		job, p := erJob(t, env.store, id)
		assert.Equal(t, queue.JobRetouch, job.Type, "job %d", i)
		assert.Equal(t, queue.StatusPending, job.Status, "job %d", i)
		assert.Equal(t, "editor", job.Source, "job %d", i)
		label := sqLabel(t, job)
		assert.True(t, strings.HasPrefix(label, "Ретушь: "), "job %d label %q", i, label)
		assert.True(t, strings.HasSuffix(label, fmt.Sprintf(" (%d из 3)", i+1)), "job %d label %q", i, label)
		assert.Equal(t, items[i], p.ItemID, "job %d", i)
		assert.Empty(t, p.ImageBase64, "job %d: item jobs carry the id, not the image", i)
		assert.Equal(t, "4k", p.Size, "job %d: default size", i)
		assert.InDelta(t, 0.25, p.Detail, 1e-9, "job %d: default detail", i)
		assert.Equal(t, i+1, p.Index, "job %d", i)
		assert.Equal(t, 3, p.Total, "job %d", i)
	}
	job0, _ := erJob(t, env.store, ids[0])
	assert.Contains(t, sqLabel(t, job0), "Портрет в студии", "label carries the source caption")
}

func TestEditorRetouch_Response(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)
	photo := edWritePhoto(t, styPhotoPNG(80, 120, erGray))

	m, out := styCall(t, env.s, "editor_retouch", map[string]any{"photo_path": photo})

	ids := sqJobIDs(t, m)
	require.Len(t, ids, 1, out)
	hint, _ := m["hint"].(string)
	require.NotEmpty(t, strings.TrimSpace(hint), out)
	assert.Contains(t, strings.ToLower(hint), "приложени")
	erAssertNoLeak(t, out, photo, filepath.Dir(photo), env.out)
}

func TestEditorRetouch_PhotoPathJob(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)
	data := styPhotoPNG(80, 120, erGray)
	photo := edWritePhoto(t, data)

	m, _ := styCall(t, env.s, "editor_retouch", map[string]any{"photo_path": photo, "size": "2x", "detail": float64(0)})

	ids := sqJobIDs(t, m)
	require.Len(t, ids, 1)
	job, p := erJob(t, env.store, ids[0])
	assert.Equal(t, queue.JobRetouch, job.Type)
	assert.Zero(t, p.ItemID)
	require.NotEmpty(t, p.ImageBase64)
	assert.Equal(t, data, styDecodeB64(t, p.ImageBase64))
	assert.Equal(t, "2x", p.Size)
	assert.Zero(t, p.Detail)
	label := sqLabel(t, job)
	assert.True(t, strings.HasPrefix(label, "Ретушь: "), label)
	assert.True(t, strings.HasSuffix(label, " (1 из 1)"), label)
	erNoSyncWork(t, env)
}

func TestEditorRetouch_ItemArg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		item any
		want int64
	}{
		{name: "active", item: "active", want: erGenItem},
		{name: "numeric id", item: float64(erPhotoItem), want: erPhotoItem},
		{name: "string id", item: "43", want: erPhotoItem},
		{name: "stylist shoot frame is accepted", item: float64(erShootFrame), want: erShootFrame},
		{name: "stylist try variant is accepted", item: float64(erTryVariant), want: erTryVariant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := erNewEnv(t, nil)

			m, out := styCall(t, env.s, "editor_retouch", map[string]any{"item": tt.item})

			ids := sqJobIDs(t, m)
			require.Len(t, ids, 1, out)
			_, p := erJob(t, env.store, ids[0])
			assert.Equal(t, tt.want, p.ItemID)
			assert.Empty(t, p.ImageBase64)
		})
	}
}

func TestEditorRetouch_AdjustAndRetouchPassedToJob(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)

	m, out := styCall(t, env.s, "editor_retouch", map[string]any{
		"item":    "active",
		"size":    "off",
		"detail":  float64(0.4),
		"adjust":  map[string]any{"brightness": float64(10), "saturation": float64(-5)},
		"retouch": map[string]any{"under_eyes": true, "teeth": true, "eye_whites": true, "light": true},
	})

	ids := sqJobIDs(t, m)
	require.Len(t, ids, 1, out)
	_, p := erJob(t, env.store, ids[0])
	assert.Equal(t, "off", p.Size)
	assert.InDelta(t, 0.4, p.Detail, 1e-9, "0.4 is the inclusive upper bound")
	require.NotNil(t, p.Adjust)
	assert.InDelta(t, 10, p.Adjust.Brightness, 1e-9)
	assert.InDelta(t, -5, p.Adjust.Saturation, 1e-9)
	require.NotNil(t, p.Retouch)
	assert.True(t, p.Retouch.UnderEyes)
	assert.True(t, p.Retouch.Teeth)
	assert.True(t, p.Retouch.EyeWhites)
	assert.True(t, p.Retouch.Light)
}

func TestEditorRetouch_OnlyAdjustIsEnough(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)

	m, out := styCall(t, env.s, "editor_retouch", map[string]any{"item": "active", "size": "off", "detail": float64(0), "adjust": map[string]any{"contrast": float64(15)}})

	assert.Len(t, sqJobIDs(t, m), 1, out)
}

func TestEditorRetouch_ValidationNoJobs(t *testing.T) {
	t.Parallel()
	photoArg := func(t *testing.T) string { return edWritePhoto(t, styPhotoPNG(40, 40, erGray)) }
	thirteen := make([]int64, 13)
	for i := range thirteen {
		thirteen[i] = erGenItem
	}
	tests := []struct {
		name string
		args func(t *testing.T) map[string]any
	}{
		{name: "no photo", args: func(*testing.T) map[string]any { return map[string]any{} }},
		{name: "photo_path and item", args: func(t *testing.T) map[string]any {
			return map[string]any{"photo_path": photoArg(t), "item": "active"}
		}},
		{name: "item and items", args: func(*testing.T) map[string]any {
			return map[string]any{"item": "active", "items": sqIDsArg(erGenItem)}
		}},
		{name: "photo_path and items", args: func(t *testing.T) map[string]any {
			return map[string]any{"photo_path": photoArg(t), "items": sqIDsArg(erGenItem)}
		}},
		{name: "items empty", args: func(*testing.T) map[string]any { return map[string]any{"items": []any{}} }},
		{name: "items above 12", args: func(*testing.T) map[string]any { return map[string]any{"items": sqIDsArg(thirteen...)} }},
		{name: "items not integers", args: func(*testing.T) map[string]any { return map[string]any{"items": []any{"a", "b"}} }},
		{name: "unknown item among items", args: func(*testing.T) map[string]any { return map[string]any{"items": sqIDsArg(erGenItem, 9999)} }},
		{name: "unknown item", args: func(*testing.T) map[string]any { return map[string]any{"item": float64(9999)} }},
		{name: "unreadable photo_path", args: func(t *testing.T) map[string]any {
			return map[string]any{"photo_path": filepath.Join(t.TempDir(), "missing.png")}
		}},
		{name: "size 8k", args: func(*testing.T) map[string]any { return map[string]any{"item": "active", "size": "8k"} }},
		{name: "size 3x", args: func(*testing.T) map[string]any { return map[string]any{"item": "active", "size": "3x"} }},
		{name: "detail negative", args: func(*testing.T) map[string]any { return map[string]any{"item": "active", "detail": float64(-0.1)} }},
		{name: "detail above 0.4", args: func(*testing.T) map[string]any { return map[string]any{"item": "active", "detail": float64(0.5)} }},
		{name: "nothing to do", args: func(*testing.T) map[string]any {
			return map[string]any{"item": "active", "size": "off", "detail": float64(0)}
		}},
		{name: "nothing to do with neutral adjust and empty retouch", args: func(*testing.T) map[string]any {
			return map[string]any{"item": "active", "size": "off", "detail": float64(0), "adjust": map[string]any{}, "retouch": map[string]any{}}
		}},
		{name: "adjust out of range", args: func(*testing.T) map[string]any {
			return map[string]any{"item": "active", "adjust": map[string]any{"brightness": float64(500)}}
		}},
		{name: "adjust not a number", args: func(*testing.T) map[string]any {
			return map[string]any{"item": "active", "adjust": map[string]any{"brightness": "bright"}}
		}},
		{name: "retouch face_width out of range", args: func(*testing.T) map[string]any {
			return map[string]any{"item": "active", "retouch": map[string]any{"face_width": float64(50)}}
		}},
		{name: "retouch bad skin_tone", args: func(*testing.T) map[string]any {
			return map[string]any{"item": "active", "retouch": map[string]any{"skin_tone": "tanned"}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := erNewEnv(t, nil)

			out, err := styExec(env.s, "editor_retouch", tt.args(t))

			require.Error(t, err, out)
			assert.Zero(t, env.jobs(t), "no jobs on a validation error")
			erNoSyncWork(t, env)
		})
	}
}

func TestEditorRetouch_TwelveItemsAllowed(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)
	twelve := make([]int64, 12)
	for i := range twelve {
		twelve[i] = erGenItem
	}

	m, out := styCall(t, env.s, "editor_retouch", map[string]any{"items": sqIDsArg(twelve...)})

	assert.Len(t, sqJobIDs(t, m), 12, out)
	assert.Equal(t, 12, env.jobs(t))
}

func TestEditorRetouch_MissingDeps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		deps func(store *queue.Store) Deps
	}{
		{name: "no queue", deps: func(*queue.Store) Deps {
			return Deps{Sessions: erItemSess(), Retoucher: &fakeRetoucher{}}
		}},
		{name: "no retoucher", deps: func(store *queue.Store) Deps {
			return Deps{Sessions: erItemSess(), Queue: store}
		}},
		{name: "no sessions", deps: func(store *queue.Store) Deps {
			return Deps{Retoucher: &fakeRetoucher{}, Queue: store}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := sqNewStore(t)
			deps := tt.deps(store)
			deps.OutDir = t.TempDir()
			s := NewServer(deps)
			RegisterTools(s)
			photo := edWritePhoto(t, styPhotoPNG(40, 40, erGray))

			out, err := styExec(s, "editor_retouch", map[string]any{"photo_path": photo})

			require.Error(t, err, out)
			n, cerr := store.ActiveCount()
			require.NoError(t, cerr)
			assert.Zero(t, n)
		})
	}
}

func TestEditorRetouch_RequireLocalBackends(t *testing.T) {
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
		{name: "both local", llmURL: localLLM, sdURL: localSD},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := erNewEnv(t, &config.Config{LLMUrl: tt.llmURL, SDUrl: tt.sdURL})

			out, err := styExec(env.s, "editor_retouch", map[string]any{"item": "active"})

			if tt.wantErr {
				require.Error(t, err, out)
				assert.Zero(t, env.jobs(t))
				erNoSyncWork(t, env)
				return
			}
			require.NoError(t, err, out)
			assert.Equal(t, 1, env.jobs(t))
		})
	}
}

func erParamsJSON(t *testing.T, p RetouchJobParams) string {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	return string(raw)
}

func erStatusJobs(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["jobs"].([]any)
	require.True(t, ok, "jobs must be an array: %v", m)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		obj, ok := v.(map[string]any)
		require.True(t, ok)
		out = append(out, obj)
	}
	return out
}

func TestEditorRetouchStatus(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)
	b64 := base64.StdEncoding.EncodeToString(styPhotoPNG(30, 30, erGray))
	mk := func(p RetouchJobParams) int64 {
		id, err := env.store.CreateLabeled(queue.JobRetouch, erParamsJSON(t, p), "editor", fmt.Sprintf("Ретушь: кадр (%d из %d)", p.Index, p.Total))
		require.NoError(t, err)
		return id
	}
	done := mk(RetouchJobParams{ItemID: erGenItem, Size: "4k", Detail: 0.25, Index: 1, Total: 5})
	doneMin := mk(RetouchJobParams{ItemID: erPhotoItem, Size: "4k", Detail: 0.25, Index: 2, Total: 5})
	failed := mk(RetouchJobParams{ImageBase64: b64, Size: "4k", Detail: 0.25, Index: 3, Total: 5})
	running := mk(RetouchJobParams{ItemID: erShootFrame, Size: "4k", Detail: 0.25, Index: 4, Total: 5})
	pending := mk(RetouchJobParams{ImageBase64: b64, Size: "2x", Index: 5, Total: 5})
	require.NoError(t, env.store.UpdateCompleted(done, `{"session_item_id":601,"frame":{"id":601,"width":2560,"height":3840,"upscale":"done","detail":"done","adjust":true,"retouch":"done"}}`))
	require.NoError(t, env.store.UpdateCompleted(doneMin, `{"session_item_id":602,"frame":{"id":602,"width":3840,"height":2560,"upscale":"done","detail":"skipped:нет данных генерации"}}`))
	require.NoError(t, env.store.UpdateStatus(failed, queue.StatusFailed, "upscale failed: out of memory"))
	require.NoError(t, env.store.UpdateStatus(running, queue.StatusRunning, ""))
	other, err := env.store.CreateLabeled(queue.JobStylistFrame, `{"shot":"front","index":1,"total":1}`, "stylist", "Стилист: anna, front (1 из 1)")
	require.NoError(t, err)

	m, out := styCall(t, env.s, "editor_retouch_status", map[string]any{"job_ids": sqIDsArg(done, doneMin, failed, running, pending, other, 99999)})

	jobs := erStatusJobs(t, m)
	require.Len(t, jobs, 7, out)
	want := []int64{done, doneMin, failed, running, pending, other, 99999}
	for i, j := range jobs {
		assert.Equal(t, float64(want[i]), j["job_id"], "job %d", i)
	}

	assert.Equal(t, "completed", jobs[0]["status"])
	assert.Equal(t, float64(601), jobs[0]["id"])
	assert.Equal(t, float64(2560), jobs[0]["width"])
	assert.Equal(t, float64(3840), jobs[0]["height"])
	assert.Equal(t, "done", jobs[0]["upscale"])
	assert.Equal(t, "done", jobs[0]["detail"])
	assert.Equal(t, true, jobs[0]["adjust"])
	assert.Equal(t, "done", jobs[0]["retouch"])
	assert.Equal(t, float64(1), jobs[0]["index"])
	assert.Equal(t, float64(5), jobs[0]["total"])

	assert.Equal(t, "completed", jobs[1]["status"])
	assert.Equal(t, float64(602), jobs[1]["id"])
	assert.Equal(t, "skipped:нет данных генерации", jobs[1]["detail"])
	assert.NotContains(t, jobs[1], "retouch")

	assert.Equal(t, "failed", jobs[2]["status"])
	errText, _ := jobs[2]["error"].(string)
	assert.Contains(t, errText, "out of memory")
	assert.Equal(t, float64(3), jobs[2]["index"])
	assert.NotContains(t, jobs[2], "id")

	assert.Equal(t, "running", jobs[3]["status"])
	assert.NotContains(t, jobs[3], "id")

	assert.Equal(t, "pending", jobs[4]["status"])
	assert.Equal(t, float64(5), jobs[4]["index"])
	assert.Equal(t, float64(5), jobs[4]["total"])
	assert.NotContains(t, jobs[4], "id")

	assert.Equal(t, "not_found", jobs[5]["status"], "a job of another type is not a retouch job")
	assert.Equal(t, "not_found", jobs[6]["status"])

	erAssertNoLeak(t, out, b64, env.out)
}

func TestEditorRetouchStatus_BadArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "job_ids missing", args: map[string]any{}},
		{name: "job_ids empty", args: map[string]any{"job_ids": []any{}}},
		{name: "job_ids not an array", args: map[string]any{"job_ids": "1,2"}},
		{name: "job_ids not integers", args: map[string]any{"job_ids": []any{1.5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := erNewEnv(t, nil)

			out, err := styExec(env.s, "editor_retouch_status", tt.args)

			require.Error(t, err, out)
		})
	}
}

func TestEditorRetouchStatus_NoQueue(t *testing.T) {
	t.Parallel()
	s := NewServer(Deps{OutDir: t.TempDir(), Sessions: erItemSess(), Retoucher: &fakeRetoucher{}})
	RegisterTools(s)

	out, err := styExec(s, "editor_retouch_status", map[string]any{"job_ids": sqIDsArg(1)})

	require.Error(t, err, out)
}

func erPromptText(t *testing.T, name string) string {
	t.Helper()
	s, _ := styNewServer(t, nil, nil)
	m := styRPC(t, s, "prompts/get", map[string]any{"name": name})
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
	return text
}

func TestRetouchPrompts(t *testing.T) {
	t.Parallel()

	t.Run("photo editor knows editor_retouch and strong detail", func(t *testing.T) {
		t.Parallel()
		text := erPromptText(t, "photo_editor")
		assert.Contains(t, text, "editor_retouch")
		assert.Contains(t, text, "editor_retouch_status")
		assert.Contains(t, text, "0.4")
	})

	t.Run("stylist offers retouch after a series", func(t *testing.T) {
		t.Parallel()
		text := erPromptText(t, "stylist")
		assert.Contains(t, text, "editor_retouch")
	})
}

func TestEditorRetouch_ToolsListed(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)
	_, ok := env.s.tools["editor_retouch"]
	assert.True(t, ok)
	_, ok = env.s.tools["editor_retouch_status"]
	assert.True(t, ok)
}
