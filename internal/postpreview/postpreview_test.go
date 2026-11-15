package postpreview

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/finish"
)

func testPNG(t *testing.T, w, h int, base uint8) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: base + uint8(x%50), G: uint8(y % 200), B: 255 - base, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodedSize(t *testing.T, b64 string) image.Point {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	return image.Pt(cfg.Width, cfg.Height)
}

func TestPost_CacheHitMatchesUncached(t *testing.T) {
	t.Parallel()
	src := testPNG(t, 1200, 600, 10)
	p := adjust.Params{Brightness: 20, Contrast: 10}
	f := finish.Params{Ratio: "1:1"}

	cached := New(DefaultCacheSize)
	first, err := cached.Post(src, p, f, 900)
	require.NoError(t, err)
	second, err := cached.Post(src, p, f, 900)
	require.NoError(t, err)
	fresh, err := New(DefaultCacheSize).Post(src, p, f, 900)
	require.NoError(t, err)

	assert.Equal(t, fresh, first)
	assert.Equal(t, fresh, second)
	assert.Equal(t, 1, cached.decodes)
}

func TestPost_DifferentParamsOnCachedImage(t *testing.T) {
	t.Parallel()
	src := testPNG(t, 1200, 600, 10)
	svc := New(DefaultCacheSize)
	_, err := svc.Post(src, adjust.Params{}, finish.Params{}, 900)
	require.NoError(t, err)

	got, err := svc.Post(src, adjust.Params{Saturation: -100}, finish.Params{}, 900)
	require.NoError(t, err)
	want, err := New(DefaultCacheSize).Post(src, adjust.Params{Saturation: -100}, finish.Params{}, 900)
	require.NoError(t, err)

	assert.Equal(t, want, got)
	assert.Equal(t, 1, svc.decodes)
}

func TestPost_DifferentInputsNotConfused(t *testing.T) {
	t.Parallel()
	a := testPNG(t, 1200, 600, 10)
	b := testPNG(t, 1000, 1000, 120)
	svc := New(DefaultCacheSize)

	outA, err := svc.Post(a, adjust.Params{}, finish.Params{}, 900)
	require.NoError(t, err)
	outB, err := svc.Post(b, adjust.Params{}, finish.Params{}, 900)
	require.NoError(t, err)
	outA2, err := svc.Post(a, adjust.Params{}, finish.Params{}, 900)
	require.NoError(t, err)

	wantB, err := New(1).Post(b, adjust.Params{}, finish.Params{}, 900)
	require.NoError(t, err)
	assert.NotEqual(t, outA, outB)
	assert.Equal(t, outA, outA2)
	assert.Equal(t, wantB, outB)
	assert.Equal(t, image.Pt(900, 450), decodedSize(t, outA))
	assert.Equal(t, image.Pt(900, 900), decodedSize(t, outB))
	assert.Equal(t, 2, svc.decodes)
}

func TestPost_MaxWidthIsPartOfKey(t *testing.T) {
	t.Parallel()
	src := testPNG(t, 1200, 600, 10)
	svc := New(DefaultCacheSize)

	small, err := svc.Post(src, adjust.Params{}, finish.Params{}, 300)
	require.NoError(t, err)
	big, err := svc.Post(src, adjust.Params{}, finish.Params{}, 900)
	require.NoError(t, err)

	assert.Equal(t, image.Pt(300, 150), decodedSize(t, small))
	assert.Equal(t, image.Pt(900, 450), decodedSize(t, big))
}

func TestPost_EvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	a := testPNG(t, 64, 64, 1)
	b := testPNG(t, 64, 64, 2)
	c := testPNG(t, 64, 64, 3)
	svc := New(2)
	for _, src := range []string{a, b, a, c, a} {
		_, err := svc.Post(src, adjust.Params{}, finish.Params{}, 900)
		require.NoError(t, err)
	}
	assert.Equal(t, 3, svc.decodes)
	_, err := svc.Post(b, adjust.Params{}, finish.Params{}, 900)
	require.NoError(t, err)
	assert.Equal(t, 4, svc.decodes)
}

func TestAdjust_EqualsPostWithNeutralFinish(t *testing.T) {
	t.Parallel()
	src := testPNG(t, 1200, 600, 10)
	p := adjust.Params{Temperature: 30}
	got, err := New(1).Adjust(src, p, 900)
	require.NoError(t, err)
	want, err := New(1).Post(src, p, finish.Params{}, 900)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestPost_Errors(t *testing.T) {
	t.Parallel()
	svc := New(1)
	_, err := svc.Post("", adjust.Params{}, finish.Params{}, 900)
	assert.ErrorContains(t, err, "no image provided")
	_, err = svc.Post("!!!", adjust.Params{}, finish.Params{}, 900)
	assert.ErrorContains(t, err, "decode base64")
	_, err = svc.Post(testPNG(t, 64, 64, 1), adjust.Params{}, finish.Params{Ratio: "bogus"}, 900)
	assert.Error(t, err)
	assert.Equal(t, 1, svc.decodes)
}

func TestPost_ConcurrentCallsSafe(t *testing.T) {
	t.Parallel()
	srcs := []string{testPNG(t, 300, 200, 1), testPNG(t, 300, 200, 90)}
	svc := New(1)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Post(srcs[i%2], adjust.Params{Brightness: float64(i)}, finish.Params{}, 900)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

func TestAuto_UsesFullImage(t *testing.T) {
	t.Parallel()
	stats, err := New(1).Auto(testPNG(t, 64, 64, 10))
	require.NoError(t, err)
	assert.Greater(t, stats.WhitePoint, stats.BlackPoint)
	_, err = New(1).Auto("")
	assert.ErrorContains(t, err, "no image provided")
}
