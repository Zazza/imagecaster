package generation

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/stylist"
)

type psSize struct{ w, h int }

type psRun struct {
	mu    sync.Mutex
	sizes []psSize
}

func (r *psRun) sd() *mockSD {
	step := func(w, h int) *sd.Txt2ImgResponse {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.sizes = append(r.sizes, psSize{w, h})
		return &sd.Txt2ImgResponse{Images: []string{"img"}, Info: json.RawMessage(`{"seed":1}`), Parameters: json.RawMessage(`{}`)}
	}
	return &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) { return step(req.Width, req.Height), nil },
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) { return step(req.Width, req.Height), nil },
	}
}

func (r *psRun) got() []psSize {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]psSize(nil), r.sizes...)
}

func psResolution(t *testing.T, db *preset.DB, name string, w, h int) *int64 {
	t.Helper()
	res := &preset.Resolution{Name: name, Width: w, Height: h}
	require.NoError(t, db.CreateResolution(res))
	return &res.ID
}

func psPipeline(t *testing.T, db *preset.DB, step2Res *int64) int64 {
	t.Helper()
	mk := func(name, base string) int64 {
		m := stylist.Models[base]
		p := makeTestPreset(t, db, &preset.Preset{Name: name, PresetType: "txt2img", Prompt: "photo", Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG, ModelName: m.Model})
		return p.ID
	}
	cp := &preset.CompoundPreset{Name: "size-" + t.Name(), Steps: []preset.CompoundPresetStep{
		{StepOrder: 1, PresetID: mk("z", "Z-Image")},
		{StepOrder: 2, PresetID: mk("x", "SDXL"), DenoisingStrength: 0.5, ResolutionID: step2Res},
	}}
	require.NoError(t, db.CreateCompoundPreset(cp))
	return cp.ID
}

func TestGenerateCompoundImage_PipelineSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		width     int
		height    int
		paramsRes bool
		stepRes   bool
		want      []psSize
	}{
		{name: "width height to every step", width: 1024, height: 1536, want: []psSize{{1024, 1536}, {1024, 1536}}},
		{name: "landscape to every step", width: 1536, height: 1024, want: []psSize{{1536, 1024}, {1536, 1024}}},
		{name: "step resolution wins for its step", width: 1024, height: 1536, stepRes: true, want: []psSize{{1024, 1536}, {768, 1152}}},
		{name: "params resolution wins over width height", width: 1024, height: 1536, paramsRes: true, want: []psSize{{832, 1216}, {832, 1216}}},
		{name: "nothing set keeps legacy default", want: []psSize{{512, 512}, {512, 512}}},
	}
	prompts := []struct {
		name   string
		params func(p *GenerateCompoundImageParams)
	}{
		{"scene prompt", func(p *GenerateCompoundImageParams) { p.ScenePrompt = "woman on a pier" }},
		{"llm prompt", func(p *GenerateCompoundImageParams) { p.ExtraPrompt = "a woman on a pier" }},
	}
	for _, pr := range prompts {
		for _, tt := range tests {
			t.Run(pr.name+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				db := openTestDB(t)
				var stepRes *int64
				if tt.stepRes {
					stepRes = psResolution(t, db, "step-768x1152", 768, 1152)
				}
				cpID := psPipeline(t, db, stepRes)
				run := &psRun{}
				llmSvc := &mockLLM{genSDPromptFn: func(string, string, string, string, int) (string, error) {
					return `{"prompt":"llm-scene","negative_prompt":""}`, nil
				}}
				svc := newTestService(t, db, llmSvc, run.sd())
				svc.ctx = context.Background()

				params := GenerateCompoundImageParams{CompoundPresetID: cpID, Width: tt.width, Height: tt.height}
				if tt.paramsRes {
					params.ResolutionID = psResolution(t, db, "params-832x1216", 832, 1216)
				}
				pr.params(&params)

				_, err := svc.GenerateCompoundImage(params)
				require.NoError(t, err)
				assert.Equal(t, tt.want, run.got())
			})
		}
	}
}
