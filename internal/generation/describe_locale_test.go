package generation

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	suffixEN = "Respond in English."
	suffixRU = "Отвечай на русском языке."
)

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

type promptCapture struct {
	mu      sync.Mutex
	prompts []string
}

func (c *promptCapture) add(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, p)
}

func (c *promptCapture) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.prompts...)
}

func capturingLLM(c *promptCapture) *mockLLM {
	return &mockLLM{
		describeFn: func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
			c.add(prompt)
			return "ok text", nil
		},
		analyzeImageFn: func(model, systemPrompt, imageBase64 string, maxTokens int) (string, error) {
			c.add(systemPrompt)
			return "ok text", nil
		},
		chatVisionFn: func(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
			c.add(userText)
			return "ok text", nil
		},
	}
}

func TestDefaultAnalyzeDescribePrompt_IsEnglish(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, strings.TrimSpace(config.DefaultAnalyzeDescribePrompt))
	assert.False(t, hasCyrillic(config.DefaultAnalyzeDescribePrompt), "default describe prompt must not contain Cyrillic")
}

func TestAnalyzeImage_LanguageSuffixByLocale(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		mode       string
		locale     *string
		custom     string
		wantSuffix string
		notSuffix  string
	}{
		{"describe default en", "describe", strPtr("en"), "", suffixEN, suffixRU},
		{"describe default ru", "describe", strPtr("ru"), "", suffixRU, suffixEN},
		{"describe default no locale", "describe", nil, "", suffixRU, suffixEN},
		{"describe default unknown locale", "describe", strPtr("de"), "", suffixRU, suffixEN},
		{"describe custom en", "describe", strPtr("en"), "my own describe prompt", suffixEN, suffixRU},
		{"describe custom ru", "describe", strPtr("ru"), "my own describe prompt", suffixRU, suffixEN},
		{"garment default en", "garment", strPtr("en"), "", suffixEN, suffixRU},
		{"garment default ru", "garment", strPtr("ru"), "", suffixRU, suffixEN},
		{"garment default no locale", "garment", nil, "", suffixRU, suffixEN},
		{"garment custom en", "garment", strPtr("en"), "my own garment prompt", suffixEN, suffixRU},
		{"garment custom ru", "garment", strPtr("ru"), "my own garment prompt", suffixRU, suffixEN},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			if tc.locale != nil {
				require.NoError(t, db.SetSetting("ui_locale", *tc.locale))
			}
			key := "analyze_describe_prompt"
			base := config.DefaultAnalyzeDescribePrompt
			if tc.mode == "garment" {
				key = "edit_garment_prompt"
				base = config.DefaultEditGarmentPrompt
			}
			if tc.custom != "" {
				require.NoError(t, db.SetSetting(key, tc.custom))
				base = tc.custom
			}

			c := &promptCapture{}
			svc := newTestService(t, db, capturingLLM(c), &mockSD{})
			_, err := svc.AnalyzeImage(makePNGBase64(t, 64, 64), tc.mode)
			require.NoError(t, err)

			prompts := c.all()
			require.Len(t, prompts, 1, "exactly one vision call expected")
			p := strings.TrimSpace(prompts[0])
			assert.True(t, strings.HasSuffix(p, tc.wantSuffix), "prompt must end with %q, got: %q", tc.wantSuffix, p)
			assert.NotContains(t, p, tc.notSuffix)
			assert.Contains(t, p, strings.TrimSpace(base), "configured prompt must be kept")
			assert.Less(t, strings.Index(p, strings.TrimSpace(base)), strings.LastIndex(p, tc.wantSuffix), "language line goes after the prompt")
		})
	}
}

func TestGenerateImage_IgnoresLeftoverPreviewMode(t *testing.T) {
	t.Parallel()

	sizeFor := func(t *testing.T, previewLeft bool, withResolution bool) (int, int, *mockSessions) {
		db := openTestDB(t)
		makeTestPreset(t, db, nil)
		if previewLeft {
			require.NoError(t, db.SetSetting("preview_mode", "true"))
			require.NoError(t, db.SetSetting("preview_width", "256"))
			require.NoError(t, db.SetSetting("preview_height", "192"))
		}
		var resID *int64
		if withResolution {
			r := &preset.Resolution{Name: "full 832x1216", Width: 832, Height: 1216}
			require.NoError(t, db.CreateResolution(r))
			require.Greater(t, r.ID, int64(0))
			resID = &r.ID
		}

		var mu sync.Mutex
		var reqs []sd.Txt2ImgRequest
		sdSvc := &mockSD{
			txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
				mu.Lock()
				reqs = append(reqs, req)
				mu.Unlock()
				return &sd.Txt2ImgResponse{
					Images:     []string{"base64imagedata"},
					Info:       json.RawMessage(`{"seed": 42}`),
					Parameters: json.RawMessage(`{}`),
				}, nil
			},
		}
		svc := newTestService(t, db, &mockLLM{}, sdSvc)
		svc.ctx = context.Background()
		sessions := svc.sessions.(*mockSessions)

		_, err := svc.GenerateImage(GenerateImageParams{PresetID: 1, ResolutionID: resID})
		require.NoError(t, err)
		mu.Lock()
		defer mu.Unlock()
		require.Len(t, reqs, 1)
		return reqs[0].Width, reqs[0].Height, sessions
	}

	t.Run("with resolution: full size of the resolution", func(t *testing.T) {
		t.Parallel()
		w, h, sessions := sizeFor(t, true, true)
		assert.Equal(t, 832, w)
		assert.Equal(t, 1216, h)
		sessions.mu.Lock()
		defer sessions.mu.Unlock()
		for _, it := range sessions.items {
			assert.False(t, it.isPreview, "session item must not be marked as preview")
		}
	})

	t.Run("without resolution: same size as when preview was never set", func(t *testing.T) {
		t.Parallel()
		wLeft, hLeft, _ := sizeFor(t, true, false)
		wClean, hClean, _ := sizeFor(t, false, false)
		assert.Equal(t, wClean, wLeft)
		assert.Equal(t, hClean, hLeft)
		assert.False(t, wLeft == 256 && hLeft == 192, "preview size must not be used")
	})
}
