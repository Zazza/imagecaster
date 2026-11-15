package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

var fastModels = []struct {
	model string
	cache string
}{
	{"moss_image_sft_bf16", "dbcache"},
	{"flux1-dev-fp8", "easycache"},
	{"qwen_image_2.1-Q8_0", "easycache"},
	{"Chroma1-HD", "dbcache"},
	{"juggernautXL_ragnarokBy", ""},
	{"dreamshaper_8", ""},
	{"z-image-turbo-fp8-aio", ""},
	{"flux-2-klein-4b", ""},
	{"unknown-model-xyz", ""},
}

func setFastGlobal(t *testing.T, db *preset.DB, on *bool) {
	t.Helper()
	if on == nil {
		return
	}
	v := "false"
	if *on {
		v = "true"
	}
	require.NoError(t, db.SetSetting("fast_generation", v))
}

func TestCacheModeFor_Matrix(t *testing.T) {
	t.Parallel()
	for _, global := range []bool{true, false} {
		for _, m := range fastModels {
			name := map[bool]string{true: "global=true", false: "global=false"}[global] + "/" + m.model
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				db := openTestDB(t)
				g := global
				setFastGlobal(t, db, &g)
				svc := newTestService(t, db, &mockLLM{}, &mockSD{})
				want := ""
				if global {
					want = m.cache
				}
				assert.Equal(t, want, svc.cacheModeFor(m.model))
			})
		}
	}
}

func TestCacheModeFor_GlobalDefaultIsOff(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	svc := newTestService(t, db, &mockLLM{}, &mockSD{})
	assert.Equal(t, "", svc.cacheModeFor("flux1-dev-fp8"))
}

type capturedReqs struct {
	mu    sync.Mutex
	modes []string
}

func (c *capturedReqs) add(m string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modes = append(c.modes, m)
}

func (c *capturedReqs) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.modes...)
}

func capturingSD(c *capturedReqs) *mockSD {
	return &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			c.add(req.CacheMode)
			return &sd.Txt2ImgResponse{Images: []string{"img"}, Info: json.RawMessage(`{"seed":1}`), Parameters: json.RawMessage(`{}`)}, nil
		},
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			c.add(req.CacheMode)
			return &sd.Txt2ImgResponse{Images: []string{"img"}, Info: json.RawMessage(`{"seed":1}`), Parameters: json.RawMessage(`{}`)}, nil
		},
	}
}

func boolp(v bool) *bool { return &v }

func TestGenerateImage_SendsCacheMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		model  string
		global *bool
		hires  bool
		want   string
	}{
		{"global on flux", "flux1-dev-fp8", boolp(true), false, "easycache"},
		{"global on sdxl gets no cache", "juggernautXL_ragnarokBy", boolp(true), false, ""},
		{"global on moss", "moss_image_sft_bf16", boolp(true), false, "dbcache"},
		{"global on chroma", "Chroma1-HD", boolp(true), false, "dbcache"},
		{"global on no-cache base", "z-image-turbo-fp8-aio", boolp(true), false, ""},
		{"global off", "flux1-dev-fp8", boolp(false), false, ""},
		{"global unset", "flux1-dev-fp8", nil, false, ""},
		{"hires on", "Chroma1-HD", boolp(true), true, "dbcache"},
		{"hires off", "Chroma1-HD", boolp(false), true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			setFastGlobal(t, db, tt.global)
			p := makeTestPreset(t, db, &preset.Preset{Name: "fm", PresetType: "txt2img", Prompt: "cat", Sampler: "euler", Steps: 10, CfgScale: 5, ModelName: tt.model})
			params := GenerateImageParams{PresetID: p.ID}
			if tt.hires {
				hp := &preset.HiresProfile{Name: "hr", Upscale: 2.0, DenoisingStrength: 0.5, Upscaler: "Latent"}
				require.NoError(t, db.CreateHiresProfile(hp))
				params.HiresProfileID = &hp.ID
			}
			reqs := &capturedReqs{}
			svc := newTestService(t, db, &mockLLM{}, capturingSD(reqs))
			svc.ctx = context.Background()
			_, err := svc.GenerateImage(params)
			require.NoError(t, err)
			got := reqs.all()
			require.NotEmpty(t, got)
			for i, m := range got {
				assert.Equal(t, tt.want, m, "request %d", i)
			}
		})
	}
}

func jobBodyServer(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/jobs":
			b, _ := io.ReadAll(r.Body)
			var body map[string]any
			if err := json.Unmarshal(b, &body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"j1","status":"completed"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/jobs/j1/result":
			_, _ = w.Write([]byte(`{"images":["img"],"info":{"seed":7}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/jobs/j1":
			_, _ = w.Write([]byte(`{"id":"j1","status":"completed"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/jobs/j1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/state":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), bodies...)
	}
}

func TestGenerateImage_JobJSONCacheMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		model  string
		global string
		want   string
	}{
		{"on sends cache_mode", "qwen_image_2.1-Q8_0", "true", "easycache"},
		{"on sends dbcache for chroma", "Chroma1-HD", "true", "dbcache"},
		{"off omits cache_mode", "qwen_image_2.1-Q8_0", "false", ""},
		{"no-cache base omits cache_mode", "flux-2-klein-4b", "true", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			require.NoError(t, db.SetSetting("fast_generation", tt.global))
			p := makeTestPreset(t, db, &preset.Preset{Name: "fm", PresetType: "txt2img", Prompt: "cat", Sampler: "euler", Steps: 10, CfgScale: 5, ModelName: tt.model})
			ts, bodies := jobBodyServer(t)
			client := sd.New(ts.URL)
			client.PollInterval = time.Millisecond
			svc := New(db, &mockLLM{}, client, &config.Config{SDPromptModel: "m", VisionModel: "v"}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
			svc.ctx = context.Background()
			_, err := svc.GenerateImage(GenerateImageParams{PresetID: p.ID})
			require.NoError(t, err)
			got := bodies()
			require.NotEmpty(t, got)
			for i, b := range got {
				params, ok := b["params"].(map[string]any)
				require.True(t, ok, "job %d params", i)
				if tt.want == "" {
					assert.NotContains(t, params, "cache_mode", "job %d", i)
					continue
				}
				assert.Equal(t, tt.want, params["cache_mode"], "job %d", i)
			}
		})
	}
}

type modeCapture struct {
	mu  sync.Mutex
	txt []string
	img []string
}

func (c *modeCapture) sd() *mockSD {
	return &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			c.mu.Lock()
			c.txt = append(c.txt, req.CacheMode)
			c.mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{"step-img"}, Info: json.RawMessage(`{"seed":1}`), Parameters: json.RawMessage(`{}`)}, nil
		},
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			c.mu.Lock()
			c.img = append(c.img, req.CacheMode)
			c.mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{"step-img"}, Info: json.RawMessage(`{"seed":1}`), Parameters: json.RawMessage(`{}`)}, nil
		},
	}
}

func (c *modeCapture) get() ([]string, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.txt...), append([]string(nil), c.img...)
}

func TestGenerateCompoundImage_SendsCacheMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		global  *bool
		wantTxt string
		wantImg string
	}{
		{"global on", boolp(true), "easycache", "dbcache"},
		{"global off", boolp(false), "", ""},
		{"global unset", nil, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			setFastGlobal(t, db, tt.global)
			first := makeTestPreset(t, db, &preset.Preset{Name: "c1", PresetType: "txt2img", Prompt: "cat", Sampler: "euler", Steps: 10, CfgScale: 5, ModelName: "flux1-dev-fp8"})
			second := makeTestPreset(t, db, &preset.Preset{Name: "c2", PresetType: "txt2img", Prompt: "oil", Sampler: "euler", Steps: 10, CfgScale: 5, ModelName: "Chroma1-HD"})
			cp := &preset.CompoundPreset{Name: "pipe", Steps: []preset.CompoundPresetStep{
				{StepOrder: 1, PresetID: first.ID},
				{StepOrder: 2, PresetID: second.ID, DenoisingStrength: 0.6},
			}}
			require.NoError(t, db.CreateCompoundPreset(cp))
			capt := &modeCapture{}
			llmSvc := &mockLLM{genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
				return `{"prompt":"scene","negative_prompt":"neg"}`, nil
			}}
			svc := newTestService(t, db, llmSvc, capt.sd())
			svc.ctx = context.Background()
			_, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{CompoundPresetID: cp.ID, ExtraPrompt: "a cat"})
			require.NoError(t, err)
			txt, img := capt.get()
			require.NotEmpty(t, txt, "txt2img step")
			require.NotEmpty(t, img, "img2img step")
			for i, m := range txt {
				assert.Equal(t, tt.wantTxt, m, "txt2img %d", i)
			}
			for i, m := range img {
				assert.Equal(t, tt.wantImg, m, "img2img %d", i)
			}
		})
	}
}

func TestGenerateFromImage_SendsCacheMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		model  string
		global *bool
		want   string
	}{
		{"global on sdxl gets no cache", "juggernautXL_ragnarokBy", boolp(true), ""},
		{"global on flux", "flux1-dev-fp8", boolp(true), "easycache"},
		{"global on chroma", "Chroma1-HD", boolp(true), "dbcache"},
		{"global on no-cache base", "z-image-turbo-fp8-aio", boolp(true), ""},
		{"global off", "flux1-dev-fp8", boolp(false), ""},
		{"global unset", "flux1-dev-fp8", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			setFastGlobal(t, db, tt.global)
			p := makeTestPreset(t, db, &preset.Preset{Name: "fi", PresetType: "txt2img", Prompt: "1girl", Sampler: "euler", Steps: 10, CfgScale: 5, ModelName: tt.model})
			capt := &modeCapture{}
			svc := newTestService(t, db, &mockLLM{}, capt.sd())
			svc.ctx = context.Background()
			_, err := svc.GenerateFromImage(GenerateFromImageParams{
				ImageBase64:       makePNGBase64(t, 64, 64),
				GenMode:           "preset",
				Mode:              "img2img",
				PresetID:          p.ID,
				DenoisingStrength: 0.6,
			})
			require.NoError(t, err)
			txt, img := capt.get()
			require.NotEmpty(t, img, "img2img request")
			for i, m := range append(txt, img...) {
				assert.Equal(t, tt.want, m, "request %d", i)
			}
		})
	}
}
