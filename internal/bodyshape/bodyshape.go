package bodyshape

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
)

const (
	MaxWidth  = 10
	MaxHeight = 7

	nose      = 0
	neck      = 1
	rShoulder = 2
	lShoulder = 5
	rHip      = 8
	rAnkle    = 10
	lHip      = 11
	lAnkle    = 13

	widthReach    = 1.25
	widthOutside  = 1.0
	widthNeckRamp = 0.5
	widthFootFade = 0.3
	headAboveNose = 0.9
	footBelow     = 0.12
	legRamp       = 0.12
	maxMarginSqz  = 0.5
	marginRamp    = 0.25
	rowSteps      = 4
)

var ErrNoBody = errors.New("body: person not found")

type Point struct {
	X, Y float64
}

func (p Point) ok() bool { return p.X >= 0 && p.Y >= 0 }

type Params struct {
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

func (p Params) IsZero() bool { return p.Width == 0 && p.Height == 0 }

func (p Params) Validate() error {
	if math.Abs(p.Width) > MaxWidth {
		return fmt.Errorf("width: от −%d до +%d", MaxWidth, MaxWidth)
	}
	if math.Abs(p.Height) > MaxHeight {
		return fmt.Errorf("height: от −%d до +%d", MaxHeight, MaxHeight)
	}
	return nil
}

type body struct {
	pts  []Point
	w, h float64
}

func (b body) at(i int) (Point, bool) {
	if i >= len(b.pts) || !b.pts[i].ok() {
		return Point{}, false
	}
	return Point{b.pts[i].X * b.w, b.pts[i].Y * b.h}, true
}

func (b body) mid(i, j int) (Point, bool) {
	a, okA := b.at(i)
	c, okC := b.at(j)
	switch {
	case okA && okC:
		return Point{(a.X + c.X) / 2, (a.Y + c.Y) / 2}, true
	case okA:
		return a, true
	case okC:
		return c, true
	}
	return Point{}, false
}

func Apply(img image.Image, pts []Point, p Params) (*image.RGBA, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil, errors.New("empty image")
	}
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	bd := body{pts: pts, w: float64(b.Dx()), h: float64(b.Dy())}
	if p.Width != 0 {
		res, err := widen(out, bd, p.Width)
		if err != nil {
			return nil, err
		}
		out = res
	}
	if p.Height != 0 {
		res, err := lengthen(out, bd, p.Height)
		if err != nil {
			return nil, err
		}
		out = res
	}
	return out, nil
}

func ApplyIfFound(img image.Image, pts []Point, p Params) (image.Image, error) {
	if p.IsZero() || len(pts) == 0 {
		return img, nil
	}
	out, err := Apply(img, pts, p)
	if errors.Is(err, ErrNoBody) {
		return img, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func widen(img *image.RGBA, b body, pct float64) (*image.RGBA, error) {
	nk, okN := b.at(neck)
	rs, okR := b.at(rShoulder)
	ls, okL := b.at(lShoulder)
	if !okN || !okR || !okL {
		return nil, ErrNoBody
	}
	half := math.Hypot(ls.X-rs.X, ls.Y-rs.Y) / 2
	if half < 1 {
		return nil, ErrNoBody
	}
	reach := half * widthReach
	type knot struct{ y, x float64 }
	knots := []knot{{nk.Y, nk.X}}
	if hp, ok := b.mid(rHip, lHip); ok && hp.Y > nk.Y {
		knots = append(knots, knot{hp.Y, hp.X})
	}
	ank, okA := b.mid(rAnkle, lAnkle)
	if okA && ank.Y > knots[len(knots)-1].y {
		knots = append(knots, knot{ank.Y, ank.X})
	}
	center := func(y float64) float64 {
		if y <= knots[0].y {
			return knots[0].x
		}
		for i := 1; i < len(knots); i++ {
			if y <= knots[i].y {
				t := (y - knots[i-1].y) / (knots[i].y - knots[i-1].y)
				return knots[i-1].x + t*(knots[i].x-knots[i-1].x)
			}
		}
		return knots[len(knots)-1].x
	}
	rampEnd := nk.Y + half*widthNeckRamp
	footFade := 0.0
	if okA {
		footFade = (ank.Y - nk.Y) * widthFootFade
	}
	wv := func(y float64) float64 {
		switch {
		case y <= nk.Y:
			return 0
		case y < rampEnd:
			return smooth((y - nk.Y) / (rampEnd - nk.Y))
		case !okA || y <= ank.Y:
			return 1
		case footFade <= 0:
			return 0
		default:
			return math.Max(0, 1-smooth((y-ank.Y)/footFade))
		}
	}
	wu := func(u float64) float64 {
		a := math.Abs(u) / reach
		if a <= 1 {
			return a
		}
		return math.Max(0, 1-(a-1)/widthOutside)
	}
	amt := -pct / 100
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	out := image.NewRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	for y := 0; y < h; y++ {
		fy := float64(y)
		v := wv(fy)
		if v == 0 {
			continue
		}
		cx := center(fy)
		for x := 0; x < w; x++ {
			u := float64(x) - cx
			d := amt * v * wu(u)
			if d == 0 {
				continue
			}
			c := bilinear(img, cx+u*(1+d), fy)
			copy(out.Pix[(y*w+x)*4:], c[:])
		}
	}
	return out, nil
}

func lengthen(img *image.RGBA, b body, pct float64) (*image.RGBA, error) {
	hip, ok := b.mid(rHip, lHip)
	if !ok {
		return nil, ErrNoBody
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	H := float64(h)
	legEnd := H
	ank, okA := b.mid(rAnkle, lAnkle)
	if okA && ank.Y > hip.Y {
		legEnd = ank.Y
	}
	legLen := legEnd - hip.Y
	if legLen < 1 {
		return nil, ErrNoBody
	}
	ramp := legLen * legRamp
	s := pct / 100
	leg := func(y float64) float64 {
		return band(y, hip.Y, legEnd, ramp)
	}
	var above, below func(float64) float64
	feet := okA && legEnd < H
	if feet || s < 0 {
		top := headTop(b, hip.Y)
		rampA := math.Min(ramp, top*marginRamp)
		above = func(y float64) float64 { return band(y, -rampA, top, rampA) }
		below = func(float64) float64 { return 0 }
	}
	if feet {
		bottom := math.Min(H, legEnd+legLen*footBelow)
		rampB := math.Min(ramp, (H-bottom)*marginRamp)
		below = func(y float64) float64 { return band(y, bottom, H+rampB, rampB) }
	}
	n := h * rowSteps
	dy := H / float64(n)
	var extra, roomA, roomB float64
	for i := 0; i < n; i++ {
		y := (float64(i) + 0.5) * dy
		extra += s * leg(y) * dy
		if above != nil {
			roomA += above(y) * dy
			roomB += below(y) * dy
		}
	}
	cA, cB := marginSqueeze(extra, roomA, roomB)
	if extra != 0 && above != nil {
		taken := cA*roomA + cB*roomB
		s *= taken / extra
	}
	fwd := make([]float64, n+1)
	for i := 0; i < n; i++ {
		y := (float64(i) + 0.5) * dy
		g := 1 + s*leg(y)
		if above != nil {
			g -= cA*above(y) + cB*below(y)
		}
		fwd[i+1] = fwd[i] + g*dy
	}
	out := image.NewRGBA(img.Bounds())
	j := 0
	for y := 0; y < h; y++ {
		dst := float64(y) + 0.5
		for j < n-1 && fwd[j+1] < dst {
			j++
		}
		seg := fwd[j+1] - fwd[j]
		t := 0.0
		if seg > 0 {
			t = (dst - fwd[j]) / seg
		}
		src := (float64(j)+t)*dy - 0.5
		for x := 0; x < w; x++ {
			col := bilinear(img, float64(x), src)
			copy(out.Pix[(y*w+x)*4:], col[:])
		}
	}
	return out, nil
}

func marginSqueeze(extra, roomA, roomB float64) (float64, float64) {
	if roomA <= 0 && roomB <= 0 {
		return 0, 0
	}
	sign := math.Copysign(1, extra)
	want := math.Abs(extra)
	capA, capB := roomA*maxMarginSqz, roomB*maxMarginSqz
	a, b := math.Min(want/2, capA), math.Min(want/2, capB)
	rest := want - a - b
	a += math.Min(rest, capA-a)
	rest = want - a - b
	b += math.Min(rest, capB-b)
	var cA, cB float64
	if roomA > 0 {
		cA = sign * a / roomA
	}
	if roomB > 0 {
		cB = sign * b / roomB
	}
	return cA, cB
}

func headTop(b body, hipY float64) float64 {
	n, okN := b.at(nose)
	nk, okK := b.at(neck)
	if okN && okK && nk.Y > n.Y {
		return math.Max(0, n.Y-(nk.Y-n.Y)*headAboveNose)
	}
	if okK {
		return math.Max(0, nk.Y-(hipY-nk.Y)*0.5)
	}
	return 0
}

func band(y, from, to, ramp float64) float64 {
	if ramp <= 0 {
		if y >= from && y <= to {
			return 1
		}
		return 0
	}
	return smooth((y-from)/ramp) * smooth((to-y)/ramp)
}

func smooth(t float64) float64 {
	t = math.Max(0, math.Min(1, t))
	return t * t * (3 - 2*t)
}

func bilinear(img *image.RGBA, x, y float64) [4]uint8 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	x = math.Max(0, math.Min(float64(w-1), x))
	y = math.Max(0, math.Min(float64(h-1), y))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, w-1), min(y0+1, h-1)
	fx, fy := x-float64(x0), y-float64(y0)
	var out [4]uint8
	for k := range 4 {
		p00 := float64(img.Pix[(y0*w+x0)*4+k])
		p10 := float64(img.Pix[(y0*w+x1)*4+k])
		p01 := float64(img.Pix[(y1*w+x0)*4+k])
		p11 := float64(img.Pix[(y1*w+x1)*4+k])
		v := p00*(1-fx)*(1-fy) + p10*fx*(1-fy) + p01*(1-fx)*fy + p11*fx*fy
		out[k] = uint8(math.Round(v))
	}
	return out
}
