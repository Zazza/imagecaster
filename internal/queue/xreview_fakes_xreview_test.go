package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

type xrSD struct {
	mu      sync.Mutex
	prompts []string
	onTxt   func()
	txtErr  error
}

func (f *xrSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, req.Prompt)
	hook := f.onTxt
	e := f.txtErr
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	if e != nil {
		return nil, e
	}
	return &sd.Txt2ImgResponse{Images: []string{"xr-image"}, Info: json.RawMessage(`{"prompt":"x"}`)}, nil
}

func (f *xrSD) Img2Img(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
	return &sd.Txt2ImgResponse{Images: []string{"xr-image"}}, nil
}
func (f *xrSD) UpscaleImage(string, string, float64) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (f *xrSD) GetModels() ([]sd.SDModel, error)       { return nil, nil }
func (f *xrSD) GetSamplers() ([]sd.Sampler, error)     { return nil, nil }
func (f *xrSD) GetSchedulers() ([]sd.Scheduler, error) { return nil, nil }
func (f *xrSD) GetUpscalers() ([]sd.Upscaler, error)   { return nil, nil }
func (f *xrSD) GetVAEs() ([]sd.VAE, error)             { return nil, nil }
func (f *xrSD) GetLoRAs() ([]sd.LoRA, error)           { return nil, nil }
func (f *xrSD) State() (*sd.ServerState, error)        { return &sd.ServerState{}, nil }
func (f *xrSD) ListJobs(context.Context, string) ([]sd.JobStatus, error) {
	return nil, nil
}
func (f *xrSD) SetProgressHandler(func(sd.JobStatus)) {}
func (f *xrSD) Interrupt() error                      { return nil }
func (f *xrSD) HealthCheck() error                    { return nil }
func (f *xrSD) SetURL(string)                         {}
func (f *xrSD) MemoryInfo() (*sd.MemoryStats, error)  { return nil, nil }

func (f *xrSD) txtPrompts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

type xrScoper struct {
	svc     sd.Service
	mu      sync.Mutex
	cleaned []int64
}

func (s *xrScoper) ForQueueJob(context.Context, int64) sd.Service { return s.svc }
func (s *xrScoper) CleanupQueueJob(_ context.Context, id int64) error {
	s.mu.Lock()
	s.cleaned = append(s.cleaned, id)
	s.mu.Unlock()
	return nil
}

type xrLLM struct {
	variant string
	mu      sync.Mutex
	calls   int
}

func (l *xrLLM) next() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	return l.calls
}

func (l *xrLLM) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func (l *xrLLM) answer() string {
	n := l.next()
	return fmt.Sprintf(`{"prompt":"%s-tags-%d, cat","negative_prompt":"%s-neg-%d"}`, l.variant, n, l.variant, n)
}

func (l *xrLLM) Chat(string, string, string, float64, int) (string, error) {
	return l.answer(), nil
}
func (l *xrLLM) ChatVision(string, string, string, string, float64, int) (string, error) {
	return l.answer(), nil
}
func (l *xrLLM) ChatWithMessages(string, []llm.Message, float64, int) (string, error) {
	return l.answer(), nil
}
func (l *xrLLM) GenerateSDPrompt(string, string, string, string, int) (string, error) {
	return l.answer(), nil
}
func (l *xrLLM) AnalyzeImage(string, string, string, int) (string, error) {
	return l.answer(), nil
}
func (l *xrLLM) AnalyzeImageDescribe(string, string, string, int) (string, error) {
	return l.answer(), nil
}
func (l *xrLLM) GetModels() ([]llm.LLMModel, error)     { return nil, nil }
func (l *xrLLM) HealthCheck() error                     { return nil }
func (l *xrLLM) SetURL(string)                          {}
func (l *xrLLM) SetBackendConfig(llm.BackendConfig)     {}
func (l *xrLLM) ListLoaded() ([]llm.LoadedModel, error) { return nil, nil }
func (l *xrLLM) UnloadAll(context.Context) error        { return nil }

type xrSessions struct {
	mu    sync.Mutex
	saved []string
}

func (s *xrSessions) AddToSession(img string, _ json.RawMessage, _ string, _ bool, _ *preset.GenerationMeta) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, img)
	return int64(len(s.saved))
}

func (s *xrSessions) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.saved)
}

type xrEnv struct {
	pdb      *preset.DB
	store    *Store
	dataDir  string
	presetID int64
}

func newXREnv(t *testing.T) *xrEnv {
	t.Helper()
	pdb, err := preset.Open(":memory:")
	if err != nil {
		t.Fatalf("open preset db: %v", err)
	}
	t.Cleanup(func() { pdb.Close() })
	p := &preset.Preset{Name: "xr-preset", PresetType: "anime", Prompt: "masterpiece", NegativePrompt: "lowres", Sampler: "euler_a", Steps: 20, CfgScale: 7}
	if err := pdb.Create(p); err != nil {
		t.Fatalf("create preset: %v", err)
	}
	qdb := openTestDB(t)
	t.Cleanup(func() { qdb.Close() })
	return &xrEnv{pdb: pdb, store: NewStore(qdb), dataDir: t.TempDir(), presetID: p.ID}
}

func (e *xrEnv) processor(t *testing.T, l llm.Service, s *xrSD, sessions *xrSessions) (Processor, *xrScoper) {
	t.Helper()
	gen := generation.New(
		e.pdb,
		l,
		s,
		&config.Config{SDPromptModel: "test-model", VisionModel: "test-vision"},
		t.TempDir(),
		&mockEmitter{},
		kids.NewManager(e.pdb),
		sessions,
		editFakeSettings{},
		nil,
		logger.New(nil),
	)
	gen.SetContext(context.Background())
	scoper := &xrScoper{svc: s}
	return NewProcessor(gen, scoper, e.store, e.dataDir, &mockEmitter{}), scoper
}

func (e *xrEnv) ideaJob(t *testing.T) int64 {
	t.Helper()
	raw, err := json.Marshal(generation.GenerateImageParams{PresetID: e.presetID, Idea: "a cat on a red sofa"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	id, err := e.store.Create(JobTxt2Img, string(raw), "test")
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	return id
}
