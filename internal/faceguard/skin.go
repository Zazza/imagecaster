package faceguard

import (
	"errors"
	"image"
	"math"
	"sort"
)

const (
	widthTop     = 0.3
	widthChin    = 0.5
	widthOutside = 0.6
)

var cheekPolys = [][]int{{1, 3, 31, 40}, {15, 13, 35, 47}}

func cheekMedian(img *image.RGBA, pts []Point) ([3]float64, error) {
	var med [3]float64
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if len(pts) < minPoints {
		return med, errors.New("no face points for cheeks")
	}
	px := toPixels(pts, w, h)
	mask := newPlane(w, h)
	for _, idx := range cheekPolys {
		m := fill(w, h, pick(px, idx))
		for i := range mask.v {
			mask.v[i] = math.Max(mask.v[i], m.v[i])
		}
	}
	var vals [3][]float64
	for i, a := range mask.v {
		if a < 0.5 {
			continue
		}
		for k := 0; k < 3; k++ {
			vals[k] = append(vals[k], float64(img.Pix[i*4+k]))
		}
	}
	if len(vals[0]) == 0 {
		return med, errors.New("cheeks are empty")
	}
	for k := range vals {
		sort.Float64s(vals[k])
		med[k] = vals[k][len(vals[k])/2]
	}
	return med, nil
}

func luma(c [3]float64) float64 { return 0.299*c[0] + 0.587*c[1] + 0.114*c[2] }

func SkinLift(orig, edited image.Image, origPts, editPts []Point) (float64, error) {
	ob, eb := orig.Bounds(), edited.Bounds()
	o, err := cheekMedian(toRGBA(orig, ob.Dx(), ob.Dy()), origPts)
	if err != nil {
		return 0, err
	}
	e, err := cheekMedian(toRGBA(edited, eb.Dx(), eb.Dy()), editPts)
	if err != nil {
		return 0, err
	}
	lo := luma(o)
	if lo < 1 {
		return 0, errors.New("cheeks are black")
	}
	return (luma(e)/lo - 1) * 100, nil
}

func ToneSkin(result, orig image.Image, resultPts, origPts []Point, matchOriginal bool, lightness float64) (*image.RGBA, error) {
	rb := result.Bounds()
	w, h := rb.Dx(), rb.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("empty image")
	}
	out := toRGBA(result, w, h)
	gain := [3]float64{1, 1, 1}
	if matchOriginal {
		ob := orig.Bounds()
		o, err := cheekMedian(toRGBA(orig, ob.Dx(), ob.Dy()), origPts)
		if err != nil {
			return nil, err
		}
		r, err := cheekMedian(out, resultPts)
		if err != nil {
			return nil, err
		}
		for k := range gain {
			gain[k] = (o[k] + 1) / (r[k] + 1)
		}
	}
	for k := range gain {
		gain[k] *= 1 + lightness/100
	}
	for i := 0; i < len(out.Pix); i += 4 {
		for k := 0; k < 3; k++ {
			out.Pix[i+k] = uint8(math.Round(math.Min(255, float64(out.Pix[i+k])*gain[k])))
		}
	}
	return out, nil
}

func ToneSkinBase64(resultB64, origB64 string, resultPts, origPts []Point, matchOriginal bool, lightness float64) (string, error) {
	r, err := decode(resultB64, "result")
	if err != nil {
		return "", err
	}
	o, err := decode(origB64, "original")
	if err != nil {
		return "", err
	}
	out, err := ToneSkin(r, o, resultPts, origPts, matchOriginal, lightness)
	if err != nil {
		return "", err
	}
	return encode(out)
}

func SkinLiftBase64(origB64, editedB64 string, origPts, editPts []Point) (float64, error) {
	o, err := decode(origB64, "original")
	if err != nil {
		return 0, err
	}
	e, err := decode(editedB64, "edited")
	if err != nil {
		return 0, err
	}
	return SkinLift(o, e, origPts, editPts)
}

func widenFace(img *image.RGBA, px []fpt, pct float64) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	l, r := px[0], px[16]
	cx, cy := (l.x+r.x)/2, (l.y+r.y)/2
	hw := math.Hypot(r.x-l.x, r.y-l.y) / 2
	el := fpt{(px[36].x + px[39].x) / 2, (px[36].y + px[39].y) / 2}
	er := fpt{(px[42].x + px[45].x) / 2, (px[42].y + px[45].y) / 2}
	ang := math.Atan2(er.y-el.y, er.x-el.x)
	ca, sa := math.Cos(ang), math.Sin(ang)
	toFace := func(x, y float64) (float64, float64) {
		dx, dy := x-cx, y-cy
		return dx*ca + dy*sa, -dx*sa + dy*ca
	}
	_, eyeV := toFace((el.x+er.x)/2, (el.y+er.y)/2)
	_, chinV := toFace(px[8].x, px[8].y)
	wv := func(v float64) float64 {
		top := eyeV - hw*widthTop
		switch {
		case v < top:
			return 0
		case v < eyeV:
			return (v - top) / (hw * widthTop)
		case v <= chinV:
			return 1
		default:
			return math.Max(0, 1-(v-chinV)/(hw*widthChin))
		}
	}
	wu := func(u float64) float64 {
		a := math.Abs(u) / hw
		if a <= 1 {
			return a
		}
		return math.Max(0, 1-(a-1)/widthOutside)
	}
	amt := -pct / 100
	src := img
	out := toRGBA(img, w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			u, v := toFace(float64(x), float64(y))
			d := amt * wv(v) * wu(u)
			if d == 0 {
				continue
			}
			su := u * (1 + d)
			sx := cx + su*ca - v*sa
			sy := cy + su*sa + v*ca
			c := bilinear(src, sx, sy)
			j := (y*w + x) * 4
			copy(out.Pix[j:j+3], c[:])
		}
	}
	return out
}

func bilinear(img *image.RGBA, x, y float64) [3]uint8 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	var c [3]uint8
	for k := 0; k < 3; k++ {
		get := func(xx, yy int) float64 { return float64(img.Pix[(clamp(yy, h)*w+clamp(xx, w))*4+k]) }
		top := get(x0, y0)*(1-fx) + get(x0+1, y0)*fx
		bot := get(x0, y0+1)*(1-fx) + get(x0+1, y0+1)*fx
		c[k] = uint8(math.Round(top*(1-fy) + bot*fy))
	}
	return c
}

const (
	bgGrid      = 48
	bgSide      = 0.6
	bgAbove     = 0.5
	bgMinCells  = 24
	bgBlurCells = 1
)

func BackgroundShift(orig, edited image.Image, origPts []Point) (float64, bool) {
	if len(origPts) < minPoints {
		return 0, false
	}
	x0, x1, y0 := 1.0, 0.0, 1.0
	for _, p := range origPts[:27] {
		x0, x1, y0 = math.Min(x0, p.X), math.Max(x1, p.X), math.Min(y0, p.Y)
	}
	fw := x1 - x0
	o := toRGBA(orig, bgGrid, bgGrid)
	e := toRGBA(edited, bgGrid, bgGrid)
	oc, ec := channels(o), channels(e)
	var sum float64
	cells := 0
	for k := 0; k < 3; k++ {
		ob := blurValues(oc[k], bgGrid, bgGrid, bgBlurCells)
		eb := blurValues(ec[k], bgGrid, bgGrid, bgBlurCells)
		for j := 0; j < bgGrid; j++ {
			for i := 0; i < bgGrid; i++ {
				x, y := (float64(i)+0.5)/bgGrid, (float64(j)+0.5)/bgGrid
				if x >= x0-fw*bgSide && x <= x1+fw*bgSide && y >= y0-fw*bgAbove {
					continue
				}
				sum += math.Abs(ob[j*bgGrid+i] - eb[j*bgGrid+i])
				if k == 0 {
					cells++
				}
			}
		}
	}
	if cells < bgMinCells {
		return 0, false
	}
	return sum / float64(cells*3), true
}

func BackgroundShiftBase64(origB64, editedB64 string, origPts []Point) (float64, bool) {
	o, err := decode(origB64, "original")
	if err != nil {
		return 0, false
	}
	e, err := decode(editedB64, "edited")
	if err != nil {
		return 0, false
	}
	return BackgroundShift(o, e, origPts)
}
