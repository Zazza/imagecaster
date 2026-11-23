package generation

import (
	"encoding/json"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	postTestFlux3      = "flux3-image-klein-Q4_K.gguf"
	postTestKlein      = "flux-2-klein-4b-Q8_0.gguf"
	postTestQwen       = "qwen_image_2.1-Q8_0.gguf"
	postTestLoraWeight = 0.65
	postTestFlux3Steps = 16
	postTestKleinSteps = 8
	postTestQwenSteps  = 20
	postTestUnitCFG    = 1.0
	postTestQwenCFG    = 6.0
)

func postTestDecode(t *testing.T, fields map[string]any, target any) {
	t.Helper()
	data, err := json.Marshal(fields)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, target))
}

func postTestEditor(model string, loras []preset.LoRAEntry) map[string]any {
	return map[string]any{"model": model, "loras": loras}
}

func TestPostEditor_EditRequestSelectedCatalogModel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		model   string
		steps   int
		cfg     float64
		encoder string
	}{
		{postTestKlein, postTestKleinSteps, postTestUnitCFG, "qwen_3_4b.safetensors"},
		{postTestFlux3, postTestFlux3Steps, postTestUnitCFG, "Qwen3VL-4B-Instruct-Q4_K_M.gguf"},
		{postTestQwen, postTestQwenSteps, postTestQwenCFG, qwenTestVision},
	} {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			svc, rec, _, _ := editModelSetup(t)
			var params EditImageParams
			postTestDecode(t, map[string]any{
				"image_base64": makePNGBase64(t, objTestSize, objTestSize), "instruction": "make hair red",
				"model": "klein", "loras": []preset.LoRAEntry{{Name: "LegacyWrong", Weight: postTestLoraWeight}},
				"editor": postTestEditor(tc.model, []preset.LoRAEntry{{Name: "SharedDetail", Weight: postTestLoraWeight}}),
			}, &params)
			_, err := svc.EditImage(params)
			require.NoError(t, err)
			_, _, reqs := rec.snapshot()
			require.Len(t, reqs, 1)
			req := reqs[0]
			assert.Equal(t, tc.model, req.Model.Checkpoint)
			assert.Contains(t, req.Model.Modules, tc.encoder)
			assert.True(t, req.Model.KleinReference)
			assert.Equal(t, tc.steps, req.Steps)
			assert.Equal(t, tc.cfg, req.CfgScale)
			assert.Contains(t, req.Prompt, "<lora:SharedDetail:0.65>")
			assert.NotContains(t, req.Prompt, "LegacyWrong")
		})
	}
}

func TestPostEditor_EditExplicitEmptyLorasSuppressesStyleAndLegacyLoras(t *testing.T) {
	t.Parallel()
	svc, rec, db, _ := editModelSetup(t)
	style := makeTestPreset(t, db, &preset.Preset{Name: "editor style", ModelName: kleinLookModel, Prompt: "soft film", Loras: `[{"name":"PresetWrong","weight":0.5}]`})
	var params EditImageParams
	postTestDecode(t, map[string]any{
		"image_base64": makePNGBase64(t, objTestSize, objTestSize), "instruction": "make hair red", "style_preset_id": style.ID,
		"loras":  []preset.LoRAEntry{{Name: "LegacyWrong", Weight: postTestLoraWeight}},
		"editor": postTestEditor(postTestKlein, []preset.LoRAEntry{}),
	}, &params)
	_, err := svc.EditImage(params)
	require.NoError(t, err)
	_, _, reqs := rec.snapshot()
	require.Len(t, reqs, 1)
	assert.NotContains(t, reqs[0].Prompt, "<lora:")
}

func TestPostEditor_EditUnknownModelRejectedBeforeGPU(t *testing.T) {
	t.Parallel()
	svc, rec, _, _ := editModelSetup(t)
	var params EditImageParams
	postTestDecode(t, map[string]any{"image_base64": makePNGBase64(t, objTestSize, objTestSize), "instruction": "make hair red", "editor": postTestEditor("flux3-image-klein-invented.gguf", nil)}, &params)
	_, err := svc.EditImage(params)
	require.Error(t, err)
	_, _, reqs := rec.snapshot()
	assert.Empty(t, reqs)
}

func TestPostEditor_ObjectRequestUsesSelectedProfile(t *testing.T) {
	t.Parallel()
	svc, rec, _ := objTestSetup(t, nil, nil)
	var params ObjectEditParams
	postTestDecode(t, map[string]any{"image_base64": makePNGBase64(t, objTestSize, objTestSize), "mask_base64": objValidMask(t), "op": "remove", "object": "cup", "editor": postTestEditor(postTestFlux3, []preset.LoRAEntry{{Name: "SharedDetail", Weight: postTestLoraWeight}})}, &params)
	_, err := svc.EditObject(params)
	require.NoError(t, err)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.reqs, 1)
	req := rec.reqs[0]
	assert.Equal(t, postTestFlux3, req.Model.Checkpoint)
	assert.Equal(t, postTestFlux3Steps, req.Steps)
	assert.Equal(t, postTestUnitCFG, req.CfgScale)
	assert.Contains(t, req.Model.Modules, "Qwen3VL-4B-Instruct-Q4_K_M.gguf")
	assert.Contains(t, req.Prompt, "<lora:SharedDetail:0.65>")
}
