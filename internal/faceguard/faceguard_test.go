package faceguard

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fgW = 400
	fgH = 600
)

var (
	fgGray = color.RGBA{R: 128, G: 128, B: 128, A: 255}
	fgRed  = color.RGBA{R: 230, G: 20, B: 20, A: 255}
	fgBlue = color.RGBA{R: 0, G: 0, B: 255, A: 255}
)

func fgEllipse(cx, cy, rx, ry float64, from, to float64, n int) []Point {
	out := make([]Point, 0, n)
	for i := 0; i < n; i++ {
		t := from + (to-from)*float64(i)/float64(n-1)
		out = append(out, Point{X: cx + rx*math.Cos(t), Y: cy + ry*math.Sin(t)})
	}
	return out
}

func fgEye(cx, cy float64) []Point {
	return []Point{
		{X: cx - 0.05, Y: cy},
		{X: cx - 0.02, Y: cy - 0.015},
		{X: cx + 0.02, Y: cy - 0.015},
		{X: cx + 0.05, Y: cy},
		{X: cx + 0.02, Y: cy + 0.015},
		{X: cx - 0.02, Y: cy + 0.015},
	}
}

func fgLipRing(cx, cy, rx, ry float64, n int) []Point {
	out := make([]Point, 0, n)
	for i := 0; i < n; i++ {
		t := math.Pi + 2*math.Pi*float64(i)/float64(n)
		out = append(out, Point{X: cx + rx*math.Cos(t), Y: cy + ry*math.Sin(t)})
	}
	return out
}

const (
	fgEyeY  = 0.40
	fgBrowY = 0.33
	fgLipX  = 0.50
	fgLipY  = 0.64
)

func fgFace() []Point {
	pts := make([]Point, 0, 70)
	pts = append(pts, fgEllipse(0.5, fgEyeY, 0.25, 0.38, math.Pi, 0, 17)...)
	for i := 0; i < 5; i++ {
		pts = append(pts, Point{X: 0.30 + 0.0375*float64(i), Y: fgBrowY})
	}
	for i := 0; i < 5; i++ {
		pts = append(pts, Point{X: 0.55 + 0.0375*float64(i), Y: fgBrowY})
	}
	for i := 0; i < 4; i++ {
		pts = append(pts, Point{X: 0.5, Y: 0.41 + 0.035*float64(i)})
	}
	for i := 0; i < 5; i++ {
		pts = append(pts, Point{X: 0.46 + 0.02*float64(i), Y: 0.55})
	}
	pts = append(pts, fgEye(0.38, fgEyeY)...)
	pts = append(pts, fgEye(0.62, fgEyeY)...)
	pts = append(pts, fgLipRing(fgLipX, fgLipY, 0.09, 0.045, 12)...)
	pts = append(pts, fgLipRing(fgLipX, fgLipY, 0.06, 0.005, 8)...)
	pts = append(pts, Point{X: 0.38, Y: fgEyeY}, Point{X: 0.62, Y: fgEyeY})
	return pts
}

func fgSolid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func fgAt(img image.Image, nx, ny float64) color.RGBA {
	b := img.Bounds()
	x := b.Min.X + int(nx*float64(b.Dx()))
	y := b.Min.Y + int(ny*float64(b.Dy()))
	r, g, bl, a := img.At(x, y).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}
}

func fgNear(t *testing.T, want, got color.RGBA, tol int, msg string) {
	t.Helper()
	d := func(a, b uint8) int {
		v := int(a) - int(b)
		if v < 0 {
			return -v
		}
		return v
	}
	assert.True(t, d(want.R, got.R) <= tol && d(want.G, got.G) <= tol && d(want.B, got.B) <= tol,
		"%s: want ≈%v, got %v (tol %d)", msg, want, got, tol)
}

func fgB64(t *testing.T, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func fgDecode(t *testing.T, s string) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(raw))
	require.NoError(t, err, "result must be PNG")
	return img
}

func TestFgFace_Has70Points(t *testing.T) {
	t.Parallel()
	require.Len(t, fgFace(), 70)
}

var fgEditFlat = color.RGBA{R: 150, G: 120, B: 100, A: 255}

func fgStripes(w, h, period int, a, b color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x/period)%2 == 0 {
				img.SetRGBA(x, y, a)
			} else {
				img.SetRGBA(x, y, b)
			}
		}
	}
	return img
}

func fgLum(c color.RGBA) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

func fgRowStats(img image.Image, nx, ny float64, n int) (mean, contrast float64) {
	b := img.Bounds()
	x0 := b.Min.X + int(nx*float64(b.Dx())) - n/2
	y := b.Min.Y + int(ny*float64(b.Dy()))
	lo, hi, sum := math.Inf(1), math.Inf(-1), 0.0
	for x := x0; x < x0+n; x++ {
		r, g, bl, _ := img.At(x, y).RGBA()
		l := fgLum(color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8)})
		sum += l
		lo = math.Min(lo, l)
		hi = math.Max(hi, l)
	}
	return sum / float64(n), hi - lo
}

var (
	fgStripeA = color.RGBA{R: 100, G: 100, B: 100, A: 255}
	fgStripeB = color.RGBA{R: 170, G: 170, B: 170, A: 255}
)

func TestApply_FeaturesFromOriginalRestFromEdited(t *testing.T) {
	t.Parallel()
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, fgEditFlat)

	out, err := Apply(orig, edited, fgFace(), fgFace())

	require.NoError(t, err)
	for _, p := range [][2]float64{{0.5, 0.48}, {0.38, fgEyeY}, {0.62, fgEyeY}, {fgLipX, fgLipY}} {
		_, c := fgRowStats(out, p[0], p[1], 6)
		assert.Greater(t, c, 30.0, "original stripes must be visible at feature point %v", p)
	}
	for _, p := range [][2]float64{{0.5, 0.08}, {0.5, 0.15}, {0.06, 0.50}, {0.94, 0.50}, {0.5, 0.95}, {0.5, 0.24}, {0.30, 0.52}, {0.70, 0.52}} {
		_, c := fgRowStats(out, p[0], p[1], 8)
		assert.Less(t, c, 8.0, "no original detail outside the features at %v", p)
		fgNear(t, fgEditFlat, fgAt(out, p[0], p[1]), 12, fmt.Sprintf("edited outside features at %v", p))
	}
}

func fgFillPolygon(img *image.RGBA, poly []Point, c color.RGBA) {
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			px := (float64(x) + 0.5) / float64(b.Dx())
			py := (float64(y) + 0.5) / float64(b.Dy())
			in := false
			for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
				if (poly[i].Y > py) != (poly[j].Y > py) &&
					px < (poly[j].X-poly[i].X)*(py-poly[i].Y)/(poly[j].Y-poly[i].Y)+poly[i].X {
					in = !in
				}
			}
			if in {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func TestApply_LipToneFromEdit(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, color.RGBA{R: 135, G: 135, B: 135, A: 255})
	fgFillPolygon(edited, pts[48:60], fgBlue)

	out, err := Apply(orig, edited, pts, pts)

	require.NoError(t, err)
	c := fgAt(out, fgLipX, fgLipY)
	assert.Greater(t, int(c.B), int(c.R)+40, "lips take the edit's blue tone: %v", c)
	_, contrast := fgRowStats(out, fgLipX, fgLipY, 6)
	assert.Greater(t, contrast, 30.0, "lips keep the original structure")
}

func TestApply_ResultSizeIsEdited(t *testing.T) {
	t.Parallel()
	orig := fgStripes(2*fgW, 2*fgH, 8, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, fgEditFlat)

	out, err := Apply(orig, edited, fgFace(), fgFace())

	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, fgW, out.Bounds().Dx())
	assert.Equal(t, fgH, out.Bounds().Dy())
	_, c := fgRowStats(out, 0.5, 0.48, 6)
	assert.Greater(t, c, 30.0, "scaled original detail on the nose")
	fgNear(t, fgEditFlat, fgAt(out, 0.5, 0.08), 12, "far above forehead")
}

func TestApplyBase64(t *testing.T) {
	t.Parallel()
	origB64 := fgB64(t, fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB))
	editedB64 := fgB64(t, fgSolid(fgW, fgH, fgEditFlat))

	t.Run("valid input gives PNG of edited size", func(t *testing.T) {
		t.Parallel()
		out, err := ApplyBase64(origB64, editedB64, fgFace(), fgFace())
		require.NoError(t, err)
		img := fgDecode(t, out)
		assert.Equal(t, fgW, img.Bounds().Dx())
		assert.Equal(t, fgH, img.Bounds().Dy())
		_, c := fgRowStats(img, 0.5, 0.48, 6)
		assert.Greater(t, c, 30.0, "original detail on the nose")
		fgNear(t, fgEditFlat, fgAt(img, 0.06, 0.50), 12, "outside oval")
	})

	errCases := []struct {
		name   string
		orig   string
		edited string
	}{
		{name: "orig broken base64", orig: "!!!not-base64!!!", edited: editedB64},
		{name: "edited broken base64", orig: origB64, edited: "!!!not-base64!!!"},
		{name: "orig not an image", orig: base64.StdEncoding.EncodeToString([]byte("hello")), edited: editedB64},
		{name: "edited empty", orig: origB64, edited: ""},
		{name: "portrait orig vs landscape edited", orig: origB64, edited: fgB64(t, fgSolid(fgH, fgW, fgRed))},
		{name: "landscape orig vs portrait edited", orig: fgB64(t, fgSolid(fgH, fgW, fgGray)), edited: editedB64},
	}
	for _, tt := range errCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ApplyBase64(tt.orig, tt.edited, fgFace(), fgFace())
			require.Error(t, err)
		})
	}
}

func fgTransform(pts []Point, scale, dx float64) []Point {
	out := make([]Point, len(pts))
	for i, p := range pts {
		out[i] = Point{X: 0.5 + scale*(p.X-0.5) + dx, Y: 0.5 + scale*(p.Y-0.5)}
	}
	return out
}

func fgLumAt(img image.Image, nx, ny float64) float64 {
	return fgLum(fgAt(img, nx, ny))
}

func TestApply_AlignsOriginalToEditedFace(t *testing.T) {
	t.Parallel()
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	const scale, dx = 1.1, 0.05
	origPts := fgFace()
	editPts := fgTransform(origPts, scale, dx)
	spot := [2]float64{0.5, 0.48}
	moved := [2]float64{0.5 + scale*(spot[0]-0.5) + dx, 0.5 + scale*(spot[1]-0.5)}
	orig := fgSolid(fgW, fgH, fgGray)
	skDot(orig, spot[0]*fgW, spot[1]*fgH, 4, dark)
	edited := fgSolid(fgW, fgH, fgEditFlat)
	flat := fgLum(fgEditFlat)

	out, err := Apply(orig, edited, origPts, editPts)

	require.NoError(t, err)
	assert.Less(t, fgLumAt(out, moved[0], moved[1]), flat-20, "nose spot of the original must land on the edited nose")
	assert.InDelta(t, flat, fgLumAt(out, spot[0], spot[1]), 8, "no spot at the unaligned position")
}

func TestApply_RejectsDifferentFace(t *testing.T) {
	t.Parallel()
	origPts := fgFace()
	flipped := make([]Point, len(origPts))
	for i, p := range origPts {
		flipped[i] = Point{X: p.X, Y: 1.05 - p.Y}
	}
	orig := fgSolid(fgW, fgH, fgGray)
	edited := fgSolid(fgW, fgH, fgRed)

	_, err := Apply(orig, edited, origPts, flipped)
	require.Error(t, err, "mouth above eyes must not fit by scale+shift")

	_, err = ApplyBase64(fgB64(t, orig), fgB64(t, edited), origPts, flipped)
	require.Error(t, err)
}

func TestApply_TooFewPoints(t *testing.T) {
	t.Parallel()
	orig := fgSolid(fgW, fgH, fgGray)
	edited := fgSolid(fgW, fgH, fgRed)
	full := fgFace()

	tests := []struct {
		name    string
		origPts []Point
		editPts []Point
	}{
		{name: "edit 67 points", origPts: full, editPts: full[:67]},
		{name: "orig 67 points", origPts: full[:67], editPts: full},
		{name: "edit empty", origPts: full, editPts: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Apply(orig, edited, tt.origPts, tt.editPts)
			require.Error(t, err)
			_, err = ApplyBase64(fgB64(t, orig), fgB64(t, edited), tt.origPts, tt.editPts)
			require.Error(t, err)
		})
	}
}

func TestApply_ExactlyMinimumPoints(t *testing.T) {
	t.Parallel()
	full := fgFace()[:68]
	_, err := Apply(fgSolid(fgW, fgH, fgGray), fgSolid(fgW, fgH, fgRed), full, full)
	require.NoError(t, err)
}

func fgRotatePx(pts []Point, w, h int, cx, cy, deg float64) []Point {
	a := deg * math.Pi / 180
	cos, sin := math.Cos(a), math.Sin(a)
	out := make([]Point, len(pts))
	for i, p := range pts {
		x := p.X*float64(w) - cx
		y := p.Y*float64(h) - cy
		out[i] = Point{X: (cx + x*cos - y*sin) / float64(w), Y: (cy + x*sin + y*cos) / float64(h)}
	}
	return out
}

func TestApply_AlignsRotatedFace(t *testing.T) {
	t.Parallel()
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	const deg = 10.0
	cx, cy := 0.5*fgW, 0.5*fgH
	mark := [2]float64{fgLipX * fgW, fgLipY * fgH}
	a := deg * math.Pi / 180
	ry := mark[1] - cy
	rotMark := [2]float64{cx - ry*math.Sin(a), cy + ry*math.Cos(a)}

	origPts := fgFace()
	editPts := fgRotatePx(origPts, fgW, fgH, cx, cy, deg)
	orig := fgSolid(fgW, fgH, fgGray)
	skDot(orig, mark[0], mark[1], 5, dark)
	edited := fgSolid(fgW, fgH, fgEditFlat)
	flat := fgLum(fgEditFlat)

	out, err := Apply(orig, edited, origPts, editPts)

	require.NoError(t, err, "10° in-plane rotation must fit")
	assert.Less(t, fgLumAt(out, rotMark[0]/fgW, rotMark[1]/fgH), flat-20, "lip marker of the original must follow the rotation")
	assert.InDelta(t, flat, fgLumAt(out, mark[0]/fgW, mark[1]/fgH), 8, "no marker at the unrotated position")
}

func TestApply_FeatureToneFromEdited(t *testing.T) {
	t.Parallel()
	editC := color.RGBA{R: 200, G: 180, B: 160, A: 255}
	edited := fgSolid(fgW, fgH, editC)
	dark := color.RGBA{R: 120, G: 108, B: 96, A: 255}
	orig := fgSolid(fgW, fgH, dark)
	lo := color.RGBA{R: 95, G: 83, B: 71, A: 255}
	hi := color.RGBA{R: 145, G: 133, B: 121, A: 255}
	for y := int(0.44 * fgH); y < int(0.56*fgH); y++ {
		for x := int(0.40 * fgW); x < int(0.60*fgW); x++ {
			if (x/2)%2 == 0 {
				orig.SetRGBA(x, y, lo)
			} else {
				orig.SetRGBA(x, y, hi)
			}
		}
	}
	pts := fgFace()

	out, err := Apply(orig, edited, pts, pts)

	require.NoError(t, err)
	mean, contrast := fgRowStats(out, 0.5, 0.48, 8)
	want := fgLum(editC)
	assert.InDelta(t, want, mean, want*0.08, "nose brightness must follow edited")
	assert.Greater(t, contrast, 30.0, "original stripe pattern must stay visible on the nose")
}

func fgJitter(pts []Point, idx []int, distPx float64, seed int64) []Point {
	out := append([]Point(nil), pts...)
	rng := rand.New(rand.NewSource(seed))
	for _, i := range idx {
		a := rng.Float64() * 2 * math.Pi
		out[i] = Point{X: out[i].X + distPx*math.Cos(a)/fgW, Y: out[i].Y + distPx*math.Sin(a)/fgH}
	}
	return out
}

func TestApply_MismatchThreshold(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	faceW := (pts[16].X - pts[0].X) * fgW
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, fgEditFlat)

	t.Run("mouth corner off by 0.08 face width", func(t *testing.T) {
		t.Parallel()
		edit := append([]Point(nil), pts...)
		edit[48].X += 0.08 * faceW / fgW
		_, err := Apply(orig, edited, pts, edit)
		require.Error(t, err)
	})

	t.Run("all points jittered by 0.02 face width", func(t *testing.T) {
		t.Parallel()
		all := make([]int, len(pts))
		for i := range all {
			all[i] = i
		}
		edit := fgJitter(pts, all, 0.02*faceW, 42)
		_, err := Apply(orig, edited, pts, edit)
		require.NoError(t, err)
	})
}

func TestMeasure(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	faceW := (pts[16].X - pts[0].X) * fgW

	t.Run("identical points", func(t *testing.T) {
		t.Parallel()
		fit, err := Measure(pts, pts, fgW, fgH)
		require.NoError(t, err)
		assert.InDelta(t, 0, fit.Mean, 1e-3)
		assert.InDelta(t, 0, fit.Key, 1e-3)
	})

	t.Run("mouth corner off by 0.08 face width", func(t *testing.T) {
		t.Parallel()
		edit := append([]Point(nil), pts...)
		edit[48].X += 0.08 * faceW / fgW
		fit, err := Measure(pts, edit, fgW, fgH)
		require.NoError(t, err)
		assert.InDelta(t, 0.08, fit.Key, 0.01)
		assert.Less(t, fit.Mean, 0.01)
	})

	t.Run("similarity transform is fitted away", func(t *testing.T) {
		t.Parallel()
		edit := fgRotatePx(fgTransform(pts, 1.1, 0.03), fgW, fgH, 0.5*fgW, 0.5*fgH, 8)
		fit, err := Measure(pts, edit, fgW, fgH)
		require.NoError(t, err)
		assert.Less(t, fit.Mean, 0.005)
		assert.Less(t, fit.Key, 0.005)
	})

	t.Run("too few points", func(t *testing.T) {
		t.Parallel()
		_, err := Measure(pts, pts[:67], fgW, fgH)
		require.Error(t, err)
		_, err = Measure(pts[:67], pts, fgW, fgH)
		require.Error(t, err)
	})
}

func TestMeasureBase64(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	edit := append([]Point(nil), pts...)
	edit[48].X += 0.08 * (pts[16].X - pts[0].X)

	fit, err := MeasureBase64(fgB64(t, fgSolid(fgW, fgH, fgEditFlat)), pts, edit)
	require.NoError(t, err)
	assert.InDelta(t, 0.08, fit.Key, 0.01)

	_, err = MeasureBase64("!!!not-base64!!!", pts, pts)
	require.Error(t, err)
	_, err = MeasureBase64(base64.StdEncoding.EncodeToString([]byte("hello")), pts, pts)
	require.Error(t, err)
	_, err = MeasureBase64(fgB64(t, fgSolid(fgW, fgH, fgEditFlat)), pts, pts[:60])
	require.Error(t, err)
}

func TestApply_MeanThresholdRejectsUniformJitter(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	faceW := (pts[16].X - pts[0].X) * fgW
	all := make([]int, len(pts))
	for i := range all {
		all[i] = i
	}
	edit := fgJitter(pts, all, 0.04*faceW, 7)
	fit, err := Measure(pts, edit, fgW, fgH)
	require.NoError(t, err)
	require.LessOrEqual(t, fit.Key, 0.05, "precondition: key points within 0.05")
	require.Greater(t, fit.Mean, 0.03, "precondition: mean above 0.03")

	_, err = Apply(fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB), fgSolid(fgW, fgH, fgEditFlat), pts, edit)

	require.Error(t, err, "mean residual > 0.03 must reject even when key points pass")
}

func fgCopy(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}
