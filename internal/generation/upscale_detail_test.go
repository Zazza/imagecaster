package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

var udWhite = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

type udRec struct {
	mu         sync.Mutex
	upscalers  []sd.Upscaler
	upscaleErr error
	nativeX4   bool
	upNames    []string
	upCalls    int
	reqs       []sd.Img2ImgRequest
	img2err    func(n int) error
	onImg2Img  func(n int)
}

func (r *udRec) mock(t *testing.T) *mockSD {
	return &mockSD{
		getUpscalers: func() ([]sd.Upscaler, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.upscalers, nil
		},
		upscale: func(in, upscaler string, scale float64) (string, error) {
			r.mu.Lock()
			r.upCalls++
			r.upNames = append(r.upNames, upscaler)
			err := r.upscaleErr
			x4 := r.nativeX4
			r.mu.Unlock()
			if err != nil {
				return "", err
			}
			w, h := udSize(t, in)
			if x4 {
				return solidPNGBase64(t, w*4, h*4, workGray), nil
			}
			return solidPNGBase64(t, int(math.Round(float64(w)*scale)), int(math.Round(float64(h)*scale)), workGray), nil
		},
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			r.mu.Lock()
			r.reqs = append(r.reqs, req)
			n := len(r.reqs)
			errFn, hook := r.img2err, r.onImg2Img
			r.mu.Unlock()
			if hook != nil {
				hook(n)
			}
			if errFn != nil {
				if err := errFn(n); err != nil {
					return nil, err
				}
			}
			return &sd.Txt2ImgResponse{Images: []string{solidPNGBase64(t, req.Width, req.Height, udWhite)}}, nil
		},
	}
}

func (r *udRec) img2imgReqs() []sd.Img2ImgRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sd.Img2ImgRequest(nil), r.reqs...)
}

func (r *udRec) upscaleCalls() (int, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.upCalls, append([]string(nil), r.upNames...)
}

func udSetup(t *testing.T, r *udRec) (*Service, *mockSD) {
	t.Helper()
	m := r.mock(t)
	return newTestService(t, openTestDB(t), &mockLLM{}, m), m
}

func udQuality() []sd.Upscaler {
	return []sd.Upscaler{{Name: "Lanczos"}, {Name: "RealESRGAN_x4plus", Scale: 4}}
}

func udSize(t *testing.T, b64 string) (int, int) {
	t.Helper()
	img := udDecode(t, b64)
	return img.Bounds().Dx(), img.Bounds().Dy()
}

func udDecode(t *testing.T, b64 string) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	img, _, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	return img
}

func udSeed(v int64) *int64 { return &v }

func udDetailParams(t *testing.T, w, h int) DetailParams {
	return DetailParams{
		ImageBase64:    solidPNGBase64(t, w, h, workGray),
		TargetLong:     3840,
		Detail:         0.25,
		Model:          "juggernautXL_ragnarokBy",
		Prompt:         "portrait of anna, studio light <lora:anna_lora_v1:1>",
		NegativePrompt: "lowres, blurry",
		Sampler:        "euler_a",
		Steps:          30,
		CfgScale:       5.5,
		Seed:           udSeed(12345),
	}
}

func TestUpscaleDetail_UpscaleToTargetKeepsAspect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		w, h         int
		target       int
		wantW, wantH int
	}{
		{name: "portrait 1024x1536 to 4k", w: 1024, h: 1536, target: 3840, wantW: 2560, wantH: 3840},
		{name: "landscape 1536x1024 to 4k", w: 1536, h: 1024, target: 3840, wantW: 3840, wantH: 2560},
		{name: "2x of 600x400", w: 600, h: 400, target: 1200, wantW: 1200, wantH: 800},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &udRec{upscalers: udQuality()}
			svc, _ := udSetup(t, rec)
			p := udDetailParams(t, tt.w, tt.h)
			p.TargetLong = tt.target
			p.Detail = 0

			res, err := svc.UpscaleDetail(p)

			require.NoError(t, err)
			require.NotNil(t, res)
			gotW, gotH := udSize(t, res.Image)
			assert.Equal(t, tt.wantW, gotW)
			assert.Equal(t, tt.wantH, gotH)
			assert.Equal(t, tt.wantW, res.Width)
			assert.Equal(t, tt.wantH, res.Height)
			assert.True(t, res.Upscaled)
			assert.False(t, res.Detailed)
			assert.Zero(t, res.Tiles)
			assert.Empty(t, rec.img2imgReqs(), "detail 0 — no redraw")
			n, names := rec.upscaleCalls()
			require.Positive(t, n, "upscale goes through the server upscaler")
			assert.Equal(t, "RealESRGAN_x4plus", names[0], "quality upscaler is used")
		})
	}
}

func TestUpscaleDetail_OddAspectLongSideExact(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 1000, 1337)
	p.Detail = 0

	res, err := svc.UpscaleDetail(p)

	require.NoError(t, err)
	w, h := udSize(t, res.Image)
	assert.Equal(t, 3840, h)
	assert.InDelta(t, 1000.0*3840/1337, float64(w), 1.0)
}

func TestUpscaleDetail_UpscalerNativeScaleFittedToTarget(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality(), nativeX4: true}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 512, 768)
	p.Detail = 0

	res, err := svc.UpscaleDetail(p)

	require.NoError(t, err)
	w, h := udSize(t, res.Image)
	assert.Equal(t, 2560, w)
	assert.Equal(t, 3840, h)
}

func TestUpscaleDetail_FallbackResize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rec  *udRec
	}{
		{name: "no upscaler listed", rec: &udRec{}},
		{name: "only a non-quality upscaler listed", rec: &udRec{upscalers: []sd.Upscaler{{Name: "Lanczos"}}}},
		{name: "upscaler fails", rec: &udRec{upscalers: udQuality(), upscaleErr: errors.New("esrgan out of memory")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := udSetup(t, tt.rec)
			p := udDetailParams(t, 1024, 1536)
			p.Detail = 0

			res, err := svc.UpscaleDetail(p)

			require.NoError(t, err)
			w, h := udSize(t, res.Image)
			assert.Equal(t, 2560, w)
			assert.Equal(t, 3840, h)
			assert.True(t, res.Upscaled)
		})
	}
}

func TestUpscaleDetail_AlreadyAtTargetNoUpscale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		w, h int
	}{
		{name: "long side above target", w: 600, h: 3900},
		{name: "long side equals target", w: 3840, h: 300},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &udRec{upscalers: udQuality()}
			svc, _ := udSetup(t, rec)
			p := udDetailParams(t, tt.w, tt.h)
			p.Detail = 0

			res, err := svc.UpscaleDetail(p)

			require.NoError(t, err)
			w, h := udSize(t, res.Image)
			assert.Equal(t, tt.w, w, "image is not shrunk")
			assert.Equal(t, tt.h, h, "image is not shrunk")
			assert.False(t, res.Upscaled)
			n, _ := rec.upscaleCalls()
			assert.Zero(t, n)
		})
	}
}

func TestUpscaleDetail_TargetZeroNoUpscale(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 512, 768)
	p.TargetLong = 0

	res, err := svc.UpscaleDetail(p)

	require.NoError(t, err)
	w, h := udSize(t, res.Image)
	assert.Equal(t, 512, w)
	assert.Equal(t, 768, h)
	assert.False(t, res.Upscaled)
	n, _ := rec.upscaleCalls()
	assert.Zero(t, n)
}

func TestUpscaleDetail_TiledRedraw(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 1024, 1536)

	res, err := svc.UpscaleDetail(p)

	require.NoError(t, err)
	reqs := rec.img2imgReqs()
	require.Len(t, reqs, 15, "2560x3840 in tiles of 1024 with overlap 128 is 3x5")
	for i, req := range reqs {
		assert.Equal(t, 1024, req.Width, "tile %d", i)
		assert.Equal(t, 1024, req.Height, "tile %d", i)
		require.Len(t, req.InitImages, 1, "tile %d", i)
		tw, th := udSize(t, req.InitImages[0])
		assert.Equal(t, 1024, tw, "tile %d init image", i)
		assert.Equal(t, 1024, th, "tile %d init image", i)
		require.NotNil(t, req.DenoisingStrength, "tile %d", i)
		assert.InDelta(t, 0.25, *req.DenoisingStrength, 1e-9, "tile %d", i)
		assert.Equal(t, "juggernautXL_ragnarokBy", req.Model.Checkpoint, "tile %d", i)
		assert.Equal(t, p.Prompt, req.Prompt, "tile %d: same prompt with the LoRA", i)
		assert.Equal(t, p.NegativePrompt, req.NegativePrompt, "tile %d", i)
		assert.InDelta(t, 5.5, req.CfgScale, 1e-9, "tile %d", i)
		require.NotNil(t, req.Seed, "tile %d", i)
		assert.Equal(t, int64(12345), *req.Seed, "tile %d", i)
	}
	w, h := udSize(t, res.Image)
	assert.Equal(t, 2560, w)
	assert.Equal(t, 3840, h)
	assert.Equal(t, 2560, res.Width)
	assert.Equal(t, 3840, res.Height)
	assert.True(t, res.Upscaled)
	assert.True(t, res.Detailed)
	assert.Equal(t, 15, res.Tiles)
}

func TestUpscaleDetail_TilesCoverWholeImage(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 1024, 1536)

	res, err := svc.UpscaleDetail(p)

	require.NoError(t, err)
	img := udDecode(t, res.Image)
	b := img.Bounds()
	xs := []int{2, 500, 896, 1024, 1280, 1700, 2000, b.Dx() - 3}
	ys := []int{2, 700, 896, 1024, 1800, 2700, 3300, b.Dy() - 3}
	for _, y := range ys {
		for _, x := range xs {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			assert.GreaterOrEqual(t, int(c.R), 230, "pixel (%d,%d) is not redrawn: %v", x, y, c)
		}
	}
}

func TestUpscaleDetail_SmallImageSingleTile(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 512, 768)
	p.TargetLong = 0
	p.Detail = 0.3

	res, err := svc.UpscaleDetail(p)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Len(t, rec.img2imgReqs(), 1)
	w, h := udSize(t, res.Image)
	assert.Equal(t, 512, w)
	assert.Equal(t, 768, h)
}

func TestUpscaleDetail_DetailWithoutModel(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 512, 768)
	p.Model = ""

	_, err := svc.UpscaleDetail(p)

	require.Error(t, err)
	assert.Empty(t, rec.img2imgReqs())
}

func TestUpscaleDetail_EmptyImage(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)
	p := udDetailParams(t, 64, 64)
	p.ImageBase64 = ""

	_, err := svc.UpscaleDetail(p)

	require.Error(t, err)
	assert.Empty(t, rec.img2imgReqs())
}

func TestUpscaleDetail_TileInterrupted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
	}{
		{name: "job cancelled", err: fmt.Errorf("sd job: %w", sd.ErrJobCancelled)},
		{name: "user cancel", err: fmt.Errorf("sd job: %w", sd.ErrUserCancel)},
		{name: "interrupted", err: ErrInterrupted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &udRec{upscalers: udQuality(), img2err: func(int) error { return tt.err }}
			svc, _ := udSetup(t, rec)

			_, err := svc.UpscaleDetail(udDetailParams(t, 1024, 1536))

			require.Error(t, err)
			assert.True(t, IsInterrupted(err), "got %v", err)
			assert.Len(t, rec.img2imgReqs(), 1, "no tiles after an interrupted one")
		})
	}
}

func TestUpscaleDetail_QueueCancelBetweenTiles(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, m := udSetup(t, rec)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec.onImg2Img = func(n int) {
		if n == 2 {
			cancel()
		}
	}
	q, _ := svc.ForQueue(ctx, m, nil)

	_, err := q.UpscaleDetail(udDetailParams(t, 1024, 1536))

	require.Error(t, err)
	assert.True(t, IsInterrupted(err), "got %v", err)
	assert.Len(t, rec.img2imgReqs(), 2, "no tiles after the queue job is cancelled")
}

func TestUpscaleDetail_QueueCancelledBeforeStart(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, m := udSetup(t, rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q, _ := svc.ForQueue(ctx, m, nil)

	_, err := q.UpscaleDetail(udDetailParams(t, 1024, 1536))

	require.Error(t, err)
	assert.True(t, IsInterrupted(err), "got %v", err)
	assert.Empty(t, rec.img2imgReqs())
}
