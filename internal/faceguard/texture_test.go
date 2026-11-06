package faceguard

import (
	"encoding/base64"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tmCell = 4.0

func tmHash(seed int64, i, j int) float64 {
	h := uint64(seed)*0x9E3779B97F4A7C15 ^ uint64(int64(i))*0xBF58476D1CE4E5B9 ^ uint64(int64(j))*0x94D049BB133111EB
	h ^= h >> 31
	h *= 0xD6E8FEB86659FD93
	h ^= h >> 32
	return float64(h%2001)/1000 - 1
}

func tmNoise(seed int64, x, y float64) float64 {
	gx, gy := x/tmCell, y/tmCell
	i, j := int(math.Floor(gx)), int(math.Floor(gy))
	fx, fy := gx-float64(i), gy-float64(j)
	a := tmHash(seed, i, j)*(1-fx) + tmHash(seed, i+1, j)*fx
	b := tmHash(seed, i, j+1)*(1-fx) + tmHash(seed, i+1, j+1)*fx
	return a*(1-fy) + b*fy
}

func tmTextured(w, h int, seed int64, toOrig func(px, py float64) (float64, float64), base, amp float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ox, oy := toOrig((float64(x)+0.5)/float64(w), (float64(y)+0.5)/float64(h))
			v := uint8(math.Max(0, math.Min(255, math.Round(base+amp*tmNoise(seed, ox*fgW, oy*fgH)))))
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

func tmSame(px, py float64) (float64, float64) { return px, py }

func TestTextureMatch(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	const scale, dx = 1.1, 0.03
	moved := fgTransform(pts, scale, dx)
	back := func(px, py float64) (float64, float64) {
		return 0.5 + (px-0.5-dx)/scale, 0.5 + (py-0.5)/scale
	}

	tests := []struct {
		name    string
		orig    *image.RGBA
		edited  *image.RGBA
		editPts []Point
		check   func(t *testing.T, r float64)
	}{
		{
			name:    "TM1 identical textured pictures: about 1",
			orig:    tmTextured(fgW, fgH, 1, tmSame, 128, 50),
			edited:  tmTextured(fgW, fgH, 1, tmSame, 128, 50),
			editPts: pts,
			check:   func(t *testing.T, r float64) { assert.InDelta(t, 1.0, r, 0.02) },
		},
		{
			name:    "TM1 same texture in another tone: about 1",
			orig:    tmTextured(fgW, fgH, 1, tmSame, 128, 50),
			edited:  tmTextured(fgW, fgH, 1, tmSame, 170, 40),
			editPts: pts,
			check:   func(t *testing.T, r float64) { assert.InDelta(t, 1.0, r, 0.05) },
		},
		{
			name:    "TM2 same texture moved and scaled with the points: above 0.5",
			orig:    tmTextured(fgW, fgH, 1, tmSame, 128, 50),
			edited:  tmTextured(fgW, fgH, 1, back, 150, 45),
			editPts: moved,
			check:   func(t *testing.T, r float64) { assert.Greater(t, r, 0.5) },
		},
		{
			name:    "TM2 orig twice the size: above 0.5",
			orig:    tmTextured(2*fgW, 2*fgH, 1, tmSame, 128, 50),
			edited:  tmTextured(fgW, fgH, 1, tmSame, 128, 50),
			editPts: pts,
			check:   func(t *testing.T, r float64) { assert.Greater(t, r, 0.5) },
		},
		{
			name:    "TM3 other noise: below 0.1 in absolute value",
			orig:    tmTextured(fgW, fgH, 1, tmSame, 128, 50),
			edited:  tmTextured(fgW, fgH, 2, tmSame, 128, 50),
			editPts: pts,
			check:   func(t *testing.T, r float64) { assert.Less(t, math.Abs(r), 0.1) },
		},
		{
			name:    "TM3 other noise, moved points: below 0.1 in absolute value",
			orig:    tmTextured(fgW, fgH, 1, tmSame, 128, 50),
			edited:  tmTextured(fgW, fgH, 3, back, 128, 50),
			editPts: moved,
			check:   func(t *testing.T, r float64) { assert.Less(t, math.Abs(r), 0.1) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, err := TextureMatch(tt.orig, tt.edited, pts, tt.editPts)
			require.NoError(t, err)
			tt.check(t, r)

			r64, err := TextureMatchBase64(fgB64(t, tt.orig), fgB64(t, tt.edited), pts, tt.editPts)
			require.NoError(t, err)
			assert.InDelta(t, r, r64, 1e-6, "base64 variant")
		})
	}
}

func TestTextureMatch_Errors(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	orig := tmTextured(fgW, fgH, 1, tmSame, 128, 50)
	edited := tmTextured(fgW, fgH, 1, tmSame, 128, 50)

	pointCases := []struct {
		name    string
		origPts []Point
		editPts []Point
	}{
		{name: "TM4 edit 67 points", origPts: pts, editPts: pts[:67]},
		{name: "TM4 orig 67 points", origPts: pts[:67], editPts: pts},
		{name: "TM4 orig without points", origPts: nil, editPts: pts},
		{name: "TM4 edit without points", origPts: pts, editPts: nil},
	}
	for _, tt := range pointCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := TextureMatch(orig, edited, tt.origPts, tt.editPts)
			require.Error(t, err)
			_, err = TextureMatchBase64(fgB64(t, orig), fgB64(t, edited), tt.origPts, tt.editPts)
			require.Error(t, err)
		})
	}

	t.Run("broken base64 or not an image", func(t *testing.T) {
		t.Parallel()
		good := fgB64(t, orig)
		for _, bad := range [][2]string{
			{"!!!not-base64!!!", good},
			{good, "!!!not-base64!!!"},
			{base64.StdEncoding.EncodeToString([]byte("hello")), good},
			{good, ""},
		} {
			_, err := TextureMatchBase64(bad[0], bad[1], pts, pts)
			require.Error(t, err)
		}
	})
}
