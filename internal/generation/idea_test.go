package generation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	ideaText       = "a cat sitting on a red sofa"
	ideaNegText    = "no dogs around"
	ideaLLMPrompt  = "idea-llm-tags, cat, red sofa"
	ideaLLMNeg     = "idea-llm-neg"
	ideaLLMOK      = `{"prompt":"` + ideaLLMPrompt + `","negative_prompt":"` + ideaLLMNeg + `"}`
	ideaStyleLabel = "STYLE REFERENCE (do NOT include in output): "
)

type ideaRecorder struct {
	mu        sync.Mutex
	llmUsers  []string
	txt2img   []sd.Txt2ImgRequest
	img2img   []sd.Img2ImgRequest
	llmResult func() (string, error)
}

func (r *ideaRecorder) llmCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.llmUsers...)
}

func (r *ideaRecorder) txt2imgCalls() []sd.Txt2ImgRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sd.Txt2ImgRequest(nil), r.txt2img...)
}

func (r *ideaRecorder) sdCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.txt2img) + len(r.img2img)
}

func (r *ideaRecorder) callsWith(substr string) []string {
	var out []string
	for _, u := range r.llmCalls() {
		if strings.Contains(u, substr) {
			out = append(out, u)
		}
	}
	return out
}

func ideaSetup(t *testing.T, llmResult func() (string, error)) (*Service, *preset.DB, *ideaRecorder) {
	t.Helper()
	db := openTestDB(t)
	rec := &ideaRecorder{llmResult: llmResult}
	if rec.llmResult == nil {
		rec.llmResult = func() (string, error) { return ideaLLMOK, nil }
	}
	llmSvc := &mockLLM{
		genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
			rec.mu.Lock()
			rec.llmUsers = append(rec.llmUsers, userMessage)
			fn := rec.llmResult
			rec.mu.Unlock()
			return fn()
		},
	}
	sdSvc := &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			rec.mu.Lock()
			rec.txt2img = append(rec.txt2img, req)
			rec.mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{"txt2img-img"}}, nil
		},
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			rec.mu.Lock()
			rec.img2img = append(rec.img2img, req)
			rec.mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{"img2img-img"}}, nil
		},
	}
	svc := newTestService(t, db, llmSvc, sdSvc)
	svc.ctx = context.Background()
	return svc, db, rec
}

func ideaPreset(t *testing.T, db *preset.DB, name, prompt string) *preset.Preset {
	t.Helper()
	return makeTestPreset(t, db, &preset.Preset{
		Name:           name,
		PresetType:     "anime",
		Prompt:         prompt,
		NegativePrompt: name + "-neg",
		Sampler:        "Euler a",
		Steps:          20,
		CfgScale:       7.0,
	})
}

func TestGenerateImage_IdeaEmptyKeepsExtraPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		idea string
	}{
		{"empty idea", ""},
		{"whitespace idea", "   \n\t "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db, rec := ideaSetup(t, nil)
			makeTestPreset(t, db, nil)

			_, err := svc.GenerateImage(GenerateImageParams{
				PresetID:            1,
				ExtraPrompt:         "custom prompt",
				ExtraNegativePrompt: "custom neg",
				Idea:                tt.idea,
				IdeaNegative:        ideaNegText,
			})
			require.NoError(t, err)

			assert.Empty(t, rec.llmCalls(), "LLM must not be called for an empty idea")
			calls := rec.txt2imgCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, "custom prompt, BREAK, masterpiece, best quality, 1girl", calls[0].Prompt)
			assert.Contains(t, calls[0].NegativePrompt, "custom neg")
		})
	}
}

func TestGenerateImage_IdeaGoesThroughLLM(t *testing.T) {
	t.Parallel()
	svc, db, rec := ideaSetup(t, nil)
	makeTestPreset(t, db, nil)

	_, err := svc.GenerateImage(GenerateImageParams{
		PresetID:            1,
		ExtraPrompt:         "stale extra prompt",
		ExtraNegativePrompt: "stale extra neg",
		Idea:                ideaText,
		IdeaNegative:        ideaNegText,
	})
	require.NoError(t, err)

	llm := rec.llmCalls()
	require.Len(t, llm, 1, "LLM must be called exactly once for the idea")
	assert.Contains(t, llm[0], ideaText)
	assert.Contains(t, llm[0], ideaNegText)
	assert.Contains(t, llm[0], ideaStyleLabel+"masterpiece, best quality, 1girl")

	calls := rec.txt2imgCalls()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0].Prompt, ideaLLMPrompt)
	assert.NotContains(t, calls[0].Prompt, ideaText, "raw idea must not reach SD")
	assert.NotContains(t, calls[0].Prompt, "stale extra prompt", "LLM result replaces ExtraPrompt")
	assert.Contains(t, calls[0].NegativePrompt, ideaLLMNeg)
	assert.NotContains(t, calls[0].NegativePrompt, "stale extra neg", "LLM result replaces ExtraNegativePrompt")
}

func TestGenerateImage_IdeaFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		idea         string
		ideaNegative string
		llmResult    func() (string, error)
		wantErrSub   string
		wantLLMCalls bool
	}{
		{
			name:         "llm error",
			idea:         ideaText,
			llmResult:    func() (string, error) { return "", fmt.Errorf("LLM connection refused") },
			wantErrSub:   "idea",
			wantLLMCalls: true,
		},
		{
			name:         "llm returns empty prompt",
			idea:         ideaText,
			llmResult:    func() (string, error) { return `{"prompt":"","negative_prompt":"only-neg"}`, nil },
			wantLLMCalls: true,
		},
		{
			name:       "idea too long",
			idea:       strings.Repeat("я", 4001),
			wantErrSub: "idea too long",
		},
		{
			name:         "idea negative too long",
			idea:         ideaText,
			ideaNegative: strings.Repeat("я", 4001),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db, rec := ideaSetup(t, tt.llmResult)
			makeTestPreset(t, db, nil)

			_, err := svc.GenerateImage(GenerateImageParams{
				PresetID:     1,
				Idea:         tt.idea,
				IdeaNegative: tt.ideaNegative,
			})
			require.Error(t, err)
			if tt.wantErrSub != "" {
				assert.Contains(t, strings.ToLower(err.Error()), tt.wantErrSub)
			}
			assert.Equal(t, 0, rec.sdCallCount(), "SD must not be called")
			if !tt.wantLLMCalls {
				assert.Empty(t, rec.llmCalls(), "LLM must not be called")
			}
		})
	}
}

func TestGenerateImage_IdeaLengthCountsRunes(t *testing.T) {
	t.Parallel()
	svc, db, rec := ideaSetup(t, nil)
	makeTestPreset(t, db, nil)

	idea := strings.Repeat("я", 4000)
	_, err := svc.GenerateImage(GenerateImageParams{
		PresetID:     1,
		Idea:         idea,
		IdeaNegative: strings.Repeat("н", 4000),
	})
	require.NoError(t, err)
	assert.Len(t, rec.llmCalls(), 1)
	assert.Len(t, rec.txt2imgCalls(), 1)
}

func TestGenerateImage_IdeaKidsFilter(t *testing.T) {
	t.Parallel()
	svc, db, rec := ideaSetup(t, nil)
	makeTestPreset(t, db, nil)
	require.NoError(t, svc.kids.SetKidsMode(true, "1234"))

	_, err := svc.GenerateImage(GenerateImageParams{PresetID: 1, Idea: "zombie attack"})
	require.Error(t, err)
	assert.Equal(t, 0, rec.sdCallCount())
}

func TestGenerateCompoundImage_IdeaUsesFirstStepStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params func(t *testing.T, db *preset.DB, first, second *preset.Preset) GenerateCompoundImageParams
	}{
		{
			name: "custom steps",
			params: func(t *testing.T, db *preset.DB, first, second *preset.Preset) GenerateCompoundImageParams {
				return GenerateCompoundImageParams{
					Steps: []CustomStep{
						{PresetID: first.ID},
						{PresetID: second.ID, DenoisingStrength: 0.6},
					},
					Idea:         ideaText,
					IdeaNegative: ideaNegText,
				}
			},
		},
		{
			name: "compound preset",
			params: func(t *testing.T, db *preset.DB, first, second *preset.Preset) GenerateCompoundImageParams {
				cp := &preset.CompoundPreset{
					Name:        "idea-workflow",
					Description: "two steps",
					Steps: []preset.CompoundPresetStep{
						{PresetID: first.ID},
						{PresetID: second.ID, DenoisingStrength: 0.6},
					},
				}
				require.NoError(t, db.CreateCompoundPreset(cp))
				return GenerateCompoundImageParams{
					CompoundPresetID: cp.ID,
					Idea:             ideaText,
					IdeaNegative:     ideaNegText,
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db, rec := ideaSetup(t, nil)
			second := ideaPreset(t, db, "second-style", "second-style-tags")
			first := ideaPreset(t, db, "first-style", "first-style-tags")

			_, err := svc.GenerateCompoundImage(tt.params(t, db, first, second))
			require.NoError(t, err)

			withIdea := rec.callsWith(ideaText)
			require.Len(t, withIdea, 1, "exactly one LLM call must carry the idea")
			assert.Contains(t, withIdea[0], ideaStyleLabel+"first-style-tags")
			assert.NotContains(t, withIdea[0], "second-style-tags")
			assert.Contains(t, withIdea[0], ideaNegText)

			calls := rec.txt2imgCalls()
			require.Len(t, calls, 1)
			assert.Contains(t, calls[0].Prompt, ideaLLMPrompt)
			assert.NotContains(t, calls[0].Prompt, ideaText, "raw idea must not reach SD")
		})
	}
}

func TestGenerateCompoundImage_IdeaEmptyNoLLM(t *testing.T) {
	t.Parallel()
	svc, db, rec := ideaSetup(t, nil)
	first := ideaPreset(t, db, "first-style", "first-style-tags")

	_, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{
		Steps: []CustomStep{{PresetID: first.ID}},
		Idea:  "   ",
	})
	require.NoError(t, err)
	assert.Empty(t, rec.llmCalls())
	require.Len(t, rec.txt2imgCalls(), 1)
}

func TestGenerateCompoundImage_IdeaFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		idea         string
		ideaNegative string
		llmResult    func() (string, error)
		wantErrSub   string
		wantLLMCalls bool
	}{
		{
			name:         "llm error",
			idea:         ideaText,
			llmResult:    func() (string, error) { return "", fmt.Errorf("LLM connection refused") },
			wantErrSub:   "idea",
			wantLLMCalls: true,
		},
		{
			name:         "llm returns empty prompt",
			idea:         ideaText,
			llmResult:    func() (string, error) { return `{"prompt":"","negative_prompt":"only-neg"}`, nil },
			wantLLMCalls: true,
		},
		{
			name:       "idea too long",
			idea:       strings.Repeat("я", 4001),
			wantErrSub: "idea too long",
		},
		{
			name:         "idea negative too long",
			idea:         ideaText,
			ideaNegative: strings.Repeat("я", 4001),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, db, rec := ideaSetup(t, tt.llmResult)
			first := ideaPreset(t, db, "first-style", "first-style-tags")

			_, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{
				Steps:        []CustomStep{{PresetID: first.ID}},
				Idea:         tt.idea,
				IdeaNegative: tt.ideaNegative,
			})
			require.Error(t, err)
			if tt.wantErrSub != "" {
				assert.Contains(t, strings.ToLower(err.Error()), tt.wantErrSub)
			}
			assert.Equal(t, 0, rec.sdCallCount(), "SD must not be called")
			if !tt.wantLLMCalls {
				assert.Empty(t, rec.llmCalls(), "LLM must not be called")
			}
		})
	}
}

func TestIdea_InterruptDuringLLMSkipsGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(svc *Service, db *preset.DB) error
	}{
		{
			name: "txt2img",
			run: func(svc *Service, db *preset.DB) error {
				makeTestPreset(t, db, nil)
				_, err := svc.GenerateImage(GenerateImageParams{PresetID: 1, Idea: ideaText})
				return err
			},
		},
		{
			name: "compound custom steps",
			run: func(svc *Service, db *preset.DB) error {
				first := ideaPreset(t, db, "first", "first-style-tags")
				_, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{
					Steps: []CustomStep{{PresetID: first.ID}},
					Idea:  ideaText,
				})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var svc *Service
			svc, db, rec := ideaSetup(t, func() (string, error) {
				_ = svc.InterruptGeneration()
				return ideaLLMOK, nil
			})
			err := tt.run(svc, db)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "interrupted")
			assert.NotEmpty(t, rec.llmCalls())
			assert.Zero(t, rec.sdCallCount(), "SD must not run after cancel during the idea LLM call")
		})
	}
}
