package generation

import (
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stylistEditorSeed int64 = 321

func TestStylistEditorSelectedProfilesUseSourceWithoutPoseLock(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		model, encoder string
		steps          int
		cfg            float64
	}{
		{postTestKlein, "qwen_3_4b.safetensors", postTestKleinSteps, postTestUnitCFG},
		{postTestFlux3, "Qwen3VL-4B-Instruct-Q4_K_M.gguf", postTestFlux3Steps, postTestUnitCFG},
		{postTestQwen, qwenTestVision, postTestQwenSteps, postTestQwenCFG},
	} {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			svc, rec, _, _ := editModelSetup(t)
			source := makePNGBase64(t, objTestSize, objTestSize)
			seed := stylistEditorSeed
			const instruction = "The existing right hand is in the hair; the right waist is clear."
			_, err := svc.StylistReferenceEdit(source, instruction, &seed, &PostEditorParams{Model: tc.model, Loras: []preset.LoRAEntry{{Name: "ChosenDetail", Weight: postTestLoraWeight}}})
			require.NoError(t, err)
			_, _, requests := rec.snapshot()
			require.Len(t, requests, 1)
			req := requests[0]
			assert.Equal(t, tc.model, req.Model.Checkpoint)
			assert.Contains(t, req.Model.Modules, tc.encoder)
			vae := "flux2-klein-vae.safetensors"
			if tc.model == postTestQwen {
				vae = "qwen_image_2.1_vae_bf16.safetensors"
			}
			assert.Contains(t, req.Model.Modules, vae)
			assert.True(t, req.Model.KleinReference)
			assert.Equal(t, tc.steps, req.Steps)
			assert.Equal(t, tc.cfg, req.CfgScale)
			require.NotNil(t, req.Seed)
			assert.Equal(t, stylistEditorSeed, *req.Seed)
			assert.True(t, req.RefImage == source || (len(req.InitImages) > 0 && req.InitImages[0] == source), "exact source reference must reach SD")
			assert.Contains(t, req.Prompt, instruction)
			assert.Contains(t, req.Prompt, "<lora:ChosenDetail:0.65>")
			for _, blocked := range []string{"preserve the pose", "preserve pose", "do not change the pose", "preserve the composition"} {
				assert.NotContains(t, strings.ToLower(req.Prompt), blocked)
			}
		})
	}
}
func TestStylistEditorInvalidExplicitSelectionNeverCallsSD(t *testing.T) {
	t.Parallel()
	svc, rec, _, _ := editModelSetup(t)
	_, err := svc.StylistReferenceEdit(makePNGBase64(t, objTestSize, objTestSize), "change pose", nil, &PostEditorParams{Model: "invented.gguf"})
	require.Error(t, err)
	_, _, requests := rec.snapshot()
	assert.Empty(t, requests)
}
