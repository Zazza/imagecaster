package generation

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
)

type visionCounter struct {
	mu    sync.Mutex
	calls int
	reply string
}

func (v *visionCounter) hit() (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls++
	return v.reply, nil
}

func (v *visionCounter) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.calls
}

func (v *visionCounter) set(reply string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.reply = reply
}

func countingLLM(v *visionCounter) *mockLLM {
	return &mockLLM{
		analyzeImageFn: func(model, systemPrompt, imageBase64 string, maxTokens int) (string, error) {
			return v.hit()
		},
		describeFn: func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
			return v.hit()
		},
		chatVisionFn: func(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
			return v.hit()
		},
		chatMsgFn: func(model string, messages []llm.Message, temperature float64, maxTokens int) (string, error) {
			return v.hit()
		},
		chatFn: func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
			return v.hit()
		},
		genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
			return v.hit()
		},
	}
}

func newTestServiceURL(t *testing.T, db *preset.DB, llmSvc *mockLLM, url string) *Service {
	t.Helper()
	return New(
		db,
		llmSvc,
		&mockSD{},
		&config.Config{
			LLMUrl:        url,
			SDPromptModel: "test-model",
			VisionModel:   "test-vision",
		},
		t.TempDir(),
		&mockEmitter{},
		kids.NewManager(db),
		&mockSessions{},
		logger.New(nil),
	)
}

func TestDescribePhoto_InvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		image string
		mode  string
	}{
		{"unknown mode", "img", "garment"},
		{"empty mode", "img", ""},
		{"uppercase mode", "img", "Quick"},
		{"empty image quick", "", "quick"},
		{"empty image describe", "", "describe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			v := &visionCounter{reply: "something"}
			svc := newTestService(t, db, countingLLM(v), &mockSD{})
			img := tt.image
			if img == "img" {
				img = makePNGBase64(t, 16, 16)
			}
			_, err := svc.DescribePhoto(img, tt.mode, false)
			require.Error(t, err)
			assert.Equal(t, 0, v.count())
		})
	}
}

func TestDescribePhoto_FirstCallAsksLLM(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"quick", "describe"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			v := &visionCounter{reply: "red dress, smiling woman"}
			svc := newTestService(t, db, countingLLM(v), &mockSD{})
			got, err := svc.DescribePhoto(makePNGBase64(t, 20, 20), mode, false)
			require.NoError(t, err)
			assert.Equal(t, "red dress, smiling woman", got)
			assert.Equal(t, 1, v.count())
		})
	}
}

func TestDescribePhoto_SecondCallFromStore(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"quick", "describe"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			v := &visionCounter{reply: "first answer"}
			svc := newTestService(t, db, countingLLM(v), &mockSD{})
			img := makePNGBase64(t, 20, 20)

			first, err := svc.DescribePhoto(img, mode, false)
			require.NoError(t, err)
			v.set("second answer")
			second, err := svc.DescribePhoto(img, mode, false)
			require.NoError(t, err)

			assert.Equal(t, first, second)
			assert.Equal(t, "first answer", second)
			assert.Equal(t, 1, v.count())
		})
	}
}

func TestDescribePhoto_StoredSurvivesNewService(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	v := &visionCounter{reply: "first answer"}
	img := makePNGBase64(t, 20, 20)
	_, err := newTestService(t, db, countingLLM(v), &mockSD{}).DescribePhoto(img, "describe", false)
	require.NoError(t, err)

	v.set("other")
	got, err := newTestService(t, db, countingLLM(v), &mockSD{}).DescribePhoto(img, "describe", false)
	require.NoError(t, err)
	assert.Equal(t, "first answer", got)
	assert.Equal(t, 1, v.count())
}

func TestDescribePhoto_FreshReplacesStored(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	v := &visionCounter{reply: "old text"}
	svc := newTestService(t, db, countingLLM(v), &mockSD{})
	img := makePNGBase64(t, 20, 20)

	_, err := svc.DescribePhoto(img, "describe", false)
	require.NoError(t, err)

	v.set("new text")
	got, err := svc.DescribePhoto(img, "describe", true)
	require.NoError(t, err)
	assert.Equal(t, "new text", got)
	assert.Equal(t, 2, v.count())

	v.set("third text")
	got, err = svc.DescribePhoto(img, "describe", false)
	require.NoError(t, err)
	assert.Equal(t, "new text", got)
	assert.Equal(t, 2, v.count())
}

func TestDescribePhoto_DataURLPrefixSameImage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		first, second func(string) string
	}{
		{"plain then prefixed", func(s string) string { return s }, func(s string) string { return "data:image/png;base64," + s }},
		{"prefixed then plain", func(s string) string { return "data:image/png;base64," + s }, func(s string) string { return s }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			v := &visionCounter{reply: "stored text"}
			svc := newTestService(t, db, countingLLM(v), &mockSD{})
			img := makePNGBase64(t, 20, 20)

			_, err := svc.DescribePhoto(tt.first(img), "quick", false)
			require.NoError(t, err)
			v.set("other text")
			got, err := svc.DescribePhoto(tt.second(img), "quick", false)
			require.NoError(t, err)
			assert.Equal(t, "stored text", got)
			assert.Equal(t, 1, v.count())
		})
	}
}

func TestDescribePhoto_DifferentKeyAsksLLM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		secondMode   string
		otherImage   bool
		secondLocale string
	}{
		{"different mode", "describe", false, ""},
		{"different image", "quick", true, ""},
		{"locale ru to en", "quick", false, "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			require.NoError(t, db.SetSetting("ui_locale", "ru"))
			v := &visionCounter{reply: "first"}
			svc := newTestService(t, db, countingLLM(v), &mockSD{})
			img := makePNGBase64(t, 20, 20)

			_, err := svc.DescribePhoto(img, "quick", false)
			require.NoError(t, err)

			if tt.secondLocale != "" {
				require.NoError(t, db.SetSetting("ui_locale", tt.secondLocale))
			}
			img2 := img
			if tt.otherImage {
				img2 = makePNGBase64(t, 24, 24)
			}
			v.set("second")
			got, err := svc.DescribePhoto(img2, tt.secondMode, false)
			require.NoError(t, err)
			assert.Equal(t, "second", got)
			assert.Equal(t, 2, v.count())
		})
	}
}

func TestDescribePhoto_LocaleKeepsSeparateRecords(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("ui_locale", "ru"))
	v := &visionCounter{reply: "ru text"}
	svc := newTestService(t, db, countingLLM(v), &mockSD{})
	img := makePNGBase64(t, 20, 20)

	_, err := svc.DescribePhoto(img, "describe", false)
	require.NoError(t, err)
	require.NoError(t, db.SetSetting("ui_locale", "en"))
	v.set("en text")
	_, err = svc.DescribePhoto(img, "describe", false)
	require.NoError(t, err)

	v.set("never")
	require.NoError(t, db.SetSetting("ui_locale", "ru"))
	got, err := svc.DescribePhoto(img, "describe", false)
	require.NoError(t, err)
	assert.Equal(t, "ru text", got)
	require.NoError(t, db.SetSetting("ui_locale", "en"))
	got, err = svc.DescribePhoto(img, "describe", false)
	require.NoError(t, err)
	assert.Equal(t, "en text", got)
	assert.Equal(t, 2, v.count())
}

func TestVision_NonLocalLLMRefused(t *testing.T) {
	t.Parallel()
	calls := []struct {
		name string
		run  func(svc *Service, img string) error
	}{
		{"DescribePhoto quick", func(s *Service, img string) error { _, err := s.DescribePhoto(img, "quick", false); return err }},
		{"DescribePhoto describe fresh", func(s *Service, img string) error { _, err := s.DescribePhoto(img, "describe", true); return err }},
		{"AnalyzeImage quick", func(s *Service, img string) error { _, err := s.AnalyzeImage(img, "quick"); return err }},
		{"AnalyzeImage describe", func(s *Service, img string) error { _, err := s.AnalyzeImage(img, "describe"); return err }},
		{"AnalyzeImage garment", func(s *Service, img string) error { _, err := s.AnalyzeImage(img, "garment"); return err }},
		{"DescribeImage", func(s *Service, img string) error { _, err := s.DescribeImage(img, "describe this"); return err }},
		{"DescribeForStyle", func(s *Service, img string) error { _, err := s.DescribeForStyle(img); return err }},
	}
	for _, url := range []string{"https://api.openai.com/v1", "http://8.8.8.8:11434"} {
		for _, c := range calls {
			t.Run(url+" "+c.name, func(t *testing.T) {
				t.Parallel()
				db := openTestDB(t)
				v := &visionCounter{reply: "a woman, red dress"}
				svc := newTestServiceURL(t, db, countingLLM(v), url)
				err := c.run(svc, makePNGBase64(t, 20, 20))
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrVisionNotLocal), "got %v", err)
				assert.Equal(t, 0, v.count())
			})
		}
	}
}

func TestVision_NonLocalDoesNotLeakStored(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	v := &visionCounter{reply: "stored"}
	img := makePNGBase64(t, 20, 20)
	_, err := newTestService(t, db, countingLLM(v), &mockSD{}).DescribePhoto(img, "quick", false)
	require.NoError(t, err)

	_, err = newTestServiceURL(t, db, countingLLM(v), "https://api.openai.com/v1").DescribePhoto(img, "quick", false)
	assert.True(t, errors.Is(err, ErrVisionNotLocal), "got %v", err)
	assert.Equal(t, 1, v.count())
}

func TestVision_LANLLMWorks(t *testing.T) {
	t.Parallel()
	for _, url := range []string{"http://127.0.0.1:11434", "http://192.168.1.184:8080"} {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			v := &visionCounter{reply: "a woman, red dress"}
			svc := newTestServiceURL(t, db, countingLLM(v), url)
			img := makePNGBase64(t, 20, 20)

			got, err := svc.DescribePhoto(img, "quick", false)
			require.NoError(t, err)
			assert.Equal(t, "a woman, red dress", got)

			_, err = svc.AnalyzeImage(img, "quick")
			assert.False(t, errors.Is(err, ErrVisionNotLocal), "AnalyzeImage quick: %v", err)
			_, err = svc.AnalyzeImage(img, "describe")
			assert.False(t, errors.Is(err, ErrVisionNotLocal), "AnalyzeImage describe: %v", err)
			_, err = svc.AnalyzeImage(img, "garment")
			assert.False(t, errors.Is(err, ErrVisionNotLocal), "AnalyzeImage garment: %v", err)
			_, err = svc.DescribeImage(img, "describe this")
			assert.False(t, errors.Is(err, ErrVisionNotLocal), "DescribeImage: %v", err)
			_, err = svc.DescribeForStyle(img)
			assert.False(t, errors.Is(err, ErrVisionNotLocal), "DescribeForStyle: %v", err)
			assert.GreaterOrEqual(t, v.count(), 6)
		})
	}
}

func TestDescribePhoto_KidsFiltersStored(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	raw := "masterpiece, 1girl, nude, blue eyes, long hair"
	v := &visionCounter{reply: raw}
	svc := newTestService(t, db, countingLLM(v), &mockSD{})
	img := makePNGBase64(t, 20, 20)

	got, err := svc.DescribePhoto(img, "quick", false)
	require.NoError(t, err)
	assert.Equal(t, raw, got)

	require.NoError(t, db.SetSetting("kids_mode", "true"))
	want := kids.NewManager(db).FilterOutput(raw)
	require.NotEqual(t, raw, want)

	got, err = svc.DescribePhoto(img, "quick", false)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.NotContains(t, got, "nude")
	assert.Equal(t, 1, v.count())
}

func TestDescribePhoto_KidsResultNotStored(t *testing.T) {
	t.Parallel()
	for _, fresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "fresh"}[fresh], func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			require.NoError(t, db.SetSetting("kids_mode", "true"))
			v := &visionCounter{reply: "kids answer"}
			svc := newTestService(t, db, countingLLM(v), &mockSD{})
			img := makePNGBase64(t, 20, 20)

			_, err := svc.DescribePhoto(img, "quick", fresh)
			require.NoError(t, err)
			require.Equal(t, 1, v.count())

			require.NoError(t, db.SetSetting("kids_mode", "false"))
			v.set("adult answer")
			got, err := svc.DescribePhoto(img, "quick", false)
			require.NoError(t, err)
			assert.Equal(t, "adult answer", got)
			assert.Equal(t, 2, v.count())
		})
	}
}

func TestDescribePhoto_KidsFreshDoesNotOverwriteStored(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	v := &visionCounter{reply: "adult answer"}
	svc := newTestService(t, db, countingLLM(v), &mockSD{})
	img := makePNGBase64(t, 20, 20)

	_, err := svc.DescribePhoto(img, "quick", false)
	require.NoError(t, err)

	require.NoError(t, db.SetSetting("kids_mode", "true"))
	v.set("kids answer")
	_, err = svc.DescribePhoto(img, "quick", true)
	require.NoError(t, err)

	require.NoError(t, db.SetSetting("kids_mode", "false"))
	got, err := svc.DescribePhoto(img, "quick", false)
	require.NoError(t, err)
	assert.Equal(t, "adult answer", got)
	assert.Equal(t, 2, v.count())
}
