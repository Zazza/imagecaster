package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

var (
	emGray = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	emRed  = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
)

type emSD struct {
	mu       sync.Mutex
	txt2img  int
	img2img  []sd.Img2ImgRequest
	upscales int
	outputs  []string
	paint    func(t *testing.T, w, h int) string
}

func (r *emSD) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.txt2img + len(r.img2img) + r.upscales
}

func (r *emSD) mock(t *testing.T) *mockSD {
	out := func(w, h int) string {
		img := solidPNGBase64(t, w, h, emGray)
		if r.paint != nil {
			img = r.paint(t, w, h)
		}
		r.mu.Lock()
		r.outputs = append(r.outputs, img)
		r.mu.Unlock()
		return img
	}
	return &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			r.mu.Lock()
			r.txt2img++
			r.mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{out(req.Width, req.Height)}}, nil
		},
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			r.mu.Lock()
			r.img2img = append(r.img2img, req)
			r.mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{out(req.Width, req.Height)}}, nil
		},
		getUpscalers: func() ([]sd.Upscaler, error) {
			return []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}}, nil
		},
		upscale: func(in, upscaler string, scale float64) (string, error) {
			r.mu.Lock()
			r.upscales++
			r.mu.Unlock()
			return in, nil
		},
	}
}

func emLLM() *mockLLM {
	return &mockLLM{
		genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
			return `{"prompt":"llm-scene-tags","negative_prompt":"llm-neg"}`, nil
		},
		chatFn: func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
			return "a portrait photo", nil
		},
		chatVisionFn: func(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
			return "a portrait photo", nil
		},
		analyzeImageFn: func(model, systemPrompt, imageBase64 string, maxTokens int) (string, error) {
			return "a portrait photo", nil
		},
		describeFn: func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
			return "a portrait photo", nil
		},
	}
}

type emEnv struct {
	svc      *Service
	sd       *emSD
	sessions *mockSessions
	db       *preset.DB
	preset   *preset.Preset
}

func emSetup(t *testing.T, paint func(t *testing.T, w, h int) string) *emEnv {
	t.Helper()
	db := openTestDB(t)
	pr := makeTestPreset(t, db, &preset.Preset{
		Name:         "effect-fabric",
		Prompt:       "silk fabric texture",
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        4,
		CfgScale:     1.0,
		ModelName:    "epicrealismXL_pureFix",
	})
	r := &emSD{paint: paint}
	svc := newTestService(t, db, emLLM(), r.mock(t))
	svc.ctx = context.Background()
	sessions := &mockSessions{}
	svc.sessions = sessions
	return &emEnv{svc: svc, sd: r, sessions: sessions, db: db, preset: pr}
}

func (e *emEnv) params(src string) GenerateFromImageParams {
	return GenerateFromImageParams{
		ImageBase64:       src,
		GenMode:           "preset",
		Mode:              "img2img",
		PresetID:          e.preset.ID,
		DenoisingStrength: 0.6,
	}
}

func emMaskBase64(t *testing.T, w, h int, painted image.Rectangle) string {
	t.Helper()
	m := image.NewGray(image.Rect(0, 0, w, h))
	painted = painted.Intersect(m.Bounds())
	for y := painted.Min.Y; y < painted.Max.Y; y++ {
		for x := painted.Min.X; x < painted.Max.X; x++ {
			m.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, m))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func emTransparentMaskBase64(t *testing.T, w, h int) string {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, m))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func emPixel(img image.Image, x, y int) color.NRGBA {
	b := img.Bounds()
	return color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
}

func emStripesPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := emGray
			if x < w/4 || x >= w*3/4 {
				c = emRed
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func emSolidRed(t *testing.T, w, h int) string {
	return solidPNGBase64(t, w, h, emRed)
}

func TestGenerateFromImage_EffectMask_NoMaskUnchanged(t *testing.T) {
	t.Parallel()
	e := emSetup(t, emSolidRed)
	src := makePNGBase64(t, 256, 256)

	result, err := e.svc.GenerateFromImage(e.params(src))
	require.NoError(t, err)
	require.NotNil(t, result)

	e.sd.mu.Lock()
	defer e.sd.mu.Unlock()
	require.Len(t, e.sd.img2img, 1)
	req := e.sd.img2img[0]
	require.Len(t, req.InitImages, 1)
	assert.Equal(t, src, req.InitImages[0])
	assert.Empty(t, req.Mask)
	assert.Equal(t, 256, req.Width)
	assert.Equal(t, 256, req.Height)
	require.Len(t, e.sd.outputs, 1)
	assert.Equal(t, e.sd.outputs[0], result.Image, "result must be SD output byte for byte")

	e.sessions.mu.Lock()
	defer e.sessions.mu.Unlock()
	require.Len(t, e.sessions.items, 1)
	assert.Equal(t, e.sd.outputs[0], e.sessions.items[0].imageBase64)
}

func TestGenerateFromImage_EffectMask_RejectedForUnsupportedModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(t *testing.T, e *emEnv, p *GenerateFromImageParams)
	}{
		{
			name: "txt2img",
			mutate: func(t *testing.T, e *emEnv, p *GenerateFromImageParams) {
				p.Mode = "txt2img"
			},
		},
		{
			name: "compound",
			mutate: func(t *testing.T, e *emEnv, p *GenerateFromImageParams) {
				cp := &preset.CompoundPreset{
					Name:  "em-compound",
					Steps: []preset.CompoundPresetStep{{PresetID: e.preset.ID, DenoisingStrength: 0.6}},
				}
				require.NoError(t, e.db.CreateCompoundPreset(cp))
				p.GenMode = "compound"
				p.PresetID = 0
				p.CompoundPresetID = cp.ID
			},
		},
		{
			name: "quality",
			mutate: func(t *testing.T, e *emEnv, p *GenerateFromImageParams) {
				res := &preset.Resolution{Name: "em-512", Width: 512, Height: 512}
				require.NoError(t, e.db.CreateResolution(res))
				p.ResolutionID = &res.ID
				p.OutputMode = "quality"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := makePNGBase64(t, 256, 256)

			control := emSetup(t, nil)
			cp := control.params(src)
			tt.mutate(t, control, &cp)
			_, err := control.svc.GenerateFromImage(cp)
			require.NoError(t, err, "same params without mask must work, otherwise the case does not test the mask")
			require.NotZero(t, control.sd.calls())

			e := emSetup(t, nil)
			p := e.params(src)
			tt.mutate(t, e, &p)
			p.MaskBase64 = emMaskBase64(t, 256, 256, image.Rect(0, 0, 128, 256))
			_, err = e.svc.GenerateFromImage(p)
			require.Error(t, err)
			assert.Zero(t, e.sd.calls(), "SD must not be called")
			e.sessions.mu.Lock()
			assert.Empty(t, e.sessions.items)
			e.sessions.mu.Unlock()
		})
	}
}

func TestGenerateFromImage_EffectMask_InvalidMask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mask func(t *testing.T) string
	}{
		{name: "all black", mask: func(t *testing.T) string { return emMaskBase64(t, 256, 256, image.Rectangle{}) }},
		{name: "all transparent", mask: func(t *testing.T) string { return emTransparentMaskBase64(t, 256, 256) }},
		{name: "broken base64", mask: func(t *testing.T) string { return "%%%not-base64%%%" }},
		{name: "not an image", mask: func(t *testing.T) string { return base64.StdEncoding.EncodeToString([]byte("not a png")) }},
		{name: "too large", mask: func(t *testing.T) string { return strings.Repeat("x", MaxImageBase64Len+1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := emSetup(t, nil)
			p := e.params(makePNGBase64(t, 256, 256))
			p.MaskBase64 = tt.mask(t)
			_, err := e.svc.GenerateFromImage(p)
			require.Error(t, err)
			assert.Zero(t, e.sd.calls(), "SD must not be called")
			e.sessions.mu.Lock()
			assert.Empty(t, e.sessions.items)
			e.sessions.mu.Unlock()
		})
	}
}

func TestGenerateFromImage_EffectMask_WholeFrameEffectGluedOnlyInPaintedHalf(t *testing.T) {
	t.Parallel()
	const w, h = 260, 196
	e := emSetup(t, emSolidRed)
	p := e.params(makePNGBase64(t, w, h))
	p.MaskBase64 = emMaskBase64(t, w, h, image.Rect(0, 0, w/2, h))

	result, err := e.svc.GenerateFromImage(p)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, e.sd.calls())

	e.sessions.mu.Lock()
	require.Len(t, e.sessions.items, 1)
	saved := e.sessions.items[0].imageBase64
	e.sessions.mu.Unlock()
	assert.Equal(t, result.Image, saved, "session and response must hold the same image")

	margin := w * 5 / 100
	for name, b64 := range map[string]string{"response": result.Image, "session": saved} {
		img := decodePNGBase64(t, b64)
		require.Equal(t, w, img.Bounds().Dx(), name)
		require.Equal(t, h, img.Bounds().Dy(), name)
		for _, y := range []int{0, h / 4, h / 2, h * 3 / 4, h - 1} {
			for _, x := range []int{0, w / 8, w/2 - margin - 1} {
				assert.Equal(t, emRed, emPixel(img, x, y), "%s: painted (%d,%d) gets the effect", name, x, y)
			}
			for _, x := range []int{w/2 + margin + 1, w * 7 / 8, w - 1} {
				assert.Equal(t, emGray, emPixel(img, x, y), "%s: unpainted (%d,%d) stays original", name, x, y)
			}
		}
	}
}

func TestGenerateFromImage_EffectMask_AppliedOnlyInPaintedArea(t *testing.T) {
	t.Parallel()
	const w, h = 260, 196
	e := emSetup(t, emStripesPNG)
	p := e.params(makePNGBase64(t, w, h))
	p.MaskBase64 = emMaskBase64(t, w, h, image.Rect(0, 0, w/2, h))

	result, err := e.svc.GenerateFromImage(p)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, e.sd.calls())

	e.sessions.mu.Lock()
	require.Len(t, e.sessions.items, 1)
	saved := e.sessions.items[0].imageBase64
	e.sessions.mu.Unlock()
	assert.Equal(t, result.Image, saved, "session and response must hold the same composited image")

	for name, b64 := range map[string]string{"response": result.Image, "session": saved} {
		img := decodePNGBase64(t, b64)
		assert.Equal(t, w, img.Bounds().Dx(), name)
		assert.Equal(t, h, img.Bounds().Dy(), name)
		assert.Equal(t, emRed, emPixel(img, w/8, h/2), "%s: painted area gets the effect", name)
		assert.Equal(t, emGray, emPixel(img, w*3/8, h/2), "%s: painted area unchanged by the effect stays gray", name)
		assert.Equal(t, emGray, emPixel(img, w*5/8, h/2), "%s: middle outside the mask is original", name)
		assert.Equal(t, emGray, emPixel(img, w*7/8, h/2), "%s: effect outside the mask is dropped", name)
		assert.Equal(t, emGray, emPixel(img, w-2, 2), "%s: corner outside the mask is original", name)
	}
}

func TestGenerateFromImage_EffectMask_EffectRunsOnWholeFrame(t *testing.T) {
	t.Parallel()
	const w, h = 256, 256
	e := emSetup(t, emStripesPNG)
	src := makePNGBase64(t, w, h)
	p := e.params(src)
	p.MaskBase64 = emMaskBase64(t, w, h, image.Rect(0, 0, w/2, h))

	_, err := e.svc.GenerateFromImage(p)
	require.NoError(t, err)

	e.sd.mu.Lock()
	defer e.sd.mu.Unlock()
	require.Len(t, e.sd.img2img, 1)
	req := e.sd.img2img[0]
	require.Len(t, req.InitImages, 1)
	assert.Equal(t, src, req.InitImages[0], "effect is computed on the whole source frame")
	assert.Empty(t, req.Mask, "mask is not sent to SD: compositing is done afterwards")
	assert.Equal(t, w, req.Width)
	assert.Equal(t, h, req.Height)
}
