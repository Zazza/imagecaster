package faceguard

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	rtSkin  = color.RGBA{R: 200, G: 160, B: 140, A: 255}
	rtShade = color.RGBA{R: 140, G: 105, B: 95, A: 255}
	rtWhite = color.RGBA{R: 230, G: 225, B: 220, A: 255}
	rtIris  = color.RGBA{R: 110, G: 70, B: 40, A: 255}
	rtPupil = color.RGBA{R: 10, G: 10, B: 10, A: 255}
	rtTooth = color.RGBA{R: 225, G: 210, B: 140, A: 255}
	rtMouth = color.RGBA{R: 50, G: 20, B: 20, A: 255}
)

const (
	rtBandTop = 0.420
	rtBandBot = 0.442
	rtCheekY  = 0.49
)

func rtFillRect(img *image.RGBA, x0, y0, x1, y1 float64, c color.RGBA) {
	b := img.Bounds()
	for y := int(y0 * float64(b.Dy())); y < int(y1*float64(b.Dy())); y++ {
		for x := int(x0 * float64(b.Dx())); x < int(x1*float64(b.Dx())); x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func rtInPoly(poly []Point, px, py float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		if (poly[i].Y > py) != (poly[j].Y > py) &&
			px < (poly[j].X-poly[i].X)*(py-poly[i].Y)/(poly[j].Y-poly[i].Y)+poly[i].X {
			in = !in
		}
	}
	return in
}

func rtScale(poly []Point, k float64) []Point {
	var cx, cy float64
	for _, p := range poly {
		cx += p.X
		cy += p.Y
	}
	cx /= float64(len(poly))
	cy /= float64(len(poly))
	out := make([]Point, len(poly))
	for i, p := range poly {
		out[i] = Point{X: cx + (p.X-cx)*k, Y: cy + (p.Y-cy)*k}
	}
	return out
}

func rtEach(img image.Image, fn func(x, y int, px, py float64)) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			fn(x, y, (float64(x-b.Min.X)+0.5)/float64(b.Dx()), (float64(y-b.Min.Y)+0.5)/float64(b.Dy()))
		}
	}
}

func rtRGBA(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func rtMaxDiff(a, b color.RGBA) int {
	d := 0
	for _, v := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B)} {
		if v < 0 {
			v = -v
		}
		if v > d {
			d = v
		}
	}
	return d
}

func rtMeanLum(img image.Image, x0, y0, x1, y1 float64) float64 {
	var sum float64
	n := 0
	rtEach(img, func(x, y int, px, py float64) {
		if px >= x0 && px < x1 && py >= y0 && py < y1 {
			sum += fgLum(rtRGBA(img, x, y))
			n++
		}
	})
	return sum / float64(n)
}

func rtFace(withBand bool) *image.RGBA {
	img := fgSolid(fgW, fgH, rtSkin)
	pts := fgFace()
	if withBand {
		for _, cx := range []float64{0.38, 0.62} {
			rtFillRect(img, cx-0.045, rtBandTop, cx+0.045, rtBandBot, rtShade)
		}
	}
	fgFillPolygon(img, pts[36:42], rtWhite)
	fgFillPolygon(img, pts[42:48], rtWhite)
	return img
}

func rtFaceWithIris() *image.RGBA {
	img := rtFace(false)
	pts := fgFace()
	b := img.Bounds()
	for _, eye := range []struct {
		poly  []Point
		pupil Point
	}{{pts[36:42], pts[68]}, {pts[42:48], pts[69]}} {
		cx, cy := eye.pupil.X*float64(b.Dx()), eye.pupil.Y*float64(b.Dy())
		rtEach(img, func(x, y int, px, py float64) {
			if !rtInPoly(eye.poly, px, py) {
				return
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			switch {
			case d <= 3:
				img.SetRGBA(x, y, rtPupil)
			case d <= 8:
				img.SetRGBA(x, y, rtIris)
			}
		})
	}
	return img
}

func rtMouthPts() []Point {
	pts := fgFace()
	copy(pts[60:68], fgLipRing(fgLipX, fgLipY, 0.07, 0.03, 8))
	return pts
}

func rtFaceWithTeeth() (*image.RGBA, []Point) {
	pts := rtMouthPts()
	img := fgSolid(fgW, fgH, rtSkin)
	fgFillPolygon(img, pts[48:60], color.RGBA{R: 180, G: 80, B: 80, A: 255})
	fgFillPolygon(img, pts[60:68], rtTooth)
	inner := pts[60:68]
	rtEach(img, func(x, y int, px, py float64) {
		if px > fgLipX+0.03 && rtInPoly(inner, px, py) {
			img.SetRGBA(x, y, rtMouth)
		}
	})
	return img, pts
}

func TestRetouch_Flags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		r         Retouch
		wantZero  bool
		wantLocal bool
	}{
		{name: "empty", r: Retouch{}, wantZero: true, wantLocal: false},
		{name: "light only", r: Retouch{Light: true}, wantZero: false, wantLocal: false},
		{name: "eye color", r: Retouch{EyeColor: "green"}, wantZero: false, wantLocal: true},
		{name: "under eyes", r: Retouch{UnderEyes: true}, wantZero: false, wantLocal: true},
		{name: "teeth", r: Retouch{Teeth: true}, wantZero: false, wantLocal: true},
		{name: "eye whites", r: Retouch{EyeWhites: true}, wantZero: false, wantLocal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantZero, tt.r.IsZero())
			assert.Equal(t, tt.wantLocal, tt.r.Local())
		})
	}
}

func TestEyeColorRGB(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		wantErr bool
		want    *[3]float64
	}{
		{name: "blue", in: "blue"},
		{name: "case insensitive", in: "Green"},
		{name: "russian", in: "зелёный"},
		{name: "hex", in: "#336699", want: &[3]float64{51, 102, 153}},
		{name: "unknown word", in: "purple-ish", wantErr: true},
		{name: "short hex", in: "#12", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := EyeColorRGB(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.want != nil {
				assert.Equal(t, *tt.want, got)
			}
		})
	}
}

func TestApplyRetouch_UnderEyes(t *testing.T) {
	t.Parallel()
	img := rtFace(true)
	pts := fgFace()
	bandBefore := rtMeanLum(img, 0.345, rtBandTop+0.004, 0.415, rtBandBot-0.004)
	cheek := rtMeanLum(img, 0.345, rtCheekY, 0.415, rtCheekY+0.02)
	require.Less(t, bandBefore, cheek-20)

	out, err := ApplyRetouch(img, pts, Retouch{UnderEyes: true})
	require.NoError(t, err)
	require.Equal(t, img.Bounds(), out.Bounds())

	bandAfter := rtMeanLum(out, 0.345, rtBandTop+0.004, 0.415, rtBandBot-0.004)
	assert.Greater(t, bandAfter, bandBefore+3, "under-eye band must get lighter")
	assert.Less(t, math.Abs(cheek-bandAfter), math.Abs(cheek-bandBefore), "band must get closer to cheek")

	for _, eye := range [][]Point{pts[36:42], pts[42:48]} {
		inner := rtScale(eye, 0.85)
		worst := 0
		rtEach(img, func(x, y int, px, py float64) {
			if rtInPoly(inner, px, py) {
				if d := rtMaxDiff(rtRGBA(img, x, y), rtRGBA(out, x, y)); d > worst {
					worst = d
				}
			}
		})
		assert.LessOrEqual(t, worst, 2, "pixels inside the eye contour must stay")
	}
}

func TestAnalyze_UnderEye(t *testing.T) {
	t.Parallel()
	pts := fgFace()

	dark, err := Analyze(rtFace(true), pts)
	require.NoError(t, err)
	clean, err := Analyze(rtFace(false), pts)
	require.NoError(t, err)

	assert.Greater(t, dark.UnderEye, 0.0)
	assert.Greater(t, dark.UnderEye, clean.UnderEye)
	assert.Less(t, math.Abs(clean.UnderEye), 0.05)

	none, err := Analyze(rtFace(true), nil)
	require.NoError(t, err)
	assert.Equal(t, 0.0, none.UnderEye)
}

func TestApplyRetouch_Teeth(t *testing.T) {
	t.Parallel()
	img, pts := rtFaceWithTeeth()
	inner := pts[60:68]

	out, err := ApplyRetouch(img, pts, Retouch{Teeth: true})
	require.NoError(t, err)

	core := rtScale(inner, 0.8)
	var teethB0, teethB1, darkL0, darkL1 float64
	nt, nd := 0, 0
	worstOutside := 0
	outer := rtScale(inner, 1.6)
	rtEach(img, func(x, y int, px, py float64) {
		a, b := rtRGBA(img, x, y), rtRGBA(out, x, y)
		switch {
		case rtInPoly(core, px, py) && a == rtTooth:
			teethB0 += float64(a.B)
			teethB1 += float64(b.B)
			nt++
		case rtInPoly(core, px, py) && a == rtMouth:
			darkL0 += fgLum(a)
			darkL1 += fgLum(b)
			nd++
		case !rtInPoly(outer, px, py):
			if d := rtMaxDiff(a, b); d > worstOutside {
				worstOutside = d
			}
		}
	})
	require.Positive(t, nt)
	require.Positive(t, nd)
	assert.Greater(t, teethB1/float64(nt), teethB0/float64(nt)+3, "teeth must get less yellow")
	assert.Less(t, darkL1/float64(nd), darkL0/float64(nd)+10, "dark mouth must not get noticeably lighter")
	assert.LessOrEqual(t, worstOutside, 2, "pixels outside the mouth must stay")
}

func TestApplyRetouch_EyeColor(t *testing.T) {
	t.Parallel()

	t.Run("iris turns blue, rest stays", func(t *testing.T) {
		t.Parallel()
		img := rtFaceWithIris()
		pts := fgFace()
		require.Len(t, pts, 70)

		out, err := ApplyRetouch(img, pts, Retouch{EyeColor: "blue"})
		require.NoError(t, err)

		b := img.Bounds()
		for i, eye := range []struct {
			poly  []Point
			pupil Point
		}{{pts[36:42], pts[68]}, {pts[42:48], pts[69]}} {
			cx, cy := eye.pupil.X*float64(b.Dx()), eye.pupil.Y*float64(b.Dy())
			var before, after float64
			n := 0
			worst := 0
			outside := rtScale(eye.poly, 1.4)
			rtEach(img, func(x, y int, px, py float64) {
				a, c := rtRGBA(img, x, y), rtRGBA(out, x, y)
				d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
				if rtInPoly(eye.poly, px, py) && d > 3.5 && d <= 6.5 && a == rtIris {
					before += float64(a.B) - float64(a.R)
					after += float64(c.B) - float64(c.R)
					n++
				}
				if !rtInPoly(outside, px, py) && px > eye.pupil.X-0.12 && px < eye.pupil.X+0.12 {
					if v := rtMaxDiff(a, c); v > worst {
						worst = v
					}
				}
			})
			require.Positive(t, n)
			assert.Greater(t, after/float64(n), before/float64(n)+10, "eye %d iris must get bluer", i)
			assert.LessOrEqual(t, worst, 2, "eye %d: pixels outside the eye contour must stay", i)
		}
	})

	t.Run("68 points without pupils is an error", func(t *testing.T) {
		t.Parallel()
		_, err := ApplyRetouch(rtFaceWithIris(), fgFace()[:68], Retouch{EyeColor: "blue"})
		require.Error(t, err)
	})
}

func TestApplyRetouch_Points(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		r       Retouch
		wantErr bool
	}{
		{name: "under eyes without points", r: Retouch{UnderEyes: true}, wantErr: true},
		{name: "teeth without points", r: Retouch{Teeth: true}, wantErr: true},
		{name: "eye whites without points", r: Retouch{EyeWhites: true}, wantErr: true},
		{name: "eye color without points", r: Retouch{EyeColor: "blue"}, wantErr: true},
		{name: "light without points", r: Retouch{Light: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			img := rtFace(true)
			out, err := ApplyRetouch(img, nil, tt.r)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, out)
			assert.Equal(t, img.Bounds().Size(), out.Bounds().Size())
		})
	}
}

func TestApplyRetouchBase64_BadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		b64  string
	}{
		{name: "not base64", b64: "!!!not-base64!!!"},
		{name: "base64 of not an image", b64: "aGVsbG8gd29ybGQ="},
		{name: "empty", b64: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ApplyRetouchBase64(tt.b64, fgFace(), Retouch{UnderEyes: true})
			require.Error(t, err)
		})
	}
}

func TestApplyRetouchBase64_ReturnsPNG(t *testing.T) {
	t.Parallel()
	img := rtFace(true)
	out, err := ApplyRetouchBase64(fgB64(t, img), fgFace(), Retouch{UnderEyes: true})
	require.NoError(t, err)
	assert.Equal(t, img.Bounds().Size(), fgDecode(t, out).Bounds().Size())
}
