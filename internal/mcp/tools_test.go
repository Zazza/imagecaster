package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

type tlSD struct {
	mu        sync.Mutex
	txt       []sd.Txt2ImgRequest
	img       []sd.Img2ImgRequest
	upscales  int
	failSeeds map[int64]bool
	badImage  bool
	models    []sd.SDModel
}

func (f *tlSD) result(prompt string, seed *int64) (*sd.Txt2ImgResponse, error) {
	var s int64
	if seed != nil {
		s = *seed
	}
	if f.failSeeds[s] {
		return nil, fmt.Errorf("sd failed for seed %d", s)
	}
	img := base64.StdEncoding.EncodeToString(styPNG(int(s%5) + 1))
	if f.badImage {
		img = "!!not-base64!!"
	}
	info, err := json.Marshal(map[string]any{"seed": s, "prompt": prompt})
	if err != nil {
		return nil, err
	}
	return &sd.Txt2ImgResponse{Images: []string{img}, Info: info}, nil
}

func (f *tlSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.mu.Lock()
	f.txt = append(f.txt, req)
	f.mu.Unlock()
	return f.result(req.Prompt, req.Seed)
}

func (f *tlSD) Img2Img(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.mu.Lock()
	f.img = append(f.img, req)
	f.mu.Unlock()
	return f.result(req.Prompt, req.Seed)
}

func (f *tlSD) UpscaleImage(b64 string, _ string, _ float64) (string, error) {
	f.mu.Lock()
	f.upscales++
	f.mu.Unlock()
	return b64, nil
}
func (f *tlSD) GetModels() ([]sd.SDModel, error)       { return f.models, nil }
func (f *tlSD) GetSamplers() ([]sd.Sampler, error)     { return nil, nil }
func (f *tlSD) GetSchedulers() ([]sd.Scheduler, error) { return nil, nil }
func (f *tlSD) GetUpscalers() ([]sd.Upscaler, error)   { return nil, nil }
func (f *tlSD) GetVAEs() ([]sd.VAE, error)             { return nil, nil }
func (f *tlSD) GetLoRAs() ([]sd.LoRA, error)           { return nil, nil }
func (f *tlSD) State() (*sd.ServerState, error)        { return &sd.ServerState{}, nil }
func (f *tlSD) ListJobs(context.Context, string) ([]sd.JobStatus, error) {
	return nil, nil
}
func (f *tlSD) SetProgressHandler(func(sd.JobStatus)) {}
func (f *tlSD) Interrupt() error                      { return nil }
func (f *tlSD) HealthCheck() error                    { return nil }
func (f *tlSD) SetURL(string)                         {}
func (f *tlSD) MemoryInfo() (*sd.MemoryStats, error)  { return nil, nil }

func (f *tlSD) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.txt) + len(f.img) + f.upscales
}

func (f *tlSD) txtSeeds() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int64, 0, len(f.txt))
	for _, r := range f.txt {
		if r.Seed == nil {
			out = append(out, -1)
			continue
		}
		out = append(out, *r.Seed)
	}
	return out
}

func (f *tlSD) txtReqs() []sd.Txt2ImgRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sd.Txt2ImgRequest(nil), f.txt...)
}

func (f *tlSD) imgReqs() []sd.Img2ImgRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sd.Img2ImgRequest(nil), f.img...)
}

type tlLLMCall struct {
	kind  string
	text  string
	image string
}

type tlLLM struct {
	mu       sync.Mutex
	recorded []tlLLMCall
	err      error
}

func (l *tlLLM) record(kind, text, image string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recorded = append(l.recorded, tlLLMCall{kind: kind, text: text, image: image})
	return l.err
}

func (l *tlLLM) calls() []tlLLMCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]tlLLMCall(nil), l.recorded...)
}

const tlLLMPrompt = `{"prompt":"llm scene tags","negative_prompt":"llm negative"}`

func (l *tlLLM) Chat(_, _, user string, _ float64, _ int) (string, error) {
	if err := l.record("chat", user, ""); err != nil {
		return "", err
	}
	return tlLLMPrompt, nil
}
func (l *tlLLM) ChatVision(_, _, user, img string, _ float64, _ int) (string, error) {
	if err := l.record("vision", user, img); err != nil {
		return "", err
	}
	return "a red mug", nil
}
func (l *tlLLM) ChatWithMessages(string, []llm.Message, float64, int) (string, error) {
	if err := l.record("messages", "", ""); err != nil {
		return "", err
	}
	return tlLLMPrompt, nil
}
func (l *tlLLM) GenerateSDPrompt(_, description, _, _ string, _ int) (string, error) {
	if err := l.record("sdprompt", description, ""); err != nil {
		return "", err
	}
	return tlLLMPrompt, nil
}
func (l *tlLLM) AnalyzeImage(_, _, img string, _ int) (string, error) {
	if err := l.record("analyze", "", img); err != nil {
		return "", err
	}
	return "masterpiece, cat, sofa", nil
}
func (l *tlLLM) AnalyzeImageDescribe(_, prompt, img string, _ int) (string, error) {
	if err := l.record("describe", prompt, img); err != nil {
		return "", err
	}
	return "A cat sleeps on a red sofa.", nil
}
func (l *tlLLM) GetModels() ([]llm.LLMModel, error)     { return nil, nil }
func (l *tlLLM) HealthCheck() error                     { return nil }
func (l *tlLLM) SetURL(string)                          {}
func (l *tlLLM) ListLoaded() ([]llm.LoadedModel, error) { return nil, nil }
func (l *tlLLM) UnloadAll(context.Context) error        { return nil }

type tlSessions struct{}

func (tlSessions) AddToSession(string, json.RawMessage, string, *preset.GenerationMeta) int64 {
	return 1
}

type tlEnv struct {
	srv    *Server
	db     *preset.DB
	sd     *tlSD
	llm    *tlLLM
	outDir string
	preset *preset.Preset
}

func newTLEnv(t *testing.T, llmURL, sdURL string) *tlEnv {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	p := &preset.Preset{Name: "Oil Paint", PresetType: "realism", Prompt: "oil painting", NegativePrompt: "lowres", ModelName: "sdxl.safetensors", Sampler: "euler_a", Steps: 20, CfgScale: 7}
	require.NoError(t, db.Create(p))
	fsd := &tlSD{models: []sd.SDModel{{Title: "sdxl.safetensors", Name: "sdxl"}}}
	fllm := &tlLLM{}
	cfg := &config.Config{LLMUrl: llmURL, SDUrl: sdURL, SDPromptModel: "prompt-model", VisionModel: "vision-model"}
	gen := generation.New(db, fllm, fsd, cfg, t.TempDir(), NoopEmitter{}, kids.NewManager(db), tlSessions{}, logger.New(nil))
	outDir := t.TempDir()
	srv := NewServer(Deps{DB: db, Gen: gen, SD: fsd, LLM: fllm, Kids: kids.NewManager(db), Cfg: cfg, OutDir: outDir})
	RegisterTools(srv)
	return &tlEnv{srv: srv, db: db, sd: fsd, llm: fllm, outDir: outDir, preset: p}
}

func newLocalTLEnv(t *testing.T) *tlEnv {
	return newTLEnv(t, "http://127.0.0.1:11434", "http://127.0.0.1:7860")
}

func tlWritePNG(t *testing.T, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), fmt.Sprintf("src-%d.png", n))
	require.NoError(t, os.WriteFile(path, styPNG(n), 0o600))
	return path
}

func tlWriteMask(t *testing.T) string {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			m.Set(x, y, color.NRGBA{A: 255})
			if x >= 16 && x < 32 && y >= 16 && y < 32 {
				m.Set(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	path := filepath.Join(t.TempDir(), "mask.png")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, png.Encode(f, m))
	return path
}

func tlDecodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	img, err := png.Decode(f)
	require.NoError(t, err)
	return img
}

func TestTools_PhotoToolsRequireLocalBackends(t *testing.T) {
	t.Parallel()
	type toolCase struct {
		tool string
		args func(t *testing.T, e *tlEnv) map[string]any
	}
	tools := []toolCase{
		{tool: "analyze", args: func(t *testing.T, e *tlEnv) map[string]any {
			return map[string]any{"image_path": tlWritePNG(t, 1), "mode": "describe"}
		}},
		{tool: "analyze", args: func(t *testing.T, e *tlEnv) map[string]any {
			return map[string]any{"image_base64": base64.StdEncoding.EncodeToString(styPNG(1))}
		}},
		{tool: "from_image", args: func(t *testing.T, e *tlEnv) map[string]any {
			return map[string]any{"image_path": tlWritePNG(t, 2), "preset_id": float64(e.preset.ID)}
		}},
		{tool: "upscale", args: func(t *testing.T, e *tlEnv) map[string]any {
			path := tlWritePNG(t, 3)
			require.NoError(t, os.WriteFile(sidecarInfoPath(path), []byte(`{"prompt":"cat","seed":5,"steps":20,"width":48,"height":48,"sd_model_name":"sdxl.safetensors"}`), 0o600))
			return map[string]any{"image_path": path}
		}},
		{tool: "edit", args: func(t *testing.T, e *tlEnv) map[string]any {
			return map[string]any{"image_path": tlWritePNG(t, 4), "instruction": "add a hat"}
		}},
		{tool: "look", args: func(t *testing.T, e *tlEnv) map[string]any {
			return map[string]any{"image_path": tlWritePNG(t, 5), "text": "golden hour"}
		}},
		{tool: "object_edit", args: func(t *testing.T, e *tlEnv) map[string]any {
			return map[string]any{"image_path": tlWritePNG(t, 1), "mask_path": tlWriteMask(t), "op": "remove"}
		}},
	}
	backends := []struct {
		name   string
		llmURL string
		sdURL  string
		remote bool
	}{
		{name: "public LLM", llmURL: "http://8.8.8.8:11434", sdURL: "http://127.0.0.1:7860", remote: true},
		{name: "public SD", llmURL: "http://127.0.0.1:11434", sdURL: "https://1.1.1.1/api/sd", remote: true},
		{name: "local", llmURL: "http://127.0.0.1:11434", sdURL: "http://127.0.0.1:7860"},
	}
	for _, b := range backends {
		for _, tc := range tools {
			t.Run(b.name+"/"+tc.tool, func(t *testing.T) {
				t.Parallel()
				e := newTLEnv(t, b.llmURL, b.sdURL)
				_, err := styExec(e.srv, tc.tool, tc.args(t, e))
				if b.remote {
					require.Error(t, err)
					assert.Contains(t, err.Error(), "не в локальной сети")
					assert.Contains(t, err.Error(), tc.tool)
					assert.Zero(t, e.sd.calls(), "SD must not receive the photo")
					assert.Empty(t, e.llm.calls(), "LLM must not receive the photo")
					entries, rerr := os.ReadDir(e.outDir)
					require.NoError(t, rerr)
					assert.Empty(t, entries)
					return
				}
				require.NoError(t, err)
				assert.True(t, e.sd.calls() > 0 || len(e.llm.calls()) > 0, "local backend must receive the request")
			})
		}
	}
}

func TestTools_TextToolsWorkWithRemoteBackends(t *testing.T) {
	t.Parallel()
	e := newTLEnv(t, "http://8.8.8.8:11434", "http://1.1.1.1:7860")
	m, _ := styCall(t, e.srv, "generate", map[string]any{"preset_id": float64(e.preset.ID), "description": "a cat", "seed": float64(3)})
	assert.NotEmpty(t, m["path"])
	assert.Len(t, e.sd.txtReqs(), 1)
}

func TestTools_Analyze(t *testing.T) {
	t.Parallel()
	src := styPNG(2)
	srcB64 := base64.StdEncoding.EncodeToString(src)
	tests := []struct {
		name     string
		args     func(path string) map[string]any
		want     string
		wantKind string
		wantErr  string
	}{
		{name: "tags from path", args: func(p string) map[string]any { return map[string]any{"image_path": p} }, want: "cat", wantKind: "analyze"},
		{name: "describe from base64", args: func(string) map[string]any {
			return map[string]any{"image_base64": srcB64, "mode": "describe"}
		}, want: "A cat sleeps on a red sofa.", wantKind: "describe"},
		{name: "bad mode", args: func(p string) map[string]any { return map[string]any{"image_path": p, "mode": "poem"} }, wantErr: "mode должен быть"},
		{name: "no image", args: func(string) map[string]any { return map[string]any{} }, wantErr: "нужен image_path или image_base64"},
		{name: "missing file", args: func(string) map[string]any { return map[string]any{"image_path": "/nonexistent/x.png"} }, wantErr: "x.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			path := filepath.Join(t.TempDir(), "a.png")
			require.NoError(t, os.WriteFile(path, src, 0o600))
			out, err := styExec(e.srv, "analyze", tt.args(path))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, e.llm.calls())
				return
			}
			require.NoError(t, err)
			assert.Contains(t, out, tt.want)
			calls := e.llm.calls()
			require.Len(t, calls, 1)
			assert.Equal(t, tt.wantKind, calls[0].kind)
			assert.Equal(t, srcB64, calls[0].image)
		})
	}
}

func TestTools_GeneratePresetBranch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       map[string]any
		wantLLM    bool
		wantPrompt string
		save       bool
	}{
		{name: "description goes through LLM", args: map[string]any{"description": "a cat on a roof", "seed": float64(42)}, wantLLM: true, wantPrompt: "llm scene tags", save: true},
		{name: "no description: preset prompt only", args: map[string]any{"seed": float64(42)}, wantPrompt: "oil painting", save: true},
		{name: "save=false returns base64", args: map[string]any{"seed": float64(42), "save": false}, wantPrompt: "oil painting"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			args := map[string]any{"preset_id": float64(e.preset.ID)}
			for k, v := range tt.args {
				args[k] = v
			}
			m, _ := styCall(t, e.srv, "generate", args)

			reqs := e.sd.txtReqs()
			require.Len(t, reqs, 1)
			assert.Contains(t, reqs[0].Prompt, tt.wantPrompt)
			require.NotNil(t, reqs[0].Seed)
			assert.Equal(t, int64(42), *reqs[0].Seed)
			assert.Equal(t, float64(42), m["seed"])

			var sdPromptCalls []tlLLMCall
			for _, c := range e.llm.calls() {
				if c.kind == "sdprompt" || c.kind == "chat" || c.kind == "messages" {
					sdPromptCalls = append(sdPromptCalls, c)
				}
			}
			if tt.wantLLM {
				require.NotEmpty(t, sdPromptCalls)
			} else {
				assert.Empty(t, sdPromptCalls)
			}

			if tt.save {
				path, _ := m["path"].(string)
				require.NotEmpty(t, path)
				assert.Equal(t, e.outDir, filepath.Dir(path))
				assert.True(t, strings.HasPrefix(filepath.Base(path), "gen-oil-paint"))
				tlDecodePNG(t, path)
				sidecar, err := os.ReadFile(sidecarInfoPath(path))
				require.NoError(t, err)
				assert.Contains(t, string(sidecar), `"seed": 42`)
				assert.Nil(t, m["image_base64"])
				return
			}
			assert.Nil(t, m["path"])
			b64, _ := m["image_base64"].(string)
			raw, err := base64.StdEncoding.DecodeString(b64)
			require.NoError(t, err)
			assert.Equal(t, styPNG(3), raw)
			entries, err := os.ReadDir(e.outDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestTools_GenerateErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    func(e *tlEnv) map[string]any
		setup   func(e *tlEnv)
		wantErr string
	}{
		{name: "unknown preset", args: func(*tlEnv) map[string]any { return map[string]any{"preset_id": float64(9999)} }, wantErr: "preset not found"},
		{name: "neither preset nor prompt+model", args: func(*tlEnv) map[string]any { return map[string]any{"prompt": "cat"} }, wantErr: "нужен preset_id или пара prompt+model"},
		{name: "bad overrides", args: func(e *tlEnv) map[string]any {
			return map[string]any{"preset_id": float64(e.preset.ID), "overrides": "nope"}
		}, wantErr: "overrides"},
		{name: "llm failure", args: func(e *tlEnv) map[string]any {
			return map[string]any{"preset_id": float64(e.preset.ID), "description": "cat"}
		}, setup: func(e *tlEnv) { e.llm.err = errors.New("llm offline") }, wantErr: "llm offline"},
		{name: "sd failure", args: func(e *tlEnv) map[string]any {
			return map[string]any{"preset_id": float64(e.preset.ID), "seed": float64(7)}
		}, setup: func(e *tlEnv) { e.sd.failSeeds = map[int64]bool{7: true} }, wantErr: "seed 7"},
		{name: "undecodable image", args: func(e *tlEnv) map[string]any {
			return map[string]any{"preset_id": float64(e.preset.ID), "seed": float64(1)}
		}, setup: func(e *tlEnv) { e.sd.badImage = true }, wantErr: "decode image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			if tt.setup != nil {
				tt.setup(e)
			}
			_, err := styExec(e.srv, "generate", tt.args(e))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			entries, rerr := os.ReadDir(e.outDir)
			require.NoError(t, rerr)
			assert.Empty(t, entries)
		})
	}
}

func TestTools_GenerateDirectBranch(t *testing.T) {
	t.Parallel()
	e := newLocalTLEnv(t)
	m, _ := styCall(t, e.srv, "generate", map[string]any{"prompt": "a lighthouse", "model": "sdxl.safetensors", "seed": float64(11)})
	reqs := e.sd.txtReqs()
	require.Len(t, reqs, 1)
	assert.Equal(t, "a lighthouse", reqs[0].Prompt)
	assert.Equal(t, 512, reqs[0].Width)
	assert.Equal(t, 512, reqs[0].Height)
	assert.Empty(t, e.llm.calls())
	assert.Equal(t, "a lighthouse", m["prompt"])
	path, _ := m["path"].(string)
	require.NotEmpty(t, path)
	assert.True(t, strings.HasPrefix(filepath.Base(path), "gen-sdxl"))
	_, err := os.Stat(sidecarInfoPath(path))
	assert.NoError(t, err)
}

func TestTools_GenerateBestOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		n          any
		failSeeds  map[int64]bool
		wantSeeds  []int64
		wantPaths  int
		wantErrors []int64
		wantErr    string
	}{
		{name: "seeds are base+i", n: float64(3), wantSeeds: []int64{100, 101, 102}, wantPaths: 3},
		{name: "default n is 4", wantSeeds: []int64{100, 101, 102, 103}, wantPaths: 4},
		{name: "n capped at 12", n: float64(50), wantSeeds: []int64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111}, wantPaths: 12},
		{name: "partial failure keeps the fan", n: float64(3), failSeeds: map[int64]bool{101: true}, wantSeeds: []int64{100, 101, 102}, wantPaths: 2, wantErrors: []int64{101}},
		{name: "all failed", n: float64(2), failSeeds: map[int64]bool{100: true, 101: true}, wantSeeds: []int64{100, 101}, wantErr: "все 2 генераций не удались: sd failed for seed 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			e.sd.failSeeds = tt.failSeeds
			args := map[string]any{"preset_id": float64(e.preset.ID), "base_seed": float64(100)}
			if tt.n != nil {
				args["n"] = tt.n
			}
			out, err := styExec(e.srv, "generate_best_of", args)
			assert.Equal(t, tt.wantSeeds, e.sd.txtSeeds())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
				return
			}
			require.NoError(t, err)
			var res struct {
				Images []struct {
					Path  string `json:"path"`
					Seed  int64  `json:"seed"`
					Error string `json:"error"`
				} `json:"images"`
				Sheet      string `json:"sheet"`
				SheetError string `json:"sheet_error"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &res))
			require.Len(t, res.Images, len(tt.wantSeeds))
			paths := 0
			var failed []int64
			for i, im := range res.Images {
				assert.Equal(t, tt.wantSeeds[i], im.Seed)
				if im.Error != "" {
					failed = append(failed, im.Seed)
					assert.Empty(t, im.Path)
					continue
				}
				paths++
				assert.Contains(t, filepath.Base(im.Path), fmt.Sprintf("bestof-oil-paint-%d", im.Seed))
				tlDecodePNG(t, im.Path)
			}
			assert.Equal(t, tt.wantPaths, paths)
			assert.Equal(t, tt.wantErrors, failed)
			assert.Empty(t, res.SheetError)
			assert.Equal(t, filepath.Join(e.outDir, "bestof-oil-paint-sheet.png"), res.Sheet)
			sheet := tlDecodePNG(t, res.Sheet)
			cols := 1
			for cols*cols < tt.wantPaths {
				cols++
			}
			assert.Equal(t, cols*384, sheet.Bounds().Dx())
		})
	}
}

func TestTools_GenerateBestOf_Validation(t *testing.T) {
	t.Parallel()
	e := newLocalTLEnv(t)
	_, err := styExec(e.srv, "generate_best_of", map[string]any{})
	assert.ErrorContains(t, err, "preset_id обязателен")
	_, err = styExec(e.srv, "generate_best_of", map[string]any{"preset_id": float64(777)})
	assert.ErrorContains(t, err, "preset not found")
	assert.Zero(t, e.sd.calls())
}

func TestTools_GenerateBestOf_DescriptionConvertedOnce(t *testing.T) {
	t.Parallel()
	e := newLocalTLEnv(t)
	_, err := styExec(e.srv, "generate_best_of", map[string]any{"preset_id": float64(e.preset.ID), "base_seed": float64(5), "n": float64(3), "description": "a fox"})
	require.NoError(t, err)
	llmCalls := 0
	for _, c := range e.llm.calls() {
		if c.kind != "vision" {
			llmCalls++
		}
	}
	assert.Equal(t, 1, llmCalls)
	for _, r := range e.sd.txtReqs() {
		assert.Contains(t, r.Prompt, "llm scene tags")
	}
}

func TestTools_PresetCreate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
		check   func(t *testing.T, p preset.Preset)
	}{
		{name: "defaults applied", args: map[string]any{"name": " Neon ", "prompt": " neon city ", "confirm": true}, check: func(t *testing.T, p preset.Preset) {
			assert.Equal(t, "Neon", p.Name)
			assert.Equal(t, "neon city", p.Prompt)
			assert.Equal(t, "euler_a", p.Sampler)
			assert.Equal(t, 20, p.Steps)
			assert.InDelta(t, 7.0, p.CfgScale, 1e-9)
			assert.Nil(t, p.TypeID)
		}},
		{name: "explicit fields and tags", args: map[string]any{"name": "Ink", "prompt": "ink", "negative_prompt": "blur", "model_name": "m.gguf", "sampler": "dpm++2m", "steps": float64(30), "cfg_scale": 4.5, "tags": []any{"ink", "bw"}, "confirm": true}, check: func(t *testing.T, p preset.Preset) {
			assert.Equal(t, "blur", p.NegativePrompt)
			assert.Equal(t, "m.gguf", p.ModelName)
			assert.Equal(t, "dpm++2m", p.Sampler)
			assert.Equal(t, 30, p.Steps)
			assert.InDelta(t, 4.5, p.CfgScale, 1e-9)
			assert.Equal(t, "ink, bw", p.Tags)
		}},
		{name: "without confirm nothing is written", args: map[string]any{"name": "Draft", "prompt": strings.Repeat("я", 80)}, wantErr: "confirm=true"},
		{name: "empty name", args: map[string]any{"name": "  ", "prompt": "x", "confirm": true}, wantErr: "name и prompt обязательны"},
		{name: "unknown type_id", args: map[string]any{"name": "T", "prompt": "x", "type_id": float64(9999), "confirm": true}, wantErr: "type_id 9999 not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			before, err := e.db.List()
			require.NoError(t, err)
			out, err := styExec(e.srv, "preset_create", tt.args)
			after, lerr := e.db.List()
			require.NoError(t, lerr)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Len(t, after, len(before))
				return
			}
			require.NoError(t, err)
			require.Len(t, after, len(before)+1)
			var id int64
			_, scanErr := fmt.Sscanf(out, "создан пресет id=%d", &id)
			require.NoError(t, scanErr)
			p, err := e.db.Get(id)
			require.NoError(t, err)
			tt.check(t, *p)
		})
	}
}

func TestTools_PresetCreate_PreviewTruncatedInConfirmError(t *testing.T) {
	t.Parallel()
	e := newLocalTLEnv(t)
	_, err := styExec(e.srv, "preset_create", map[string]any{"name": "Long", "prompt": strings.Repeat("я", 80)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), strings.Repeat("я", 60)+"…")
	assert.NotContains(t, err.Error(), strings.Repeat("я", 61))
}

func TestTools_PresetCreate_WithType(t *testing.T) {
	t.Parallel()
	e := newLocalTLEnv(t)
	pt := &preset.PresetType{Name: "watercolor"}
	require.NoError(t, e.db.CreatePresetType(pt))
	out, err := styExec(e.srv, "preset_create", map[string]any{"name": "Typed", "prompt": "x", "type_id": float64(pt.ID), "confirm": true})
	require.NoError(t, err)
	var id int64
	_, err = fmt.Sscanf(out, "создан пресет id=%d", &id)
	require.NoError(t, err)
	p, err := e.db.Get(id)
	require.NoError(t, err)
	require.NotNil(t, p.TypeID)
	assert.Equal(t, pt.ID, *p.TypeID)
	assert.Equal(t, "watercolor", p.PresetType)
}

func TestTools_PresetDelete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		args        func(id int64) map[string]any
		wantErr     string
		wantDeleted bool
	}{
		{name: "confirmed delete", args: func(id int64) map[string]any { return map[string]any{"id": float64(id), "confirm": true} }, wantDeleted: true},
		{name: "no confirm names the preset", args: func(id int64) map[string]any { return map[string]any{"id": float64(id)} }, wantErr: "подтвердите delete: preset"},
		{name: "missing id", args: func(int64) map[string]any { return map[string]any{"confirm": true} }, wantErr: "id обязателен"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			out, err := styExec(e.srv, "preset_delete", tt.args(e.preset.ID))
			_, getErr := e.db.Get(e.preset.ID)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				if strings.Contains(tt.wantErr, "подтвердите") {
					assert.Contains(t, err.Error(), "Oil Paint")
				}
				assert.NoError(t, getErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("пресет %d удалён", e.preset.ID), out)
			assert.Error(t, getErr)
		})
	}
}

func TestTools_FromImage(t *testing.T) {
	t.Parallel()
	src := styPNG(4)
	tests := []struct {
		name     string
		args     func(e *tlEnv, path string) map[string]any
		wantErr  string
		wantTags bool
	}{
		{name: "preset remix sends the photo to img2img", args: func(e *tlEnv, p string) map[string]any {
			return map[string]any{"image_path": p, "preset_id": float64(e.preset.ID), "denoising_strength": 0.4, "seed": float64(9)}
		}},
		{name: "tags go through LLM", args: func(e *tlEnv, p string) map[string]any {
			return map[string]any{"image_path": p, "preset_id": float64(e.preset.ID), "tags": "make it winter", "seed": float64(9)}
		}, wantTags: true},
		{name: "both preset and compound", args: func(e *tlEnv, p string) map[string]any {
			return map[string]any{"image_path": p, "preset_id": float64(e.preset.ID), "compound_preset_id": float64(1)}
		}, wantErr: "укажи ровно одно"},
		{name: "neither preset nor compound", args: func(_ *tlEnv, p string) map[string]any { return map[string]any{"image_path": p} }, wantErr: "укажи ровно одно"},
		{name: "no image_path", args: func(e *tlEnv, _ string) map[string]any { return map[string]any{"preset_id": float64(e.preset.ID)} }, wantErr: "image_path обязателен"},
		{name: "unknown preset", args: func(_ *tlEnv, p string) map[string]any {
			return map[string]any{"image_path": p, "preset_id": float64(4242)}
		}, wantErr: "preset not found"},
		{name: "unknown compound", args: func(_ *tlEnv, p string) map[string]any {
			return map[string]any{"image_path": p, "compound_preset_id": float64(4242)}
		}, wantErr: "compound preset not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			path := filepath.Join(t.TempDir(), "src.png")
			require.NoError(t, os.WriteFile(path, src, 0o600))
			out, err := styExec(e.srv, "from_image", tt.args(e, path))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Zero(t, e.sd.calls())
				return
			}
			require.NoError(t, err)
			reqs := e.sd.imgReqs()
			require.Len(t, reqs, 1)
			require.Len(t, reqs[0].InitImages, 1)
			assert.Equal(t, base64.StdEncoding.EncodeToString(src), reqs[0].InitImages[0])
			require.NotNil(t, reqs[0].Seed)
			assert.Equal(t, int64(9), *reqs[0].Seed)
			if tt.wantTags {
				assert.Contains(t, reqs[0].Prompt, "llm scene tags")
			} else {
				assert.Contains(t, reqs[0].Prompt, "oil painting")
			}
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &m))
			p, _ := m["path"].(string)
			assert.True(t, strings.HasPrefix(filepath.Base(p), "fromimg-oil-paint"))
			tlDecodePNG(t, p)
		})
	}
}

func TestTools_FromImage_CompoundRejectsPresetOnlyFlags(t *testing.T) {
	t.Parallel()
	e := newLocalTLEnv(t)
	cp := &preset.CompoundPreset{Name: "Chain", Steps: []preset.CompoundPresetStep{{PresetID: e.preset.ID, StepOrder: 1}}}
	require.NoError(t, e.db.CreateCompoundPreset(cp))
	for _, flag := range []string{"quality", "preserve_identity"} {
		_, err := styExec(e.srv, "from_image", map[string]any{"image_path": tlWritePNG(t, 1), "compound_preset_id": float64(cp.ID), flag: true})
		require.Error(t, err, flag)
		assert.Contains(t, err.Error(), "только с preset_id")
	}
	assert.Zero(t, e.sd.calls())
}

func TestTools_EditLookObjectInputValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tool    string
		args    map[string]any
		wantErr string
	}{
		{tool: "edit", args: map[string]any{"instruction": "x"}, wantErr: "image_path обязателен"},
		{tool: "edit", args: map[string]any{"image_path": "/nonexistent/a.png", "instruction": "x"}, wantErr: "a.png"},
		{tool: "look", args: map[string]any{"text": "warm"}, wantErr: "image_path обязателен"},
		{tool: "object_edit", args: map[string]any{"image_path": "/nonexistent/a.png", "mask_path": "/nonexistent/m.png", "op": "remove"}, wantErr: "a.png"},
	}
	for _, tt := range tests {
		t.Run(tt.tool+"/"+tt.wantErr, func(t *testing.T) {
			t.Parallel()
			e := newLocalTLEnv(t)
			_, err := styExec(e.srv, tt.tool, tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Zero(t, e.sd.calls())
			assert.Empty(t, e.llm.calls())
		})
	}
	e := newLocalTLEnv(t)
	_, err := styExec(e.srv, "edit", map[string]any{"image_path": tlWritePNG(t, 1), "instruction": "x", "loras": "bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loras")
	assert.Zero(t, e.sd.calls())
}
