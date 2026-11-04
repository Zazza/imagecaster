package maskcomp

import (
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

const (
	cellSize        = 8
	diffThreshold   = 28.0
	maskCellCover   = 0.5
	featherCells    = 2
	outlineHalfLine = 3
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
