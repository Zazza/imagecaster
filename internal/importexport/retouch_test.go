package importexport

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/faceguard"
)

func rtGradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(40 + x%120), G: uint8(30 + y%100), B: uint8(60 + (x+y)%80), A: 255})
		}
	}
	return img
}

func rtB64(t *testing.T, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func rtPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestProcessExportImage_RetouchDefaultsKeepOutput(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	src := rtB64(t, rtGradient(64, 48))
	base, err := svc.ProcessExportImage(ExportImageParams{ImageBase64: src, Format: "png"})
	require.NoError(t, err)
	tests := []struct {
		name string
		p    ExportImageParams
	}{
		{name: "zero retouch", p: ExportImageParams{ImageBase64: src, Format: "png", Retouch: &faceguard.Retouch{}}},
		{name: "local retouch without points", p: ExportImageParams{ImageBase64: src, Format: "png", Retouch: &faceguard.Retouch{Teeth: true, EyeWhites: true}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.ProcessExportImage(tc.p)
			require.NoError(t, err)
			assert.Equal(t, base.Data, got.Data)
		})
	}
}

func TestProcessExportImage_AdjustBeforeRetouch(t *testing.T) {
	t.Parallel()
	src := rtGradient(64, 48)
	adj := adjust.Params{Brightness: 30, Contrast: -40, WhitePoint: 255, Gamma: 1}
	light := faceguard.Retouch{Light: true}

	adjusted := adjust.Apply(src, adj)
	want, err := faceguard.ApplyRetouch(adjusted, nil, light)
	require.NoError(t, err)
	lightFirst, err := faceguard.ApplyRetouch(src, nil, light)
	require.NoError(t, err)
	reversed := adjust.Apply(lightFirst, adj)
	require.NotEqual(t, rtPNG(t, want), rtPNG(t, reversed))

	got, err := (&Service{}).ProcessExportImage(ExportImageParams{ImageBase64: rtB64(t, src), Format: "png", Adjust: &adj, Retouch: &light})
	require.NoError(t, err)
	assert.Equal(t, rtPNG(t, want), got.Data)
}

func TestProcessExportImage_RetouchRejectsBadValues(t *testing.T) {
	t.Parallel()
	src := rtB64(t, rtGradient(32, 32))
	_, err := (&Service{}).ProcessExportImage(ExportImageParams{ImageBase64: src, Format: "png", Retouch: &faceguard.Retouch{FaceWidth: 40, Light: true}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retouch")
}

func rtRing(cx, cy, rx, ry, from, to float64, n int, closed bool) []faceguard.Point {
	out := make([]faceguard.Point, 0, n)
	d := float64(n - 1)
	if closed {
		d = float64(n)
	}
	for i := 0; i < n; i++ {
		a := from + (to-from)*float64(i)/d
		out = append(out, faceguard.Point{X: cx + rx*math.Cos(a), Y: cy + ry*math.Sin(a)})
	}
	return out
}

func rtFace() []faceguard.Point {
	pts := rtRing(0.5, 0.40, 0.25, 0.38, math.Pi, 0, 17, false)
	for i := 0; i < 5; i++ {
		pts = append(pts, faceguard.Point{X: 0.30 + 0.0375*float64(i), Y: 0.33})
	}
	for i := 0; i < 5; i++ {
		pts = append(pts, faceguard.Point{X: 0.55 + 0.0375*float64(i), Y: 0.33})
	}
	for i := 0; i < 4; i++ {
		pts = append(pts, faceguard.Point{X: 0.5, Y: 0.41 + 0.035*float64(i)})
	}
	for i := 0; i < 5; i++ {
		pts = append(pts, faceguard.Point{X: 0.46 + 0.02*float64(i), Y: 0.55})
	}
	for _, cx := range []float64{0.38, 0.62} {
		pts = append(pts, faceguard.Point{X: cx - 0.05, Y: 0.40}, faceguard.Point{X: cx - 0.02, Y: 0.385}, faceguard.Point{X: cx + 0.02, Y: 0.385},
			faceguard.Point{X: cx + 0.05, Y: 0.40}, faceguard.Point{X: cx + 0.02, Y: 0.415}, faceguard.Point{X: cx - 0.02, Y: 0.415})
	}
	pts = append(pts, rtRing(0.5, 0.64, 0.09, 0.045, math.Pi, 3*math.Pi, 12, true)...)
	pts = append(pts, rtRing(0.5, 0.64, 0.06, 0.005, math.Pi, 3*math.Pi, 8, true)...)
	return append(pts, faceguard.Point{X: 0.38, Y: 0.40}, faceguard.Point{X: 0.62, Y: 0.40})
}

func rtChangedBox(t *testing.T, before, after image.Image) [4]float64 {
	t.Helper()
	b := before.Bounds()
	x0, y0, x1, y1 := b.Dx(), b.Dy(), -1, -1
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r1, g1, b1, _ := before.At(x, y).RGBA()
			r2, g2, b2, _ := after.At(x, y).RGBA()
			if r1>>8 != r2>>8 || g1>>8 != g2>>8 || b1>>8 != b2>>8 {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
		}
	}
	require.GreaterOrEqual(t, x1, 0, "retouch changed nothing")
	w, h := float64(b.Dx()), float64(b.Dy())
	return [4]float64{float64(x0) / w, float64(y0) / h, float64(x1) / w, float64(y1) / h}
}

func TestApplyRetouch_PointsAreFractionsOfAnySize(t *testing.T) {
	t.Parallel()
	pts := rtFace()
	require.Len(t, pts, 70)
	r := &faceguard.Retouch{UnderEyes: true}
	var boxes [][4]float64
	for _, size := range []int{300, 1200} {
		src := image.NewRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				c := color.RGBA{R: 170, G: 120, B: 100, A: 255}
				fx, fy := float64(x)/float64(size), float64(y)/float64(size)
				if fy > 0.415 && fy < 0.46 && ((fx > 0.33 && fx < 0.43) || (fx > 0.57 && fx < 0.67)) {
					c = color.RGBA{R: 110, G: 75, B: 65, A: 255}
				}
				src.SetRGBA(x, y, c)
			}
		}
		out, err := applyRetouch(src, r, pts)
		require.NoError(t, err)
		boxes = append(boxes, rtChangedBox(t, src, out))
	}
	for i := range boxes[0] {
		assert.InDelta(t, boxes[0][i], boxes[1][i], 0.03, "under-eye area lands at the same place after a 4x upscale (side %d)", i)
	}
	assert.Less(t, boxes[0][1], 0.55, "change is around the eyes, not the mouth")
	assert.Greater(t, boxes[0][3], 0.40, "change is below the eye line")
}
