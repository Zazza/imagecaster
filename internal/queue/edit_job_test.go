package queue

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
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

func editPNGBase64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestEnqueueEdit_RejectsMissingImageOrInstruction(t *testing.T) {
	cases := []struct {
		name   string
		params generation.EditImageParams
	}{
		{name: "empty image", params: generation.EditImageParams{ImageBase64: "", Instruction: "add a cat"}},
		{name: "empty instruction", params: generation.EditImageParams{ImageBase64: "aGVsbG8=", Instruction: ""}},
		{name: "whitespace instruction", params: generation.EditImageParams{ImageBase64: "aGVsbG8=", Instruction: "   "}},
		{name: "both empty", params: generation.EditImageParams{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, emitter := newLookTestService(t)

			_, err := svc.Enqueue(JobEdit, tc.params, "edit")

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if n := countJobs(t, svc); n != 0 {
				t.Errorf("job count = %d, want 0", n)
			}
			if emitter.hasEvent(EventQueueChanged) {
				t.Error("expected no EventQueueChanged for rejected job")
			}
		})
	}
}

func TestEnqueueEdit_ValidParamsCreatePendingEditJob(t *testing.T) {
	svc, _ := newLookTestService(t)
	seed := int64(7)

	id, err := svc.Enqueue(JobEdit, generation.EditImageParams{ImageBase64: "aGVsbG8=", Instruction: "add a cat", Seed: &seed}, "edit")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.Type != JobEdit {
		t.Errorf("type = %q, want %q", job.Type, JobEdit)
	}
	if job.Status != StatusPending {
		t.Errorf("status = %q, want %q", job.Status, StatusPending)
	}
	if job.Source != "edit" {
		t.Errorf("source = %q, want %q", job.Source, "edit")
	}
	for _, want := range []string{`"instruction":"add a cat"`, `"image_base64":"aGVsbG8="`, `"seed":7`} {
		if !strings.Contains(job.Params, want) {
			t.Errorf("params = %s, want it to contain %s", job.Params, want)
		}
	}
}

type editFakeSD struct {
	mu       sync.Mutex
	img2img  []sd.Img2ImgRequest
	txt2img  int
	response string
}

func (f *editFakeSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txt2img++
	return nil, fmt.Errorf("txt2img not expected")
}

func (f *editFakeSD) Img2Img(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.img2img = append(f.img2img, req)
	return &sd.Txt2ImgResponse{Images: []string{f.response}}, nil
}

func (f *editFakeSD) GetModels() ([]sd.SDModel, error)            { return nil, nil }
func (f *editFakeSD) GetSamplers() ([]sd.Sampler, error)          { return nil, nil }
func (f *editFakeSD) GetSchedulers() ([]sd.Scheduler, error)      { return nil, nil }
func (f *editFakeSD) GetUpscalers() ([]sd.Upscaler, error)        { return nil, nil }
func (f *editFakeSD) GetVAEs() ([]sd.VAE, error)                  { return nil, nil }
func (f *editFakeSD) GetLoRAs() ([]sd.LoRA, error)                { return nil, nil }
func (f *editFakeSD) GetOptions() (map[string]interface{}, error) { return nil, nil }
func (f *editFakeSD) GetProgress() (*sd.ProgressResponse, error)  { return &sd.ProgressResponse{}, nil }
func (f *editFakeSD) Interrupt() error                            { return nil }
func (f *editFakeSD) HealthCheck() error                          { return nil }
func (f *editFakeSD) SetURL(baseURL string)                       {}
func (f *editFakeSD) SetModel(modelName string) error             { return nil }
func (f *editFakeSD) SetModelWithModules(string, []string) error  { return nil }
func (f *editFakeSD) SetVAE(vaeName string) error                 { return nil }
func (f *editFakeSD) SetOption(key string, value any) error       { return nil }
func (f *editFakeSD) UpscaleImage(string, string, float64) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (f *editFakeSD) MemoryInfo() (*sd.MemoryStats, error) { return nil, nil }

type editFakeLLM struct{}

func (editFakeLLM) Chat(string, string, string, float64, int) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (editFakeLLM) ChatVision(string, string, string, string, float64, int) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (editFakeLLM) ChatWithMessages(string, []llm.Message, float64, int) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (editFakeLLM) GenerateSDPrompt(string, string, string, string, int) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (editFakeLLM) AnalyzeImage(string, string, string, int) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (editFakeLLM) AnalyzeImageDescribe(string, string, string, int) (string, error) {
	return "", fmt.Errorf("not expected")
}
func (editFakeLLM) GetModels() ([]llm.LLMModel, error)     { return nil, nil }
func (editFakeLLM) HealthCheck() error                     { return nil }
func (editFakeLLM) SetURL(string)                          {}
func (editFakeLLM) SetBackendConfig(llm.BackendConfig)     {}
func (editFakeLLM) ListLoaded() ([]llm.LoadedModel, error) { return nil, nil }
func (editFakeLLM) UnloadAll(ctx context.Context) error    { return nil }

type editFakeSessions struct{}

func (editFakeSessions) AddToSession(string, json.RawMessage, string, bool, *preset.GenerationMeta) int64 {
	return 1
}

type editFakeSettings struct{}

func (editFakeSettings) ApplyLLMConfig(string) {}

func TestProcessJob_EditCallsEditImageWithJobParams(t *testing.T) {
	pdb, err := preset.Open(":memory:")
	if err != nil {
		t.Fatalf("open preset db: %v", err)
	}
	t.Cleanup(func() { pdb.Close() })

	fakeSD := &editFakeSD{response: "edited-image-b64"}
	emitter := &mockEmitter{}
	gen := generation.New(
		pdb,
		editFakeLLM{},
		fakeSD,
		&config.Config{SDPromptModel: "test-model", VisionModel: "test-vision"},
		t.TempDir(),
		emitter,
		kids.NewManager(pdb),
		editFakeSessions{},
		editFakeSettings{},
		nil,
		logger.New(nil),
	)

	qdb := openTestDB(t)
	t.Cleanup(func() { qdb.Close() })
	p := NewProcessor(gen, NewStore(qdb), t.TempDir(), emitter)

	src := editPNGBase64(t)
	seed := int64(123)
	raw, err := json.Marshal(generation.EditImageParams{ImageBase64: src, Instruction: "add a cat", Seed: &seed})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res, err := p.ProcessJob(context.Background(), &Job{ID: 1, Type: JobEdit, Params: string(raw), Source: "edit"})
	if err != nil {
		t.Fatalf("ProcessJob: %v", err)
	}
	if res == nil {
		t.Fatal("result is nil")
	}
	if res.ImageBase64 != "edited-image-b64" {
		t.Errorf("result image = %q, want %q", res.ImageBase64, "edited-image-b64")
	}

	fakeSD.mu.Lock()
	defer fakeSD.mu.Unlock()
	if len(fakeSD.img2img) != 1 {
		t.Fatalf("img2img calls = %d, want 1", len(fakeSD.img2img))
	}
	req := fakeSD.img2img[0]
	if len(req.InitImages) != 1 || req.InitImages[0] != src {
		t.Errorf("init image is not the job's image_base64")
	}
	if !strings.Contains(req.Prompt, "add a cat") {
		t.Errorf("prompt = %q, want it to contain the job instruction", req.Prompt)
	}
	if req.Seed == nil || *req.Seed != seed {
		t.Errorf("seed = %v, want %d", req.Seed, seed)
	}
}

func TestProcessJob_EditInvalidParamsIsDispatched(t *testing.T) {
	p := NewProcessor(nil, nil, t.TempDir(), &mockEmitter{})

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ProcessJob panicked on edit job: %v", r)
			}
		}()
		_, err = p.ProcessJob(context.Background(), &Job{Type: JobEdit, Params: "not json"})
	}()

	if err == nil {
		t.Fatal("expected error for invalid edit params, got nil")
	}
	if strings.Contains(err.Error(), "unknown job type") {
		t.Errorf("error = %q: edit job is not dispatched", err.Error())
	}
}
