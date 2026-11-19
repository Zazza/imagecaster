package faceguard

import (
	"image"
	"math"
)

const (
	maxHeadSize  = 15
	maxHeadShift = 20
	headRadius   = 1.15
	headFalloff  = 1.0
)

func reshapeHead(img *image.RGBA, px []fpt, sizePct, shiftPct float64) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	eyes := fpt{
		(px[36].x + px[39].x + px[42].x + px[45].x) / 4,
		(px[36].y + px[39].y + px[42].y + px[45].y) / 4,
	}
	chin := px[8]
	d := math.Hypot(chin.x-eyes.x, chin.y-eyes.y)
	if d < 1 {
		return img
	}
	r0 := d * headRadius
	r1 := r0 * (1 + headFalloff)
	inv := 1 / (1 + sizePct/100)
	shift := -shiftPct / 100 * d
	fall := func(r float64) float64 {
		if r <= r0 {
			return 1
		}
		return 1 - smooth((r-r0)/(r1-r0))
	}
	x0, x1 := max(0, int(eyes.x-r1-math.Abs(shift))-1), min(w, int(eyes.x+r1+math.Abs(shift))+2)
	y0, y1 := max(0, int(eyes.y-r1-math.Abs(shift))-1), min(h, int(eyes.y+r1+math.Abs(shift))+2)
	out := toRGBA(img, w, h)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			fx, fy := float64(x), float64(y)
			wt := fall(math.Hypot(fx-eyes.x, fy-eyes.y))
			if wt == 0 {
				continue
			}
			qx, qy := fx, fy-shift*wt
			dx, dy := qx-eyes.x, qy-eyes.y
			k := 1 + (inv-1)*fall(math.Hypot(dx, dy))
			c := bilinear(img, eyes.x+dx*k, eyes.y+dy*k)
			j := (y*w + x) * 4
			copy(out.Pix[j:j+3], c[:])
		}
	}
	return out
}
