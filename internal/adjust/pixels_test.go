package adjust

import (
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func referenceApply(img image.Image, p Params) *image.RGBA {
	bounds := img.Bounds()
	dst := image.NewRGBA(bounds)
	lut, levelsActive := levelsLUT(p)
	brightness := clampF(p.Brightness, -100, 100)
	contrast := contrastFactor(p.Contrast)
	sat := 1 + clampF(p.Saturation, -100, 100)/100
	temp := clampF(p.Temperature, -100, 100)
	tGainR := 1 + temp*channelGainStep
	tGainB := 1 - temp*channelGainStep
	gainR := 1 + clampF(p.BalanceR, -100, 100)*channelGainStep
	gainG := 1 + clampF(p.BalanceG, -100, 100)*channelGainStep
	gainB := 1 + clampF(p.BalanceB, -100, 100)*channelGainStep
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			a := uint8(a16 >> 8)
			if a == 0 {
				continue
			}
			r, g, b := float64(r16>>8), float64(g16>>8), float64(b16>>8)
			if a < 255 {
				r, g, b = r*255/float64(a), g*255/float64(a), b*255/float64(a)
			}
			if levelsActive {
				r, g, b = lut[lutIndex(r)], lut[lutIndex(g)], lut[lutIndex(b)]
			}
			if brightness != 0 {
				r, g, b = r+brightness, g+brightness, b+brightness
			}
			if contrast != 1 {
				r, g, b = contrast*(r-128)+128, contrast*(g-128)+128, contrast*(b-128)+128
			}
			if sat != 1 {
				l := 0.299*r + 0.587*g + 0.114*b
				r, g, b = l+sat*(r-l), l+sat*(g-l), l+sat*(b-l)
			}
			if tGainR != 1 {
				r *= tGainR
			}
			if tGainB != 1 {
				b *= tGainB
			}
			if gainR != 1 {
				r *= gainR
			}
			if gainG != 1 {
				g *= gainG
			}
			if gainB != 1 {
				b *= gainB
			}
			cr, cg, cb := toUint8(r), toUint8(g), toUint8(b)
			if a < 255 {
				f := float64(a) / 255
				cr, cg, cb = toUint8(float64(cr)*f), toUint8(float64(cg)*f), toUint8(float64(cb)*f)
			}
			dst.SetRGBA(x, y, color.RGBA{cr, cg, cb, a})
		}
	}
	if p.Sharpen > 0 {
		dst = sharpenPass(dst, clampF(p.Sharpen, 0, 100)/100)
	}
	if p.Vignette > 0 {
		vignettePass(dst, clampF(p.Vignette, 0, 100)/100)
	}
	return dst
}

func referenceAnalyze(img image.Image) (hist [256]int, sumR, sumB float64, total int) {
	bounds := img.Bounds()
	stride := 1
	if bounds.Dx()*bounds.Dy() > autoSamplePixelLimit {
		stride = 2
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stride {
		for x := bounds.Min.X; x < bounds.Max.X; x += stride {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			a := uint8(a16 >> 8)
			if a == 0 {
				continue
			}
			r, g, b := float64(r16>>8), float64(g16>>8), float64(b16>>8)
			if a < 255 {
				scale := 255 / float64(a)
				r, g, b = r*scale, g*scale, b*scale
			}
			hist[lutIndex(0.299*r+0.587*g+0.114*b)]++
			sumR += r
			sumB += b
			total++
		}
	}
	return
}

func referenceAutoStats(img image.Image) AutoStats {
	hist, sumR, sumB, total := referenceAnalyze(img)
	if total == 0 {
		return neutralAutoStats()
	}
	black, white := autoLevels(&hist, total)
	return AutoStats{BlackPoint: black, WhitePoint: white, Gamma: autoGamma(&hist, total, black, white), Temperature: autoTemperature(sumR, sumB, total)}
}

func randomImages(rng *rand.Rand, r image.Rectangle) map[string]image.Image {
	rgba := image.NewRGBA(r)
	nrgba := image.NewNRGBA(r)
	gray := image.NewGray(r)
	ycc := image.NewYCbCr(r, image.YCbCrSubsampleRatio420)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			a := uint8(rng.Intn(256))
			if rng.Intn(3) == 0 {
				a = 255
			}
			pr, pg, pb := uint8(rng.Intn(int(a)+1)), uint8(rng.Intn(int(a)+1)), uint8(rng.Intn(int(a)+1))
			rgba.SetRGBA(x, y, color.RGBA{pr, pg, pb, a})
			nrgba.SetNRGBA(x, y, color.NRGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), a})
			gray.SetGray(x, y, color.Gray{uint8(rng.Intn(256))})
		}
	}
	rng.Read(ycc.Y)
	rng.Read(ycc.Cb)
	rng.Read(ycc.Cr)
	return map[string]image.Image{"rgba": rgba, "nrgba": nrgba, "gray": gray, "ycbcr": ycc, "rgba_sub": rgba.SubImage(image.Rect(r.Min.X+3, r.Min.Y+2, r.Max.X-1, r.Max.Y-4))}
}

func TestApply_FastPathMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	params := []struct {
		name string
		p    Params
	}{
		{"neutral", Params{}},
		{"all", Params{Brightness: 12, Contrast: -30, BlackPoint: 10, WhitePoint: 240, Gamma: 1.3, Saturation: 40, Temperature: -25, BalanceR: 10, BalanceG: -5, BalanceB: 20, Sharpen: 50, Vignette: 30}},
		{"levels", Params{BlackPoint: 30, WhitePoint: 200, Gamma: 0.7}},
		{"color", Params{Saturation: -60, Temperature: 70}},
	}
	for _, rect := range []image.Rectangle{image.Rect(0, 0, 37, 23), image.Rect(5, -4, 41, 29)} {
		for kind, img := range randomImages(rng, rect) {
			for _, tt := range params {
				t.Run(kind+"/"+tt.name, func(t *testing.T) {
					want := referenceApply(img, tt.p)
					got := Apply(img, tt.p)
					require.Equal(t, want.Rect, got.Rect)
					require.Equal(t, want.Pix, got.Pix)
				})
			}
		}
	}
}

func TestAnalyzeAuto_FastPathMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for _, rect := range []image.Rectangle{image.Rect(0, 0, 64, 48), image.Rect(-3, 2, 1203, 902)} {
		for kind, img := range randomImages(rng, rect) {
			t.Run(kind, func(t *testing.T) {
				require.Equal(t, referenceAutoStats(img), AnalyzeAuto(img))
			})
		}
	}
}
