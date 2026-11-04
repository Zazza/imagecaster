package adjust

import (
	"image"
	"image/color"
	"math"
)

const channelGainStep = 0.002

type Params struct {
	Brightness  float64 `json:"brightness"`
	Contrast    float64 `json:"contrast"`
	BlackPoint  float64 `json:"black_point"`
	WhitePoint  float64 `json:"white_point"`
	Gamma       float64 `json:"gamma"`
	Saturation  float64 `json:"saturation"`
	Temperature float64 `json:"temperature"`
	BalanceR    float64 `json:"balance_r"`
	BalanceG    float64 `json:"balance_g"`
	BalanceB    float64 `json:"balance_b"`
	Sharpen     float64 `json:"sharpen"`
	Vignette    float64 `json:"vignette"`
}

func (p Params) IsNeutral() bool {
	black, white := effectiveLevels(p.BlackPoint, p.WhitePoint)
	return p.Brightness == 0 &&
		p.Contrast == 0 &&
		black == 0 &&
		white == 255 &&
		effectiveGamma(p.Gamma) == 1 &&
		p.Saturation == 0 &&
		p.Temperature == 0 &&
		p.BalanceR == 0 &&
		p.BalanceG == 0 &&
		p.BalanceB == 0 &&
		p.Sharpen == 0 &&
		p.Vignette == 0
}

func Apply(img image.Image, p Params) *image.RGBA {
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

func effectiveLevels(black, white float64) (float64, float64) {
	b := clampF(black, 0, 254)
	w := 255.0
	if white > 0 {
		w = clampF(white, 1, 255)
	}
	if b >= w {
		b, w = w, b
	}
	if w-b < 1 {
		w = min(255, b+1)
	}
	return b, w
}

func effectiveGamma(gamma float64) float64 {
	if gamma <= 0 {
		return 1
	}
	return clampF(gamma, 0.1, 3)
}

func levelsLUT(p Params) ([256]float64, bool) {
	var lut [256]float64
	black, white := effectiveLevels(p.BlackPoint, p.WhitePoint)
	gamma := effectiveGamma(p.Gamma)
	if black == 0 && white == 255 && gamma == 1 {
		for i := range lut {
			lut[i] = float64(i)
		}
		return lut, false
	}
	span := white - black
	inv := 1 / gamma
	for i := range lut {
		x := (float64(i) - black) / span
		x = clampF(x, 0, 1)
		lut[i] = 255 * math.Pow(x, inv)
	}
	return lut, true
}

func contrastFactor(c float64) float64 {
	c = clampF(c, -100, 100)
	return (259 * (c + 255)) / (255 * (259 - c))
}

func sharpenPass(src *image.RGBA, amount float64) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var sr, sg, sb float64
			for dy := -1; dy <= 1; dy++ {
				yy := y + dy
				if yy < b.Min.Y {
					yy = b.Min.Y
				}
				if yy >= b.Max.Y {
					yy = b.Max.Y - 1
				}
				for dx := -1; dx <= 1; dx++ {
					xx := x + dx
					if xx < b.Min.X {
						xx = b.Min.X
					}
					if xx >= b.Max.X {
						xx = b.Max.X - 1
					}
					c := src.RGBAAt(xx, yy)
					sr += float64(c.R)
					sg += float64(c.G)
					sb += float64(c.B)
				}
			}
			c := src.RGBAAt(x, y)
			dst.SetRGBA(x, y, color.RGBA{
				toUint8(float64(c.R) + amount*(float64(c.R)-sr/9)),
				toUint8(float64(c.G) + amount*(float64(c.G)-sg/9)),
				toUint8(float64(c.B) + amount*(float64(c.B)-sb/9)),
				c.A,
			})
		}
	}
	return dst
}

func vignettePass(img *image.RGBA, k float64) {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	halfW := float64(w-1) / 2
	halfH := float64(h-1) / 2
	rmax2 := halfW*halfW + halfH*halfH
	if rmax2 <= 0 {
		return
	}
	cx := float64(b.Min.X) + halfW
	cy := float64(b.Min.Y) + halfH
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			f := 1 - k*(dx*dx+dy*dy)/rmax2
			c := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{
				toUint8(float64(c.R) * f),
				toUint8(float64(c.G) * f),
				toUint8(float64(c.B) * f),
				c.A,
			})
		}
	}
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func toUint8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func lutIndex(v float64) int {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return int(v + 0.5)
}
