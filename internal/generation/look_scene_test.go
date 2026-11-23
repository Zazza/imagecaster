package generation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sceneWish = "raise the right hand into the hair and turn the head left, cold blue cinematic light"
const scenePrepared = "The person has their right hand in their hair and head turned left under cold blue cinematic light."
const sceneSeed int64 = 173

func TestLookSceneExactSourceAndSelectedRenderer(t *testing.T) {
	for _, model := range []string{postTestKlein, postTestFlux3, postTestQwen} {
		t.Run(model, func(t *testing.T) {
			rec := &emoRec{}
			source := makePNGBase64(t, objTestSize, objTestSize)
			llm := emLLM()
			visionCalls, chatCalls := 0, 0
			llm.chatVisionFn = func(_, _, wish, photo string, _ float64, _ int) (string, error) {
				visionCalls++
				assert.Equal(t, source, photo)
				assert.Contains(t, wish, sceneWish)
				return scenePrepared, nil
			}
			llm.chatFn = func(_, _, _ string, _ float64, _ int) (string, error) { chatCalls++; return "style-only rewrite", nil }
			svc := newTestService(t, openTestDB(t), llm, rec.mock(t))
			svc.sessions = &mockSessions{}
			seed := sceneSeed
			result, err := svc.ApplyPostLook(ApplyLookParams{ChangeScene: true, ImageBase64: source, Text: sceneWish, Strength: freeLookPartial, Seed: &seed, Editor: &PostEditorParams{Model: model, Loras: []preset.LoRAEntry{{Name: "SharedDetail", Weight: postTestLoraWeight}}}})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, 1, visionCalls)
			assert.Zero(t, chatCalls)
			requests := rec.requests()
			require.Len(t, requests, 1)
			req := requests[0]
			assert.Equal(t, model, req.Model.Checkpoint)
			assert.True(t, req.Model.KleinReference)
			assert.Contains(t, req.InitImages, source)
			switch model {
			case postTestKlein:
				assert.Equal(t, postTestKleinSteps, req.Steps)
				assert.Equal(t, postTestUnitCFG, req.CfgScale)
			case postTestFlux3:
				assert.Equal(t, postTestFlux3Steps, req.Steps)
				assert.Equal(t, postTestUnitCFG, req.CfgScale)
			case postTestQwen:
				assert.Equal(t, postTestQwenSteps, req.Steps)
				assert.Equal(t, postTestQwenCFG, req.CfgScale)
			}
			require.NotNil(t, req.Seed)
			assert.Equal(t, sceneSeed, *req.Seed)
			require.NotNil(t, req.DenoisingStrength)
			assert.Equal(t, freeLookPartial, *req.DenoisingStrength)
			assert.Contains(t, req.Prompt, scenePrepared)
			assert.Contains(t, strings.ToLower(req.Prompt), "not affected")
			assert.NotContains(t, req.Prompt, "Retain the original subject, people, pose")
			assert.Contains(t, req.Prompt, "<lora:SharedDetail:0.65>")
		})
	}
}

func TestLookScenePreparationFailureNeverRenders(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		err         error
	}{
		{name: "vision unavailable", err: errors.New("vision offline")}, {name: "empty"}, {name: "blank", reply: " \n "}, {name: "over limit", reply: strings.Repeat("я", prepareTestMaxRunes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &emoRec{}
			llm := emLLM()
			chats := 0
			llm.chatVisionFn = func(_, _, _, _ string, _ float64, _ int) (string, error) { return tc.reply, tc.err }
			llm.chatFn = func(_, _, _ string, _ float64, _ int) (string, error) { chats++; return "fallback", nil }
			svc := newTestService(t, openTestDB(t), llm, rec.mock(t))
			result, err := svc.ApplyPostLook(ApplyLookParams{ChangeScene: true, ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: sceneWish, Strength: freeLookPartial, Editor: &PostEditorParams{Model: postTestKlein}})
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Empty(t, rec.requests())
			assert.Zero(t, chats)
		})
	}
}

func TestLookSceneRejectsEditorBeforeVision(t *testing.T) {
	for _, editor := range []*PostEditorParams{nil, {Model: "unknown-editor.gguf"}, {Model: ""}} {
		rec := &emoRec{}
		llm := emLLM()
		calls := 0
		llm.chatVisionFn = func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return scenePrepared, nil }
		svc := newTestService(t, openTestDB(t), llm, rec.mock(t))
		_, err := svc.ApplyPostLook(ApplyLookParams{ChangeScene: true, ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: sceneWish, Editor: editor})
		require.Error(t, err)
		assert.Zero(t, calls)
		assert.Empty(t, rec.requests())
	}
}

func TestLookSceneRemoteVisionAndCancellationNeverRender(t *testing.T) {
	t.Run("remote receives no photo", func(t *testing.T) {
		calls := 0
		llm := emLLM()
		llm.chatVisionFn = func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return scenePrepared, nil }
		svc := newTestServiceURL(t, openTestDB(t), llm, "https://vision.example.com")
		_, err := svc.ApplyPostLook(ApplyLookParams{ChangeScene: true, ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: sceneWish, Editor: &PostEditorParams{Model: postTestKlein}})
		require.ErrorIs(t, err, ErrVisionNotLocal)
		assert.Zero(t, calls)
	})
	t.Run("cancel after vision", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rec := &emoRec{}
		llm := emLLM()
		llm.chatVisionFn = func(_, _, _, _ string, _ float64, _ int) (string, error) { cancel(); return scenePrepared, nil }
		sd := rec.mock(t)
		svc := newTestService(t, openTestDB(t), llm, sd)
		svc, _ = svc.ForQueue(ctx, sd, llm)
		_, err := svc.ApplyPostLook(ApplyLookParams{ChangeScene: true, ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: sceneWish, Editor: &PostEditorParams{Model: postTestKlein}})
		require.Error(t, err)
		assert.Empty(t, rec.requests())
	})
}

func TestLookSceneDefaultUsesTextOnly(t *testing.T) {
	rec := &emoRec{}
	llm := emLLM()
	calls := 0
	llm.chatVisionFn = func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return "unexpected", nil }
	svc := newTestService(t, openTestDB(t), llm, rec.mock(t))
	svc.sessions = &mockSessions{}
	_, err := svc.ApplyPostLook(ApplyLookParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), Text: sceneWish, Strength: freeLookFull, Editor: &PostEditorParams{Model: postTestKlein}})
	require.NoError(t, err)
	assert.Zero(t, calls)
	reqs := rec.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, freeLookFull, *reqs[0].DenoisingStrength)
}
