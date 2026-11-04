package adjust

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func grayImage(v uint8) *image.RGBA {
	return solid(3, 3, color.RGBA{v, v, v, 255})
}

func stepImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 5, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			v := uint8(10)
			if x >= 2 {
				v = 200
			}
			img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

func variedImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.SetRGBA(0, 0, color.RGBA{0, 128, 255, 255})
	img.SetRGBA(1, 0, color.RGBA{255, 255, 255, 255})
	img.SetRGBA(2, 0, color.RGBA{1, 2, 3, 255})
	img.SetRGBA(3, 0, color.RGBA{128, 64, 200, 255})
	img.SetRGBA(0, 1, color.RGBA{0, 0, 0, 255})
	img.SetRGBA(1, 1, color.RGBA{200, 100, 50, 255})
	img.SetRGBA(2, 1, color.RGBA{0, 0, 0, 0})
	img.SetRGBA(3, 1, color.RGBA{254, 253, 252, 255})
	return img
}

func assertPixelsEqual(t *testing.T, want, got *image.RGBA) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, want.Bounds(), got.Bounds())
	for i := range want.Pix {
		if want.Pix[i] != got.Pix[i] {
			assert.Failf(t, "images differ", "pix[%d]: want %d, got %d", i, want.Pix[i], got.Pix[i])
			return
		}
	}
}

func TestApply_Identity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
	}{
		{"zero value", Params{}},
		{"explicit defaults", Params{BlackPoint: 0, WhitePoint: 255, Gamma: 1}},
		{"neutral sliders", Params{Brightness: 0, Contrast: 0, Saturation: 0, Temperature: 0, Sharpen: 0, Vignette: 0}},
		{"unset gamma and white point", Params{Gamma: 0, WhitePoint: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := variedImage()
			out := Apply(src, tt.params)
			assertPixelsEqual(t, src, out)
		})
	}
}

func TestApply_Identity_NonZeroOrigin(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(1, 1, 4, 3))
	for y := 1; y < 3; y++ {
		for x := 1; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 60), uint8(y * 90), 128, 255})
		}
	}
	out := Apply(img, Params{})
	assertPixelsEqual(t, img, out)
}

func TestApply_Levels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		in     uint8
		want   uint8
	}{
		{"black point clips below", Params{BlackPoint: 50}, 49, 0},
		{"black point at point", Params{BlackPoint: 50}, 50, 0},
		{"black point rescales above", Params{BlackPoint: 50}, 100, 62},
		{"white point clips above", Params{WhitePoint: 200}, 201, 255},
		{"white point at point", Params{WhitePoint: 200}, 200, 255},
		{"white point rescales below", Params{WhitePoint: 200}, 150, 191},
		{"gamma 2 brightens mids", Params{Gamma: 2}, 64, 128},
		{"gamma 0.5 darkens mids", Params{Gamma: 0.5}, 64, 16},
		{"gamma leaves black", Params{Gamma: 2}, 0, 0},
		{"gamma leaves white", Params{Gamma: 2}, 255, 255},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := Apply(grayImage(tt.in), tt.params)
			c := out.RGBAAt(1, 1)
			assert.Equal(t, tt.want, c.R)
			assert.Equal(t, tt.want, c.G)
			assert.Equal(t, tt.want, c.B)
		})
	}
}

func TestApply_Brightness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		in     color.RGBA
		want   color.RGBA
	}{
		{"plus 50", Params{Brightness: 50}, color.RGBA{100, 150, 200, 255}, color.RGBA{150, 200, 250, 255}},
		{"plus 50 clamps at 255", Params{Brightness: 50}, color.RGBA{210, 230, 255, 255}, color.RGBA{255, 255, 255, 255}},
		{"minus 50", Params{Brightness: -50}, color.RGBA{40, 100, 200, 255}, color.RGBA{0, 50, 150, 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := Apply(solid(2, 2, tt.in), tt.params)
			assert.Equal(t, tt.want, out.RGBAAt(1, 1))
		})
	}
}

func TestApply_Contrast(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		in     uint8
		want   uint8
	}{
		{"plus 50 raises highs", Params{Contrast: 50}, 200, 235},
		{"plus 50 lowers lows", Params{Contrast: 50}, 50, 12},
		{"plus 50 keeps midpoint", Params{Contrast: 50}, 128, 128},
		{"minus 50 compresses highs", Params{Contrast: -50}, 200, 177},
		{"minus 50 compresses lows", Params{Contrast: -50}, 50, 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := Apply(grayImage(tt.in), tt.params)
			assert.Equal(t, tt.want, out.RGBAAt(1, 1).R)
		})
	}
}

func TestApply_Saturation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		in     color.RGBA
		want   color.RGBA
	}{
		{"minus 100 grayscale", Params{Saturation: -100}, color.RGBA{100, 150, 200, 255}, color.RGBA{141, 141, 141, 255}},
		{"plus 50 boosts distance from luma", Params{Saturation: 50}, color.RGBA{200, 100, 50, 255}, color.RGBA{238, 88, 13, 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := Apply(solid(2, 2, tt.in), tt.params)
			assert.Equal(t, tt.want, out.RGBAAt(1, 1))
		})
	}
}

func TestApply_Temperature(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		want   color.RGBA
	}{
		{"warm raises r lowers b", Params{Temperature: 100}, color.RGBA{120, 100, 80, 255}},
		{"cool lowers r raises b", Params{Temperature: -100}, color.RGBA{80, 100, 120, 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := Apply(solid(2, 2, color.RGBA{100, 100, 100, 255}), tt.params)
			assert.Equal(t, tt.want, out.RGBAAt(1, 1))
		})
	}
}

func TestApply_Balance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		want   color.RGBA
	}{
		{"green only", Params{BalanceG: 50}, color.RGBA{100, 110, 100, 255}},
		{"red negative", Params{BalanceR: -100}, color.RGBA{80, 100, 100, 255}},
		{"blue only", Params{BalanceB: 50}, color.RGBA{100, 100, 110, 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := Apply(solid(2, 2, color.RGBA{100, 100, 100, 255}), tt.params)
			assert.Equal(t, tt.want, out.RGBAAt(1, 1))
		})
	}
}

func TestApply_Sharpen(t *testing.T) {
	t.Parallel()
	out := Apply(stepImage(), Params{Sharpen: 50})
	assert.Equal(t, uint8(0), out.RGBAAt(1, 1).R)
	assert.Equal(t, uint8(232), out.RGBAAt(2, 1).R)
	assert.Equal(t, uint8(200), out.RGBAAt(3, 1).R)
}

func TestApply_Sharpen_ZeroNeutral(t *testing.T) {
	t.Parallel()
	src := stepImage()
	out := Apply(src, Params{Sharpen: 0})
	assertPixelsEqual(t, src, out)
}

func TestApply_Vignette(t *testing.T) {
	t.Parallel()
	src := solid(5, 5, color.RGBA{200, 100, 50, 255})
	out := Apply(src, Params{Vignette: 50})
	assert.Equal(t, color.RGBA{200, 100, 50, 255}, out.RGBAAt(2, 2))
	assert.Equal(t, color.RGBA{100, 50, 25, 255}, out.RGBAAt(0, 0))
}

func TestApply_Vignette_ZeroNeutral(t *testing.T) {
	t.Parallel()
	src := solid(5, 5, color.RGBA{200, 100, 50, 255})
	out := Apply(src, Params{Vignette: 0})
	assertPixelsEqual(t, src, out)
}

func TestApply_AlphaPreserved(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{64, 32, 100, 128})
	img.SetRGBA(1, 0, color.RGBA{10, 20, 30, 0})
	out := Apply(img, Params{Brightness: 20})
	assert.Equal(t, uint8(128), out.RGBAAt(0, 0).A)
	assert.NotEqual(t, img.RGBAAt(0, 0), out.RGBAAt(0, 0))
	assert.Equal(t, color.RGBA{0, 0, 0, 0}, out.RGBAAt(1, 0))
}

func TestApply_ParamClamps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		sameAs Params
	}{
		{"brightness over range", Params{Brightness: 1000}, Params{Brightness: 100}},
		{"contrast over range", Params{Contrast: 999}, Params{Contrast: 100}},
		{"gamma over range", Params{Gamma: 10}, Params{Gamma: 3}},
		{"gamma negative treated as unset", Params{Gamma: -5}, Params{}},
		{"white below black swapped", Params{BlackPoint: 200, WhitePoint: 10}, Params{BlackPoint: 10, WhitePoint: 200}},
		{"black equals white widened", Params{BlackPoint: 100, WhitePoint: 100}, Params{BlackPoint: 100, WhitePoint: 101}},
		{"saturation over range", Params{Saturation: 500}, Params{Saturation: 100}},
		{"temperature over range", Params{Temperature: 9999}, Params{Temperature: 100}},
		{"balance over range", Params{BalanceR: -999}, Params{BalanceR: -100}},
		{"sharpen over range", Params{Sharpen: 1000}, Params{Sharpen: 100}},
		{"vignette over range", Params{Vignette: 500}, Params{Vignette: 100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := stepImage()
			assertPixelsEqual(t, Apply(src, tt.sameAs), Apply(src, tt.params))
		})
	}
}

func TestApply_LevelsBlackEqualsWhite(t *testing.T) {
	t.Parallel()
	params := Params{BlackPoint: 100, WhitePoint: 100}
	assert.Equal(t, uint8(0), Apply(grayImage(100), params).RGBAAt(1, 1).R)
	assert.Equal(t, uint8(255), Apply(grayImage(150), params).RGBAAt(1, 1).R)
}

func TestApply_SinglePixelImage(t *testing.T) {
	t.Parallel()
	src := solid(1, 1, color.RGBA{100, 150, 200, 255})
	out := Apply(src, Params{Sharpen: 100, Vignette: 100})
	assert.Equal(t, color.RGBA{100, 150, 200, 255}, out.RGBAAt(0, 0))
}

func TestIsNeutral(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params Params
		want   bool
	}{
		{"zero value", Params{}, true},
		{"explicit defaults", Params{BlackPoint: 0, WhitePoint: 255, Gamma: 1}, true},
		{"brightness", Params{Brightness: 1}, false},
		{"contrast", Params{Contrast: 1}, false},
		{"black point", Params{BlackPoint: 1}, false},
		{"white point", Params{WhitePoint: 254}, false},
		{"gamma", Params{Gamma: 1.5}, false},
		{"saturation", Params{Saturation: 1}, false},
		{"temperature", Params{Temperature: 1}, false},
		{"balance r", Params{BalanceR: 1}, false},
		{"balance g", Params{BalanceG: 1}, false},
		{"balance b", Params{BalanceB: 1}, false},
		{"sharpen", Params{Sharpen: 1}, false},
		{"vignette", Params{Vignette: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.params.IsNeutral())
		})
	}
}
