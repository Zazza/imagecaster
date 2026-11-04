package generation

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

func recipeTestCatalog() []postEffect {
	return []postEffect{
		{ID: 11, Name: "Golden Glow", Family: "post_light", Purpose: "warm light haze"},
		{ID: 12, Name: "Film Fade", Family: "post_film", Purpose: "faded film look"},
		{ID: 13, Name: "Soft Skin", Family: "post_skin", Purpose: "smooth skin"},
		{ID: 14, Name: "Silver Morning", Family: "post_material", Purpose: "matte metal sheen"},
	}
}

func TestParseAndValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		wantSteps []RecipeStep
		wantErr   string
		check     func(t *testing.T, r *CompiledRecipe)
	}{
		{
			name: "case-insensitive match with clamps and unknown adjust dropped",
			text: `{"effects":[{"name":"golden GLOW","strength":0.95},{"name":"film fade","strength":0.1}],"adjust":{"temperature":25,"vignette":150,"mystery":7}}`,
			wantSteps: []RecipeStep{
				{PresetID: 11, Name: "Golden Glow", Strength: recipeStrengthMax},
				{PresetID: 12, Name: "Film Fade", Strength: recipeStrengthMin},
			},
			check: func(t *testing.T, r *CompiledRecipe) {
				assert.InDelta(t, 25, r.Adjust.Temperature, 1e-9)
				assert.InDelta(t, 100, r.Adjust.Vignette, 1e-9)
				assert.False(t, r.Adjust.IsNeutral())
			},
		},
		{
			name: "unknown effect names dropped, known kept",
			text: `{"effects":[{"name":"Vintage Vibe","strength":0.5},{"name":"Soft Skin","strength":0.5}]}`,
			wantSteps: []RecipeStep{
				{PresetID: 13, Name: "Soft Skin", Strength: 0.5},
			},
		},
		{
			name:    "empty effects and empty adjust is an error",
			text:    `{"effects":[],"adjust":{}}`,
			wantErr: "could not compile a recipe from this description",
		},
		{
			name:    "all names unknown without adjust is an error",
			text:    `{"effects":[{"name":"Nope","strength":0.5}],"adjust":{}}`,
			wantErr: "could not compile a recipe from this description",
		},
		{
			name:    "malformed JSON is an error",
			text:    `{"effects":[{"name":`,
			wantErr: "parse recipe response",
		},
		{
			name:    "empty response is an error",
			text:    ``,
			wantErr: "parse recipe response",
		},
		{
			name: "capped at three steps in order",
			text: `{"effects":[` +
				`{"name":"Golden Glow","strength":0.5},{"name":"Film Fade","strength":0.5},` +
				`{"name":"Soft Skin","strength":0.5},{"name":"Silver Morning","strength":0.5}]}`,
			wantSteps: []RecipeStep{
				{PresetID: 11, Name: "Golden Glow", Strength: 0.5},
				{PresetID: 12, Name: "Film Fade", Strength: 0.5},
				{PresetID: 13, Name: "Soft Skin", Strength: 0.5},
			},
		},
		{
			name: "duplicate effect appears once",
			text: `{"effects":[{"name":"Golden Glow","strength":0.4},{"name":"golden glow","strength":0.6}]}`,
			wantSteps: []RecipeStep{
				{PresetID: 11, Name: "Golden Glow", Strength: 0.4},
			},
		},
		{
			name:      "adjust-only recipe is valid",
			text:      `{"effects":[],"adjust":{"white_point":240,"gamma":5,"black_point":300,"sharpen":-20}}`,
			wantSteps: []RecipeStep{},
			check: func(t *testing.T, r *CompiledRecipe) {
				assert.Empty(t, r.Steps)
				assert.InDelta(t, 254, r.Adjust.BlackPoint, 1e-9)
				assert.InDelta(t, 240, r.Adjust.WhitePoint, 1e-9)
				assert.InDelta(t, 3, r.Adjust.Gamma, 1e-9)
				assert.InDelta(t, 0, r.Adjust.Sharpen, 1e-9)
			},
		},
		{
			name: "explicit neutral adjust values still compile when effects exist",
			text: `{"effects":[{"name":"Golden Glow","strength":0.5}],"adjust":{"black_point":0}}`,
			wantSteps: []RecipeStep{
				{PresetID: 11, Name: "Golden Glow", Strength: 0.5},
			},
			check: func(t *testing.T, r *CompiledRecipe) {
				assert.InDelta(t, 0, r.Adjust.BlackPoint, 1e-9)
			},
		},
		{
			name: "fenced JSON block is parsed",
			text: "```json\n{\"effects\":[{\"name\":\"Film Fade\",\"strength\":0.55}]}\n```",
			wantSteps: []RecipeStep{
				{PresetID: 12, Name: "Film Fade", Strength: 0.55},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recipe, err := parseAndValidate(tt.text, recipeTestCatalog())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, recipe)
			assert.Equal(t, tt.wantSteps, recipe.Steps)
			if tt.check != nil {
				tt.check(t, recipe)
			}
		})
	}
}

func TestParseAndValidate_ZeroStrengthClamped(t *testing.T) {
	t.Parallel()

	recipe, err := parseAndValidate(`{"effects":[{"name":"Golden Glow"}]}`, recipeTestCatalog())
	require.NoError(t, err)
	require.Len(t, recipe.Steps, 1)
	assert.InDelta(t, recipeStrengthMin, recipe.Steps[0].Strength, 1e-9)
}

func TestParseAndValidate_AdjustClamps(t *testing.T) {
	t.Parallel()

	recipe, err := parseAndValidate(`{"adjust":{"brightness":250,"contrast":-250,"saturation":99.5,"balance_r":-120,"balance_g":120,"balance_b":7}}`, nil)
	require.NoError(t, err)
	assert.InDelta(t, 100, recipe.Adjust.Brightness, 1e-9)
	assert.InDelta(t, -100, recipe.Adjust.Contrast, 1e-9)
	assert.InDelta(t, 99.5, recipe.Adjust.Saturation, 1e-9)
	assert.InDelta(t, -100, recipe.Adjust.BalanceR, 1e-9)
	assert.InDelta(t, 100, recipe.Adjust.BalanceG, 1e-9)
	assert.InDelta(t, 7, recipe.Adjust.BalanceB, 1e-9)
}

func TestEffectPurpose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{name: "first phrase up to comma", prompt: "add warm golden haze to the photo, cinematic, glowing", want: "add warm golden haze to the photo"},
		{name: "first phrase up to period", prompt: "matte metal sheen. Second sentence.", want: "matte metal sheen"},
		{name: "long phrase truncated", prompt: strings.Repeat("very long purpose text ", 20), want: strings.Repeat("very long purpose text ", 4)[:80]},
		{name: "empty", prompt: "   ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, effectPurpose(tt.prompt))
		})
	}
}

func TestBuildRecipeSystemPrompt(t *testing.T) {
	t.Parallel()

	prompt := buildRecipeSystemPrompt(recipeTestCatalog())
	assert.Contains(t, prompt, "post-processing recipe compiler")
	assert.Contains(t, prompt, "post_light: Golden Glow — warm light haze")
	assert.Contains(t, prompt, "black_point 0..254")
	assert.Contains(t, prompt, `Reply with ONLY valid JSON`)
	assert.Contains(t, prompt, "post_material: Silver Morning")

	empty := buildRecipeSystemPrompt(nil)
	assert.Contains(t, empty, "Available effects: none.")
	assert.Contains(t, empty, "black_point 0..254")
}

func seedPostEffects(t *testing.T, db *preset.DB) *preset.Preset {
	t.Helper()
	pt := &preset.PresetType{Name: "post_light"}
	require.NoError(t, db.CreatePresetType(pt))
	return makeTestPreset(t, db, &preset.Preset{
		Name:       "Golden Glow",
		PresetType: "post_light",
		Prompt:     "add warm golden haze to the photo, cinematic, glowing",
		Sampler:    "Euler a",
		Steps:      20,
		CfgScale:   7.0,
		TypeID:     &pt.ID,
	})
}

func TestCompilePostStyle_Success(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	effect := seedPostEffects(t, db)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		assert.Equal(t, "test-model", model)
		assert.InDelta(t, recipeTemperature, temperature, 1e-9)
		assert.Contains(t, systemPrompt, "Golden Glow — add warm golden haze to the photo")
		assert.Contains(t, userMessage, "warm sunset glow")
		return `{"effects":[{"name":"golden glow","strength":0.9}],"adjust":{"temperature":-15}}`, nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	recipe, err := svc.CompilePostStyle("warm sunset glow")
	require.NoError(t, err)
	require.Len(t, recipe.Steps, 1)
	assert.Equal(t, effect.ID, recipe.Steps[0].PresetID)
	assert.Equal(t, "Golden Glow", recipe.Steps[0].Name)
	assert.InDelta(t, recipeStrengthMax, recipe.Steps[0].Strength, 1e-9)
	assert.InDelta(t, -15, recipe.Adjust.Temperature, 1e-9)
	assert.Equal(t, "warm sunset glow", recipe.SourceText)
}

func TestCompilePostStyle_RetriesOnceOnGarbage(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	effect := seedPostEffects(t, db)
	var calls atomic.Int32
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		if calls.Add(1) == 1 {
			return "sorry, I cannot answer that", nil
		}
		return `{"effects":[{"name":"Golden Glow","strength":0.5}]}`, nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	recipe, err := svc.CompilePostStyle("golden look")
	require.NoError(t, err)
	require.Len(t, recipe.Steps, 1)
	assert.Equal(t, effect.ID, recipe.Steps[0].PresetID)
	assert.Equal(t, int32(2), calls.Load())
}

func TestCompilePostStyle_FailsAfterRetry(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	seedPostEffects(t, db)
	var calls atomic.Int32
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		calls.Add(1)
		return "not json at all", nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	_, err := svc.CompilePostStyle("golden look")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse recipe response")
	assert.Equal(t, int32(2), calls.Load())
}

func TestCompilePostStyle_EmptyAndUncompilable(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	svc := newTestService(t, db, &mockLLM{}, &mockSD{})

	_, err := svc.CompilePostStyle("   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "look description is required")

	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return `{"effects":[],"adjust":{}}`, nil
	}
	svc = newTestService(t, db, llmSvc, &mockSD{})
	_, err = svc.CompilePostStyle("something exotic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not compile a recipe from this description")
}

func TestDescribeForStyle(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	svc := newTestService(t, db, &mockLLM{}, &mockSD{})

	_, err := svc.DescribeForStyle("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image is required")

	_, err = svc.DescribeForStyle(strings.Repeat("x", MaxImageBase64Len+1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")

	llmSvc := &mockLLM{}
	llmSvc.describeFn = func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
		assert.Equal(t, "test-vision", model)
		assert.Contains(t, prompt, "photo style assistant")
		assert.Equal(t, "fake-image", imageBase64)
		return "  A woman in a red coat at dusk.  ", nil
	}
	svc = newTestService(t, db, llmSvc, &mockSD{})

	text, err := svc.DescribeForStyle("fake-image")
	require.NoError(t, err)
	assert.Equal(t, "A woman in a red coat at dusk.", text)
}

func TestValidateLookParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		imageBase64 string
		text        string
		wantText    string
		wantErr     string
	}{
		{name: "image required", imageBase64: "", text: "warm glow", wantErr: "image is required"},
		{name: "image too large", imageBase64: strings.Repeat("x", MaxImageBase64Len+1), text: "warm glow", wantErr: "image too large"},
		{name: "text required", imageBase64: "fake-image", text: "   ", wantErr: "look text is required"},
		{name: "text trimmed", imageBase64: "fake-image", text: "  warm glow  ", wantText: "warm glow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateLookParams(tt.imageBase64, tt.text)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantText, got)
		})
	}
}

func TestClampLookStrength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "above max clamped", in: 0.9, want: 0.65},
		{name: "below min clamped", in: 0.1, want: 0.35},
		{name: "zero clamped to min", in: 0, want: 0.35},
		{name: "nan clamped to min", in: math.NaN(), want: 0.35},
		{name: "in range kept", in: 0.5, want: 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, clampLookStrength(tt.in), 1e-9)
		})
	}
}

func TestAlignLookSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{name: "already aligned", w: 512, h: 768, wantW: 512, wantH: 768},
		{name: "rounds down to 16", w: 513, h: 511, wantW: 512, wantH: 496},
		{name: "600x800 to multiples of 16", w: 600, h: 800, wantW: 592, wantH: 800},
		{name: "caps at 2048 proportionally", w: 4096, h: 2048, wantW: 2048, wantH: 1024},
		{name: "portrait capped", w: 1360, h: 4096, wantW: 672, wantH: 2048},
		{name: "floor 64", w: 8, h: 8, wantW: 64, wantH: 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := alignLookSize(tt.w, tt.h)
			assert.Equal(t, tt.wantW, gotW)
			assert.Equal(t, tt.wantH, gotH)
		})
	}
}

func TestBuildLookPrompt_Success(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		assert.Equal(t, "test-model", model)
		assert.InDelta(t, kleinLookTemperature, temperature, 1e-9)
		assert.Contains(t, systemPrompt, "restyle model")
		assert.Contains(t, systemPrompt, "60 words")
		assert.Contains(t, userMessage, "LOOK DESCRIPTION:")
		assert.Contains(t, userMessage, "moody film look")
		return "  Moody film look, soft grain, warm tungsten light.  ", nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	prompt, err := svc.buildLookPrompt("moody film look")
	require.NoError(t, err)
	assert.Equal(t, "Moody film look, soft grain, warm tungsten light.", prompt)
}

func TestBuildLookPrompt_RetriesOnceOnEmpty(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var calls atomic.Int32
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		if calls.Add(1) == 1 {
			return "   ", nil
		}
		return "cold blue neon dusk", nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	prompt, err := svc.buildLookPrompt("neon night")
	require.NoError(t, err)
	assert.Equal(t, "cold blue neon dusk", prompt)
	assert.Equal(t, int32(2), calls.Load())
}

func TestBuildLookPrompt_FailsAfterEmptyRetry(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var calls atomic.Int32
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		calls.Add(1)
		return "", nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	_, err := svc.buildLookPrompt("neon night")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty look prompt response")
	assert.Equal(t, int32(2), calls.Load())
}

func TestBuildLookPrompt_PropagatesChatError(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "", fmt.Errorf("llm down")
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	_, err := svc.buildLookPrompt("neon night")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm down")
}

func TestBuildLookPrompt_ClampsRunes(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return strings.Repeat("с", 600), nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	prompt, err := svc.buildLookPrompt("long look")
	require.NoError(t, err)
	assert.Len(t, []rune(prompt), kleinLookMaxPromptRunes)
}

func TestApplyLookDirect_HappyPath(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string
	var capturedReq sd.Img2ImgRequest

	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "cold blue neon dusk, wet asphalt reflections", nil
	}
	sdSvc := recordingSD(&ops, &mu, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		capturedReq = req
		return &sd.Txt2ImgResponse{Images: []string{"look-b64"}, Info: []byte(`{"look":true}`)}, nil
	})
	sessions := &mockSessions{}
	svc := newTestService(t, db, llmSvc, sdSvc)
	svc.sessions = sessions

	seed := int64(42)
	result, err := svc.ApplyLookDirect(tinyPNGBase64(t), "  neon night look  ", 0.5, &seed)
	require.NoError(t, err)
	require.NotNil(t, result)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, []string{
		"option:" + kleinReferenceOption + "=true",
		"option:" + kleinReferenceOption + "=false",
	}, ops)

	assert.Equal(t, "cold blue neon dusk, wet asphalt reflections", capturedReq.Prompt)
	assert.Equal(t, kleinLookSteps, capturedReq.Steps)
	assert.InDelta(t, kleinLookCfgScale, capturedReq.CfgScale, 1e-9)
	assert.Equal(t, kleinLookSampler, capturedReq.SamplerName)
	assert.Equal(t, kleinLookScheduler, capturedReq.Scheduler)
	require.NotNil(t, capturedReq.DenoisingStrength)
	assert.InDelta(t, 0.5, *capturedReq.DenoisingStrength, 1e-9)
	assert.Equal(t, 512, capturedReq.Width)
	assert.Equal(t, 512, capturedReq.Height)
	require.NotNil(t, capturedReq.Seed)
	assert.Equal(t, int64(42), *capturedReq.Seed)
	assert.True(t, capturedReq.DoNotSaveImages)

	assert.Equal(t, "look-b64", result.Image)
	assert.Equal(t, "cold blue neon dusk, wet asphalt reflections", result.EffectivePrompt)
	assert.False(t, result.PreserveIdentityResetFailed)

	require.Len(t, sessions.items, 1)
	item := sessions.items[0]
	assert.Equal(t, "look", item.source)
	assert.False(t, item.isPreview)
	require.NotNil(t, item.meta)
	assert.Equal(t, "Look: neon night look", item.meta.Label)
	assert.Equal(t, kleinLookModel, item.meta.Model)
}

func TestApplyLookDirect_EnableReferenceFails(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string
	sdSvc := &mockSD{}
	sdSvc.setOption = func(key string, value any) error {
		mu.Lock()
		defer mu.Unlock()
		ops = append(ops, fmt.Sprintf("option:%s=%v", key, value))
		if value == true {
			return fmt.Errorf("options boom")
		}
		return nil
	}
	imgCalls := 0
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		imgCalls++
		return &sd.Txt2ImgResponse{Images: []string{"must-not-happen"}}, nil
	}
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "some look", nil
	}
	svc := newTestService(t, db, llmSvc, sdSvc)

	_, err := svc.ApplyLookDirect(tinyPNGBase64(t), "neon night look", 0.5, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enable look reference")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{
		"option:" + kleinReferenceOption + "=true",
		"option:" + kleinReferenceOption + "=false",
	}, ops)
	assert.Equal(t, 0, imgCalls)
}

func TestApplyLookDirect_StrengthClamped(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string
	var capturedReq sd.Img2ImgRequest
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "some look", nil
	}
	sdSvc := recordingSD(&ops, &mu, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		capturedReq = req
		return &sd.Txt2ImgResponse{Images: []string{"look-b64"}}, nil
	})
	svc := newTestService(t, db, llmSvc, sdSvc)

	_, err := svc.ApplyLookDirect(tinyPNGBase64(t), "neon night look", 0.9, nil)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, capturedReq.DenoisingStrength)
	assert.InDelta(t, recipeStrengthMax, *capturedReq.DenoisingStrength, 1e-9)
}

func TestApplyLookDirect_KidsModeBlocks(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "some look", nil
	}
	var ops []string
	sdSvc := recordingSD(&ops, &sync.Mutex{}, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		return &sd.Txt2ImgResponse{Images: []string{"must-not-happen"}}, nil
	})
	svc := newTestService(t, db, llmSvc, sdSvc)
	require.NoError(t, svc.kids.SetKidsMode(true, "1234"))

	_, err := svc.ApplyLookDirect(tinyPNGBase64(t), "zombie neon look", 0.5, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Kids Mode")
}

func TestApplyLookDirect_LLMFailureFallsBackToOriginalText(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string
	var capturedReq sd.Img2ImgRequest
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "", fmt.Errorf("llm down")
	}
	sdSvc := recordingSD(&ops, &mu, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		capturedReq = req
		return &sd.Txt2ImgResponse{Images: []string{"look-b64"}}, nil
	})
	svc := newTestService(t, db, llmSvc, sdSvc)

	result, err := svc.ApplyLookDirect(tinyPNGBase64(t), "хмурый осенний свет", 0.5, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "хмурый осенний свет", capturedReq.Prompt)
	assert.Equal(t, "хмурый осенний свет", result.EffectivePrompt)
}
