package generation

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	textRuleMarker   = "TEXT IN THE IMAGE:"
	textRuleExact    = "exactly as the user wrote it"
	textRuleNoTransl = "do NOT add a translation"
)

type sysPromptRecorder struct {
	mu      sync.Mutex
	prompts []string
}

func (r *sysPromptRecorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompts = append(r.prompts, s)
}

func (r *sysPromptRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.prompts...)
}

func recordingLLM(rec *sysPromptRecorder) *mockLLM {
	const answer = `{"prompt":"llm-scene, sign","negative_prompt":"lowres"}`
	return &mockLLM{
		genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
			rec.add(systemPrompt)
			return answer, nil
		},
		chatFn: func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
			rec.add(systemPrompt)
			return answer, nil
		},
		chatMsgFn: func(model string, messages []llm.Message, temperature float64, maxTokens int) (string, error) {
			for _, m := range messages {
				if m.Role == "system" {
					if s, ok := m.Content.(string); ok {
						rec.add(s)
					}
				}
			}
			return answer, nil
		},
	}
}

func TestTextInImage_RuleInSystemPrompt(t *testing.T) {
	t.Parallel()

	const (
		customTags  = "CUSTOM-TAGS-INSTRUCTION: write comma separated booru tags only."
		customProse = "CUSTOM-PROSE-INSTRUCTION: write one flowing English paragraph."
	)

	tests := []struct {
		name       string
		model      string
		settingKey string
		custom     string
	}{
		{name: "tags model default instruction", model: "juggernautXL"},
		{name: "prose model default instruction", model: "flux1-dev-fp8"},
		{name: "tags model custom instruction", model: "juggernautXL", settingKey: "sd_prompt_instruction", custom: customTags},
		{name: "prose model custom instruction", model: "flux1-dev-fp8", settingKey: "sd_prompt_instruction_prose", custom: customProse},
	}

	paths := []struct {
		name string
		run  func(t *testing.T, svc *Service, p *preset.Preset)
	}{
		{
			name: "from description",
			run: func(t *testing.T, svc *Service, p *preset.Preset) {
				_, err := svc.GenerateSDPrompt(GenerateSDPromptParams{
					PresetID:    p.ID,
					Description: `a shop with a sign that says "Пекарня"`,
				})
				require.NoError(t, err)
			},
		},
		{
			name: "from image tags",
			run: func(t *testing.T, svc *Service, p *preset.Preset) {
				svc.ctx = context.Background()
				_, err := svc.GenerateFromImage(GenerateFromImageParams{
					ImageBase64:       makePNGBase64(t, 512, 512),
					GenMode:           "preset",
					Mode:              "img2img",
					PresetID:          p.ID,
					DenoisingStrength: 0.6,
					Tags:              `a shop with a sign that says "Пекарня"`,
				})
				require.NoError(t, err)
			},
		},
	}

	for _, path := range paths {
		for _, tt := range tests {
			t.Run(path.name+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				db := openTestDB(t)
				if tt.settingKey != "" {
					require.NoError(t, db.SetSetting(tt.settingKey, tt.custom))
				}
				p := makeTestPreset(t, db, &preset.Preset{
					Name:         "text-rule-" + tt.model,
					PresetType:   "realistic",
					Prompt:       "photo",
					Sampler:      "Euler",
					ScheduleType: "Simple",
					Steps:        4,
					CfgScale:     1.0,
					ModelName:    tt.model,
				})

				rec := &sysPromptRecorder{}
				sdSvc := &mockSD{
					img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
						return &sd.Txt2ImgResponse{Images: []string{"img"}}, nil
					},
					txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
						return &sd.Txt2ImgResponse{Images: []string{"img"}}, nil
					},
				}
				svc := newTestService(t, db, recordingLLM(rec), sdSvc)

				path.run(t, svc, p)

				prompts := rec.all()
				require.Len(t, prompts, 1, "exactly one LLM call that turns the user text into an image prompt")
				sys := prompts[0]

				assert.Equal(t, 1, strings.Count(sys, textRuleMarker), "text rule must appear exactly once")
				assert.Contains(t, sys, textRuleExact)
				assert.Contains(t, sys, textRuleNoTransl)
				if tt.custom != "" {
					assert.Contains(t, sys, tt.custom, "user's saved instruction must be kept")
				}
			})
		}
	}
}

func fixedAnswerLLM(answer string) *mockLLM {
	return &mockLLM{
		genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
			return answer, nil
		},
		chatFn: func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
			return answer, nil
		},
		chatMsgFn: func(model string, messages []llm.Message, temperature float64, maxTokens int) (string, error) {
			return answer, nil
		},
	}
}

func textInImageEnv(t *testing.T, model, answer string) (*Service, *preset.Preset, func() []string) {
	t.Helper()
	db := openTestDB(t)
	p := makeTestPreset(t, db, &preset.Preset{
		Name:         "text-keep-" + model,
		PresetType:   "realistic",
		Prompt:       "photo",
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        4,
		CfgScale:     1.0,
		ModelName:    model,
	})
	var mu sync.Mutex
	var sent []string
	sdSvc := &mockSD{
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			mu.Lock()
			sent = append(sent, req.Prompt)
			mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{"img"}}, nil
		},
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			mu.Lock()
			sent = append(sent, req.Prompt)
			mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{"img"}}, nil
		},
	}
	svc := newTestService(t, db, fixedAnswerLLM(answer), sdSvc)
	svc.ctx = context.Background()
	return svc, p, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), sent...)
	}
}

const textInImageUserText = `a shop with a sign that says "Пекарня"`

func runTextInImageFromImage(t *testing.T, svc *Service, p *preset.Preset) error {
	t.Helper()
	_, err := svc.GenerateFromImage(GenerateFromImageParams{
		ImageBase64:       makePNGBase64(t, 512, 512),
		GenMode:           "preset",
		Mode:              "img2img",
		PresetID:          p.ID,
		DenoisingStrength: 0.6,
		Tags:              textInImageUserText,
	})
	return err
}

func TestTextInImage_QuotedCyrillicKeptInFinalPrompt(t *testing.T) {
	t.Parallel()

	const answer = `{"prompt":"a cozy bakery storefront, wooden sign with text \"Пекарня\", warm morning light, detailed","negative_prompt":"blurry"}`
	want := []string{"Пекарня", "cozy bakery storefront", "warm morning light"}

	for _, model := range []string{"juggernautXL", "flux1-dev-fp8"} {
		t.Run("from description/"+model, func(t *testing.T) {
			t.Parallel()
			svc, p, _ := textInImageEnv(t, model, answer)
			res, err := svc.GenerateSDPrompt(GenerateSDPromptParams{PresetID: p.ID, Description: textInImageUserText})
			require.NoError(t, err)
			require.NotNil(t, res)
			for _, w := range want {
				assert.Contains(t, res.Prompt, w)
			}
		})
		t.Run("from image tags/"+model, func(t *testing.T) {
			t.Parallel()
			svc, p, sent := textInImageEnv(t, model, answer)
			require.NoError(t, runTextInImageFromImage(t, svc, p))
			prompts := sent()
			require.NotEmpty(t, prompts, "generation must reach SD")
			for _, w := range want {
				assert.Contains(t, prompts[0], w)
			}
		})
	}
}

func TestTextInImage_UnquotedCyrillicDoesNotFail(t *testing.T) {
	t.Parallel()

	const answer = `{"prompt":"уютная пекарня, деревянная вывеска","negative_prompt":""}`

	for _, model := range []string{"juggernautXL", "flux1-dev-fp8"} {
		t.Run("from description/"+model, func(t *testing.T) {
			t.Parallel()
			svc, p, _ := textInImageEnv(t, model, answer)
			_, err := svc.GenerateSDPrompt(GenerateSDPromptParams{PresetID: p.ID, Description: textInImageUserText})
			require.NoError(t, err)
		})
		t.Run("from image tags/"+model, func(t *testing.T) {
			t.Parallel()
			svc, p, _ := textInImageEnv(t, model, answer)
			require.NoError(t, runTextInImageFromImage(t, svc, p))
		})
	}
}
