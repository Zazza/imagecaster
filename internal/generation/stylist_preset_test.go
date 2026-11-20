package generation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func presetShotService(t *testing.T) (*Service, *preset.DB, *[]sd.Txt2ImgRequest) {
	t.Helper()
	db := openTestDB(t)
	var reqs []sd.Txt2ImgRequest
	mock := &mockSD{txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
		reqs = append(reqs, req)
		return &sd.Txt2ImgResponse{Images: []string{"img"}, Info: json.RawMessage(`{"seed":77}`), Parameters: json.RawMessage(`{}`)}, nil
	}}
	svc := newTestService(t, db, &mockLLM{}, mock)
	svc.ctx = context.Background()
	return svc, db, &reqs
}

func shotPreset(t *testing.T, db *preset.DB, base, prompt, neg, loras string) int64 {
	t.Helper()
	m := stylist.Models[base]
	p := makeTestPreset(t, db, &preset.Preset{Name: "style " + base, PresetType: "txt2img", Prompt: prompt, NegativePrompt: neg,
		Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG, ModelName: m.Model, Loras: loras})
	return p.ID
}

func TestGeneratePresetShot_PromptJoinedLikePipelineStep(t *testing.T) {
	t.Parallel()
	const scene = "A woman in a red coat on a pier"
	tests := []struct {
		name string
		base string
		want string
	}{
		{"prose model joins as sentences", "Z-Image", scene + ". cinematic film still"},
		{"SDXL joins with BREAK", "SDXL", scene + ", BREAK, cinematic film still"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db, reqs := presetShotService(t)
			id := shotPreset(t, db, tt.base, "cinematic film still", "cartoon", "")
			res, err := svc.GeneratePresetShot(PresetShotParams{PresetID: id, ScenePrompt: scene, ExtraNegativePrompt: "blurry", PersonLora: "anna_z", Width: 832, Height: 1216})
			require.NoError(t, err)
			require.Len(t, *reqs, 1)
			r := (*reqs)[0]
			assert.Contains(t, r.Prompt, tt.want)
			assert.Contains(t, r.Prompt, "<lora:anna_z:1>")
			assert.Equal(t, "cartoon, blurry", r.NegativePrompt)
			assert.Equal(t, stylist.Models[tt.base].Model, r.Model.Checkpoint)
			assert.Equal(t, 832, r.Width)
			assert.Equal(t, 1216, r.Height)
			assert.Equal(t, "img", res.Image)
			assert.Equal(t, int64(77), res.Seed)
			assert.Equal(t, "style "+tt.base, res.Model)
		})
	}
}

func TestGeneratePresetShot_Overrides(t *testing.T) {
	t.Parallel()
	steps, cfg, model := 12, 2.5, "juggernautXL_ragnarokBy"
	tests := []struct {
		name      string
		loras     string
		wantLoras []string
		noLoras   []string
	}{
		{"own LoRA list replaces style's, person stays", `[{"name":"grain","weight":0.3}]`, []string{"<lora:grain:0.3>", "<lora:anna_z:1>"}, []string{"film"}},
		{"ignore style LoRAs keeps person", `[]`, []string{"<lora:anna_z:1>"}, []string{"film"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db, reqs := presetShotService(t)
			id := shotPreset(t, db, "Z-Image", "photo", "", `[{"name":"film","weight":0.5}]`)
			loras := tt.loras
			ov := &PresetOverrides{Steps: &steps, CfgScale: &cfg, ModelName: &model, Loras: &loras, Person: &PersonOverride{Name: "intruder"}}
			_, err := svc.GeneratePresetShot(PresetShotParams{PresetID: id, ScenePrompt: "A woman", PersonLora: "anna_z", Overrides: ov, Width: 832, Height: 1216})
			require.NoError(t, err)
			r := (*reqs)[0]
			assert.Equal(t, 12, r.Steps)
			assert.InDelta(t, 2.5, r.CfgScale, 1e-9)
			assert.Equal(t, stylist.Models["Z-Image"].Model, r.Model.Checkpoint, "model override is ignored")
			for _, l := range tt.wantLoras {
				assert.Contains(t, r.Prompt, l)
			}
			for _, l := range tt.noLoras {
				assert.NotContains(t, r.Prompt, l)
			}
			assert.NotContains(t, r.Prompt, "intruder")
		})
	}
}

func TestGeneratePresetShot_ModelWithoutPreset(t *testing.T) {
	t.Parallel()
	svc, _, reqs := presetShotService(t)
	m := stylist.Models["Z-Image"]
	steps := 4
	seed := int64(5)
	_, err := svc.GeneratePresetShot(PresetShotParams{Model: m.Model, Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG,
		ScenePrompt: "A woman", PersonLora: "anna_z", Overrides: &PresetOverrides{Steps: &steps}, Width: 832, Height: 1216, Seed: &seed})
	require.NoError(t, err)
	r := (*reqs)[0]
	assert.Equal(t, m.Model, r.Model.Checkpoint)
	assert.Equal(t, m.Sampler, r.SamplerName)
	assert.Equal(t, 4, r.Steps)
	assert.InDelta(t, m.CFG, r.CfgScale, 1e-9)
	assert.Equal(t, "A woman <lora:anna_z:1>", r.Prompt)
	require.NotNil(t, r.Seed)
	assert.Equal(t, int64(5), *r.Seed)
}

func TestGeneratePresetShot_MissingPreset(t *testing.T) {
	t.Parallel()
	svc, _, reqs := presetShotService(t)
	_, err := svc.GeneratePresetShot(PresetShotParams{PresetID: 9999, ScenePrompt: "A woman"})
	require.Error(t, err)
	assert.Empty(t, *reqs)
}
