package generation

import (
	"context"
	"encoding/json"
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/stylist"
)

var loraTagRe = regexp.MustCompile(`<lora:([^:>]+):([^>]+)>`)

func promptLoras(prompt string) map[string]string {
	out := map[string]string{}
	for _, m := range loraTagRe.FindAllStringSubmatch(prompt, -1) {
		out[m[1]] = m[2]
	}
	return out
}

type pipelineRun struct {
	mu      sync.Mutex
	events  []string
	prompts []string
	swaps   []faceSwapCall
}

func (r *pipelineRun) sd() *mockSD {
	step := func(prompt string) *sd.Txt2ImgResponse {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.prompts = append(r.prompts, prompt)
		r.events = append(r.events, "step")
		img := []string{"img-s1", "img-s2", "img-s3", "img-s4"}[len(r.prompts)-1]
		return &sd.Txt2ImgResponse{Images: []string{img}, Info: json.RawMessage(`{"seed":1}`), Parameters: json.RawMessage(`{}`)}
	}
	return &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) { return step(req.Prompt), nil },
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) { return step(req.Prompt), nil },
		faceSwap: func(_ context.Context, img, person string, blend float64) (*sd.FaceSwapResult, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.swaps = append(r.swaps, faceSwapCall{img: img, person: person, blend: blend})
			r.events = append(r.events, "swap")
			return &sd.FaceSwapResult{Status: sd.FaceSwapDone, Image: "img-swapped"}, nil
		},
	}
}

func makePersonPipeline(t *testing.T, db *preset.DB) int64 {
	t.Helper()
	mk := func(name, base, loras string) int64 {
		m := stylist.Models[base]
		p := makeTestPreset(t, db, &preset.Preset{Name: name, PresetType: "txt2img", Prompt: "photo", Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG, ModelName: m.Model, Loras: loras})
		return p.ID
	}
	cp := &preset.CompoundPreset{Name: "zx-sdxl-f1", Steps: []preset.CompoundPresetStep{
		{StepOrder: 1, PresetID: mk("z", "Z-Image", "")},
		{StepOrder: 2, PresetID: mk("x", "SDXL", `[{"name":"film","weight":0.55}]`), DenoisingStrength: 0.5},
		{StepOrder: 3, PresetID: mk("f1", "Flux.1", `[{"name":"grain","weight":0.3}]`), DenoisingStrength: 0.4},
	}}
	require.NoError(t, db.CreateCompoundPreset(cp))
	return cp.ID
}

func TestModelBase_StylistModels(t *testing.T) {
	t.Parallel()
	for base, m := range stylist.Models {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, base, ModelBase(m.Model))
		})
	}
}

func TestGenerateCompoundImage_PersonPerStep(t *testing.T) {
	t.Parallel()
	natasha := map[string]string{"Z-Image": "nat_z", "Flux.1": "nat_f1"}
	tests := []struct {
		name        string
		person      *PersonOverride
		wantLoras   []map[string]string
		wantSwaps   []faceSwapCall
		wantEvents  []string
		wantImage   string
		wantSwapRes string
	}{
		{
			name:        "loras per base, swap once after last step",
			person:      &PersonOverride{Name: "Наташа", Loras: natasha, FaceSwap: true, FaceSwapName: "nat_z"},
			wantLoras:   []map[string]string{{"nat_z": "1"}, {"film": "0.55"}, {"grain": "0.3", "nat_f1": "1"}},
			wantSwaps:   []faceSwapCall{{img: "img-s3", person: "nat_z", blend: faceSwapBlend}},
			wantEvents:  []string{"step", "step", "step", "swap"},
			wantImage:   "img-swapped",
			wantSwapRes: FaceSwapDone,
		},
		{
			name:       "loras per base, no swap without flag",
			person:     &PersonOverride{Name: "Наташа", Loras: natasha, FaceSwapName: "nat_z"},
			wantLoras:  []map[string]string{{"nat_z": "1"}, {"film": "0.55"}, {"grain": "0.3", "nat_f1": "1"}},
			wantEvents: []string{"step", "step", "step"},
			wantImage:  "img-s3",
		},
		{
			name:       "no person keeps own loras only",
			wantLoras:  []map[string]string{{}, {"film": "0.55"}, {"grain": "0.3"}},
			wantEvents: []string{"step", "step", "step"},
			wantImage:  "img-s3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			cpID := makePersonPipeline(t, db)
			run := &pipelineRun{}
			llmCalls := 0
			llmSvc := &mockLLM{genSDPromptFn: func(string, string, string, string, int) (string, error) {
				llmCalls++
				return `{"prompt":"llm-scene","negative_prompt":""}`, nil
			}}
			svc := newTestService(t, db, llmSvc, run.sd())
			svc.ctx = context.Background()
			svc.cfg.SDUrl = "http://192.168.1.10:7861"

			res, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{
				CompoundPresetID: cpID,
				ScenePrompt:      "woman in a red coat on a pier",
				Person:           tt.person,
			})
			require.NoError(t, err)

			assert.Zero(t, llmCalls)
			require.Len(t, run.prompts, 3)
			for i, p := range run.prompts {
				assert.Contains(t, p, "woman in a red coat on a pier", "step %d", i+1)
				assert.NotContains(t, p, "llm-scene", "step %d", i+1)
				assert.Equal(t, tt.wantLoras[i], promptLoras(p), "step %d: %q", i+1, p)
			}
			assert.Equal(t, tt.wantSwaps, run.swaps)
			assert.Equal(t, tt.wantEvents, run.events)
			assert.Equal(t, tt.wantImage, res.Image)
			assert.Equal(t, tt.wantSwapRes, res.FaceSwap)
			items := svc.sessions.(*mockSessions).items
			require.NotEmpty(t, items)
			assert.Equal(t, tt.wantImage, items[len(items)-1].imageBase64)
		})
	}
}

func TestGenerateCompoundImage_NoScenePromptUsesLLM(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	cpID := makePersonPipeline(t, db)
	run := &pipelineRun{}
	llmCalls := 0
	llmSvc := &mockLLM{genSDPromptFn: func(string, string, string, string, int) (string, error) {
		llmCalls++
		return `{"prompt":"llm-scene","negative_prompt":""}`, nil
	}}
	svc := newTestService(t, db, llmSvc, run.sd())
	svc.ctx = context.Background()

	_, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{CompoundPresetID: cpID, ExtraPrompt: "a woman on a pier"})
	require.NoError(t, err)
	assert.Positive(t, llmCalls)
	require.NotEmpty(t, run.prompts)
	assert.Contains(t, run.prompts[0], "llm-scene")
}
