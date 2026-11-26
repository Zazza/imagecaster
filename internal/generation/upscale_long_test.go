package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

type udRec struct {
	mu         sync.Mutex
	upscalers  []sd.Upscaler
	upscaleErr error
	nativeX4   bool
	upNames    []string
	upCalls    int
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
	}
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
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	img, _, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	return img.Bounds().Dx(), img.Bounds().Dy()
}

func udImage(t *testing.T, w, h int) string {
	return solidPNGBase64(t, w, h, workGray)
}

func TestUpscaleLong_ToTargetKeepsAspect(t *testing.T) {
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

			res, err := svc.UpscaleLong(udImage(t, tt.w, tt.h), tt.target)

			require.NoError(t, err)
			require.NotNil(t, res)
			gotW, gotH := udSize(t, res.Image)
			assert.Equal(t, tt.wantW, gotW)
			assert.Equal(t, tt.wantH, gotH)
			assert.Equal(t, tt.wantW, res.Width)
			assert.Equal(t, tt.wantH, res.Height)
			assert.True(t, res.Upscaled)
			assert.False(t, res.Resized)
			n, names := rec.upscaleCalls()
			require.Positive(t, n, "upscale goes through the server upscaler")
			assert.Equal(t, "RealESRGAN_x4plus", names[0], "quality upscaler is used")
		})
	}
}

func TestUpscaleLong_OddAspectLongSideExact(t *testing.T) {
	t.Parallel()
	svc, _ := udSetup(t, &udRec{upscalers: udQuality()})

	res, err := svc.UpscaleLong(udImage(t, 1000, 1337), 3840)

	require.NoError(t, err)
	w, h := udSize(t, res.Image)
	assert.Equal(t, 3840, h)
	assert.InDelta(t, 1000.0*3840/1337, float64(w), 1.0)
}

func TestUpscaleLong_UpscalerNativeScaleFittedToTarget(t *testing.T) {
	t.Parallel()
	svc, _ := udSetup(t, &udRec{upscalers: udQuality(), nativeX4: true})

	res, err := svc.UpscaleLong(udImage(t, 512, 768), 3840)

	require.NoError(t, err)
	w, h := udSize(t, res.Image)
	assert.Equal(t, 2560, w)
	assert.Equal(t, 3840, h)
}

func TestUpscaleLong_FallbackResizeMarkedAndTakesJPEG(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 200, 300)), nil))
	jpg := base64.StdEncoding.EncodeToString(buf.Bytes())
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

			res, err := svc.UpscaleLong(jpg, 3840)

			require.NoError(t, err)
			w, h := udSize(t, res.Image)
			assert.Equal(t, 2560, w)
			assert.Equal(t, 3840, h)
			assert.True(t, res.Upscaled)
			assert.True(t, res.Resized)
		})
	}
}

func TestUpscaleLong_NoUpscale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		w, h   int
		target int
	}{
		{name: "long side above target", w: 600, h: 3900, target: 3840},
		{name: "long side equals target", w: 3840, h: 300, target: 3840},
		{name: "target zero", w: 512, h: 768, target: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &udRec{upscalers: udQuality()}
			svc, _ := udSetup(t, rec)
			src := udImage(t, tt.w, tt.h)

			res, err := svc.UpscaleLong(src, tt.target)

			require.NoError(t, err)
			assert.Equal(t, src, res.Image, "image is returned as is")
			assert.Equal(t, tt.w, res.Width)
			assert.Equal(t, tt.h, res.Height)
			assert.False(t, res.Upscaled)
			n, _ := rec.upscaleCalls()
			assert.Zero(t, n)
		})
	}
}

func TestUpscaleLong_EmptyImage(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, _ := udSetup(t, rec)

	_, err := svc.UpscaleLong("", 3840)

	require.Error(t, err)
	n, _ := rec.upscaleCalls()
	assert.Zero(t, n)
}

func TestUpscaleLong_InterruptedUpscalePropagates(t *testing.T) {
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
			svc, _ := udSetup(t, &udRec{upscalers: udQuality(), upscaleErr: tt.err})

			_, err := svc.UpscaleLong(udImage(t, 1024, 1536), 3840)

			require.Error(t, err)
			assert.True(t, IsInterrupted(err), "got %v", err)
		})
	}
}

func TestUpscaleLong_QueueCancelledBeforeStart(t *testing.T) {
	t.Parallel()
	rec := &udRec{upscalers: udQuality()}
	svc, m := udSetup(t, rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q, _ := svc.ForQueue(ctx, m, nil)

	_, err := q.UpscaleLong(udImage(t, 1024, 1536), 3840)

	require.Error(t, err)
	assert.True(t, IsInterrupted(err), "got %v", err)
	n, _ := rec.upscaleCalls()
	assert.Zero(t, n)
}

func TestUpscaleLongStrict_NoResizeFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		rec  *udRec
	}{
		{"no neural upscaler", &udRec{}},
		{"resize only", &udRec{upscalers: []sd.Upscaler{{Name: "Lanczos"}}}},
		{"neural failure", &udRec{upscalers: udQuality(), upscaleErr: errors.New("neural unavailable")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := udSetup(t, tc.rec)
			out, err := svc.UpscaleLongStrict(udImage(t, 32, 48), 1920)
			require.Error(t, err)
			require.Nil(t, out, "a failed neural upscale must not return a resized substitute")
		})
	}
}

func TestUpscaleLongStrict_LevelsAndAspect(t *testing.T) {
	for _, target := range []int{1920, 2560, 3840} {
		for _, shape := range []struct {
			name string
			w, h int
		}{{"portrait", 30, 40}, {"square", 40, 40}, {"landscape", 40, 30}} {
			t.Run(fmt.Sprintf("%s/%d", shape.name, target), func(t *testing.T) {
				rec := &udRec{upscalers: udQuality()}
				svc, _ := udSetup(t, rec)
				out, err := svc.UpscaleLongStrict(udImage(t, shape.w, shape.h), target)
				require.NoError(t, err)
				w, h := udSize(t, out.Image)
				require.Equal(t, target, max(w, h))
				require.Equal(t, shape.w*h, shape.h*w)
				require.True(t, out.Upscaled)
				require.False(t, out.Resized)
			})
		}
	}
}

func TestUpscaleLongStrict_AlreadyLargeLeavesOriginal(t *testing.T) {
	rec := &udRec{upscalers: udQuality(), upscaleErr: errors.New("must not run")}
	svc, m := udSetup(t, rec)
	m.getUpscalers = func() ([]sd.Upscaler, error) {
		t.Fatal("already large image must not query neural service")
		return nil, nil
	}
	src := udImage(t, 1920, 16)
	out, err := svc.UpscaleLongStrict(src, 1920)
	require.NoError(t, err)
	require.Equal(t, src, out.Image)
	require.False(t, out.Upscaled)
	n, _ := rec.upscaleCalls()
	require.Zero(t, n)
}
