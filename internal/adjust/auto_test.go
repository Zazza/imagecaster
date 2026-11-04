package adjust

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
)

func grayGradient(from, to, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := from
			if w > 1 {
				v = from + (to-from)*x/(w-1)
			}
			img.SetRGBA(x, y, color.RGBA{uint8(v), uint8(v), uint8(v), 255})
		}
	}
	return img
}

func TestAnalyzeAuto(t *testing.T) {
	t.Parallel()
	neutral := AutoStats{BlackPoint: 0, WhitePoint: 255, Gamma: 1, Temperature: 0}
	tests := []struct {
		name   string
		img    image.Image
		verify func(t *testing.T, s AutoStats)
	}{
		{
			name: "dark gradient lifts black point",
			img:  grayGradient(30, 90, 61, 5),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, 30.0, s.BlackPoint)
				assert.Equal(t, 175.0, s.WhitePoint)
				assert.Equal(t, 1.8, s.Gamma)
				assert.Equal(t, 0.0, s.Temperature)
			},
		},
		{
			name: "overexposed gradient pulls white point down",
			img:  grayGradient(200, 250, 51, 3),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, 80.0, s.BlackPoint)
				assert.Equal(t, 250.0, s.WhitePoint)
				assert.Less(t, s.WhitePoint, 255.0)
				assert.Equal(t, 0.6, s.Gamma)
				assert.Equal(t, 0.0, s.Temperature)
			},
		},
		{
			name: "warm frame gets negative temperature",
			img:  solid(4, 4, color.RGBA{115, 100, 100, 255}),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, -36.0, s.Temperature)
				assert.GreaterOrEqual(t, s.Temperature, -45.0)
				assert.LessOrEqual(t, s.Temperature, -35.0)
				assert.Equal(t, 0.0, s.BlackPoint)
				assert.Equal(t, 255.0, s.WhitePoint)
				assert.InDelta(t, 1.294, s.Gamma, 0.02)
			},
		},
		{
			name: "cold frame gets positive temperature",
			img:  solid(4, 4, color.RGBA{100, 100, 115, 255}),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, 36.0, s.Temperature)
				assert.Greater(t, s.Temperature, 0.0)
			},
		},
		{
			name: "neutral gradient stays neutral",
			img:  grayGradient(0, 255, 256, 3),
			verify: func(t *testing.T, s AutoStats) {
				assert.LessOrEqual(t, s.BlackPoint, 5.0)
				assert.GreaterOrEqual(t, s.WhitePoint, 250.0)
				assert.Equal(t, 0.0, s.BlackPoint)
				assert.Equal(t, 255.0, s.WhitePoint)
				assert.Equal(t, 1.0, s.Gamma)
				assert.LessOrEqual(t, s.Temperature, 3.0)
				assert.GreaterOrEqual(t, s.Temperature, -3.0)
			},
		},
		{
			name: "flat image keeps neutral levels",
			img:  solid(3, 3, color.RGBA{128, 128, 128, 255}),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, neutral, s)
			},
		},
		{
			name: "single pixel does not panic",
			img:  solid(1, 1, color.RGBA{100, 150, 200, 255}),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, 0.0, s.BlackPoint)
				assert.Equal(t, 255.0, s.WhitePoint)
				assert.InDelta(t, 0.855, s.Gamma, 0.01)
				assert.Equal(t, 60.0, s.Temperature)
			},
		},
		{
			name: "fully transparent image is neutral",
			img:  solid(3, 3, color.RGBA{0, 0, 0, 0}),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, neutral, s)
			},
		},
		{
			name: "nil image is neutral",
			img:  nil,
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, neutral, s)
			},
		},
		{
			name: "empty bounds are neutral",
			img:  image.NewRGBA(image.Rect(0, 0, 0, 0)),
			verify: func(t *testing.T, s AutoStats) {
				assert.Equal(t, neutral, s)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.verify(t, AnalyzeAuto(tt.img))
		})
	}
}

func TestAnalyzeAuto_TransparentPixelsExcluded(t *testing.T) {
	t.Parallel()
	opaque := grayGradient(100, 150, 51, 2)
	withTransparent := image.NewRGBA(image.Rect(0, 0, 51, 3))
	for x := 0; x < 51; x++ {
		v := uint8(100 + x)
		withTransparent.SetRGBA(x, 0, color.RGBA{v, v, v, 255})
		withTransparent.SetRGBA(x, 1, color.RGBA{255, 0, 0, 0})
		withTransparent.SetRGBA(x, 2, color.RGBA{v, v, v, 255})
	}
	assert.Equal(t, AnalyzeAuto(opaque), AnalyzeAuto(withTransparent))
}

func TestAnalyzeAuto_LargeImageSampling(t *testing.T) {
	t.Parallel()
	sampled := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			v := uint8((x / 2) % 256)
			sampled.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	full := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			v := uint8(x % 256)
			full.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	assert.Equal(t, AnalyzeAuto(full), AnalyzeAuto(sampled))
}
