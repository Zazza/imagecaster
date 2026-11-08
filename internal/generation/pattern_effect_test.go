package generation

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateFromImage_EM4_PatternMask_PromptUsesPatternTemplateWithItem(t *testing.T) {
	t.Parallel()
	const w, h = 512, 512
	src := makePNGBase64(t, w, h)

	control := mkSetup(t, "post_pattern", nil, nil)
	_, err := control.svc.GenerateFromImage(control.params(src))
	require.NoError(t, err)
	asIs := control.onlyPrompt(t)

	e := mkSetup(t, "post_pattern", func() (string, error) { return "  a white silk blouse \n", nil }, nil)
	p := e.params(src)
	p.MaskBase64 = mkMask(t, w, h, image.Rect(128, 128, 384, 384))

	result, err := e.svc.GenerateFromImage(p)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, e.rec.visionCount(), "vision names the item under the mask exactly once")
	prompt := e.onlyPrompt(t)
	assert.Contains(t, prompt, "a white silk blouse", "detected item is in the prompt, trimmed")
	assert.NotContains(t, prompt, "  a white silk blouse \n")
	assert.Contains(t, prompt, mkEffectText, "effect text is in the prompt")
	assert.NotEqual(t, asIs, prompt, "masked pattern prompt goes through a template")
	assert.NotContains(t, prompt, "Change only the material", "pattern has its own template, not the material one")
}

func TestGenerateFromImage_EM4_PatternMask_VisionFailureStopsBeforeSD(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		vision func() (string, error)
	}{
		{name: "vision error", vision: func() (string, error) { return "", fmt.Errorf("vision down") }},
		{name: "empty answer", vision: func() (string, error) { return "", nil }},
		{name: "whitespace answer", vision: func() (string, error) { return " \n\t ", nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			const w, h = 512, 512
			e := mkSetup(t, "post_pattern", tt.vision, nil)
			p := e.params(makePNGBase64(t, w, h))
			p.MaskBase64 = mkMask(t, w, h, image.Rect(128, 128, 384, 384))

			_, err := e.svc.GenerateFromImage(p)
			require.Error(t, err)
			assert.Zero(t, e.rec.sdCount(), "SD must not be called when the item is unknown")
			e.sessions.mu.Lock()
			assert.Empty(t, e.sessions.items)
			e.sessions.mu.Unlock()
		})
	}
}

func TestGenerateFromImage_EM4_PatternNoMask_PromptAsIsNoVision(t *testing.T) {
	t.Parallel()
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep_color=%v", keep), func(t *testing.T) {
			t.Parallel()
			const w, h = 512, 512
			src := makePNGBase64(t, w, h)

			control := mkSetup(t, "post", nil, nil)
			_, err := control.svc.GenerateFromImage(control.params(src))
			require.NoError(t, err)
			want := control.onlyPrompt(t)

			e := mkSetup(t, "post_pattern", nil, nil)
			p := e.params(src)
			p.KeepColor = keep
			_, err = e.svc.GenerateFromImage(p)
			require.NoError(t, err)

			assert.Zero(t, e.rec.visionCount(), "vision must not be called without a mask")
			assert.Equal(t, want, e.onlyPrompt(t), "pattern effect without mask sends the effect prompt as any other effect")
		})
	}
}

func TestGenerateFromImage_EM4_PatternKeepColorIgnored(t *testing.T) {
	t.Parallel()
	const w, h = 768, 512
	for _, withMask := range []bool{false, true} {
		t.Run(fmt.Sprintf("mask=%v", withMask), func(t *testing.T) {
			t.Parallel()
			src := mkBlueRedPNG(t, w, h)
			run := func(keep bool) (*mkEnv, string) {
				e := mkSetup(t, "post_pattern", func() (string, error) { return "a dress", nil }, mkYellowChecker)
				p := e.params(src)
				p.KeepColor = keep
				if withMask {
					p.MaskBase64 = mkMask(t, w, h, image.Rect(0, 0, w, h))
				}
				result, err := e.svc.GenerateFromImage(p)
				require.NoError(t, err)
				require.NotNil(t, result)
				return e, result.Image
			}

			off, offImg := run(false)
			on, onImg := run(true)

			assert.Equal(t, off.onlyPrompt(t), on.onlyPrompt(t), "keep_color must not change the prompt for patterns")
			assert.Equal(t, off.rec.visionCount(), on.rec.visionCount(), "keep_color must not add vision calls")

			on.rec.mu.Lock()
			require.Len(t, on.rec.outputs, 1)
			out := on.rec.outputs[0]
			on.rec.mu.Unlock()
			require.True(t, strings.TrimSpace(out) != "")
			if !withMask {
				assert.Equal(t, out, onImg, "without mask, keep_color=true result is SD output byte for byte")
			}
			assert.Equal(t, offImg, onImg, "keep_color=true gives the same result as keep_color=false")
		})
	}
}

func TestPostTypeFamilies_EM6_Unchanged(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"post", "post_material", "post_skin", "post_light", "post_film"}, postTypeFamilies)
}
