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
		p := makeTestPreset(t, db, &preset.Preset{Name: "z-style", ModelName: "z-image-turbo", Prompt: "photo"})
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		_, err := svc.loadEditStyle(p.ID)
		assert.ErrorContains(t, err, "must use a moss model")
	})

	t.Run("moss preset loads tags and loras", func(t *testing.T) {
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

func recordingSD(ops *[]string, mu *sync.Mutex, img2imgFn func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error)) *mockSD {
	sdSvc := &mockSD{}
	sdSvc.setOption = func(key, value string) error {
		mu.Lock()
		defer mu.Unlock()
		*ops = append(*ops, "option:"+key+"="+value)
		return nil
	}
	sdSvc.img2img = img2imgFn
	return sdSvc
}

func TestEditImage_ReferenceSequence(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string
	var capturedReq sd.Img2ImgRequest

	sdSvc := recordingSD(&ops, &mu, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		capturedReq = req
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	})
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: tinyPNGBase64(t),
		Instruction: "make hair red",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{
		"option:" + mossReferenceOption + "=" + mossReferenceOn,
		"option:" + mossReferenceOption + "=" + mossReferenceOff,
	}, ops)
	require.NotNil(t, capturedReq.DenoisingStrength)
	assert.InDelta(t, 1.0, *capturedReq.DenoisingStrength, 1e-9)
	assert.Equal(t, mossEditSteps, capturedReq.Steps)
	assert.False(t, result.EditReferenceResetFailed)
}

func TestEditImage_ReferenceResetFailureFlagged(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	sdSvc := &mockSD{}
	sdSvc.setOption = func(key, value string) error {
		if value == mossReferenceOff {
			return fmt.Errorf("options boom")
		}
		return nil
	}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: tinyPNGBase64(t),
		Instruction: "make hair red",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.EditReferenceResetFailed)
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

	assert.Equal(t, "<lora:Moss_Edit_LoRA:1.0> <lora:Moss_DMD_8step:1> <lora:Detail_Boost_Moss:0.8> <lora:Extra_Lora:0.5>Change only make hair red"+editPromptSuffix, capturedReq.Prompt)
	assert.Equal(t, mossEditSteps, capturedReq.Steps)
	assert.InDelta(t, mossEditCfgScale, capturedReq.CfgScale, 1e-9)

	require.Len(t, sessions.items, 1)
	meta := sessions.items[0].meta
	require.NotNil(t, meta)
	require.NotNil(t, meta.PresetID)
	assert.Equal(t, style.ID, *meta.PresetID)
}

func TestEditImage_StyleTagsOptIn(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	style := makeTestPreset(t, db, &preset.Preset{
		Name:      "Moss Film Real",
		ModelName: "moss_image_sft_bf16",
		Prompt:    "moss_xvstyle, RAW photo",
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
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	_, err := svc.EditImage(EditImageParams{
		ImageBase64:   tinyPNGBase64(t),
		Instruction:   "make hair red",
		StylePresetID: style.ID,
		StyleTags:     true,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, capturedReq.Prompt, "moss_xvstyle, RAW photo. Change only")
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
	sdSvc.setOption = func(key, value string) error {
		mu.Lock()
		defer mu.Unlock()
		ops = append(ops, "option:"+key+"="+value)
		return nil
	}
	sdSvc.getUpscalers = func() ([]sd.Upscaler, error) {
		return []sd.Upscaler{{Name: "ESRGAN"}}, nil
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
		"option:" + mossReferenceOption + "=" + mossReferenceOn,
		"img2img#1:512x512:denoise=1.00",
		"option:" + mossReferenceOption + "=" + mossReferenceOff,
		"upscale:ESRGAN:4.00",
		"img2img#2:2048x2048:denoise=0.35",
		"option:" + mossReferenceOption + "=" + mossReferenceOff,
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
	require.Len(t, meta.Chain, 3)
	assert.Equal(t, "base", meta.Chain[0].Op)
	assert.Equal(t, "upscale", meta.Chain[1].Op)
	assert.Equal(t, "redenoise", meta.Chain[2].Op)
}

func TestEditImage_BigSourceEditsAtSourceResolution(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	upscaleCalls := 0
	var mu sync.Mutex
	var capturedReq sd.Img2ImgRequest
	sdSvc := &mockSD{}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		capturedReq = req
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
	}
	sdSvc.upscale = func(base64Img, upscaler string, scale float64) (string, error) {
		upscaleCalls++
		return "must-not-be-called", nil
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: makePNGBase64(t, 2048, 4096),
		Instruction: "the pants to a skirt",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1024, capturedReq.Width)
	assert.Equal(t, 2048, capturedReq.Height)
	assert.Equal(t, 0, upscaleCalls)
	assert.Equal(t, "edited-b64", result.Image)
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
		{name: "phone portrait kept", w: 1360, h: 2048, wantW: 1360, wantH: 2048},
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
