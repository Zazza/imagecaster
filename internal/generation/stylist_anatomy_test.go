package generation

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const anatomyTestResponseBytes = 256
const anatomyTestTokens = 64

func TestStylistAnatomyLocalExactInput(t *testing.T) {
	image := "data:image/png;base64," + makePNGBase64(t, 64, 64)
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("llm_analyze_model", "local-anatomy"))
	calls := 0
	svc := newTestService(t, db, &mockLLM{chatVisionFn: func(model, system, user, source string, temp float64, tokens int) (string, error) {
		calls++
		assert.Equal(t, "local-anatomy", model)
		assert.Equal(t, image, source)
		assert.Contains(t, user, "hands behind back")
		assert.Contains(t, strings.ToLower(system), "occlu")
		assert.Contains(t, strings.ToLower(system), "uncertain")
		assert.Zero(t, temp)
		assert.Equal(t, anatomyTestTokens, tokens)
		return `{"verdict":"ok"}`, nil
	}}, &mockSD{})
	got, err := svc.CheckStylistAnatomy(image, "hands behind back")
	require.NoError(t, err)
	assert.Equal(t, StylistAnatomyOK, got.Verdict)
	assert.Equal(t, 1, calls)
}

func TestStylistAnatomyStrictReply(t *testing.T) {
	for _, reply := range []string{``, `ok`, "```json\n{\"verdict\":\"ok\"}\n```", `{"verdict":"ok","reason":"secret"}`, `{"verdict":"ok"} {}`, `{"verdict":"OK"}`, `{"verdict":null}`, `{"verdict":"ok","verdict":"defect"}`, strings.Repeat(" ", anatomyTestResponseBytes) + `{"verdict":"ok"}`} {
		t.Run(reply, func(t *testing.T) {
			svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return reply, nil }}, &mockSD{})
			got, err := svc.CheckStylistAnatomy(makePNGBase64(t, 64, 64), "standing")
			require.Error(t, err)
			assert.NotEqual(t, StylistAnatomyOK, got.Verdict)
		})
	}
	for _, verdict := range []string{"ok", "defect", "uncertain"} {
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return `{"verdict":"` + verdict + `"}`, nil }}, &mockSD{})
		got, err := svc.CheckStylistAnatomy(makePNGBase64(t, 64, 64), "standing")
		require.NoError(t, err)
		assert.Equal(t, verdict, got.Verdict)
	}
}

func TestStylistAnatomyRemoteNeverReceivesPhoto(t *testing.T) {
	calls := 0
	svc := newTestServiceURL(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return `{"verdict":"ok"}`, nil }}, "https://vision.example.com")
	_, err := svc.CheckStylistAnatomy(makePNGBase64(t, 64, 64), "standing")
	require.ErrorIs(t, err, ErrVisionNotLocal)
	assert.Zero(t, calls)
}

func TestStylistAnatomyVisionFailure(t *testing.T) {
	visionErr := errors.New("private https://vision.example.com/photo token-secret")
	svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { return "", visionErr }}, &mockSD{})
	_, err := svc.CheckStylistAnatomy(makePNGBase64(t, 64, 64), "standing")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), visionErr.Error())
}

func TestStylistAnatomyValidationBeforeVision(t *testing.T) {
	const maxPromptRunes = 12000
	for _, tc := range []struct{ image, prompt string }{
		{prompt: "standing"}, {image: "image", prompt: " "}, {image: strings.Repeat("x", MaxImageBase64Len+1), prompt: "standing"}, {image: "image", prompt: strings.Repeat("я", maxPromptRunes+1)},
	} {
		calls := 0
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) { calls++; return `{"verdict":"ok"}`, nil }}, &mockSD{})
		_, err := svc.CheckStylistAnatomy(tc.image, tc.prompt)
		require.Error(t, err)
		assert.Zero(t, calls)
	}
}
func TestStylistAnatomyCancellation(t *testing.T) {
	for _, before := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		svc := newTestService(t, openTestDB(t), &mockLLM{chatVisionFn: func(_, _, _, _ string, _ float64, _ int) (string, error) {
			calls++
			cancel()
			return `{"verdict":"ok"}`, nil
		}}, &mockSD{})
		q, _ := svc.ForQueue(ctx, nil, nil)
		if before {
			cancel()
		}
		got, err := q.CheckStylistAnatomy(makePNGBase64(t, 64, 64), "standing")
		require.ErrorIs(t, err, ErrInterrupted)
		assert.NotEqual(t, StylistAnatomyOK, got.Verdict)
		if before {
			assert.Zero(t, calls)
		} else {
			assert.Equal(t, 1, calls)
		}
		cancel()
	}
}
