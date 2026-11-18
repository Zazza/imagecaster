package generation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

func TestEditImage_TryOnReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
	}{
		{name: "klein", model: "klein"},
		{name: "default model", model: ""},
		{name: "qwen", model: "qwen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _, _ := editModelSetup(t)
			garment := makePNGBase64(t, 64, 96)

			_, err := svc.EditImage(EditImageParams{
				ImageBase64:          makePNGBase64(t, 512, 512),
				ReferenceImageBase64: garment,
				Instruction:          "make the jacket fit loosely",
				Model:                tt.model,
			})
			require.NoError(t, err)

			_, _, reqs := rec.snapshot()
			require.Len(t, reqs, 1)
			req := reqs[0]
			assert.Equal(t, garment, req.RefImage)
			lower := strings.ToLower(req.Prompt)
			iTpl := strings.Index(lower, "second image")
			iInstr := strings.Index(lower, "make the jacket fit loosely")
			require.GreaterOrEqual(t, iTpl, 0, "try-on template must mention the second image: %q", req.Prompt)
			require.GreaterOrEqual(t, iInstr, 0, "instruction must stay in the prompt: %q", req.Prompt)
			assert.Less(t, iTpl, iInstr, "template goes before the instruction: %q", req.Prompt)
		})
	}
}

func TestEditImage_TryOnTemplateBeforeInstructionWithLoras(t *testing.T) {
	t.Parallel()
	svc, rec, _, _ := editModelSetup(t)

	_, err := svc.EditImage(EditImageParams{
		ImageBase64:          makePNGBase64(t, 512, 512),
		ReferenceImageBase64: makePNGBase64(t, 64, 64),
		Instruction:          "roll up the sleeves",
		Loras:                []preset.LoRAEntry{{Name: "Klein_Detail", Weight: 0.7}},
		Model:                "klein",
	})
	require.NoError(t, err)

	_, _, reqs := rec.snapshot()
	require.Len(t, reqs, 1)
	p := reqs[0].Prompt
	assert.Contains(t, p, "<lora:Klein_Detail:0.7>")
	lower := strings.ToLower(p)
	iTpl := strings.Index(lower, "second image")
	iInstr := strings.Index(lower, "roll up the sleeves")
	require.GreaterOrEqual(t, iTpl, 0, p)
	require.GreaterOrEqual(t, iInstr, 0, p)
	assert.Less(t, iTpl, iInstr, p)
}

func TestEditImage_NoReferenceKeepsRequest(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"klein", "qwen"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			svc, rec, _, _ := editModelSetup(t)

			_, err := svc.EditImage(EditImageParams{
				ImageBase64: makePNGBase64(t, 512, 512),
				Instruction: "make hair red",
				Model:       model,
			})
			require.NoError(t, err)

			_, _, reqs := rec.snapshot()
			require.Len(t, reqs, 1)
			assert.Empty(t, reqs[0].RefImage)
			assert.Equal(t, buildEditPrompt("make hair red", "", nil), reqs[0].Prompt)
			assert.NotContains(t, strings.ToLower(reqs[0].Prompt), "second image")
		})
	}
}

func TestEditImage_TryOnQualitySecondPassWithoutReference(t *testing.T) {
	t.Parallel()
	svc, rec, db, _ := editModelSetup(t)
	res := &preset.Resolution{Name: "q-1024", Width: 1024, Height: 1024}
	require.NoError(t, db.CreateResolution(res))
	resID := res.ID
	garment := makePNGBase64(t, 64, 64)

	_, err := svc.EditImage(EditImageParams{
		ImageBase64:          makePNGBase64(t, 512, 512),
		ReferenceImageBase64: garment,
		Instruction:          "put on the coat",
		OutputMode:           "quality",
		ResolutionID:         &resID,
		Model:                "klein",
	})
	require.NoError(t, err)

	_, _, reqs := rec.snapshot()
	require.GreaterOrEqual(t, len(reqs), 2, "base edit + second pass expected")
	assert.Equal(t, garment, reqs[0].RefImage)
	for i, req := range reqs[1:] {
		assert.Empty(t, req.RefImage, "second pass %d must not carry the garment", i+1)
	}
}

func TestEditImage_TryOnReferenceTooLarge(t *testing.T) {
	t.Parallel()
	svc, rec, _, sessions := editModelSetup(t)

	_, err := svc.EditImage(EditImageParams{
		ImageBase64:          makePNGBase64(t, 512, 512),
		ReferenceImageBase64: strings.Repeat("A", MaxImageBase64Len+1),
		Instruction:          "put on the coat",
		Model:                "klein",
	})
	require.Error(t, err)

	ops, _, reqs := rec.snapshot()
	assert.Empty(t, reqs)
	assert.NotContains(t, ops, "img2img")
	assert.Empty(t, sessions.items)
}
