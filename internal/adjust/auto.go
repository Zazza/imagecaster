package adjust

import (
	"image"
	"math"
)

const (
	autoSamplePixelLimit = 1_000_000
	autoCutFraction      = 0.002
	autoBlackPointMax    = 80
	autoWhitePointMin    = 175
	autoMinLevelSpan     = 40
	autoGammaMin         = 0.6
	autoGammaMax         = 1.8
	autoGammaNeutralBand = 0.05
	autoTempGain         = 260
	autoTempLimit        = 60
)

type AutoStats struct {
	BlackPoint  float64 `json:"black_point"`
	WhitePoint  float64 `json:"white_point"`
	Gamma       float64 `json:"gamma"`
	Temperature float64 `json:"temperature"`
}

func neutralAutoStats() AutoStats {
	return AutoStats{BlackPoint: 0, WhitePoint: 255, Gamma: 1}
}

func AnalyzeAuto(img image.Image) AutoStats {
	if img == nil {
		return neutralAutoStats()
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return neutralAutoStats()
	}

	stride := 1
	if w*h > autoSamplePixelLimit {
		stride = 2
	}

	var hist [256]int
	var sumR, sumB float64
	total := 0
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
	if total == 0 {
		return neutralAutoStats()
	}

	black, white := autoLevels(&hist, total)
	return AutoStats{
		BlackPoint:  black,
		WhitePoint:  white,
		Gamma:       autoGamma(&hist, total, black, white),
		Temperature: autoTemperature(sumR, sumB, total),
	}
}

func autoLevels(hist *[256]int, total int) (float64, float64) {
	black := cutLevel(hist, total, false)
	white := cutLevel(hist, total, true)
	if white < black+autoMinLevelSpan {
		return 0, 255
	}
	return clampF(black, 0, autoBlackPointMax), clampF(white, autoWhitePointMin, 255)
}

func cutLevel(hist *[256]int, total int, fromTop bool) float64 {
	cut := autoCutFraction * float64(total)
	cum := 0
	if fromTop {
		for lvl := 255; lvl >= 0; lvl-- {
			cum += hist[lvl]
			if float64(cum) >= cut {
				return float64(lvl)
			}
		}
		return 255
	}
	for lvl := 0; lvl < 256; lvl++ {
		cum += hist[lvl]
		if float64(cum) >= cut {
			return float64(lvl)
		}
	}
	return 0
}

func autoGamma(hist *[256]int, total int, black, white float64) float64 {
	cum := 0
	median := 0.0
	for lvl := 0; lvl < 256; lvl++ {
		cum += hist[lvl]
		if cum*2 >= total {
			median = float64(lvl)
			break
		}
	}
	norm := clampF((median-black)/(white-black), 0.01, 0.99)
	gamma := clampF(math.Log(norm)/math.Log(0.5), autoGammaMin, autoGammaMax)
	if math.Abs(gamma-1) <= autoGammaNeutralBand {
		return 1
	}
	return gamma
}

func autoTemperature(sumR, sumB float64, total int) float64 {
	meanR := sumR / float64(total)
	meanB := sumB / float64(total)
	avg := (meanR + meanB) / 2
	if avg <= 0 {
		return 0
	}
	return clampF(math.Round((meanB-meanR)/avg*autoTempGain), -autoTempLimit, autoTempLimit)
}
