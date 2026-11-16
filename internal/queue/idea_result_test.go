package queue

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	ideaJobLLMPrompt = "queue-idea-tags, castle"
	ideaJobLLMNeg    = "queue-idea-neg"
)

type ideaJobFakeSD struct {
	*editFakeSD
	mu      sync.Mutex
	prompts []string
}

func (f *ideaJobFakeSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, req.Prompt)
	return &sd.Txt2ImgResponse{Images: []string{"txt2img-b64"}}, nil
}

type ideaJobFakeLLM struct {
	editFakeLLM
}

func (ideaJobFakeLLM) GenerateSDPrompt(string, string, string, string, int) (string, error) {
	return `{"prompt":"` + ideaJobLLMPrompt + `","negative_prompt":"` + ideaJobLLMNeg + `"}`, nil
}

func newIdeaJobProcessor(t *testing.T) (Processor, *preset.DB, *ideaJobFakeSD) {
	t.Helper()
	pdb, err := preset.Open(":memory:")
	if err != nil {
		t.Fatalf("open preset db: %v", err)
	}
	t.Cleanup(func() { pdb.Close() })

	fakeSD := &ideaJobFakeSD{editFakeSD: &editFakeSD{}}
	emitter := &mockEmitter{}
	gen := generation.New(
		pdb,
		ideaJobFakeLLM{},
		fakeSD,
		&config.Config{SDPromptModel: "test-model", VisionModel: "test-vision"},
		t.TempDir(),
		emitter,
		kids.NewManager(pdb),
		editFakeSessions{},
		logger.New(nil),
	)
	qdb := openTestDB(t)
	t.Cleanup(func() { qdb.Close() })
	return NewProcessor(gen, nil, NewStore(qdb), t.TempDir(), emitter), pdb, fakeSD
}

func ideaJobPreset(t *testing.T, pdb *preset.DB, name string) *preset.Preset {
	t.Helper()
	p := &preset.Preset{
		Name:           name,
		PresetType:     "anime",
		Prompt:         name + "-tags",
		NegativePrompt: name + "-neg",
		Sampler:        "Euler a",
		Steps:          20,
		CfgScale:       7.0,
	}
	if err := pdb.Create(p); err != nil {
		t.Fatalf("create preset: %v", err)
	}
	return p
}

func TestProcessJob_IdeaPromptInJobResult(t *testing.T) {
	cases := []struct {
		name    string
		jobType JobType
		params  func(t *testing.T, pdb *preset.DB) any
	}{
		{
			name:    "txt2img",
			jobType: JobTxt2Img,
			params: func(t *testing.T, pdb *preset.DB) any {
				p := ideaJobPreset(t, pdb, "style")
				return generation.GenerateImageParams{PresetID: p.ID, Idea: "a castle", IdeaNegative: "no people"}
			},
		},
		{
			name:    "compound custom steps",
			jobType: JobCompound,
			params: func(t *testing.T, pdb *preset.DB) any {
				p := ideaJobPreset(t, pdb, "style")
				return generation.GenerateCompoundImageParams{Steps: []generation.CustomStep{{PresetID: p.ID}}, Idea: "a castle", IdeaNegative: "no people"}
			},
		},
		{
			name:    "compound workflow",
			jobType: JobCompound,
			params: func(t *testing.T, pdb *preset.DB) any {
				p := ideaJobPreset(t, pdb, "style")
				cp := &preset.CompoundPreset{Name: "flow", Steps: []preset.CompoundPresetStep{{PresetID: p.ID}}}
				if err := pdb.CreateCompoundPreset(cp); err != nil {
					t.Fatalf("create workflow: %v", err)
				}
				return generation.GenerateCompoundImageParams{CompoundPresetID: cp.ID, Idea: "a castle", IdeaNegative: "no people"}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, pdb, fakeSD := newIdeaJobProcessor(t)
			raw, err := json.Marshal(tc.params(t, pdb))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			res, err := p.ProcessJob(context.Background(), &Job{ID: 1, Type: tc.jobType, Params: string(raw), Source: "generate"})
			if err != nil {
				t.Fatalf("ProcessJob: %v", err)
			}
			if res == nil {
				t.Fatal("result is nil")
			}
			if res.IdeaPrompt != ideaJobLLMPrompt {
				t.Errorf("IdeaPrompt = %q, want %q", res.IdeaPrompt, ideaJobLLMPrompt)
			}
			if res.IdeaNegativePrompt != ideaJobLLMNeg {
				t.Errorf("IdeaNegativePrompt = %q, want %q", res.IdeaNegativePrompt, ideaJobLLMNeg)
			}

			encoded, err := json.Marshal(res)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			for _, want := range []string{`"idea_prompt":"` + ideaJobLLMPrompt + `"`, `"idea_negative_prompt":"` + ideaJobLLMNeg + `"`} {
				if !strings.Contains(string(encoded), want) {
					t.Errorf("result JSON %s does not contain %s", encoded, want)
				}
			}

			fakeSD.mu.Lock()
			defer fakeSD.mu.Unlock()
			if len(fakeSD.prompts) == 0 || !strings.Contains(fakeSD.prompts[0], ideaJobLLMPrompt) {
				t.Errorf("SD prompts = %v, want the LLM prompt", fakeSD.prompts)
			}
		})
	}
}

func TestProcessJob_NoIdeaNoIdeaPromptInJobResult(t *testing.T) {
	p, pdb, _ := newIdeaJobProcessor(t)
	pr := ideaJobPreset(t, pdb, "style")
	raw, err := json.Marshal(generation.GenerateImageParams{PresetID: pr.ID, ExtraPrompt: "ready prompt"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res, err := p.ProcessJob(context.Background(), &Job{ID: 1, Type: JobTxt2Img, Params: string(raw), Source: "generate"})
	if err != nil {
		t.Fatalf("ProcessJob: %v", err)
	}
	if res.IdeaPrompt != "" || res.IdeaNegativePrompt != "" {
		t.Errorf("idea fields = %q / %q, want empty", res.IdeaPrompt, res.IdeaNegativePrompt)
	}
	encoded, _ := json.Marshal(res)
	if strings.Contains(string(encoded), "idea_prompt") {
		t.Errorf("result JSON %s must not carry idea_prompt", encoded)
	}
}

func TestProcessJob_IdeaReusedAcrossJobsSurvivesRestart(t *testing.T) {
	env := newXREnv(t)
	id1 := env.ideaJob(t)
	id2 := env.ideaJob(t)

	llmA := &xrLLM{variant: "A"}
	sdA := &xrSD{}
	procA, _ := env.processor(t, llmA, sdA, &xrSessions{})

	job1, err := env.store.Get(id1)
	if err != nil {
		t.Fatalf("get job 1: %v", err)
	}
	if _, err := procA.ProcessJob(context.Background(), job1); err != nil {
		t.Fatalf("job 1: %v", err)
	}
	if llmA.count() != 1 {
		t.Fatalf("LLM calls after job 1 = %d, want 1", llmA.count())
	}

	sdA.mu.Lock()
	sdA.txtErr = context.Canceled
	sdA.mu.Unlock()
	job2, err := env.store.Get(id2)
	if err != nil {
		t.Fatalf("get job 2: %v", err)
	}
	_, _ = procA.ProcessJob(context.Background(), job2)
	if llmA.count() != 1 {
		t.Fatalf("LLM calls after job 2 = %d, want 1: job 2 must reuse the idea prompt", llmA.count())
	}
	first := sdA.txtPrompts()
	if len(first) != 2 {
		t.Fatalf("SD calls in the first run = %d, want 2", len(first))
	}
	if !strings.Contains(first[1], "A-tags-1") {
		t.Errorf("job 2 prompt %q does not carry the first LLM answer", first[1])
	}
	if first[0] != first[1] {
		t.Errorf("job 2 prompt %q differs from job 1 prompt %q", first[1], first[0])
	}

	llmB := &xrLLM{variant: "B"}
	sdB := &xrSD{}
	procB, _ := env.processor(t, llmB, sdB, &xrSessions{})
	job2, err = env.store.Get(id2)
	if err != nil {
		t.Fatalf("get job 2 after restart: %v", err)
	}
	if _, err := procB.ProcessJob(context.Background(), job2); err != nil {
		t.Fatalf("job 2 after restart: %v", err)
	}
	if llmB.count() != 0 {
		t.Errorf("LLM calls after restart = %d, want 0", llmB.count())
	}
	resumed := sdB.txtPrompts()
	if len(resumed) != 1 {
		t.Fatalf("SD calls after restart = %d, want 1", len(resumed))
	}
	if resumed[0] != first[1] {
		t.Errorf("prompt after restart = %q, want the first-run prompt %q", resumed[0], first[1])
	}
}
