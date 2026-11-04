package generation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

func TestAlignLookSize_MultipleOf16(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{name: "600x800 failing klein size", w: 600, h: 800, wantW: 592, wantH: 800},
		{name: "300x400 failing klein size", w: 300, h: 400, wantW: 288, wantH: 400},
		{name: "already multiple of 16", w: 736, h: 1024, wantW: 736, wantH: 1024},
		{name: "multiple of 8 not of 16", w: 520, h: 1000, wantW: 512, wantH: 992},
		{name: "odd sizes", w: 513, h: 511, wantW: 512, wantH: 496},
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

func TestAlignLookSize_AlwaysMultipleOf16(t *testing.T) {
	t.Parallel()
	for _, w := range []int{64, 100, 257, 600, 777, 1023, 1500, 2047, 3001, 5000} {
		for _, h := range []int{64, 99, 400, 801, 1024, 1999, 2049, 4100} {
			gotW, gotH := alignLookSize(w, h)
			assert.Zero(t, gotW%16, "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
			assert.Zero(t, gotH%16, "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
		}
	}
}

func TestKleinImg2ImgSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{name: "small portrait upscaled to 1024 long side", w: 300, h: 400, wantW: 768, wantH: 1024},
		{name: "medium portrait upscaled to 1024 long side", w: 600, h: 800, wantW: 768, wantH: 1024},
		{name: "small square upscaled", w: 512, h: 512, wantW: 1024, wantH: 1024},
		{name: "small landscape upscaled", w: 800, h: 600, wantW: 1024, wantH: 768},
		{name: "long side exactly 1024 unchanged", w: 1024, h: 768, wantW: 1024, wantH: 768},
		{name: "large not upscaled only aligned", w: 1500, h: 1000, wantW: 1488, wantH: 992},
		{name: "huge capped at 2048", w: 4000, h: 3000, wantW: 2048, wantH: 1536},
		{name: "huge portrait capped at 2048", w: 3000, h: 4000, wantW: 1536, wantH: 2048},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := kleinImg2ImgSize(tt.w, tt.h)
			assert.Equal(t, tt.wantW, gotW)
			assert.Equal(t, tt.wantH, gotH)
		})
	}
}

func TestKleinImg2ImgSize_Properties(t *testing.T) {
	t.Parallel()
	for _, w := range []int{120, 333, 600, 777, 1000, 1023, 1500, 2100, 5000} {
		for _, h := range []int{90, 400, 801, 1000, 1024, 1999, 4100} {
			gotW, gotH := kleinImg2ImgSize(w, h)
			assert.Zero(t, gotW%16, "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
			assert.Zero(t, gotH%16, "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
			assert.LessOrEqual(t, max(gotW, gotH), 2048, "w=%d h=%d", w, h)
			if max(w, h) < 1024 {
				assert.Equal(t, 1024, max(gotW, gotH), "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
			} else {
				assert.LessOrEqual(t, gotW, w, "not upscaled w=%d h=%d", w, h)
				assert.LessOrEqual(t, gotH, h, "not upscaled w=%d h=%d", w, h)
			}
			if min(gotW, gotH) >= 256 {
				srcRatio := float64(w) / float64(h)
				gotRatio := float64(gotW) / float64(gotH)
				assert.InDelta(t, srcRatio, gotRatio, srcRatio*0.05, "aspect w=%d h=%d -> %dx%d", w, h, gotW, gotH)
			}
		}
	}
}

func runFromImageSize(t *testing.T, modelName string, w, h int) sd.Img2ImgRequest {
	t.Helper()
	db := openTestDB(t)
	p := makeTestPreset(t, db, &preset.Preset{
		Name:         "from-image-size-" + modelName,
		Prompt:       "a portrait photo",
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        4,
		CfgScale:     1.0,
		ModelName:    modelName,
	})

	var captured sd.Img2ImgRequest
	sdSvc := &mockSD{
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			captured = req
			return &sd.Txt2ImgResponse{Images: []string{"img"}}, nil
		},
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	svc.ctx = context.Background()

	_, err := svc.GenerateFromImage(GenerateFromImageParams{
		ImageBase64:       makePNGBase64(t, w, h),
		GenMode:           "preset",
		Mode:              "img2img",
		PresetID:          p.ID,
		DenoisingStrength: 0.6,
	})
	require.NoError(t, err)
	return captured
}

func TestGenerateFromImage_KleinSmallImageUpscaledTo1024(t *testing.T) {
	t.Parallel()
	req := runFromImageSize(t, "flux-2-klein-4b", 600, 800)
	assert.Equal(t, 768, req.Width)
	assert.Equal(t, 1024, req.Height)
}

func TestGenerateFromImage_KleinLargeImageNotUpscaled(t *testing.T) {
	t.Parallel()
	req := runFromImageSize(t, "flux-2-klein-4b", 1500, 1000)
	assert.Equal(t, 1488, req.Width)
	assert.Equal(t, 992, req.Height)
}

func TestGenerateFromImage_NonKleinSizeUnchanged(t *testing.T) {
	t.Parallel()
	req := runFromImageSize(t, "epicrealismXL_pureFix", 600, 800)
	assert.Equal(t, 600, req.Width)
	assert.Equal(t, 800, req.Height)
}
