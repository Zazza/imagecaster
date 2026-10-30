package generation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-sd/internal/preset"
	"go-sd/internal/sd"
)

func TestBuildEditPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		instruction string
		styleTags   string
		loras       []preset.LoRAEntry
		want        string
	}{
		{
			name:        "no loras no style",
			instruction: "make hair red",
			want:        "<lora:Moss_Edit_LoRA:1.0>Change only make hair red" + editPromptSuffix,
		},
		{
			name:        "user loras after edit tag, before formula",
			instruction: "make hair red",
			loras: []preset.LoRAEntry{
				{Name: "Detail_Boost_Moss", Weight: 0.8},
				{Name: "Moss_DMD_8step", Weight: 1},
			},
			want: "<lora:Moss_Edit_LoRA:1.0> <lora:Detail_Boost_Moss:0.8> <lora:Moss_DMD_8step:1>Change only make hair red" + editPromptSuffix,
		},
		{
			name:        "style tags before formula",
			instruction: "make hair red",
			styleTags:   "moss_xvstyle, RAW photo, film grain",
			loras:       []preset.LoRAEntry{{Name: "Moss_DMD_8step", Weight: 1}},
			want:        "<lora:Moss_Edit_LoRA:1.0> <lora:Moss_DMD_8step:1> moss_xvstyle, RAW photo, film grain. Change only make hair red" + editPromptSuffix,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, buildEditPrompt(tt.instruction, tt.styleTags, tt.loras))
		})
	}
}

func TestSanitizeEditLoras(t *testing.T) {
	t.Parallel()

	t.Run("empty is nil", func(t *testing.T) {
		t.Parallel()
		out, err := sanitizeEditLoras(nil)
		require.NoError(t, err)
		assert.Nil(t, out)
	})

	t.Run("valid pass through", func(t *testing.T) {
		t.Parallel()
		out, err := sanitizeEditLoras([]preset.LoRAEntry{{Name: " lora-a ", Weight: 0.8}})
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, "lora-a", out[0].Name)
		assert.InDelta(t, 0.8, out[0].Weight, 1e-9)
	})

	t.Run("weight clamped to 0-2", func(t *testing.T) {
		t.Parallel()
		out, err := sanitizeEditLoras([]preset.LoRAEntry{
			{Name: "a", Weight: -1},
			{Name: "b", Weight: 5},
		})
		require.NoError(t, err)
		assert.InDelta(t, 0, out[0].Weight, 1e-9)
		assert.InDelta(t, 2, out[1].Weight, 1e-9)
	})

	t.Run("angle brackets rejected", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"<lora:evil:1>real", "a><lora:evil", "name>with>brackets"} {
			_, err := sanitizeEditLoras([]preset.LoRAEntry{{Name: name, Weight: 0.5}})
			assert.ErrorContains(t, err, "invalid lora name", name)
		}
	})

	t.Run("colon rejected", func(t *testing.T) {
		t.Parallel()
		_, err := sanitizeEditLoras([]preset.LoRAEntry{{Name: "a:b", Weight: 0.5}})
		assert.ErrorContains(t, err, "invalid lora name")
	})

	t.Run("overlong name rejected", func(t *testing.T) {
		t.Parallel()
		_, err := sanitizeEditLoras([]preset.LoRAEntry{{Name: strings.Repeat("a", 201), Weight: 0.5}})
		assert.ErrorContains(t, err, "lora name too long")
	})

	t.Run("NaN weight defaults to 0.6", func(t *testing.T) {
		t.Parallel()
		out, err := sanitizeEditLoras([]preset.LoRAEntry{{Name: "a", Weight: math.NaN()}})
		require.NoError(t, err)
		assert.InDelta(t, 0.6, out[0].Weight, 1e-9)
	})
}

func TestSanitizeStyleTags(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "moss_xvstyle, RAW photo", sanitizeStyleTags(" moss_xvstyle, RAW photo, "))
	assert.Equal(t, "clean text", sanitizeStyleTags("clean <lora:evil:1>text"))
	assert.Equal(t, "sneaky:0.8 no prefix", sanitizeStyleTags("<lora:sneaky:0.8 no prefix"))
	assert.Equal(t, strings.Repeat("a", 2000), sanitizeStyleTags(strings.Repeat("a", 3000)))
}

func TestLoadEditStyle(t *testing.T) {
	t.Parallel()

	t.Run("zero id is no style", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		st, err := svc.loadEditStyle(0)
		require.NoError(t, err)
		assert.Nil(t, st)
	})

	t.Run("missing preset", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		_, err := svc.loadEditStyle(999)
		assert.ErrorContains(t, err, "style preset not found")
	})

	t.Run("non-moss preset rejected", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		makeTestPreset(t, db, &preset.Preset{Name: "z-style", ModelName: "z-image-turbo", Prompt: "photo"})
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		_, err := svc.loadEditStyle(1)
		assert.ErrorContains(t, err, "must use a moss model")
	})

	t.Run("moss preset loads tags loras steps cfg", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		p := makeTestPreset(t, db, &preset.Preset{
			Name:      "Moss Film",
			ModelName: "moss_image_sft_bf16",
			Prompt:    "moss_xvstyle, RAW photo <lora:sneaky:1>",
			Steps:     8,
			CfgScale:  1.0,
			Loras:     `[{"name": "Moss_DMD_8step", "weight": 1.0}]`,
		})
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		st, err := svc.loadEditStyle(p.ID)
		require.NoError(t, err)
		require.NotNil(t, st)
		assert.Equal(t, p.ID, st.presetID)
		assert.Equal(t, "moss_xvstyle, RAW photo", st.tags)
		require.Len(t, st.loras, 1)
		assert.Equal(t, "Moss_DMD_8step", st.loras[0].Name)
		assert.Equal(t, 8, st.steps)
		assert.InDelta(t, 1.0, st.cfg, 1e-9)
	})

	t.Run("out of range steps and cfg fall back to defaults", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		p := makeTestPreset(t, db, &preset.Preset{
			Name:      "Moss Weird",
			ModelName: "moss_image_sft_bf16",
			Prompt:    "tags",
			Steps:     500,
			CfgScale:  30,
		})
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		st, err := svc.loadEditStyle(p.ID)
		require.NoError(t, err)
		assert.Equal(t, mossEditSteps, st.steps)
		assert.InDelta(t, mossEditCfgScale, st.cfg, 1e-9)
	})
}

func TestEditImage_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  EditImageParams
		wantErr string
	}{
		{
			name:    "invalid output mode",
			params:  EditImageParams{ImageBase64: tinyPNGBase64(t), Instruction: "make hair red", OutputMode: "ultra"},
			wantErr: "output_mode must be standard or quality",
		},
		{
			name:    "quality without resolution",
			params:  EditImageParams{ImageBase64: tinyPNGBase64(t), Instruction: "make hair red", OutputMode: "quality"},
			wantErr: "quality output mode requires resolution",
		},
		{
			name: "too many loras",
			params: func() EditImageParams {
				var loras []preset.LoRAEntry
				for i := 0; i < 9; i++ {
					loras = append(loras, preset.LoRAEntry{Name: fmt.Sprintf("lora-%d", i), Weight: 0.5})
				}
				return EditImageParams{ImageBase64: tinyPNGBase64(t), Instruction: "make hair red", Loras: loras}
			}(),
			wantErr: "too many loras",
		},
		{
			name:    "missing style preset",
			params:  EditImageParams{ImageBase64: tinyPNGBase64(t), Instruction: "make hair red", StylePresetID: 999},
			wantErr: "style preset not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			svc := newTestService(t, db, &mockLLM{}, &mockSD{})
			_, err := svc.EditImage(tt.params)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestEditImage_NonMossStylePreset(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	p := makeTestPreset(t, db, &preset.Preset{Name: "z-style", ModelName: "z-image-turbo"})
	svc := newTestService(t, db, &mockLLM{}, &mockSD{})

	_, err := svc.EditImage(EditImageParams{
		ImageBase64:   tinyPNGBase64(t),
		Instruction:   "make hair red",
		StylePresetID: p.ID,
	})
	assert.ErrorContains(t, err, "must use a moss model")
}

func TestEditImage_StyleAndUserLorasOverflow(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var styleLoras []preset.LoRAEntry
	for i := 0; i < 6; i++ {
		styleLoras = append(styleLoras, preset.LoRAEntry{Name: fmt.Sprintf("style-%d", i), Weight: 0.5})
	}
	lorasJSON, err := json.Marshal(styleLoras)
	require.NoError(t, err)
	style := makeTestPreset(t, db, &preset.Preset{
		Name:      "Moss Big",
		ModelName: "moss_image_sft_bf16",
		Prompt:    "tags",
		Loras:     string(lorasJSON),
	})

	var userLoras []preset.LoRAEntry
	for i := 0; i < 3; i++ {
		userLoras = append(userLoras, preset.LoRAEntry{Name: fmt.Sprintf("own-%d", i), Weight: 0.5})
	}
	svc := newTestService(t, db, &mockLLM{}, &mockSD{})

	_, err = svc.EditImage(EditImageParams{
		ImageBase64:   tinyPNGBase64(t),
		Instruction:   "make hair red",
		StylePresetID: style.ID,
		Loras:         userLoras,
	})
	assert.ErrorContains(t, err, "too many loras: style 6 + own 3 > max 8")
}

func TestEditImage_QualityPipeline(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	res := &preset.Resolution{Name: "q-2048", Width: 2048, Height: 2048}
	require.NoError(t, db.CreateResolution(res))

	var mu sync.Mutex
	var ops []string
	imgCalls := 0

	sdSvc := &mockSD{}
	sdSvc.getUpscalers = func() ([]sd.Upscaler, error) {
		return []sd.Upscaler{
			{Name: "None"}, {Name: "Lanczos"}, {Name: "ESRGAN"},
		}, nil
	}
	sdSvc.upscale = func(base64Img, upscaler string, scale float64) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		ops = append(ops, fmt.Sprintf("upscale:%s:%.2f", upscaler, scale))
		return "upscaled-b64", nil
	}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		imgCalls++
		n := imgCalls
		defer mu.Unlock()
		denoise := 0.0
		if req.DenoisingStrength != nil {
			denoise = *req.DenoisingStrength
		}
		ops = append(ops, fmt.Sprintf("img2img#%d:%dx%d:denoise=%.2f", n, req.Width, req.Height, denoise))
		if denoise > 0.5 {
			return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}, Info: []byte(`{"base":true}`)}, nil
		}
		return &sd.Txt2ImgResponse{Images: []string{"final-b64"}, Info: []byte(`{"redenoise":true}`)}, nil
	}
	sessions := &mockSessions{}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	svc.sessions = sessions

	resID := res.ID
	result, err := svc.EditImage(EditImageParams{
		ImageBase64:  makePNGBase64(t, 512, 512),
		Instruction:  "make hair red and add a white sweater",
		Loras:        []preset.LoRAEntry{{Name: "Detail_Boost_Moss", Weight: 0.8}},
		OutputMode:   "quality",
		ResolutionID: &resID,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	mu.Lock()
	defer mu.Unlock()

	wantOps := []string{
		"img2img#1:512x512:denoise=0.65",
		"upscale:ESRGAN:4.00",
		"img2img#2:2048x2048:denoise=0.35",
	}
	assert.Equal(t, wantOps, ops)

	assert.Equal(t, "final-b64", result.Image)
	assert.False(t, result.QualityUpscaleSkipped)
	assert.False(t, result.QualityRedenoiseSkipped)
	assert.Contains(t, result.EffectivePrompt, "<lora:Moss_Edit_LoRA:1.0> <lora:Detail_Boost_Moss:0.8>Change only")

	require.Len(t, sessions.items, 1)
	item := sessions.items[0]
	assert.Equal(t, "edit", item.source)
	meta := item.meta
	require.NotNil(t, meta)
	assert.Equal(t, "moss_image_sft_bf16", meta.Model)
	assert.Contains(t, meta.Label, "Edit: ")
	assert.Nil(t, meta.PresetID)
	require.Len(t, meta.Chain, 3)
	assert.Equal(t, "base", meta.Chain[0].Op)
	assert.Equal(t, "upscale", meta.Chain[1].Op)
	assert.Equal(t, "redenoise", meta.Chain[2].Op)
	assert.Contains(t, meta.Chain[0].Loras, "Detail_Boost_Moss")
	require.NotNil(t, meta.Chain[0].Denoise)
	assert.InDelta(t, 0.65, *meta.Chain[0].Denoise, 1e-9)
}

func TestEditImage_StylePreset(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	style := makeTestPreset(t, db, &preset.Preset{
		Name:      "Moss Film Real",
		ModelName: "moss_image_sft_bf16",
		Prompt:    "moss_xvstyle, RAW photo, film grain",
		Steps:     8,
		CfgScale:  1.0,
		Loras:     `[{"name": "Moss_DMD_8step", "weight": 1.0}, {"name": "Detail_Boost_Moss", "weight": 0.8}]`,
	})

	var mu sync.Mutex
	var capturedReq sd.Img2ImgRequest
	sdSvc := &mockSD{}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		capturedReq = req
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	sessions := &mockSessions{}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	svc.sessions = sessions

	result, err := svc.EditImage(EditImageParams{
		ImageBase64:   tinyPNGBase64(t),
		Instruction:   "make hair red",
		StylePresetID: style.ID,
		Loras:         []preset.LoRAEntry{{Name: "Extra_Lora", Weight: 0.5}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, "<lora:Moss_Edit_LoRA:1.0> <lora:Moss_DMD_8step:1> <lora:Detail_Boost_Moss:0.8> <lora:Extra_Lora:0.5> moss_xvstyle, RAW photo, film grain. Change only make hair red"+editPromptSuffix, capturedReq.Prompt)
	assert.Equal(t, 8, capturedReq.Steps)
	assert.InDelta(t, 1.0, capturedReq.CfgScale, 1e-9)
	require.NotNil(t, capturedReq.DenoisingStrength)
	assert.InDelta(t, 0.65, *capturedReq.DenoisingStrength, 1e-9)

	require.Len(t, sessions.items, 1)
	meta := sessions.items[0].meta
	require.NotNil(t, meta)
	require.NotNil(t, meta.PresetID)
	assert.Equal(t, style.ID, *meta.PresetID)
}

func TestEditImage_DenoiseClamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{name: "nil keeps default", input: 0, want: 0.65},
		{name: "too low clamped", input: 0.001, want: 0.05},
		{name: "too high clamped", input: 5, want: 1.0},
		{name: "valid kept", input: 0.8, want: 0.8},
		{name: "nan keeps default", input: math.NaN(), want: 0.65},
		{name: "inf keeps default", input: math.Inf(1), want: 0.65},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			var captured float64
			sdSvc := &mockSD{}
			sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
				if req.DenoisingStrength != nil {
					captured = *req.DenoisingStrength
				}
				return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
			}
			svc := newTestService(t, db, &mockLLM{}, sdSvc)
			params := EditImageParams{
				ImageBase64: tinyPNGBase64(t),
				Instruction: "make hair red",
			}
			if tt.name != "nil keeps default" {
				params.DenoisingStrength = &tt.input
			}
			_, err := svc.EditImage(params)
			require.NoError(t, err)
			assert.InDelta(t, tt.want, captured, 1e-9)
		})
	}
}

func TestEditImage_QualityRedenoiseFailureSkips(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	res := &preset.Resolution{Name: "q-2048", Width: 2048, Height: 2048}
	require.NoError(t, db.CreateResolution(res))

	sdSvc := &mockSD{}
	sdSvc.getUpscalers = func() ([]sd.Upscaler, error) {
		return []sd.Upscaler{{Name: "ESRGAN"}}, nil
	}
	sdSvc.upscale = func(base64Img, upscaler string, scale float64) (string, error) {
		return "upscaled-b64", nil
	}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		denoise := 0.0
		if req.DenoisingStrength != nil {
			denoise = *req.DenoisingStrength
		}
		if denoise > 0.5 {
			return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
		}
		return nil, fmt.Errorf("redenoise boom")
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	resID := res.ID
	result, err := svc.EditImage(EditImageParams{
		ImageBase64:  makePNGBase64(t, 512, 512),
		Instruction:  "make hair red",
		OutputMode:   "quality",
		ResolutionID: &resID,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "upscaled-b64", result.Image)
	assert.False(t, result.QualityUpscaleSkipped)
	assert.True(t, result.QualityRedenoiseSkipped)
}

func TestEditImage_StripsResidualLoraTagInInstruction(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	sdSvc := &mockSD{}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: tinyPNGBase64(t),
		Instruction: "make <lora:evil_lora:1 hair red",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotContains(t, result.EffectivePrompt, "<lora:evil")
	assert.Contains(t, result.EffectivePrompt, "hair red")
}

func TestEditImage_TranslatesNonEnglishInstruction(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var llmCalled bool
	var capturedSystem, capturedUser string
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		llmCalled = true
		capturedSystem = systemPrompt
		capturedUser = userMessage
		return "\"the blue floral dress to a long red evening gown\"\nextra line", nil
	}
	var capturedPrompt string
	sessions := &mockSessions{}
	sdSvc := &mockSD{}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		capturedPrompt = req.Prompt
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	svc := newTestService(t, db, llmSvc, sdSvc)
	svc.sessions = sessions

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: tinyPNGBase64(t),
		Instruction: "замени синее платье на длинное красное вечернее",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, llmCalled)
	assert.Equal(t, "замени синее платье на длинное красное вечернее", capturedUser)
	assert.Contains(t, capturedSystem, "English")
	assert.Contains(t, capturedPrompt, "Change only the blue floral dress to a long red evening gown.")
	assert.NotContains(t, capturedPrompt, "замени")
	require.Len(t, sessions.items, 1)
	assert.Contains(t, sessions.items[0].meta.Label, "замени")
}

func TestEditImage_AsciiInstructionSkipsTranslation(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		t.Error("LLM must not be called for ASCII instructions")
		return "should not be used", nil
	}
	sdSvc := &mockSD{}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		assert.Contains(t, req.Prompt, "Change only make the dress red.")
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	svc := newTestService(t, db, llmSvc, sdSvc)

	_, err := svc.EditImage(EditImageParams{
		ImageBase64: tinyPNGBase64(t),
		Instruction: "make the dress red",
	})
	require.NoError(t, err)
}

func TestEditImage_TranslationFailureFallsBackToOriginal(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "", fmt.Errorf("llm down")
	}
	var capturedPrompt string
	sdSvc := &mockSD{}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		capturedPrompt = req.Prompt
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	svc := newTestService(t, db, llmSvc, sdSvc)

	_, err := svc.EditImage(EditImageParams{
		ImageBase64: tinyPNGBase64(t),
		Instruction: "замени платье на красное",
	})
	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, "замени платье на красное")
}

func TestSanitizeTranslatedInstruction(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "clean phrase", sanitizeTranslatedInstruction("\"clean phrase\"\n"))
	assert.Equal(t, "no tags", sanitizeTranslatedInstruction("<lora:evil:1>no tags"))
	assert.Equal(t, "sneaky:1 stripped prefix", sanitizeTranslatedInstruction("<lora:sneaky:1 stripped prefix"))
}

func TestResolveQualityUpscaler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		upscalers []sd.Upscaler
		listErr   error
		want      string
	}{
		{name: "keeps legacy R-ESRGAN when both exist", upscalers: []sd.Upscaler{{Name: "R-ESRGAN 4x+"}, {Name: "ESRGAN"}}, want: "R-ESRGAN 4x+"},
		{name: "falls back to ESRGAN on forge neo", upscalers: []sd.Upscaler{{Name: "None"}, {Name: "Lanczos"}, {Name: "ESRGAN"}}, want: "ESRGAN"},
		{name: "none available", upscalers: []sd.Upscaler{{Name: "None"}, {Name: "Lanczos"}}, want: ""},
		{name: "list error", listErr: fmt.Errorf("boom"), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			sdSvc := &mockSD{}
			upscalers, listErr := tt.upscalers, tt.listErr
			sdSvc.getUpscalers = func() ([]sd.Upscaler, error) {
				return upscalers, listErr
			}
			svc := newTestService(t, db, &mockLLM{}, sdSvc)
			assert.Equal(t, tt.want, svc.resolveQualityUpscaler())
		})
	}
}

func TestStripAngleTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "plain text", want: "plain text"},
		{in: "<lora:evil:1>keep me", want: "keep me"},
		{in: "a <b> c <d> e", want: "a  c  e"},
		{in: "unclosed < bracket", want: "unclosed < bracket"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, stripAngleTags(tt.in))
		})
	}
}

func TestAlignEditSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{name: "already aligned", w: 512, h: 768, wantW: 512, wantH: 768},
		{name: "rounds down to 16", w: 513, h: 511, wantW: 512, wantH: 496},
		{name: "caps at 2048 proportionally", w: 4096, h: 2048, wantW: 2048, wantH: 1024},
		{name: "floor 64", w: 8, h: 8, wantW: 64, wantH: 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := alignEditSize(tt.w, tt.h)
			assert.Equal(t, tt.wantW, gotW)
			assert.Equal(t, tt.wantH, gotH)
		})
	}
}

func TestAlignEditSizeCapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		w, h         int
		cap          int
		wantW, wantH int
	}{
		{name: "phone portrait 1360x2048 to edit canvas", w: 1360, h: 2048, cap: 1408, wantW: 928, wantH: 1408},
		{name: "small source untouched", w: 832, h: 1248, cap: 1408, wantW: 832, wantH: 1248},
		{name: "square big", w: 2048, h: 2048, cap: 1408, wantW: 1408, wantH: 1408},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := alignEditSizeCapped(tt.w, tt.h, tt.cap)
			assert.Equal(t, tt.wantW, gotW)
			assert.Equal(t, tt.wantH, gotH)
		})
	}
}

func TestEditImage_BigSourceEditsOnCanvasAndRestoresResolution(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var mu sync.Mutex
	var editReq sd.Img2ImgRequest
	upscaleCalls := 0
	var upscaleScale float64

	sdSvc := &mockSD{}
	sdSvc.getUpscalers = func() ([]sd.Upscaler, error) {
		return []sd.Upscaler{{Name: "ESRGAN"}}, nil
	}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		editReq = req
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	sdSvc.upscale = func(base64Img, upscaler string, scale float64) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		upscaleCalls++
		upscaleScale = scale
		return "restored-b64", nil
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: makePNGBase64(t, 2048, 4096),
		Instruction: "the pants to a skirt",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 704, editReq.Width)
	assert.Equal(t, 1408, editReq.Height)
	assert.Equal(t, 1, upscaleCalls)
	assert.InDelta(t, float64(1024)/float64(704), upscaleScale, 0.001)
	assert.Equal(t, "restored-b64", result.Image)
}

func TestEditImage_SmallSourceNoRestoreUpscale(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	upscaleCalls := 0
	sdSvc := &mockSD{}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		assert.LessOrEqual(t, req.Width, 1408)
		assert.LessOrEqual(t, req.Height, 1408)
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	sdSvc.upscale = func(base64Img, upscaler string, scale float64) (string, error) {
		upscaleCalls++
		return "must-not-be-called", nil
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: makePNGBase64(t, 832, 1248),
		Instruction: "the pants to a skirt",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, upscaleCalls)
	assert.Equal(t, "edited-b64", result.Image)
}
