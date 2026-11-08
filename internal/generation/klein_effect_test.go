package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

var kePainted = color.NRGBA{R: 255, G: 0, B: 0, A: 255}

type keSD struct {
	mu        sync.Mutex
	img2img   []sd.Img2ImgRequest
	upscales  []upscaleCall
	upscalers []sd.Upscaler
	outW      int
	outH      int
}

func (r *keSD) mock(t *testing.T) *mockSD {
	return &mockSD{
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			r.mu.Lock()
			r.img2img = append(r.img2img, req)
			w, h := r.outW, r.outH
			r.mu.Unlock()
			if w == 0 || h == 0 {
				w, h = req.Width, req.Height
			}
			return &sd.Txt2ImgResponse{Images: []string{solidPNGBase64(t, w, h, kePainted)}}, nil
		},
		getUpscalers: func() ([]sd.Upscaler, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.upscalers, nil
		},
		upscale: func(in, upscaler string, scale float64) (string, error) {
			r.mu.Lock()
			r.upscales = append(r.upscales, upscaleCall{in: in, upscaler: upscaler, scale: scale})
			r.mu.Unlock()
			raw, err := base64.StdEncoding.DecodeString(in)
			if err != nil {
				return "", err
			}
			cfg, err := png.DecodeConfig(bytes.NewReader(raw))
			if err != nil {
				return "", err
			}
			w := int(math.Round(float64(cfg.Width) * scale))
			h := int(math.Round(float64(cfg.Height) * scale))
			return solidPNGBase64(t, w, h, kePainted), nil
		},
	}
}

func keRun(t *testing.T, r *keSD, srcW, srcH int) (*GenerateImageResult, error) {
	t.Helper()
	svc := newTestService(t, openTestDB(t), &mockLLM{}, r.mock(t))
	svc.ctx = context.Background()
	svc.sessions = &mockSessions{}
	return svc.KleinEffect(solidPNGBase64(t, srcW, srcH, workGray), "golden hour light", 0.6)
}

func keDecode(t *testing.T, b64 string) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	img, _, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	return img
}

func keCenter(img image.Image) color.NRGBA {
	b := img.Bounds()
	return color.NRGBAModel.Convert(img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)).(color.NRGBA)
}

func TestKleinEffect_SmallerResultBackToSourceSize(t *testing.T) {
	t.Parallel()
	neural := []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}}
	tests := []struct {
		name       string
		upscalers  []sd.Upscaler
		srcW, srcH int
		outW, outH int
	}{
		{name: "source over work cap, SD returns work size, neural upscaler", upscalers: neural, srcW: 3200, srcH: 2400},
		{name: "source over work cap, SD returns work size, no neural upscaler", upscalers: nil, srcW: 3200, srcH: 2400},
		{name: "SD returns half of source, neural upscaler", upscalers: neural, srcW: 2048, srcH: 1536, outW: 1024, outH: 768},
		{name: "SD returns half of source, no neural upscaler", upscalers: nil, srcW: 2048, srcH: 1536, outW: 1024, outH: 768},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &keSD{upscalers: tt.upscalers, outW: tt.outW, outH: tt.outH}

			result, err := keRun(t, r, tt.srcW, tt.srcH)

			require.NoError(t, err)
			require.NotNil(t, result)
			r.mu.Lock()
			require.Len(t, r.img2img, 1)
			req := r.img2img[0]
			r.mu.Unlock()
			if tt.outW == 0 {
				require.Less(t, req.Width, tt.srcW, "precondition: model works smaller than the source")
			}
			img := keDecode(t, result.Image)
			assert.Equal(t, tt.srcW, img.Bounds().Dx(), "result must be back at source width")
			assert.Equal(t, tt.srcH, img.Bounds().Dy(), "result must be back at source height")
			assert.Equal(t, kePainted, keCenter(img), "result must be the model output, not the source")
		})
	}
}

func TestKleinEffect_SameSizeNoUpscale(t *testing.T) {
	t.Parallel()
	r := &keSD{upscalers: []sd.Upscaler{{Name: qualityUpscalerPreferences[0]}}, outW: 512, outH: 512}

	result, err := keRun(t, r, 512, 512)

	require.NoError(t, err)
	require.NotNil(t, result)
	r.mu.Lock()
	assert.Len(t, r.img2img, 1)
	assert.Empty(t, r.upscales, "same size as source — no upscale")
	r.mu.Unlock()
	img := keDecode(t, result.Image)
	assert.Equal(t, 512, img.Bounds().Dx())
	assert.Equal(t, 512, img.Bounds().Dy())
	assert.Equal(t, kePainted, keCenter(img))
}
