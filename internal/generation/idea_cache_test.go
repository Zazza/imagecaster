package generation

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateImage_IdeaSameInputsCallLLMOnce(t *testing.T) {
	t.Parallel()
	svc, db, rec := ideaSetup(t, nil)
	makeTestPreset(t, db, nil)

	params := GenerateImageParams{PresetID: 1, Idea: ideaText, IdeaNegative: ideaNegText}
	first, err := svc.GenerateImage(params)
	require.NoError(t, err)
	second, err := svc.GenerateImage(params)
	require.NoError(t, err)

	assert.Len(t, rec.llmCalls(), 1, "same idea, style and negative must reuse the LLM prompt")
	calls := rec.txt2imgCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, calls[0].Prompt, calls[1].Prompt)
	assert.Equal(t, calls[0].NegativePrompt, calls[1].NegativePrompt)
	assert.Contains(t, calls[1].Prompt, ideaLLMPrompt)

	for i, res := range []*GenerateImageResult{first, second} {
		require.NotNil(t, res, "result %d", i)
		assert.Equal(t, ideaLLMPrompt, res.IdeaPrompt, "result %d", i)
		assert.Equal(t, ideaLLMNeg, res.IdeaNegativePrompt, "result %d", i)
	}
}

func TestGenerateImage_IdeaChangedInputsCallLLMAgain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		second func(other int64) GenerateImageParams
	}{
		{
			name: "other style with a different prompt",
			second: func(other int64) GenerateImageParams {
				return GenerateImageParams{PresetID: other, Idea: ideaText, IdeaNegative: ideaNegText}
			},
		},
		{
			name: "different idea",
			second: func(int64) GenerateImageParams {
				return GenerateImageParams{PresetID: 1, Idea: ideaText + " at night", IdeaNegative: ideaNegText}
			},
		},
		{
			name: "different idea negative",
			second: func(int64) GenerateImageParams {
				return GenerateImageParams{PresetID: 1, Idea: ideaText, IdeaNegative: "no birds"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db, rec := ideaSetup(t, nil)
			makeTestPreset(t, db, nil)
			other := ideaPreset(t, db, "other-style", "other-style-tags")

			_, err := svc.GenerateImage(GenerateImageParams{PresetID: 1, Idea: ideaText, IdeaNegative: ideaNegText})
			require.NoError(t, err)
			_, err = svc.GenerateImage(tt.second(other.ID))
			require.NoError(t, err)

			llm := rec.llmCalls()
			require.Len(t, llm, 2)
			if tt.name == "other style with a different prompt" {
				assert.Contains(t, llm[1], ideaStyleLabel+"other-style-tags")
			}
		})
	}
}

func TestGenerateImage_IdeaLLMErrorIsNotCached(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	n := 0
	svc, db, rec := ideaSetup(t, func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return "", fmt.Errorf("LLM connection refused")
		}
		return ideaLLMOK, nil
	})
	makeTestPreset(t, db, nil)

	params := GenerateImageParams{PresetID: 1, Idea: ideaText, IdeaNegative: ideaNegText}
	_, err := svc.GenerateImage(params)
	require.Error(t, err)

	res, err := svc.GenerateImage(params)
	require.NoError(t, err)
	assert.Len(t, rec.llmCalls(), 2, "a failed LLM call must not be remembered")
	require.Len(t, rec.txt2imgCalls(), 1)
	assert.Contains(t, rec.txt2imgCalls()[0].Prompt, ideaLLMPrompt)
	assert.Equal(t, ideaLLMPrompt, res.IdeaPrompt)
}

func TestGenerateImage_IdeaEmptyLLMPromptIsNotCached(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	n := 0
	svc, db, rec := ideaSetup(t, func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return `{"prompt":"","negative_prompt":"only-neg"}`, nil
		}
		return ideaLLMOK, nil
	})
	makeTestPreset(t, db, nil)

	params := GenerateImageParams{PresetID: 1, Idea: ideaText}
	_, err := svc.GenerateImage(params)
	require.Error(t, err)
	_, err = svc.GenerateImage(params)
	assert.Len(t, rec.llmCalls(), 2, "an empty LLM prompt must not be remembered")
	require.NoError(t, err)
}

func TestGenerateCompoundImage_IdeaPromptInResult(t *testing.T) {
	t.Parallel()
	svc, db, _ := ideaSetup(t, nil)
	first := ideaPreset(t, db, "first-style", "first-style-tags")
	second := ideaPreset(t, db, "second-style", "second-style-tags")

	res, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{
		Steps:        []CustomStep{{PresetID: first.ID}, {PresetID: second.ID, DenoisingStrength: 0.6}},
		Idea:         ideaText,
		IdeaNegative: ideaNegText,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ideaLLMPrompt, res.IdeaPrompt)
	assert.Equal(t, ideaLLMNeg, res.IdeaNegativePrompt)
}

func TestGenerate_NoIdeaLeavesIdeaPromptEmpty(t *testing.T) {
	t.Parallel()

	t.Run("txt2img", func(t *testing.T) {
		t.Parallel()
		svc, db, rec := ideaSetup(t, nil)
		makeTestPreset(t, db, nil)
		res, err := svc.GenerateImage(GenerateImageParams{PresetID: 1, ExtraPrompt: "custom prompt", ExtraNegativePrompt: "custom neg"})
		require.NoError(t, err)
		assert.Empty(t, rec.llmCalls())
		assert.Empty(t, res.IdeaPrompt)
		assert.Empty(t, res.IdeaNegativePrompt)
	})

	t.Run("compound", func(t *testing.T) {
		t.Parallel()
		svc, db, rec := ideaSetup(t, nil)
		first := ideaPreset(t, db, "first-style", "first-style-tags")
		res, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{
			Steps:       []CustomStep{{PresetID: first.ID}},
			ExtraPrompt: "custom prompt",
		})
		require.NoError(t, err)
		assert.Empty(t, rec.callsWith(ideaText))
		assert.Empty(t, res.IdeaPrompt)
		assert.Empty(t, res.IdeaNegativePrompt)
	})
}

func TestGenerateSDPrompt_ManualButtonIsNeverCached(t *testing.T) {
	t.Parallel()

	t.Run("two identical manual calls", func(t *testing.T) {
		t.Parallel()
		svc, db, rec := ideaSetup(t, nil)
		makeTestPreset(t, db, nil)
		params := GenerateSDPromptParams{PresetID: 1, Description: ideaText, Negative: ideaNegText}
		_, err := svc.GenerateSDPrompt(params)
		require.NoError(t, err)
		_, err = svc.GenerateSDPrompt(params)
		require.NoError(t, err)
		assert.Len(t, rec.llmCalls(), 2)
	})

	t.Run("manual call after a queued idea with the same inputs", func(t *testing.T) {
		t.Parallel()
		svc, db, rec := ideaSetup(t, nil)
		makeTestPreset(t, db, nil)
		_, err := svc.GenerateImage(GenerateImageParams{PresetID: 1, Idea: ideaText, IdeaNegative: ideaNegText})
		require.NoError(t, err)
		_, err = svc.GenerateSDPrompt(GenerateSDPromptParams{PresetID: 1, Description: ideaText, Negative: ideaNegText})
		require.NoError(t, err)
		assert.Len(t, rec.llmCalls(), 2)
	})
}

func TestGenerateImage_IdeaReuseReappliesKidsFilter(t *testing.T) {
	t.Parallel()
	svc, db, rec := ideaSetup(t, func() (string, error) {
		return `{"prompt":"cat, red sofa, knife, cozy room, warm light","negative_prompt":"blurry"}`, nil
	})
	makeTestPreset(t, db, nil)
	require.NoError(t, svc.kids.SetKidsMode(true, ""))
	require.NoError(t, svc.kids.SetCategory("weapons", false, ""))

	params := GenerateImageParams{PresetID: 1, Idea: ideaText}
	first, err := svc.GenerateImage(params)
	require.NoError(t, err)
	calls := rec.txt2imgCalls()
	require.Len(t, calls, 1)
	require.Contains(t, calls[0].Prompt, "knife", "weapons are allowed for the first run")

	require.NoError(t, svc.kids.SetCategory("weapons", true, ""))
	second, err := svc.GenerateImage(params)
	require.NoError(t, err)

	calls = rec.txt2imgCalls()
	require.Len(t, calls, 2)
	assert.NotContains(t, calls[1].Prompt, "knife")
	assert.Contains(t, calls[1].Prompt, "red sofa")
	assert.NotContains(t, second.IdeaPrompt, "knife")
	assert.Contains(t, first.IdeaPrompt, "knife")
	assert.Len(t, rec.llmCalls(), 1, "the second run must go through the reused LLM answer")
}
