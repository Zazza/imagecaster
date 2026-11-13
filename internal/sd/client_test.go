package sd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "http", in: "http://new:9090", want: "http://new:9090"},
		{name: "https", in: "https://host/api/sd", want: "https://host/api/sd"},
		{name: "empty", in: "", want: ""},
		{name: "bad scheme ignored", in: "ftp://x", want: "http://old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := New("http://old")
			c.SetURL(tt.in)
			assert.Equal(t, tt.want, c.base())
		})
	}
}

func serveJSON(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"no route"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestHealthCheck(t *testing.T) {
	t.Parallel()
	ok := serveJSON(t, map[string]string{"GET /state": `{"loaded_checkpoint":"m.safetensors","running_job":""}`})
	require.NoError(t, New(ok.URL).HealthCheck())

	bad := serveJSON(t, map[string]string{})
	assert.Error(t, New(bad.URL).HealthCheck())

	assert.Error(t, New("http://127.0.0.1:1").HealthCheck())
}

func TestState(t *testing.T) {
	t.Parallel()
	ts := serveJSON(t, map[string]string{"GET /state": `{"loaded_checkpoint":"flux/m.safetensors","running_job":"j1"}`})
	st, err := New(ts.URL).State()
	require.NoError(t, err)
	assert.Equal(t, "flux/m.safetensors", st.LoadedCheckpoint)
	assert.Equal(t, "j1", st.RunningJob)
}

func TestLists(t *testing.T) {
	t.Parallel()
	ts := serveJSON(t, map[string]string{
		"GET /checkpoints": `{"checkpoints":[{"name":"flux/flux1-dev.safetensors","file":"/m/flux/flux1-dev.safetensors"}]}`,
		"GET /samplers":    `{"samplers":["euler","euler_a"]}`,
		"GET /schedulers":  `{"schedulers":["","karras"]}`,
		"GET /upscalers":   `{"upscalers":[{"name":"lanczos","scale":4},{"name":"RealESRGAN_x4plus","scale":4}]}`,
		"GET /vaes":        `{"vaes":[{"name":"ae.safetensors","file":"/v/ae.safetensors"}]}`,
		"GET /loras":       `{"loras":[{"name":"detail","path":"sd15/detail.safetensors"}]}`,
	})
	c := New(ts.URL)

	models, err := c.GetModels()
	require.NoError(t, err)
	assert.Equal(t, []SDModel{{Title: "flux/flux1-dev.safetensors", Name: "flux/flux1-dev", Config: "/m/flux/flux1-dev.safetensors"}}, models)

	samplers, err := c.GetSamplers()
	require.NoError(t, err)
	assert.Equal(t, []Sampler{{Name: "euler"}, {Name: "euler_a"}}, samplers)

	schedulers, err := c.GetSchedulers()
	require.NoError(t, err)
	assert.Equal(t, []Scheduler{{Name: "karras", Label: "karras"}}, schedulers)

	upscalers, err := c.GetUpscalers()
	require.NoError(t, err)
	assert.Equal(t, []Upscaler{{Name: "lanczos", Model: "lanczos", Scale: 4}, {Name: "RealESRGAN_x4plus", Model: "RealESRGAN_x4plus", Scale: 4}}, upscalers)

	vaes, err := c.GetVAEs()
	require.NoError(t, err)
	assert.Equal(t, []VAE{{ModelName: "ae.safetensors", Path: "/v/ae.safetensors"}}, vaes)

	loras, err := c.GetLoRAs()
	require.NoError(t, err)
	assert.Equal(t, []LoRA{{Name: "detail", Path: "sd15/detail.safetensors"}}, loras)
}

func TestLists_ServerErrorCarriesCodeAndMessage(t *testing.T) {
	t.Parallel()
	ts := serveJSON(t, map[string]string{})
	_, err := New(ts.URL).GetSamplers()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not_found")
	assert.Contains(t, err.Error(), "no route")
}

func captureSubmit(t *testing.T) (*httptest.Server, *map[string]any) {
	t.Helper()
	var body map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/jobs":
			b, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(b, &body))
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"j1","status":"completed"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/jobs/j1/result":
			_, _ = w.Write([]byte(`{"images":["img"],"info":{"seed":7}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/jobs/j1":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, &body
}

func TestTxt2Img_JobCarriesModelAndParams(t *testing.T) {
	t.Parallel()
	ts, body := captureSubmit(t)
	c := New(ts.URL)
	c.PollInterval = time.Millisecond

	seed := int64(42)
	clip := 2
	bs, bc := 2, 3
	hr := true
	scale, hrDen := 1.5, 0.4
	resp, err := c.Txt2Img(Txt2ImgRequest{
		Model:                  ModelSpec{Checkpoint: "flux1-dev", VAE: "ae.safetensors", Modules: []string{"t5.safetensors"}},
		Prompt:                 "cat",
		NegativePrompt:         "dog",
		SamplerName:            "euler",
		Scheduler:              "simple",
		Steps:                  20,
		CfgScale:               3.5,
		Width:                  768,
		Height:                 512,
		Seed:                   &seed,
		ClipSkip:               &clip,
		BatchSize:              &bs,
		BatchCount:             &bc,
		HiresFix:               &hr,
		HiresUpscale:           &scale,
		HiresDenoisingStrength: &hrDen,
		HiresUpscaler:          "RealESRGAN_x4plus",
		HiresSecondPassSteps:   10,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"img"}, resp.Images)
	assert.JSONEq(t, `{"seed":7}`, string(resp.Info))

	b := *body
	assert.Equal(t, "txt2img", b["kind"])
	assert.NotEmpty(t, b["client_ref"])
	assert.Equal(t, map[string]any{"checkpoint": "flux1-dev", "vae": "ae.safetensors", "modules": []any{"t5.safetensors"}, "klein_reference": false, "moss_reference": false}, b["model"])
	p := b["params"].(map[string]any)
	assert.Equal(t, "cat", p["prompt"])
	assert.Equal(t, "dog", p["negative_prompt"])
	assert.Equal(t, "euler", p["sampler"])
	assert.Equal(t, "simple", p["scheduler"])
	assert.Equal(t, float64(42), p["seed"])
	assert.Equal(t, float64(2), p["clip_skip"])
	assert.Equal(t, float64(6), p["batch_size"])
	assert.Equal(t, map[string]any{"enable": true, "scale": 1.5, "upscaler": "RealESRGAN_x4plus", "denoising": 0.4, "steps": float64(10), "resize_x": float64(0), "resize_y": float64(0)}, p["hires"])
}

func TestImg2Img_JobCarriesReferenceAndInitImage(t *testing.T) {
	t.Parallel()
	ts, body := captureSubmit(t)
	c := New(ts.URL)
	c.PollInterval = time.Millisecond

	den := 0.6
	_, err := c.Img2Img(Img2ImgRequest{
		Model:             ModelSpec{Checkpoint: "flux-2-klein-4b.safetensors", KleinReference: true},
		InitImages:        []string{"init"},
		Mask:              "mask",
		Prompt:            "p",
		DenoisingStrength: &den,
	})
	require.NoError(t, err)

	b := *body
	assert.Equal(t, "img2img", b["kind"])
	m := b["model"].(map[string]any)
	assert.Equal(t, true, m["klein_reference"])
	assert.Equal(t, []any{}, m["modules"])
	p := b["params"].(map[string]any)
	assert.Equal(t, "init", p["init_image"])
	assert.Equal(t, "mask", p["mask"])
	assert.Equal(t, 0.6, p["denoising_strength"])
	assert.Equal(t, float64(-1), p["seed"])
	assert.Equal(t, float64(1), p["batch_size"])
	assert.NotContains(t, p, "hires")
}

func TestSetClientName(t *testing.T) {
	t.Parallel()
	ts, body := captureSubmit(t)
	c := New(ts.URL)
	c.PollInterval = time.Millisecond
	c.SetClientName("imagecaster-mcp")
	_, err := c.UpscaleImage("img", "lanczos", 2)
	require.NoError(t, err)
	assert.Equal(t, "imagecaster-mcp", (*body)["client"])
	assert.NotContains(t, *body, "model")
}
