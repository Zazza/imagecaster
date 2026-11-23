package generation

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	freeLookFull          = 1.0
	freeLookPartial       = 0.5
	freeLookMinimum       = 0.35
	freeLookLegacyMaximum = 0.65
	freeLookOutside       = 2.0
	freeLookRequested     = "cold blue cinematic lighting with fine film grain"
)

func TestFreeLookFix_StrengthContract(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input, want float64
		legacy      bool
	}{
		{"full", freeLookFull, freeLookFull, false},
		{"partial", freeLookPartial, freeLookPartial, false},
		{"upper bound", freeLookOutside, freeLookFull, false},
		{"lower bound", -freeLookFull, freeLookMinimum, false},
		{"legacy full", freeLookFull, freeLookLegacyMaximum, true},
		{"legacy partial", freeLookPartial, freeLookPartial, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, rec, _ := emoSetup(t)
			var editor *PostEditorParams
			if !tc.legacy {
				editor = &PostEditorParams{Model: postTestKlein}
			}
			_, err := svc.ApplyPostLook(ApplyLookParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: freeLookRequested, Strength: tc.input, Editor: editor})
			require.NoError(t, err)
			reqs := rec.requests()
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].DenoisingStrength)
			assert.Equal(t, tc.want, *reqs[0].DenoisingStrength)
		})
	}
	for _, input := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		t.Run("nonfinite", func(t *testing.T) {
			svc, rec, _ := emoSetup(t)
			_, err := svc.ApplyPostLook(ApplyLookParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: freeLookRequested, Strength: input, Editor: &PostEditorParams{Model: postTestKlein}})
			require.NoError(t, err)
			reqs := rec.requests()
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].DenoisingStrength)
			got := *reqs[0].DenoisingStrength
			assert.False(t, math.IsNaN(got) || math.IsInf(got, 0))
			assert.GreaterOrEqual(t, got, freeLookMinimum)
			assert.LessOrEqual(t, got, freeLookFull)
		})
	}
}

func TestFreeLookFix_EditCommandAndSelectedProfile(t *testing.T) {
	for _, model := range []string{postTestKlein, postTestFlux3, postTestQwen} {
		for _, fallback := range []bool{false, true} {
			t.Run(model+"/fallback="+map[bool]string{false: "no", true: "yes"}[fallback], func(t *testing.T) {
				db := openTestDB(t)
				rec := &emoRec{}
				llm := emLLM()
				llm.chatFn = func(model, system, user string, temp float64, tokens int) (string, error) {
					if fallback {
						return "", errors.New("LLM unavailable")
					}
					return "diffuse cobalt light and fine film grain", nil
				}
				svc := newTestService(t, db, llm, rec.mock(t))
				svc.sessions = &mockSessions{}
				source := makePNGBase64(t, objTestSize, objTestSize)
				_, err := svc.ApplyPostLook(ApplyLookParams{ImageBase64: source, Text: freeLookRequested, Strength: freeLookFull, Editor: &PostEditorParams{Model: model, Loras: []preset.LoRAEntry{{Name: "SharedDetail", Weight: postTestLoraWeight}}}})
				require.NoError(t, err)
				reqs := rec.requests()
				require.Len(t, reqs, 1)
				req := reqs[0]
				assert.Equal(t, model, req.Model.Checkpoint)
				assert.True(t, req.Model.KleinReference)
				switch model {
				case postTestKlein:
					assert.Equal(t, postTestKleinSteps, req.Steps)
					assert.Equal(t, postTestUnitCFG, req.CfgScale)
					assert.Contains(t, req.Model.Modules, "qwen_3_4b.safetensors")
				case postTestFlux3:
					assert.Equal(t, postTestFlux3Steps, req.Steps)
					assert.Equal(t, postTestUnitCFG, req.CfgScale)
					assert.Contains(t, req.Model.Modules, "Qwen3VL-4B-Instruct-Q4_K_M.gguf")
				case postTestQwen:
					assert.Equal(t, postTestQwenSteps, req.Steps)
					assert.Equal(t, postTestQwenCFG, req.CfgScale)
					assert.Contains(t, req.Model.Modules, qwenTestVision)
				}
				assert.True(t, req.RefImage == source || slices.Contains(req.InitImages, source), "the input photo must remain a reference")
				if fallback {
					assert.Contains(t, req.Prompt, freeLookRequested)
				} else {
					assert.Contains(t, req.Prompt, "diffuse cobalt light and fine film grain")
				}
				lower := strings.ToLower(req.Prompt)
				for _, concept := range []string{"edit", "photo", "people", "cloth", "pose", "framing"} {
					assert.Contains(t, lower, concept)
				}
				assert.True(t, strings.Contains(lower, "face") || strings.Contains(lower, "facial"), "instruct preservation of faces or facial features")
				assert.True(t, strings.Contains(lower, "preserv") || strings.Contains(lower, "retain") || strings.Contains(lower, "keep"), "instruct preservation")
				assert.Contains(t, req.Prompt, "<lora:SharedDetail:0.65>")
			})
		}
	}
}

func TestFreeLookFix_TranslatorRoutingPreservesConstraints(t *testing.T) {
	const request = "Холодный синий свет, без бликов и без жёсткого контраста"
	const legacySystem = "You translate a description of the desired look into a short English prose prompt for an image restyle model. Describe only the visual look: light, color, mood, texture, atmosphere. 1-2 sentences, at most 60 words. Reply with the prompt only."
	for _, modern := range []bool{false, true} {
		name := "legacy"
		if modern {
			name = "explicit editor"
		}
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			rec := &emoRec{}
			llm := emLLM()
			var capturedSystem, capturedUser string
			llm.chatFn = func(model, system, user string, temp float64, tokens int) (string, error) {
				capturedSystem, capturedUser = system, user
				return "Cold blue light, without highlights or hard contrast.", nil
			}
			svc := newTestService(t, db, llm, rec.mock(t))
			svc.sessions = &mockSessions{}
			params := ApplyLookParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: request, Strength: freeLookPartial}
			if modern {
				params.Editor = &PostEditorParams{Model: postTestKlein}
			}
			_, err := svc.ApplyPostLook(params)
			require.NoError(t, err)
			assert.Contains(t, capturedUser, request, "pass the unmodified user constraints to the translator")
			if !modern {
				assert.Equal(t, legacySystem, capturedSystem, "legacy translator contract stays unchanged")
				return
			}
			lower := strings.ToLower(capturedSystem)
			assert.Contains(t, lower, "translat")
			assert.Contains(t, lower, "preserve")
			assert.Contains(t, lower, "constraints")
			assert.Contains(t, lower, "negations")
			assert.Contains(t, lower, "not embellish")
			assert.Contains(t, lower, "not requested")
			for _, motif := range []string{"genre", "effect", "object"} {
				assert.Contains(t, lower, motif)
			}
		})
	}
}
