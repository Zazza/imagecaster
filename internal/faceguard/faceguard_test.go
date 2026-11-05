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

func TestApply_FaceDetailFromOriginalRestFromEdited(t *testing.T) {
	t.Parallel()
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, fgEditFlat)

	out, err := Apply(orig, edited, fgFace(), fgFace(), false)

	require.NoError(t, err)
	for _, p := range [][2]float64{{0.5, 0.50}, {0.42, 0.47}} {
		_, c := fgRowStats(out, p[0], p[1], 8)
		assert.Greater(t, c, 30.0, "original stripes must be visible at face point %v", p)
	}
	for _, p := range [][2]float64{{0.5, 0.08}, {0.5, 0.15}, {0.06, 0.50}, {0.94, 0.50}, {0.5, 0.95}} {
		_, c := fgRowStats(out, p[0], p[1], 8)
		assert.Less(t, c, 8.0, "no original detail outside the face at %v", p)
		fgNear(t, fgEditFlat, fgAt(out, p[0], p[1]), 12, fmt.Sprintf("edited outside face at %v", p))
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

func TestApply_Makeup(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, color.RGBA{R: 135, G: 135, B: 135, A: 255})
	fgFillPolygon(edited, pts[48:60], fgBlue)
	lowerLip := fgLipY + 0.025
	blueness := func(c color.RGBA) int { return int(c.B) - (int(c.R)+int(c.G))/2 }

	off, err := Apply(orig, edited, pts, pts, false)
	require.NoError(t, err)
	on, err := Apply(orig, edited, pts, pts, true)
	require.NoError(t, err)

	cOff := fgAt(off, fgLipX, lowerLip)
	cOn := fgAt(on, fgLipX, lowerLip)
	assert.Greater(t, blueness(cOn), blueness(cOff)+30, "makeup on must take lip colour from edited: on=%v off=%v", cOn, cOff)
	assert.Greater(t, int(cOn.B), int(cOn.R)+40, "blue must dominate on lips with makeup: %v", cOn)
	_, c := fgRowStats(on, 0.5, 0.50, 8)
	assert.Greater(t, c, 30.0, "face centre keeps original detail with makeup")
}

func TestApply_ResultSizeIsEdited(t *testing.T) {
	t.Parallel()
	orig := fgStripes(2*fgW, 2*fgH, 8, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, fgEditFlat)

	out, err := Apply(orig, edited, fgFace(), fgFace(), false)

	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, fgW, out.Bounds().Dx())
	assert.Equal(t, fgH, out.Bounds().Dy())
	_, c := fgRowStats(out, 0.5, 0.50, 12)
	assert.Greater(t, c, 30.0, "scaled original detail at face centre")
	fgNear(t, fgEditFlat, fgAt(out, 0.5, 0.08), 12, "far above forehead")
}

func TestApplyBase64(t *testing.T) {
	t.Parallel()
	origB64 := fgB64(t, fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB))
	editedB64 := fgB64(t, fgSolid(fgW, fgH, fgEditFlat))

	t.Run("valid input gives PNG of edited size", func(t *testing.T) {
		t.Parallel()
		out, err := ApplyBase64(origB64, editedB64, fgFace(), fgFace(), false)
		require.NoError(t, err)
		img := fgDecode(t, out)
		assert.Equal(t, fgW, img.Bounds().Dx())
		assert.Equal(t, fgH, img.Bounds().Dy())
		_, c := fgRowStats(img, 0.5, 0.50, 8)
		assert.Greater(t, c, 30.0, "original detail at face centre")
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
			_, err := ApplyBase64(tt.orig, tt.edited, fgFace(), fgFace(), false)
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

func fgWithDisk(w, h int, bg, disk color.RGBA, cx, cy, r float64) *image.RGBA {
	img := fgSolid(w, h, bg)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := (float64(x)+0.5)/float64(w) - cx
			dy := ((float64(y)+0.5)/float64(h) - cy) * float64(h) / float64(w)
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, disk)
			}
		}
	}
	return img
}

func TestApply_AlignsOriginalToEditedFace(t *testing.T) {
	t.Parallel()
	green := color.RGBA{R: 20, G: 220, B: 20, A: 255}
	const scale, dx, r = 1.1, 0.05, 0.04
	origPts := fgFace()
	editPts := fgTransform(origPts, scale, dx)
	orig := fgWithDisk(fgW, fgH, fgGray, green, 0.5, 0.5, r)
	edited := fgWithDisk(fgW, fgH, fgGray, fgBlue, 0.5+dx, 0.5, r*scale)

	out, err := Apply(orig, edited, origPts, editPts, false)

	require.NoError(t, err)
	fgGreenish(t, fgAt(out, 0.5+dx, 0.5), "edited face center must show the original face center")
	fgNear(t, fgGray, fgAt(out, 0.5+dx+0.08, 0.5), 40, "next to the face center")
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

	_, err := Apply(orig, edited, origPts, flipped, false)
	require.Error(t, err, "mouth above eyes must not fit by scale+shift")

	_, err = ApplyBase64(fgB64(t, orig), fgB64(t, edited), origPts, flipped, false)
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
			_, err := Apply(orig, edited, tt.origPts, tt.editPts, false)
			require.Error(t, err)
			_, err = ApplyBase64(fgB64(t, orig), fgB64(t, edited), tt.origPts, tt.editPts, false)
			require.Error(t, err)
		})
	}
}

func TestApply_ExactlyMinimumPoints(t *testing.T) {
	t.Parallel()
	full := fgFace()[:68]
	_, err := Apply(fgSolid(fgW, fgH, fgGray), fgSolid(fgW, fgH, fgRed), full, full, false)
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

func fgDisksPx(w, h int, bg color.RGBA, centerC, markC color.RGBA, centers [][2]float64, r float64) *image.RGBA {
	img := fgSolid(w, h, bg)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for i, c := range centers {
				dx, dy := float64(x)+0.5-c[0], float64(y)+0.5-c[1]
				if dx*dx+dy*dy <= r*r {
					if i == 0 {
						img.SetRGBA(x, y, centerC)
					} else {
						img.SetRGBA(x, y, markC)
					}
				}
			}
		}
	}
	return img
}

func TestApply_AlignsRotatedFace(t *testing.T) {
	t.Parallel()
	green := color.RGBA{R: 20, G: 220, B: 20, A: 255}
	yellow := color.RGBA{R: 230, G: 220, B: 20, A: 255}
	const deg, r = 10.0, 8.0
	cx, cy := 0.5*fgW, 0.5*fgH
	mark := [2]float64{cx, cy + 72}
	a := deg * math.Pi / 180
	rotMark := [2]float64{cx - 72*math.Sin(a), cy + 72*math.Cos(a)}

	origPts := fgFace()
	editPts := fgRotatePx(origPts, fgW, fgH, cx, cy, deg)
	orig := fgDisksPx(fgW, fgH, fgGray, green, yellow, [][2]float64{{cx, cy}, mark}, r)
	edited := fgDisksPx(fgW, fgH, fgGray, fgBlue, fgBlue, [][2]float64{{cx, cy}, rotMark}, r)

	out, err := Apply(orig, edited, origPts, editPts, false)

	require.NoError(t, err, "10° in-plane rotation must fit")
	fgGreenish(t, fgAt(out, cx/fgW, cy/fgH), "edited face center must show the original face center")
	c := fgAt(out, rotMark[0]/fgW, rotMark[1]/fgH)
	assert.True(t, int(c.R) > int(c.B)+60 && int(c.G) > int(c.B)+60, "rotated marker must show the original yellow marker: %v", c)
}

func fgGreenish(t *testing.T, c color.RGBA, msg string) {
	t.Helper()
	assert.True(t, int(c.G) > int(c.R)+60 && int(c.G) > int(c.B)+60, "%s: want green-dominant, got %v", msg, c)
}

func TestApply_RelightsToEditedBrightness(t *testing.T) {
	t.Parallel()
	edited := fgSolid(fgW, fgH, color.RGBA{R: 200, G: 180, B: 160, A: 255})
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

	out, err := Apply(orig, edited, pts, pts, false)

	require.NoError(t, err)
	mean, contrast := fgRowStats(out, 0.5, 0.50, 16)
	want := fgLum(color.RGBA{R: 200, G: 180, B: 160})
	assert.InDelta(t, want, mean, want*0.12, "face brightness must follow edited")
	assert.Greater(t, contrast, 30.0, "original stripe pattern must stay visible")
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
		_, err := Apply(orig, edited, pts, edit, false)
		require.Error(t, err)
	})

	t.Run("all points jittered by 0.02 face width", func(t *testing.T) {
		t.Parallel()
		all := make([]int, len(pts))
		for i := range all {
			all[i] = i
		}
		edit := fgJitter(pts, all, 0.02*faceW, 42)
		_, err := Apply(orig, edited, pts, edit, false)
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

	_, err = Apply(fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB), fgSolid(fgW, fgH, fgEditFlat), pts, edit, false)

	require.Error(t, err, "mean residual > 0.03 must reject even when key points pass")
}

func fgCopy(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

func fgHairGreen(orig *image.RGBA) *image.RGBA {
	green := fgCopy(orig)
	for y := 50; y < 250; y++ {
		for x := 80; x < 100; x++ {
			green.SetRGBA(x, y, color.RGBA{G: 255, A: 255})
		}
	}
	green.SetRGBA(20, 20, color.RGBA{G: 255, A: 255})
	return green
}

func TestHairMask(t *testing.T) {
	t.Parallel()
	orig := fgSolid(200, 300, fgGray)

	t.Run("strand is hair, single-pixel noise is removed", func(t *testing.T) {
		t.Parallel()
		mask := HairMask(orig, fgHairGreen(orig))
		require.NotNil(t, mask)
		assert.Equal(t, orig.Bounds(), mask.Bounds())
		assert.Greater(t, int(mask.AlphaAt(90, 150).A), 200, "strand centre")
		assert.Greater(t, int(mask.AlphaAt(85, 60).A), 200, "strand near its end")
		assert.Equal(t, uint8(0), mask.AlphaAt(150, 150).A, "away from strand")
		assert.Equal(t, uint8(0), mask.AlphaAt(90, 280).A, "below strand")
		assert.Equal(t, uint8(0), mask.AlphaAt(20, 20).A, "1x1 noise removed by opening")
	})

	t.Run("green equal to orig gives empty mask", func(t *testing.T) {
		t.Parallel()
		mask := HairMask(orig, fgCopy(orig))
		require.NotNil(t, mask)
		b := mask.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				require.Equal(t, uint8(0), mask.AlphaAt(x, y).A, "pixel %d,%d", x, y)
			}
		}
	})

	t.Run("green-ish orig is not hair without a change", func(t *testing.T) {
		t.Parallel()
		greenOrig := fgSolid(200, 300, color.RGBA{G: 255, A: 255})
		mask := HairMask(greenOrig, fgCopy(greenOrig))
		assert.Equal(t, uint8(0), mask.AlphaAt(100, 150).A)
	})
}

func TestHairMaskBase64(t *testing.T) {
	t.Parallel()
	orig := fgSolid(200, 300, fgGray)

	mask, err := HairMaskBase64(fgB64(t, orig), fgB64(t, fgHairGreen(orig)))
	require.NoError(t, err)
	require.NotNil(t, mask)
	assert.Equal(t, orig.Bounds(), mask.Bounds())
	a, _, _, _ := mask.At(90, 150).RGBA()
	assert.Greater(t, int(a>>8), 200)

	_, err = HairMaskBase64("!!!not-base64!!!", fgB64(t, orig))
	require.Error(t, err)
	_, err = HairMaskBase64(fgB64(t, orig), base64.StdEncoding.EncodeToString([]byte("hello")))
	require.Error(t, err)
}

func fgRectAlpha(w, h int, rects ...[4]float64) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	for _, r := range rects {
		for y := int(r[1] * float64(h)); y < int(r[3]*float64(h)); y++ {
			for x := int(r[0] * float64(w)); x < int(r[2]*float64(w)); x++ {
				m.SetAlpha(x, y, color.Alpha{A: 255})
			}
		}
	}
	return m
}

func TestApplyWithHair(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	green := color.RGBA{R: 20, G: 220, B: 20, A: 255}
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	for y := 0; y < fgH; y++ {
		for x := 0; x < fgW; x++ {
			dx, dy := float64(x)+0.5-0.5*fgW, float64(y)+0.5-0.5*fgH
			if dx*dx+dy*dy <= 64 {
				orig.SetRGBA(x, y, green)
			}
		}
	}
	edited := fgSolid(fgW, fgH, fgEditFlat)
	cheek := [4]float64{0.34, 0.46, 0.42, 0.56}
	throughEye := [4]float64{0.60, 0.30, 0.64, 0.47}
	hair := fgRectAlpha(fgW, fgH, cheek, throughEye)

	out, err := ApplyWithHair(orig, edited, hair, pts, pts, false)

	require.NoError(t, err)
	_, c := fgRowStats(out, 0.38, 0.51, 8)
	assert.Less(t, c, 8.0, "old strand on cheek must be replaced by edited")
	fgNear(t, fgEditFlat, fgAt(out, 0.38, 0.51), 25, "cheek strand")
	fgGreenish(t, fgAt(out, 0.5, 0.5), "marker between eyes and lips outside strand stays original")
	_, c = fgRowStats(out, 0.62, 0.40, 8)
	assert.Greater(t, c, 30.0, "eye under a strand stays original")
	_, c = fgRowStats(out, 0.62, 0.455, 6)
	assert.Less(t, c, 8.0, "strand below the eye is replaced by edited")
	_, c = fgRowStats(out, 0.42, 0.66, 8)
	assert.Greater(t, c, 30.0, "lips without strand stay original")
}

func TestApplyWithHair_NilIsApply(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	edited := fgSolid(fgW, fgH, fgEditFlat)

	for _, makeup := range []bool{false, true} {
		want, err := Apply(orig, edited, pts, pts, makeup)
		require.NoError(t, err)
		got, err := ApplyWithHair(orig, edited, nil, pts, pts, makeup)
		require.NoError(t, err)
		assert.Equal(t, want.Bounds(), got.Bounds())
		assert.True(t, bytes.Equal(want.Pix, got.Pix), "makeup=%v: nil hair must equal Apply", makeup)

		wantB64, err := ApplyBase64(fgB64(t, orig), fgB64(t, edited), pts, pts, makeup)
		require.NoError(t, err)
		gotB64, err := ApplyBase64WithHair(fgB64(t, orig), fgB64(t, edited), nil, pts, pts, makeup)
		require.NoError(t, err)
		assert.Equal(t, wantB64, gotB64, "makeup=%v: nil hair must equal ApplyBase64", makeup)
	}
}

func TestApplyBase64WithHair(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	orig := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	hair := fgRectAlpha(fgW, fgH, [4]float64{0.34, 0.46, 0.42, 0.56})

	out, err := ApplyBase64WithHair(fgB64(t, orig), fgB64(t, fgSolid(fgW, fgH, fgEditFlat)), hair, pts, pts, false)

	require.NoError(t, err)
	img := fgDecode(t, out)
	_, c := fgRowStats(img, 0.38, 0.51, 8)
	assert.Less(t, c, 8.0, "strand replaced")
	_, err = ApplyBase64WithHair("!!!", fgB64(t, orig), hair, pts, pts, false)
	require.Error(t, err)
}
