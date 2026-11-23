package generation

import (
	"image"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostEditor_FreeLookUsesSelectedProfileAndKeepsLegacyDefault(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		editor      *PostEditorParams
		steps       int
	}{
		{"legacy", kleinLookModel, nil, kleinLookSteps},
		{"selected", postTestFlux3, &PostEditorParams{Model: postTestFlux3, Loras: []preset.LoRAEntry{{Name: "SharedDetail", Weight: postTestLoraWeight}}}, postTestFlux3Steps},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			rec := &emoRec{}
			svc := newTestService(t, db, emLLM(), rec.mock(t))
			svc.sessions = &mockSessions{}
			_, err := svc.ApplyPostLook(ApplyLookParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: "film look", Editor: tc.editor})
			require.NoError(t, err)
			reqs := rec.requests()
			require.Len(t, reqs, 1)
			assert.Equal(t, tc.model, reqs[0].Model.Checkpoint)
			assert.Equal(t, tc.steps, reqs[0].Steps)
			assert.Equal(t, postTestUnitCFG, reqs[0].CfgScale)
			assert.Contains(t, reqs[0].Prompt, "a portrait photo")
			if tc.editor != nil {
				assert.Contains(t, reqs[0].Model.Modules, "Qwen3VL-4B-Instruct-Q4_K_M.gguf")
				assert.Contains(t, reqs[0].Prompt, "<lora:SharedDetail:0.65>")
			}
		})
	}
}

func TestPostEditor_RepairUsesSelectedProfileForEveryTileAndMetadata(t *testing.T) {
	svc, rec, sessions := repSetup(t)
	src, w, h := repWide3(t)
	_, err := svc.RepairImage(RepairParams{ImageBase64: src, MaskBase64: repMask(t, w, h, image.Rect(0, 0, w, h)), Editor: &PostEditorParams{Model: postTestQwen, Loras: []preset.LoRAEntry{{Name: "SharedDetail", Weight: postTestLoraWeight}}}})
	require.NoError(t, err)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Greater(t, len(rec.reqs), 1, "the fixture must exercise multiple repair tiles")
	for _, req := range rec.reqs {
		assert.Equal(t, postTestQwen, req.Model.Checkpoint)
		assert.Equal(t, postTestQwenSteps, req.Steps)
		assert.Equal(t, postTestQwenCFG, req.CfgScale)
		assert.Contains(t, req.Model.Modules, qwenTestVision)
		assert.Contains(t, req.Prompt, "<lora:SharedDetail:0.65>")
	}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	require.Len(t, sessions.items, 1)
	require.NotNil(t, sessions.items[0].meta)
	assert.Equal(t, postTestQwen, sessions.items[0].meta.Model)
}

func TestPostEditor_EffectOverridesPresetWithoutMutatingStoredPreset(t *testing.T) {
	for _, tc := range []struct {
		name  string
		loras []preset.LoRAEntry
	}{
		{"global lora", []preset.LoRAEntry{{Name: "SharedDetail", Weight: postTestLoraWeight}}},
		{"explicit empty", []preset.LoRAEntry{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			p := makeTestPreset(t, db, &preset.Preset{Name: "old effect", PresetType: "post_light", Prompt: "golden light <lora:EmbeddedWrong:0.5>", NegativePrompt: "<lora:NegativeWrong:0.5>", ModelName: qwenTestModel, Steps: postTestQwenSteps, CfgScale: postTestQwenCFG, Loras: `[{"name":"PresetWrong","weight":0.5}]`})
			before, err := db.Get(p.ID)
			require.NoError(t, err)
			rec := &emoRec{}
			svc := newTestService(t, db, emLLM(), rec.mock(t))
			svc.sessions = &mockSessions{}
			params := emoParams(t, p.ID, qwenTestModel, `[{"name":"OverrideWrong","weight":0.5}]`)
			params.Editor = &PostEditorParams{Model: postTestFlux3, Loras: tc.loras}
			_, err = svc.GenerateFromImage(params)
			require.NoError(t, err)
			reqs := rec.requests()
			require.Len(t, reqs, 1)
			req := reqs[0]
			assert.Equal(t, postTestFlux3, req.Model.Checkpoint)
			assert.Equal(t, postTestFlux3Steps, req.Steps)
			assert.Equal(t, postTestUnitCFG, req.CfgScale)
			assert.Contains(t, req.Prompt, "golden light")
			assert.Contains(t, req.Prompt, "Edit this photo by applying the following effect")
			assert.Contains(t, req.Prompt, "Retain the original subject, people, pose, camera framing and scene")
			for _, hidden := range []string{"EmbeddedWrong", "NegativeWrong", "PresetWrong", "OverrideWrong"} {
				assert.NotContains(t, req.Prompt, hidden)
				assert.NotContains(t, req.NegativePrompt, hidden)
			}
			if len(tc.loras) == 0 {
				assert.NotContains(t, req.Prompt, "<lora:")
			} else {
				assert.Contains(t, req.Prompt, "<lora:SharedDetail:0.65>")
			}
			after, err := db.Get(p.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after, "an operation must not rewrite the saved effect")
		})
	}
}

func TestPostEditor_OtherEntrypointsRejectUnknownModelBeforeGPU(t *testing.T) {
	invalid := &PostEditorParams{Model: "unknown-editor.gguf"}
	t.Run("look", func(t *testing.T) {
		svc, rec, _, _ := editModelSetup(t)
		_, err := svc.ApplyPostLook(ApplyLookParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: "film look", Editor: invalid})
		require.Error(t, err)
		_, _, reqs := rec.snapshot()
		assert.Empty(t, reqs)
	})
	t.Run("repair", func(t *testing.T) {
		svc, rec, _ := repSetup(t)
		_, err := svc.RepairImage(RepairParams{ImageBase64: repSource(t, objTestSize, objTestSize), MaskBase64: repMask(t, objTestSize, objTestSize, image.Rect(0, 0, objTestSize, objTestSize)), Editor: invalid})
		require.Error(t, err)
		assert.Zero(t, rec.img2imgCount())
	})
	t.Run("effect", func(t *testing.T) {
		svc, rec, id := emoSetup(t)
		params := emoParams(t, id, "", "")
		params.Editor = invalid
		_, err := svc.GenerateFromImage(params)
		require.Error(t, err)
		assert.Empty(t, rec.requests())
	})
}

func TestPostEditor_ExplicitBlankModelRejectedBeforeGPU(t *testing.T) {
	for _, model := range []string{"", "   "} {
		for _, path := range []string{"edit", "look", "object", "repair", "effect"} {
			t.Run(path+"/model="+model, func(t *testing.T) {
				svc, rec, id := emoSetup(t)
				editor := &PostEditorParams{Model: model}
				img := makePNGBase64(t, objTestSize, objTestSize)
				mask := repMask(t, objTestSize, objTestSize, image.Rect(0, 0, objTestSize, objTestSize))
				var err error
				switch path {
				case "edit":
					_, err = svc.EditImage(EditImageParams{ImageBase64: img, Instruction: "make hair red", Editor: editor})
				case "look":
					_, err = svc.ApplyPostLook(ApplyLookParams{ImageBase64: img, Text: "film look", Editor: editor})
				case "object":
					_, err = svc.EditObject(ObjectEditParams{ImageBase64: img, MaskBase64: mask, Op: "remove", Object: "cup", Editor: editor})
				case "repair":
					_, err = svc.RepairImage(RepairParams{ImageBase64: img, MaskBase64: mask, Editor: editor})
				case "effect":
					params := emoParams(t, id, "", "")
					params.Editor = editor
					_, err = svc.GenerateFromImage(params)
				}
				assert.Error(t, err, "an explicit editor requires a selected model")
				assert.Zero(t, len(rec.requests()), "invalid editor must be rejected before GPU work")
			})
		}
	}
}

func TestPostEditor_EffectFullStrength(t *testing.T) {
	const fullStrength = 1.0
	for _, model := range []string{postTestFlux3, postTestKlein, postTestQwen, ""} {
		t.Run(model, func(t *testing.T) {
			svc, rec, id := emoSetup(t)
			params := emoParams(t, id, "", "")
			params.DenoisingStrength = fullStrength
			if model != "" {
				params.Editor = &PostEditorParams{Model: model}
			}
			_, err := svc.GenerateFromImage(params)
			if model == "" {
				require.Error(t, err, "legacy preserve-identity strength remains limited")
				assert.Empty(t, rec.requests(), "reject legacy full strength before GPU work")
				return
			}
			require.NoError(t, err)
			reqs := rec.requests()
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].DenoisingStrength)
			assert.Equal(t, fullStrength, *reqs[0].DenoisingStrength)
			assert.Contains(t, reqs[0].Prompt, "Edit this photo by applying the following effect")
			assert.Contains(t, reqs[0].Prompt, emoPresetPrompt)
			assert.Contains(t, reqs[0].Prompt, "Retain the original subject, people, pose, camera framing and scene")
		})
	}
}

func TestPostEditor_LegacyEffectKeepsRawPrompt(t *testing.T) {
	svc, rec, id := emoSetup(t)
	_, err := svc.GenerateFromImage(emoParams(t, id, "", ""))
	require.NoError(t, err)
	reqs := rec.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, emoPresetPrompt, reqs[0].Prompt)
}
