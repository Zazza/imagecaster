package faceguard

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"strings"

	_ "image/jpeg"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/vector"
)

type Point struct {
	X, Y float64
}

const (
	minPoints        = 68
	underReach       = 0.8
	underLift        = 0.15
	underBlur        = 0.025
	faceBlurRatio    = 0.017
	lipsBlurRatio    = 0.005
	toneBlurRatio    = 0.02
	maxMisfit        = 0.03
	maxKeyMisfit     = 0.05
	textureGrid      = 48
	textureBlurRatio = 0.012
	featToneRatio    = 0.03
	featGrowRatio    = 0.02
)

var (
	keyPoints   = []int{36, 39, 42, 45, 30, 48, 54}
	outerLips   = []int{48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59}
	innerLips   = []int{60, 61, 62, 63, 64, 65, 66, 67}
	underLids   = [][]int{{36, 41, 40, 39}, {42, 47, 46, 45}}
	eyeContours = [][]int{{36, 37, 38, 39, 40, 41}, {42, 43, 44, 45, 46, 47}}
)

type fpt struct{ x, y float64 }

type plane struct {
	w, h int
	v    []float64
}

func newPlane(w, h int) *plane { return &plane{w: w, h: h, v: make([]float64, w*h)} }

func fill(w, h int, poly []fpt) *plane {
	p := newPlane(w, h)
	if len(poly) < 3 {
		return p
	}
	r := vector.NewRasterizer(w, h)
	r.MoveTo(float32(poly[0].x), float32(poly[0].y))
	for _, q := range poly[1:] {
		r.LineTo(float32(q.x), float32(q.y))
	}
	r.ClosePath()
	dst := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	for i, a := range dst.Pix {
		p.v[i] = float64(a) / 255
	}
	return p
}

func boxPass(src []float64, n, stride, count, step int, r int) []float64 {
	out := make([]float64, len(src))
	for c := 0; c < count; c++ {
		base := c * step
		var sum float64
		for i := -r; i <= r; i++ {
			sum += src[base+clamp(i, n)*stride]
		}
		for i := 0; i < n; i++ {
			out[base+i*stride] = sum / float64(2*r+1)
			sum += src[base+clamp(i+r+1, n)*stride] - src[base+clamp(i-r, n)*stride]
		}
	}
	return out
}

func clamp(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

func blurValues(v []float64, w, h int, sigma float64) []float64 {
	r := int(math.Round(sigma))
	if r < 1 {
		return v
	}
	for range 3 {
		v = boxPass(v, w, 1, h, w, r)
		v = boxPass(v, h, w, w, 1, r)
	}
	return v
}

func (p *plane) blur(sigma float64) *plane {
	return &plane{w: p.w, h: p.h, v: blurValues(p.v, p.w, p.h, sigma)}
}

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

func channels(img *image.RGBA) [3][]float64 {
	n := len(img.Pix) / 4
	var c [3][]float64
	for k := range c {
		c[k] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for k := 0; k < 3; k++ {
			c[k][i] = float64(img.Pix[i*4+k])
		}
	}
	return c
}

func toPixels(pts []Point, w, h int) []fpt {
	px := make([]fpt, len(pts))
	for i, p := range pts {
		px[i] = fpt{p.X * float64(w), p.Y * float64(h)}
	}
	return px
}

func fitSimilarity(src, dst []fpt) (m f64.Aff3, res []float64) {
	n := float64(len(src))
	var sx, sy, dx, dy float64
	for i := range src {
		sx, sy, dx, dy = sx+src[i].x, sy+src[i].y, dx+dst[i].x, dy+dst[i].y
	}
	sx, sy, dx, dy = sx/n, sy/n, dx/n, dy/n
	var a, b, den float64
	for i := range src {
		ax, ay := src[i].x-sx, src[i].y-sy
		bx, by := dst[i].x-dx, dst[i].y-dy
		a += ax*bx + ay*by
		b += ax*by - ay*bx
		den += ax*ax + ay*ay
	}
	if den == 0 {
		return m, nil
	}
	a, b = a/den, b/den
	if math.Hypot(a, b) == 0 {
		return m, nil
	}
	m = f64.Aff3{a, -b, dx - (a*sx - b*sy), b, a, dy - (b*sx + a*sy)}
	res = make([]float64, len(src))
	for i := range src {
		res[i] = math.Hypot(m[0]*src[i].x+m[1]*src[i].y+m[2]-dst[i].x, m[3]*src[i].x+m[4]*src[i].y+m[5]-dst[i].y)
	}
	return m, res
}

type Fit struct {
	Mean, Key float64
}

func faceWidth(px []fpt) float64 {
	return math.Hypot(px[16].x-px[0].x, px[16].y-px[0].y)
}

func measure(res []float64, faceW float64) Fit {
	var f Fit
	for _, r := range res {
		f.Mean += r
	}
	f.Mean /= float64(len(res)) * faceW
	for _, i := range keyPoints {
		f.Key = math.Max(f.Key, res[i]/faceW)
	}
	return f
}

func Measure(origPts, editPts []Point, w, h int) (Fit, error) {
	if len(origPts) < minPoints || len(editPts) < minPoints {
		return Fit{}, fmt.Errorf("need %d face points", minPoints)
	}
	px := toPixels(editPts, w, h)
	faceW := faceWidth(px)
	if faceW < 1 {
		return Fit{}, errors.New("face too small")
	}
	n := min(len(origPts), len(editPts))
	_, res := fitSimilarity(toPixels(origPts[:n], w, h), px[:n])
	if len(res) == 0 {
		return Fit{}, errors.New("face points are degenerate")
	}
	return measure(res, faceW), nil
}

func faceMatches(res []float64, faceW float64) bool {
	if len(res) == 0 {
		return false
	}
	f := measure(res, faceW)
	return f.Mean <= maxMisfit && f.Key <= maxKeyMisfit
}

func TextureMatch(orig, edited image.Image, origPts, editPts []Point) (float64, error) {
	eb := edited.Bounds()
	w, h := eb.Dx(), eb.Dy()
	if len(origPts) < minPoints || len(editPts) < minPoints {
		return 0, fmt.Errorf("need %d face points", minPoints)
	}
	px := toPixels(editPts, w, h)
	faceW := faceWidth(px)
	if faceW < 1 {
		return 0, errors.New("face too small")
	}
	n := min(len(origPts), len(editPts))
	m, res := fitSimilarity(toPixels(origPts[:n], w, h), px[:n])
	if len(res) == 0 {
		return 0, errors.New("face points are degenerate")
	}
	o := image.NewRGBA(image.Rect(0, 0, w, h))
	scaled := toRGBA(orig, w, h)
	xdraw.CatmullRom.Transform(o, m, scaled, scaled.Bounds(), draw.Src, nil)
	hpO, hpE := highPass(o, faceW), highPass(toRGBA(edited, w, h), faceW)
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range px[17:minPoints] {
		x0, y0, x1, y1 = math.Min(x0, p.x), math.Min(y0, p.y), math.Max(x1, p.x), math.Max(y1, p.y)
	}
	var a, b []float64
	for j := 0; j < textureGrid; j++ {
		for i := 0; i < textureGrid; i++ {
			x := clamp(int(x0+(x1-x0)*(float64(i)+0.5)/textureGrid), w)
			y := clamp(int(y0+(y1-y0)*(float64(j)+0.5)/textureGrid), h)
			a, b = append(a, hpO[y*w+x]), append(b, hpE[y*w+x])
		}
	}
	return pearson(a, b), nil
}

func highPass(img *image.RGBA, faceW float64) []float64 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	g := make([]float64, w*h)
	for i := range g {
		g[i] = 0.299*float64(img.Pix[i*4]) + 0.587*float64(img.Pix[i*4+1]) + 0.114*float64(img.Pix[i*4+2])
	}
	bl := blurValues(g, w, h, math.Max(2, faceW*textureBlurRatio))
	for i := range g {
		g[i] -= bl[i]
	}
	return g
}

func pearson(a, b []float64) float64 {
	var ma, mb float64
	for i := range a {
		ma, mb = ma+a[i], mb+b[i]
	}
	ma, mb = ma/float64(len(a)), mb/float64(len(b))
	var sab, saa, sbb float64
	for i := range a {
		da, db := a[i]-ma, b[i]-mb
		sab, saa, sbb = sab+da*db, saa+da*da, sbb+db*db
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

func TextureMatchBase64(origB64, editedB64 string, origPts, editPts []Point) (float64, error) {
	o, err := decode(origB64, "original")
	if err != nil {
		return 0, err
	}
	e, err := decode(editedB64, "edited")
	if err != nil {
		return 0, err
	}
	return TextureMatch(o, e, origPts, editPts)
}

func spread(p *plane, sigma float64) *plane {
	b := p.blur(sigma)
	for i, v := range b.v {
		if v > 0.05 {
			b.v[i] = 1
		} else {
			b.v[i] = 0
		}
	}
	return b
}

func featureMask(px []fpt, w, h int, faceW float64) *plane {
	f := newPlane(w, h)
	add := func(poly []fpt) {
		l := fill(w, h, poly)
		for i := range f.v {
			f.v[i] = math.Max(f.v[i], l.v[i])
		}
	}
	add(pick(px, []int{36, 37, 38, 39, 40, 41}))
	add(pick(px, []int{42, 43, 44, 45, 46, 47}))
	add(pick(px, outerLips))
	add(pick(px, []int{27, 31, 32, 33, 34, 35}))
	return spread(f, faceW*featGrowRatio).blur(faceW * faceBlurRatio)
}

func Apply(orig, edited image.Image, origPts, editPts []Point) (*image.RGBA, error) {
	eb := edited.Bounds()
	w, h := eb.Dx(), eb.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("empty edited image")
	}
	if len(origPts) < minPoints || len(editPts) < minPoints {
		return nil, fmt.Errorf("need %d face points", minPoints)
	}
	e := toRGBA(edited, w, h)
	px := toPixels(editPts, w, h)
	faceW := faceWidth(px)
	if faceW < 1 {
		return nil, errors.New("face too small")
	}
	n := min(len(origPts), len(editPts))
	m, res := fitSimilarity(toPixels(origPts[:n], w, h), px[:n])
	if !faceMatches(res, faceW) {
		return nil, errors.New("face on the edit does not match the original")
	}
	scaled := toRGBA(orig, w, h)
	o := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(o, o.Bounds(), e, image.Point{}, draw.Src)
	xdraw.CatmullRom.Transform(o, m, scaled, scaled.Bounds(), draw.Src, nil)

	feat := featureMask(px, w, h, faceW)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(out.Pix, e.Pix)
	sigma := faceW * featToneRatio
	oc, ec := channels(o), channels(e)
	for k := 0; k < 3; k++ {
		ob, eb := blurValues(oc[k], w, h, sigma), blurValues(ec[k], w, h, sigma)
		for i, a := range feat.v {
			if a <= 0 {
				continue
			}
			t := eb[i] + oc[k][i] - ob[i]
			j := i*4 + k
			v := t*a + float64(e.Pix[j])*(1-a)
			out.Pix[j] = uint8(math.Round(math.Max(0, math.Min(255, v))))
		}
	}
	return out, nil
}

func underEyeBand(px []fpt, idx []int, w, h int, down float64) *plane {
	lid := pick(px, idx)
	reach := math.Hypot(lid[len(lid)-1].x-lid[0].x, lid[len(lid)-1].y-lid[0].y)
	poly := make([]fpt, 0, 2*len(lid))
	for _, p := range lid {
		poly = append(poly, fpt{p.x, p.y + reach*(down-underLift)})
	}
	for i := len(lid) - 1; i >= 0; i-- {
		poly = append(poly, fpt{lid[i].x, lid[i].y + reach*(down+underReach)})
	}
	return fill(w, h, poly)
}

func eyesMask(px []fpt, w, h int) *plane {
	eyes := newPlane(w, h)
	for _, idx := range eyeContours {
		e := fill(w, h, pick(px, idx))
		for i := range eyes.v {
			eyes.v[i] = math.Max(eyes.v[i], e.v[i])
		}
	}
	return eyes
}

func pick(px []fpt, idx []int) []fpt {
	out := make([]fpt, len(idx))
	for i, j := range idx {
		out[i] = px[j]
	}
	return out
}

func decode(b64, name string) (image.Image, error) {
	if i := strings.Index(b64, ","); i >= 0 && strings.HasPrefix(b64, "data:") {
		b64 = b64[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	return img, nil
}

func MeasureBase64(editedB64 string, origPts, editPts []Point) (Fit, error) {
	if i := strings.Index(editedB64, ","); i >= 0 && strings.HasPrefix(editedB64, "data:") {
		editedB64 = editedB64[i+1:]
	}
	cfg, _, err := image.DecodeConfig(base64.NewDecoder(base64.StdEncoding, strings.NewReader(editedB64)))
	if err != nil {
		return Fit{}, fmt.Errorf("decode edited: %w", err)
	}
	return Measure(origPts, editPts, cfg.Width, cfg.Height)
}

func ApplyBase64(origB64, editedB64 string, origPts, editPts []Point) (string, error) {
	o, err := decode(origB64, "original")
	if err != nil {
		return "", err
	}
	e, err := decode(editedB64, "edited")
	if err != nil {
		return "", err
	}
	ob, eb := o.Bounds(), e.Bounds()
	if (ob.Dx() > ob.Dy()) != (eb.Dx() > eb.Dy()) {
		return "", errors.New("orientation of original and edited differ")
	}
	res, err := Apply(o, e, origPts, editPts)
	if err != nil {
		return "", err
	}
	return encode(res)
}

func encode(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
