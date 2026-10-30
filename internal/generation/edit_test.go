package generation

import (
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
		loras       []preset.LoRAEntry
		want        string
	}{
		{
			name:        "no user loras",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, buildEditPrompt(tt.instruction, tt.loras))
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

	t.Run("empty name rejected", func(t *testing.T) {
		t.Parallel()
		_, err := sanitizeEditLoras([]preset.LoRAEntry{{Name: "   ", Weight: 0.5}})
		assert.ErrorContains(t, err, "lora name is required")
	})

	t.Run("more than 8 rejected", func(t *testing.T) {
		t.Parallel()
		var loras []preset.LoRAEntry
		for i := 0; i < 9; i++ {
			loras = append(loras, preset.LoRAEntry{Name: fmt.Sprintf("lora-%d", i), Weight: 0.5})
		}
		_, err := sanitizeEditLoras(loras)
		assert.ErrorContains(t, err, "too many loras")
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
		if denoise >= 1 {
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
		"option:moss_do_reference=Moss-Image-Edit",
		"img2img#1:512x512:denoise=1.00",
		"option:moss_do_reference=Disable",
		"upscale:ESRGAN:4.00",
		"img2img#2:2048x2048:denoise=0.35",
		"option:moss_do_reference=Disable",
	}
	assert.Equal(t, wantOps, ops)

	assert.Equal(t, "final-b64", result.Image)
	assert.False(t, result.QualityUpscaleSkipped)
	assert.False(t, result.QualityRedenoiseSkipped)
	assert.False(t, result.EditReferenceResetFailed)
	assert.Contains(t, result.EffectivePrompt, "<lora:Moss_Edit_LoRA:1.0> <lora:Detail_Boost_Moss:0.8>Change only")

	require.Len(t, sessions.items, 1)
	item := sessions.items[0]
	assert.Equal(t, "edit", item.source)
	meta := item.meta
	require.NotNil(t, meta)
	assert.Equal(t, "moss_image_sft_bf16", meta.Model)
	assert.Contains(t, meta.Label, "Edit: ")
	require.Len(t, meta.Chain, 3)
	assert.Equal(t, "base", meta.Chain[0].Op)
	assert.Equal(t, "upscale", meta.Chain[1].Op)
	assert.Equal(t, "redenoise", meta.Chain[2].Op)
	assert.Contains(t, meta.Chain[0].Loras, "Detail_Boost_Moss")
}

func TestEditImage_QualityRedenoiseFailureSkips(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	res := &preset.Resolution{Name: "q-2048", Width: 2048, Height: 2048}
	require.NoError(t, db.CreateResolution(res))

	var mu sync.Mutex
	var ops []string

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
		return "upscaled-b64", nil
	}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		denoise := 0.0
		if req.DenoisingStrength != nil {
			denoise = *req.DenoisingStrength
		}
		if denoise >= 1 {
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

func TestEditImage_StandardModeKeepsDeferOnlyReset(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string

	sdSvc := &mockSD{}
	sdSvc.setOption = func(key, value string) error {
		mu.Lock()
		defer mu.Unlock()
		ops = append(ops, "option:"+key+"="+value)
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

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{
		"option:moss_do_reference=Moss-Image-Edit",
		"option:moss_do_reference=Disable",
	}, ops)
	assert.False(t, result.QualityUpscaleSkipped)
	assert.False(t, result.QualityRedenoiseSkipped)
}

func TestEditImage_QualitySkippedWhenDisableFails(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	res := &preset.Resolution{Name: "q-2048", Width: 2048, Height: 2048}
	require.NoError(t, db.CreateResolution(res))

	var mu sync.Mutex
	var ops []string

	sdSvc := &mockSD{}
	sdSvc.setOption = func(key, value string) error {
		mu.Lock()
		defer mu.Unlock()
		ops = append(ops, "option:"+key+"="+value)
		if value == mossReferenceOff {
			return fmt.Errorf("options boom")
		}
		return nil
	}
	sdSvc.getUpscalers = func() ([]sd.Upscaler, error) {
		return []sd.Upscaler{{Name: "ESRGAN"}}, nil
	}
	sdSvc.upscale = func(base64Img, upscaler string, scale float64) (string, error) {
		return "must-not-be-called", nil
	}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		return &sd.Txt2ImgResponse{Images: []string{"edited-b64"}}, nil
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

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, []string{
		"option:moss_do_reference=Moss-Image-Edit",
		"option:moss_do_reference=Disable",
		"option:moss_do_reference=Disable",
	}, ops)
	assert.Equal(t, "edited-b64", result.Image)
	assert.True(t, result.QualityUpscaleSkipped)
	assert.True(t, result.QualityRedenoiseSkipped)
	assert.True(t, result.EditReferenceResetFailed)
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
