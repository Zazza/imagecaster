package maskcomp

import (
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

const (
	keepColorWorkSide = 512
	cellSize          = 8
	diffThreshold     = 28.0
	maskCellCover     = 0.5
	featherCells      = 2
	outlineHalfLine   = 3
)

var outlineColor = color.RGBA{R: 255, A: 255}

func toRGBA(img image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := img.Bounds()
	if b.Dx() == w && b.Dy() == h {
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func maskValues(mask image.Image, w, h int) []bool {
	m := toRGBA(mask, w, h)
	out := make([]bool, w*h)
	for i := range out {
		p := m.Pix[i*4 : i*4+4]
		lum := (int(p[0]) + int(p[1]) + int(p[2])) / 3
		if p[3] > 127 && lum > 127 {
			out[i] = true
		}
	}
	return out
}

func HasPaint(mask image.Image) bool {
	b := mask.Bounds()
	for _, v := range maskValues(mask, b.Dx(), b.Dy()) {
		if v {
			return true
		}
	}
	return false
}

func Outline(img, mask image.Image) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := toRGBA(img, w, h)
	m := maskValues(mask, w, h)
	at := func(x, y int) bool {
		if x < 0 || y < 0 || x >= w || y >= h {
			return false
		}
		return m[y*w+x]
	}
	edge := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := m[y*w+x]
			if at(x-1, y) != v || at(x+1, y) != v || at(x, y-1) != v || at(x, y+1) != v {
				edge[y*w+x] = true
			}
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !edge[y*w+x] {
				continue
			}
			for dy := -outlineHalfLine; dy <= outlineHalfLine; dy++ {
				for dx := -outlineHalfLine; dx <= outlineHalfLine; dx++ {
					X, Y := x+dx, y+dy
					if X >= 0 && Y >= 0 && X < w && Y < h {
						dst.SetRGBA(X, Y, outlineColor)
					}
				}
			}
		}
	}
	return dst
}

func Composite(orig, edited, mask image.Image) *image.RGBA {
	b := orig.Bounds()
	w, h := b.Dx(), b.Dy()
	o := toRGBA(orig, w, h)
	e := toRGBA(edited, w, h)
	m := maskValues(mask, w, h)

	gw, gh := (w+cellSize-1)/cellSize, (h+cellSize-1)/cellSize
	changed := make([]bool, gw*gh)
	inMask := make([]bool, gw*gh)
	touchMask := make([]bool, gw*gh)
	for gy := 0; gy < gh; gy++ {
		for gx := 0; gx < gw; gx++ {
			var diff float64
			var masked, n int
			for y := gy * cellSize; y < min(h, (gy+1)*cellSize); y++ {
				for x := gx * cellSize; x < min(w, (gx+1)*cellSize); x++ {
					i := y*w + x
					po, pe := o.Pix[i*4:i*4+3], e.Pix[i*4:i*4+3]
					diff += (absDiff(po[0], pe[0]) + absDiff(po[1], pe[1]) + absDiff(po[2], pe[2])) / 3
					if m[i] {
						masked++
					}
					n++
				}
			}
			changed[gy*gw+gx] = diff/float64(n) > diffThreshold
			inMask[gy*gw+gx] = float64(masked)/float64(n) >= maskCellCover
			touchMask[gy*gw+gx] = masked > 0
		}
	}

	keep := make([]float64, gw*gh)
	for i, v := range inMask {
		if v {
			keep[i] = 1
		}
	}
	seen := make([]bool, gw*gh)
	queue := make([]int, 0, 64)
	for start := range changed {
		if !changed[start] || seen[start] {
			continue
		}
		queue = append(queue[:0], start)
		seen[start] = true
		touches := false
		for q := 0; q < len(queue); q++ {
			c := queue[q]
			if touchMask[c] {
				touches = true
			}
			cx, cy := c%gw, c/gw
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := cx+d[0], cy+d[1]
				if nx < 0 || ny < 0 || nx >= gw || ny >= gh {
					continue
				}
				j := ny*gw + nx
				if changed[j] && !seen[j] {
					seen[j] = true
					queue = append(queue, j)
				}
			}
		}
		if touches {
			for _, c := range queue {
				keep[c] = 1
			}
		}
	}

	alpha := boxBlur(dilate(keep, gw, gh, featherCells), gw, gh, featherCells)

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := sampleBilinear(alpha, gw, gh, (float64(x)+0.5)/cellSize-0.5, (float64(y)+0.5)/cellSize-0.5)
			if m[y*w+x] {
				a = 1
			}
			i := (y*w + x) * 4
			for c := 0; c < 3; c++ {
				dst.Pix[i+c] = uint8(float64(o.Pix[i+c])*(1-a) + float64(e.Pix[i+c])*a + 0.5)
			}
			dst.Pix[i+3] = 255
		}
	}
	return dst
}

func absDiff(a, b uint8) float64 {
	if a > b {
		return float64(a - b)
	}
	return float64(b - a)
}

func dilate(a []float64, w, h, r int) []float64 {
	out := make([]float64, len(a))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var v float64
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					X, Y := x+dx, y+dy
					if X >= 0 && Y >= 0 && X < w && Y < h && a[Y*w+X] > v {
						v = a[Y*w+X]
					}
				}
			}
			out[y*w+x] = v
		}
	}
	return out
}

func boxBlur(a []float64, w, h, r int) []float64 {
	out := make([]float64, len(a))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum, n float64
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					X, Y := x+dx, y+dy
					if X >= 0 && Y >= 0 && X < w && Y < h {
						sum += a[Y*w+X]
						n++
					}
				}
			}
			out[y*w+x] = sum / n
		}
	}
	return out
}

func sampleBilinear(a []float64, w, h int, fx, fy float64) float64 {
	clamp := func(v, hi int) int {
		if v < 0 {
			return 0
		}
		if v > hi {
			return hi
		}
		return v
	}
	x0, y0 := int(floor(fx)), int(floor(fy))
	tx, ty := fx-floor(fx), fy-floor(fy)
	x1, y1 := clamp(x0+1, w-1), clamp(y0+1, h-1)
	x0, y0 = clamp(x0, w-1), clamp(y0, h-1)
	top := a[y0*w+x0]*(1-tx) + a[y0*w+x1]*tx
	bot := a[y1*w+x0]*(1-tx) + a[y1*w+x1]*tx
	return top*(1-ty) + bot*ty
}

func floor(v float64) float64 {
	i := float64(int(v))
	if v < 0 && i != v {
		return i - 1
	}
	return i
}

func Blend(orig, edited, mask image.Image) *image.RGBA {
	b := orig.Bounds()
	w, h := b.Dx(), b.Dy()
	o := toRGBA(orig, w, h)
	e := toRGBA(edited, w, h)
	m := maskValues(mask, w, h)
	a := make([]float64, w*h)
	for i, v := range m {
		if v {
			a[i] = 1
		}
	}
	r := max(2, min(w, h)/200)
	soft := boxBlurSep(a, w, h, r)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		k := soft[i] * 2
		if k > 1 || m[i] {
			k = 1
		}
		p := i * 4
		for c := 0; c < 3; c++ {
			dst.Pix[p+c] = uint8(float64(o.Pix[p+c])*(1-k) + float64(e.Pix[p+c])*k + 0.5)
		}
		dst.Pix[p+3] = 255
	}
	return dst
}

func boxBlurSep(a []float64, w, h, r int) []float64 {
	tmp := make([]float64, len(a))
	out := make([]float64, len(a))
	blurLine(a, tmp, w, h, 1, w, r)
	blurLine(tmp, out, h, w, w, 1, r)
	return out
}

func blurLine(src, dst []float64, n, lines, step, lineStep, r int) {
	for l := 0; l < lines; l++ {
		base := l * lineStep
		var s float64
		for k := 0; k <= min(r, n-1); k++ {
			s += src[base+k*step]
		}
		for x := 0; x < n; x++ {
			lo, hi := max(0, x-r), min(n-1, x+r)
			dst[base+x*step] = s / float64(hi-lo+1)
			if x+r+1 < n {
				s += src[base+(x+r+1)*step]
			}
			if x-r >= 0 {
				s -= src[base+(x-r)*step]
			}
		}
	}
}

func KeepColor(orig, edited image.Image, sigmaRatio float64) *image.RGBA {
	b := orig.Bounds()
	w, h := b.Dx(), b.Dy()
	o := toRGBA(orig, w, h)
	e := toRGBA(edited, w, h)
	scale := min(1, float64(keepColorWorkSide)/float64(min(w, h)))
	sw, sh := max(1, int(float64(w)*scale+0.5)), max(1, int(float64(h)*scale+0.5))
	so, se := o, e
	if sw != w || sh != h {
		so = image.NewRGBA(image.Rect(0, 0, sw, sh))
		se = image.NewRGBA(image.Rect(0, 0, sw, sh))
		xdraw.ApproxBiLinear.Scale(so, so.Bounds(), o, o.Bounds(), draw.Src, nil)
		xdraw.ApproxBiLinear.Scale(se, se.Bounds(), e, e.Bounds(), draw.Src, nil)
	}
	r := max(1, int(sigmaRatio*float64(min(sw, sh))+0.5))
	diff := make([][]float64, 3)
	for c := 0; c < 3; c++ {
		ob := make([]float64, sw*sh)
		eb := make([]float64, sw*sh)
		for i := range ob {
			ob[i] = float64(so.Pix[i*4+c])
			eb[i] = float64(se.Pix[i*4+c])
		}
		for range 3 {
			ob = boxBlurSep(ob, sw, sh, r)
			eb = boxBlurSep(eb, sw, sh, r)
		}
		for i := range ob {
			ob[i] -= eb[i]
		}
		diff[c] = ob
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			i := (y*w + x) * 4
			for c := 0; c < 3; c++ {
				v := float64(e.Pix[i+c]) + sampleBilinear(diff[c], sw, sh, fx, fy)
				dst.Pix[i+c] = uint8(min(255, max(0, v)) + 0.5)
			}
			dst.Pix[i+3] = 255
		}
	}
	return dst
}
