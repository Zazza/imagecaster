package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const kleinWorkArea = 1024 * 1024

func assertKleinWorkSize(t *testing.T, srcW, srcH, gotW, gotH int) {
	t.Helper()
	assert.Zero(t, gotW%16, "src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	assert.Zero(t, gotH%16, "src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	assert.GreaterOrEqual(t, gotW, 64, "src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	assert.GreaterOrEqual(t, gotH, 64, "src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	area := float64(gotW * gotH)
	assert.InDelta(t, kleinWorkArea, area, kleinWorkArea*0.10, "area src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	srcRatio := float64(srcW) / float64(srcH)
	gotRatio := float64(gotW) / float64(gotH)
	tol := max(0.02, 16.0/float64(min(gotW, gotH)))
	assert.InDelta(t, srcRatio, gotRatio, srcRatio*tol, "aspect src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
}

func solidPNGBase64(t *testing.T, w, h int, c color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodePNGBase64(t *testing.T, b64 string) image.Image {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return img
}

var (
	workGray    = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	upscaledRed = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
)

func TestKleinWorkSize_Cases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		w, h int
	}{
		{name: "large portrait 1360x2048", w: 1360, h: 2048},
		{name: "small portrait 300x400", w: 300, h: 400},
		{name: "huge source 3000x4500", w: 3000, h: 4500},
		{name: "landscape 1500x1000", w: 1500, h: 1000},
		{name: "small square 512", w: 512, h: 512},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := kleinWorkSize(tt.w, tt.h)
			assertKleinWorkSize(t, tt.w, tt.h, gotW, gotH)
		})
	}
}

func TestKleinWorkSize_Square4096Is1024(t *testing.T) {
	t.Parallel()
	gotW, gotH := kleinWorkSize(4096, 4096)
	assert.Equal(t, 1024, gotW)
	assert.Equal(t, 1024, gotH)
}

func TestKleinWorkSize_Properties(t *testing.T) {
	t.Parallel()
	for _, w := range []int{120, 300, 600, 777, 1000, 1024, 1500, 2100, 3000, 5000} {
		for _, h := range []int{90, 400, 801, 1024, 1999, 2048, 4100} {
			gotW, gotH := kleinWorkSize(w, h)
			ratio := float64(w) / float64(h)
			if ratio < 0.25 || ratio > 4 {
				assert.Zero(t, gotW%16, "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
				assert.Zero(t, gotH%16, "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
				assert.GreaterOrEqual(t, min(gotW, gotH), 64, "w=%d h=%d -> %dx%d", w, h, gotW, gotH)
				continue
			}
			assertKleinWorkSize(t, w, h, gotW, gotH)
		}
	}
}

func TestKleinWorkSize_ExtremeAspectMinSide64(t *testing.T) {
	t.Parallel()
	for _, sz := range [][2]int{{10, 5000}, {5000, 10}, {64, 8000}} {
		gotW, gotH := kleinWorkSize(sz[0], sz[1])
		assert.GreaterOrEqual(t, gotW, 64, "%v -> %dx%d", sz, gotW, gotH)
		assert.GreaterOrEqual(t, gotH, 64, "%v -> %dx%d", sz, gotW, gotH)
		assert.Zero(t, gotW%16, "%v -> %dx%d", sz, gotW, gotH)
		assert.Zero(t, gotH%16, "%v -> %dx%d", sz, gotW, gotH)
	}
}

type kleinRun struct {
	mu         sync.Mutex
	img2img    []sd.Img2ImgRequest
	upscales   []upscaleCall
	upscalers  []sd.Upscaler
	upscaleErr error
	img2imgOut func(req sd.Img2ImgRequest) string
	upscaleOut func(t *testing.T, in string, scale float64) string
}

type upscaleCall struct {
	in       string
	upscaler string
	scale    float64
}

func (r *kleinRun) mock(t *testing.T) *mockSD {
	return &mockSD{
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			r.mu.Lock()
			r.img2img = append(r.img2img, req)
			out := r.img2imgOut
			r.mu.Unlock()
			img := solidPNGBase64(t, req.Width, req.Height, workGray)
			if out != nil {
				img = out(req)
			}
			return &sd.Txt2ImgResponse{Images: []string{img}}, nil
		},
		getUpscalers: func() ([]sd.Upscaler, error) {
			return r.upscalers, nil
		},
		upscale: func(in, upscaler string, scale float64) (string, error) {
			r.mu.Lock()
			r.upscales = append(r.upscales, upscaleCall{in: in, upscaler: upscaler, scale: scale})
			err := r.upscaleErr
			out := r.upscaleOut
			r.mu.Unlock()
			if err != nil {
				return "", err
			}
			if out != nil {
				return out(t, in, scale), nil
			}
			return in, nil
		},
	}
}

func runKleinFromImage(t *testing.T, r *kleinRun, srcW, srcH int, mutate func(db *preset.DB, p *GenerateFromImageParams)) (*GenerateImageResult, error) {
	t.Helper()
	db := openTestDB(t)
	pr := makeTestPreset(t, db, &preset.Preset{
		Name:         "klein-work-size",
		Prompt:       "a portrait photo",
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        4,
		CfgScale:     1.0,
		ModelName:    "flux-2-klein-4b",
	})
	svc := newTestService(t, db, &mockLLM{}, r.mock(t))
	svc.ctx = context.Background()
	params := GenerateFromImageParams{
		ImageBase64:       makePNGBase64(t, srcW, srcH),
		GenMode:           "preset",
		Mode:              "img2img",
		PresetID:          pr.ID,
		DenoisingStrength: 0.6,
	}
	if mutate != nil {
		mutate(db, &params)
	}
	return svc.GenerateFromImage(params)
}

func neuralUpscaleTo(w, h int) func(t *testing.T, in string, scale float64) string {
	return func(t *testing.T, in string, scale float64) string {
		return solidPNGBase64(t, w, h, upscaledRed)
	}
}

func assertImage(t *testing.T, b64 string, wantW, wantH int, wantColor color.NRGBA) {
	t.Helper()
	img := decodePNGBase64(t, b64)
	assert.Equal(t, wantW, img.Bounds().Dx())
	assert.Equal(t, wantH, img.Bounds().Dy())
	got := color.NRGBAModel.Convert(img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2)).(color.NRGBA)
	assert.Equal(t, wantColor, got)
}

func TestGenerateFromImage_KleinLargeSource_WorkSizeThenNeuralUpscale(t *testing.T) {
	t.Parallel()
	r := &kleinRun{
		upscalers:  []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}},
		upscaleOut: neuralUpscaleTo(1360, 2048),
	}
	result, err := runKleinFromImage(t, r, 3000, 4500, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	req := r.img2img[0]
	assertKleinWorkSize(t, 3000, 4500, req.Width, req.Height)

	require.Len(t, r.upscales, 1)
	call := r.upscales[0]
	assert.Equal(t, qualityUpscalerPreferences[0], call.upscaler)
	in := decodePNGBase64(t, call.in)
	assert.Equal(t, req.Width, in.Bounds().Dx())
	assert.Equal(t, req.Height, in.Bounds().Dy())
	assert.InDelta(t, 1360, call.scale*float64(req.Width), 1360*0.02)
	assert.InDelta(t, 2048, call.scale*float64(req.Height), 2048*0.02)

	assertImage(t, result.Image, 1360, 2048, upscaledRed)
}

func TestGenerateFromImage_KleinLargeSource_NoNeuralUpscaler_ResizedToFinal(t *testing.T) {
	t.Parallel()
	r := &kleinRun{}
	result, err := runKleinFromImage(t, r, 3000, 4500, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	assertKleinWorkSize(t, 3000, 4500, r.img2img[0].Width, r.img2img[0].Height)
	assert.Empty(t, r.upscales)
	assertImage(t, result.Image, 1360, 2048, workGray)
}

func TestGenerateFromImage_KleinLargeSource_NeuralFails_FallbackResize(t *testing.T) {
	t.Parallel()
	r := &kleinRun{
		upscalers:  []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}},
		upscaleErr: fmt.Errorf("esrgan boom"),
	}
	result, err := runKleinFromImage(t, r, 3000, 4500, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	assertKleinWorkSize(t, 3000, 4500, r.img2img[0].Width, r.img2img[0].Height)
	assert.Len(t, r.upscales, 1)
	assertImage(t, result.Image, 1360, 2048, workGray)
}

func TestGenerateFromImage_KleinUpscaleError_ReturnsWorkImage(t *testing.T) {
	t.Parallel()
	r := &kleinRun{
		upscalers:  []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}},
		upscaleErr: fmt.Errorf("esrgan boom"),
		img2imgOut: func(req sd.Img2ImgRequest) string { return "work-not-a-png" },
	}
	result, err := runKleinFromImage(t, r, 3000, 4500, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	assertKleinWorkSize(t, 3000, 4500, r.img2img[0].Width, r.img2img[0].Height)
	assert.Equal(t, "work-not-a-png", result.Image)
}

func TestGenerateFromImage_KleinSmallSource_WorkSizeNoDownscale(t *testing.T) {
	t.Parallel()
	for _, sz := range [][2]int{{600, 800}, {300, 400}, {512, 512}} {
		t.Run(fmt.Sprintf("%dx%d", sz[0], sz[1]), func(t *testing.T) {
			t.Parallel()
			r := &kleinRun{upscalers: []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}}}
			result, err := runKleinFromImage(t, r, sz[0], sz[1], nil)
			require.NoError(t, err)
			require.NotNil(t, result)

			r.mu.Lock()
			defer r.mu.Unlock()
			require.Len(t, r.img2img, 1)
			req := r.img2img[0]
			assertKleinWorkSize(t, sz[0], sz[1], req.Width, req.Height)
			assert.Empty(t, r.upscales)
			assertImage(t, result.Image, req.Width, req.Height, workGray)
		})
	}
}

func TestGenerateFromImage_KleinWithResolution_SizeUnchangedNoUpscale(t *testing.T) {
	t.Parallel()
	r := &kleinRun{upscalers: []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}}}
	_, err := runKleinFromImage(t, r, 3000, 4500, func(db *preset.DB, p *GenerateFromImageParams) {
		res := &preset.Resolution{Name: "r-1024x1536", Width: 1024, Height: 1536}
		require.NoError(t, db.CreateResolution(res))
		p.ResolutionID = &res.ID
	})
	require.NoError(t, err)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	assert.Equal(t, 1024, r.img2img[0].Width)
	assert.Equal(t, 1520, r.img2img[0].Height)
	assert.Empty(t, r.upscales)
}

func TestGenerateFromImage_KleinQualityMode_Unchanged(t *testing.T) {
	t.Parallel()
	r := &kleinRun{upscalers: []sd.Upscaler{{Name: "RealESRGAN_x4plus"}}}
	_, err := runKleinFromImage(t, r, 600, 800, func(db *preset.DB, p *GenerateFromImageParams) {
		res := &preset.Resolution{Name: "r-1024x1536", Width: 1024, Height: 1536}
		require.NoError(t, db.CreateResolution(res))
		p.ResolutionID = &res.ID
		p.OutputMode = "quality"
	})
	require.NoError(t, err)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 2)
	assert.Equal(t, [2]int{592, 800}, [2]int{r.img2img[0].Width, r.img2img[0].Height})
	assert.Equal(t, [2]int{1024, 1376}, [2]int{r.img2img[1].Width, r.img2img[1].Height})
	require.Len(t, r.upscales, 1)
	assert.InDelta(t, 1.73, r.upscales[0].scale, 0.01)
}

func TestGenerateFromImage_NonKleinLargeSource_NoWorkSizeNoUpscale(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	pr := makeTestPreset(t, db, &preset.Preset{
		Name:         "non-klein-large",
		Prompt:       "a portrait photo",
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        4,
		CfgScale:     1.0,
		ModelName:    "epicrealismXL_pureFix",
	})
	r := &kleinRun{upscalers: []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}}}
	svc := newTestService(t, db, &mockLLM{}, r.mock(t))
	svc.ctx = context.Background()
	_, err := svc.GenerateFromImage(GenerateFromImageParams{
		ImageBase64:       makePNGBase64(t, 1500, 1000),
		GenMode:           "preset",
		Mode:              "img2img",
		PresetID:          pr.ID,
		DenoisingStrength: 0.6,
	})
	require.NoError(t, err)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	assert.Equal(t, 1496, r.img2img[0].Width)
	assert.Equal(t, 1000, r.img2img[0].Height)
	assert.Empty(t, r.upscales)
}

func runLookDirect(t *testing.T, r *kleinRun, srcW, srcH int) (*GenerateImageResult, error) {
	t.Helper()
	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "warm film grain", nil
	}
	svc := newTestService(t, db, llmSvc, r.mock(t))
	seed := int64(7)
	return svc.ApplyLookDirect(makePNGBase64(t, srcW, srcH), "film look", 0.7, &seed)
}

func TestApplyLookDirect_SmallSource_Work1024NoUpscale(t *testing.T) {
	t.Parallel()
	r := &kleinRun{upscalers: []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}}}
	result, err := runLookDirect(t, r, 512, 512)
	require.NoError(t, err)
	require.NotNil(t, result)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	assert.Equal(t, 1024, r.img2img[0].Width)
	assert.Equal(t, 1024, r.img2img[0].Height)
	assert.Empty(t, r.upscales)
	assertImage(t, result.Image, 1024, 1024, workGray)
}

func TestApplyLookDirect_LargeSource_WorkSizeThenUpscaleToAlignLookSize(t *testing.T) {
	t.Parallel()
	wantW, wantH := alignLookSize(3000, 4500)
	require.Equal(t, [2]int{1360, 2048}, [2]int{wantW, wantH})

	r := &kleinRun{
		upscalers:  []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}},
		upscaleOut: neuralUpscaleTo(wantW, wantH),
	}
	result, err := runLookDirect(t, r, 3000, 4500)
	require.NoError(t, err)
	require.NotNil(t, result)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	req := r.img2img[0]
	assertKleinWorkSize(t, 3000, 4500, req.Width, req.Height)
	require.Len(t, r.upscales, 1)
	assert.True(t, math.Abs(r.upscales[0].scale*float64(req.Width)-float64(wantW)) <= float64(wantW)*0.02)
	assertImage(t, result.Image, wantW, wantH, upscaledRed)
}

func TestApplyLookDirect_LargeSource_NoNeuralUpscaler_ResizedToAlignLookSize(t *testing.T) {
	t.Parallel()
	r := &kleinRun{}
	result, err := runLookDirect(t, r, 3000, 4500)
	require.NoError(t, err)
	require.NotNil(t, result)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.img2img, 1)
	assertKleinWorkSize(t, 3000, 4500, r.img2img[0].Width, r.img2img[0].Height)
	assertImage(t, result.Image, 1360, 2048, workGray)
}

func TestApplyLookDirect_UpscaleError_ReturnsWorkImage(t *testing.T) {
	t.Parallel()
	r := &kleinRun{
		upscalers:  []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}},
		upscaleErr: fmt.Errorf("esrgan boom"),
		img2imgOut: func(req sd.Img2ImgRequest) string { return "look-not-a-png" },
	}
	result, err := runLookDirect(t, r, 3000, 4500)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "look-not-a-png", result.Image)
}
