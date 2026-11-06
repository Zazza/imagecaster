package faceguard

import (
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var skCheekL = [2]float64{0.35, 0.51}

func skGray(w, h int, v uint8) *image.RGBA {
	return fgSolid(w, h, color.RGBA{R: v, G: v, B: v, A: 255})
}

func skDot(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func skCheekMedian(img image.Image, pts []Point) [3]float64 {
	var ch [3][]float64
	for _, poly := range [][]Point{
		{pts[1], pts[3], pts[31], pts[40]},
		{pts[15], pts[13], pts[35], pts[47]},
	} {
		rtEach(img, func(x, y int, px, py float64) {
			if !rtInPoly(poly, px, py) {
				return
			}
			c := rtRGBA(img, x, y)
			ch[0] = append(ch[0], float64(c.R))
			ch[1] = append(ch[1], float64(c.G))
			ch[2] = append(ch[2], float64(c.B))
		})
	}
	var out [3]float64
	for i := range ch {
		sort.Float64s(ch[i])
		out[i] = ch[i][len(ch[i])/2]
	}
	return out
}

func TestSkinLift(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	tests := []struct {
		name   string
		orig   *image.RGBA
		edited func() *image.RGBA
		want   float64
	}{
		{name: "SK4 edit 20% lighter", orig: skGray(fgW, fgH, 100), edited: func() *image.RGBA { return skGray(fgW, fgH, 120) }, want: 20},
		{name: "SK4 edit 3% lighter", orig: skGray(fgW, fgH, 100), edited: func() *image.RGBA { return skGray(fgW, fgH, 103) }, want: 3},
		{name: "edit 20% darker", orig: skGray(fgW, fgH, 100), edited: func() *image.RGBA { return skGray(fgW, fgH, 80) }, want: -20},
		{name: "same tone", orig: skGray(fgW, fgH, 100), edited: func() *image.RGBA { return skGray(fgW, fgH, 100) }, want: 0},
		{name: "orig twice the size", orig: skGray(2*fgW, 2*fgH, 100), edited: func() *image.RGBA { return skGray(fgW, fgH, 120) }, want: 20},
		{name: "only forehead and background lighter", orig: skGray(fgW, fgH, 100), edited: func() *image.RGBA {
			img := skGray(fgW, fgH, 100)
			grFillRect(img, [4]float64{0, 0, 1, 0.30}, color.RGBA{R: 200, G: 200, B: 200, A: 255})
			grFillRect(img, [4]float64{0, 0, 0.2, 1}, color.RGBA{R: 200, G: 200, B: 200, A: 255})
			return img
		}, want: 0},
		{name: "median ignores a small spot on the cheek", orig: skGray(fgW, fgH, 100), edited: func() *image.RGBA {
			img := skGray(fgW, fgH, 120)
			skDot(img, skCheekL[0]*fgW, skCheekL[1]*fgH, 4, color.RGBA{A: 255})
			return img
		}, want: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := SkinLift(tt.orig, tt.edited(), pts, pts)
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 1.0)
		})
	}

	t.Run("too few points", func(t *testing.T) {
		t.Parallel()
		_, err := SkinLift(skGray(fgW, fgH, 100), skGray(fgW, fgH, 120), pts, pts[:40])
		require.Error(t, err)
		_, err = SkinLift(skGray(fgW, fgH, 100), skGray(fgW, fgH, 120), nil, pts)
		require.Error(t, err)
	})
}

func TestToneSkin(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	resultSkin := color.RGBA{R: 150, G: 120, B: 100, A: 255}
	origSkin := color.RGBA{R: 120, G: 108, B: 90, A: 255}
	far := [2]float64{0.05, 0.05}
	farC := color.RGBA{R: 100, G: 100, B: 100, A: 255}
	newResult := func() *image.RGBA {
		img := fgSolid(fgW, fgH, resultSkin)
		grFillRect(img, [4]float64{0, 0, 0.1, 0.1}, farC)
		return img
	}
	scale := func(c color.RGBA, k [3]float64) color.RGBA {
		f := func(v uint8, k float64) uint8 { return uint8(math.Min(255, math.Round(float64(v)*k))) }
		return color.RGBA{R: f(c.R, k[0]), G: f(c.G, k[1]), B: f(c.B, k[2]), A: 255}
	}
	ratio := [3]float64{120.0 / 150, 108.0 / 120, 90.0 / 100}

	tests := []struct {
		name      string
		match     bool
		lightness float64
		wantCheek color.RGBA
		wantFar   color.RGBA
		tol       int
	}{
		{name: "SK5 original tone: cheeks equal orig, whole frame scaled", match: true, wantCheek: origSkin, wantFar: scale(farC, ratio), tol: 2},
		{name: "SK6 lightness -10: frame x0.9", lightness: -10, wantCheek: scale(resultSkin, [3]float64{0.9, 0.9, 0.9}), wantFar: scale(farC, [3]float64{0.9, 0.9, 0.9}), tol: 1},
		{name: "lightness +10: frame x1.1", lightness: 10, wantCheek: scale(resultSkin, [3]float64{1.1, 1.1, 1.1}), wantFar: scale(farC, [3]float64{1.1, 1.1, 1.1}), tol: 1},
		{name: "nothing asked: unchanged", wantCheek: resultSkin, wantFar: farC, tol: 1},
		{name: "original tone then +10", match: true, lightness: 10, wantCheek: scale(origSkin, [3]float64{1.1, 1.1, 1.1}), wantFar: scale(scale(farC, ratio), [3]float64{1.1, 1.1, 1.1}), tol: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orig := fgSolid(2*fgW, 2*fgH, origSkin)
			out, err := ToneSkin(newResult(), orig, pts, pts, tt.match, tt.lightness)
			require.NoError(t, err)
			require.NotNil(t, out)
			assert.Equal(t, image.Rect(0, 0, fgW, fgH), out.Bounds(), "result keeps its size")
			med := skCheekMedian(out, pts)
			got := color.RGBA{R: uint8(med[0]), G: uint8(med[1]), B: uint8(med[2]), A: 255}
			fgNear(t, tt.wantCheek, got, tt.tol, "cheek median")
			fgNear(t, tt.wantFar, fgAt(out, far[0], far[1]), tt.tol, "far from face")
		})
	}

	t.Run("original tone needs points", func(t *testing.T) {
		t.Parallel()
		_, err := ToneSkin(newResult(), fgSolid(fgW, fgH, origSkin), pts[:40], pts, true, 0)
		require.Error(t, err)
		_, err = ToneSkin(newResult(), fgSolid(fgW, fgH, origSkin), pts, nil, true, 0)
		require.Error(t, err)
	})
}

func TestToneSkinBase64(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	res := fgB64(t, fgSolid(fgW, fgH, color.RGBA{R: 150, G: 120, B: 100, A: 255}))
	orig := fgB64(t, fgSolid(fgW, fgH, color.RGBA{R: 120, G: 108, B: 90, A: 255}))

	out, err := ToneSkinBase64(res, orig, pts, pts, true, 0)
	require.NoError(t, err)
	img := fgDecode(t, out)
	assert.Equal(t, image.Rect(0, 0, fgW, fgH), img.Bounds())
	fgNear(t, color.RGBA{R: 120, G: 108, B: 90, A: 255}, fgAt(img, skCheekL[0], skCheekL[1]), 2, "cheek")

	for _, bad := range [][2]string{
		{"!!!not-base64!!!", orig},
		{res, "!!!not-base64!!!"},
		{base64.StdEncoding.EncodeToString([]byte("hello")), orig},
	} {
		_, err := ToneSkinBase64(bad[0], bad[1], pts, pts, true, 0)
		require.Error(t, err)
	}
}

func TestRetouch_SkinFieldsValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		r       Retouch
		wantErr bool
	}{
		{name: "empty", r: Retouch{}},
		{name: "skin tone original", r: Retouch{SkinTone: "original"}},
		{name: "skin tone other word", r: Retouch{SkinTone: "dark"}, wantErr: true},
		{name: "lightness -30", r: Retouch{SkinLightness: -30}},
		{name: "lightness 30", r: Retouch{SkinLightness: 30}},
		{name: "lightness -10", r: Retouch{SkinLightness: -10}},
		{name: "lightness 40", r: Retouch{SkinLightness: 40}, wantErr: true},
		{name: "lightness -30.5", r: Retouch{SkinLightness: -30.5}, wantErr: true},
		{name: "face width -10", r: Retouch{FaceWidth: -10}},
		{name: "face width 10", r: Retouch{FaceWidth: 10}},
		{name: "face width 15", r: Retouch{FaceWidth: 15}, wantErr: true},
		{name: "face width -11", r: Retouch{FaceWidth: -11}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.r.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRetouch_SkinFieldsFlags(t *testing.T) {
	t.Parallel()
	for _, r := range []Retouch{{SkinTone: "original"}, {SkinLightness: -10}, {FaceWidth: -8}} {
		assert.False(t, r.IsZero(), "%+v", r)
	}
	assert.True(t, Retouch{FaceWidth: 5}.Local())
	assert.True(t, Retouch{FaceWidth: -5}.Local())

	var r Retouch
	require.NoError(t, json.Unmarshal([]byte(`{"skin_tone":"original","skin_lightness":-10,"face_width":5}`), &r))
	assert.Equal(t, "original", r.SkinTone)
	assert.Equal(t, -10.0, r.SkinLightness)
	assert.Equal(t, 5.0, r.FaceWidth)
}

func skCentroid(img image.Image, cx, cy float64, win int, bg float64) (float64, float64) {
	var sx, sy, sw float64
	for y := int(cy) - win; y <= int(cy)+win; y++ {
		for x := int(cx) - win; x <= int(cx)+win; x++ {
			w := bg - fgLum(rtRGBA(img, x, y))
			if w <= 10 {
				continue
			}
			sx += w * (float64(x) + 0.5)
			sy += w * (float64(y) + 0.5)
			sw += w
		}
	}
	if sw == 0 {
		return math.NaN(), math.NaN()
	}
	return sx / sw, sy / sw
}

func TestApplyRetouch_FaceWidth(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	skin := color.RGBA{R: 200, G: 170, B: 150, A: 255}
	mark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	bg := fgLum(skin)
	p4 := [2]float64{pts[4].X * fgW, pts[4].Y * fgH}
	p12 := [2]float64{pts[12].X * fgW, pts[12].Y * fgH}
	mid := [2]float64{0.5 * fgW, fgEyeY * fgH}
	marked := func() *image.RGBA {
		img := fgSolid(fgW, fgH, skin)
		for _, p := range [][2]float64{p4, p12, mid} {
			skDot(img, p[0], p[1], 3, mark)
		}
		return img
	}

	tests := []struct {
		name     string
		width    float64
		narrower bool
	}{
		{name: "SK7 -8 narrows the jaw", width: -8, narrower: true},
		{name: "+8 widens the jaw", width: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			img := marked()
			out, err := ApplyRetouch(img, pts, Retouch{FaceWidth: tt.width})
			require.NoError(t, err)
			require.NotNil(t, out)
			assert.Equal(t, img.Bounds().Size(), out.Bounds().Size())

			x4, _ := skCentroid(out, p4[0], p4[1], 25, bg)
			x12, _ := skCentroid(out, p12[0], p12[1], 25, bg)
			require.False(t, math.IsNaN(x4) || math.IsNaN(x12), "jaw markers must still be visible")
			before := p12[0] - p4[0]
			after := x12 - x4
			if tt.narrower {
				assert.Less(t, after, before-2, "distance 4–12: before %.1f px, after %.1f px", before, after)
			} else {
				assert.Greater(t, after, before+2, "distance 4–12: before %.1f px, after %.1f px", before, after)
			}

			mx, my := skCentroid(out, mid[0], mid[1], 12, bg)
			assert.InDelta(t, mid[0], mx, 1.5, "point between the eyes must not move horizontally")
			assert.InDelta(t, mid[1], my, 1.5, "point between the eyes must not move vertically")
		})
	}

	t.Run("SK7 far from the face nothing changes", func(t *testing.T) {
		t.Parallel()
		img := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
		out, err := ApplyRetouch(img, pts, Retouch{FaceWidth: -8})
		require.NoError(t, err)
		worst := 0
		rtEach(img, func(x, y int, px, py float64) {
			if px >= 0.08 && px <= 0.92 {
				return
			}
			if d := rtMaxDiff(rtRGBA(img, x, y), rtRGBA(out, x, y)); d > worst {
				worst = d
			}
		})
		assert.LessOrEqual(t, worst, 1, "pixels beyond 1.6 half face widths must stay")
	})

	t.Run("without points is an error", func(t *testing.T) {
		t.Parallel()
		_, err := ApplyRetouch(marked(), nil, Retouch{FaceWidth: -8})
		require.Error(t, err)
	})
}

func skPerson(w, h int, bg color.RGBA, pts []Point) *image.RGBA {
	img := fgSolid(w, h, bg)
	jaw := append([]Point(nil), pts[0:17]...)
	fgFillPolygon(img, jaw, grSkin)
	return img
}

func skRepaintOutside(img *image.RGBA, keep [4]float64, c color.RGBA) *image.RGBA {
	out := fgCopy(img)
	b := out.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			nx, ny := (float64(x)+0.5)/float64(b.Dx()), (float64(y)+0.5)/float64(b.Dy())
			if nx >= keep[0] && nx < keep[2] && ny >= keep[1] && ny < keep[3] {
				continue
			}
			out.SetRGBA(x, y, c)
		}
	}
	return out
}

func TestBackgroundShift(t *testing.T) {
	t.Parallel()
	small := fgTransform(fgFace(), 0.4, 0)
	closeUp := fgTransform(fgFace(), 1.8, 0)
	for i := range closeUp {
		closeUp[i].Y -= 0.05
	}
	other := color.RGBA{R: 30, G: 60, B: 200, A: 255}
	tests := []struct {
		name      string
		pts       []Point
		orig      func(pts []Point) *image.RGBA
		edited    func(orig *image.RGBA) *image.RGBA
		wantKnown bool
		check     func(t *testing.T, v float64)
	}{
		{
			name:      "RT14 same picture: about 0, measurable",
			pts:       small,
			orig:      func(p []Point) *image.RGBA { return skPerson(fgW, fgH, fgGray, p) },
			edited:    fgCopy,
			wantKnown: true,
			check:     func(t *testing.T, v float64) { assert.Less(t, v, 3.0) },
		},
		{
			name: "RT14 same picture, orig twice the size: about 0",
			pts:  small,
			orig: func(p []Point) *image.RGBA { return skPerson(2*fgW, 2*fgH, fgGray, p) },
			edited: func(o *image.RGBA) *image.RGBA {
				return skPerson(fgW, fgH, fgGray, small)
			},
			wantKnown: true,
			check:     func(t *testing.T, v float64) { assert.Less(t, v, 3.0) },
		},
		{
			name: "RT14 other background around the same person: above 30",
			pts:  small,
			orig: func(p []Point) *image.RGBA { return skPerson(fgW, fgH, fgGray, p) },
			edited: func(o *image.RGBA) *image.RGBA {
				return skRepaintOutside(o, [4]float64{0.3, 0.38, 0.7, 1}, other)
			},
			wantKnown: true,
			check:     func(t *testing.T, v float64) { assert.Greater(t, v, 30.0) },
		},
		{
			name: "RT14 only the person zone changed: background kept",
			pts:  small,
			orig: func(p []Point) *image.RGBA { return skPerson(fgW, fgH, fgGray, p) },
			edited: func(o *image.RGBA) *image.RGBA {
				out := fgCopy(o)
				grFillRect(out, [4]float64{0.4, 0.45, 0.6, 1}, other)
				return out
			},
			wantKnown: true,
			check:     func(t *testing.T, v float64) { assert.Less(t, v, 10.0) },
		},
		{
			name: "RT14 face fills the frame: not measurable",
			pts:  closeUp,
			orig: func(p []Point) *image.RGBA { return skPerson(fgW, fgH, fgGray, p) },
			edited: func(o *image.RGBA) *image.RGBA {
				return fgSolid(fgW, fgH, other)
			},
			wantKnown: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orig := tt.orig(tt.pts)
			edited := tt.edited(orig)

			v, known := BackgroundShift(orig, edited, tt.pts)

			require.Equal(t, tt.wantKnown, known, "shift %.2f", v)
			if tt.check != nil {
				tt.check(t, v)
			}
			v64, known64 := BackgroundShiftBase64(fgB64(t, orig), fgB64(t, edited), tt.pts)
			assert.Equal(t, known, known64, "base64 variant")
			if known {
				assert.InDelta(t, v, v64, 0.01, "base64 variant")
			}
		})
	}
}

func skRowMean(img image.Image, nx, ny float64, n int) [3]float64 {
	b := img.Bounds()
	x0 := b.Min.X + int(nx*float64(b.Dx())) - n/2
	y := b.Min.Y + int(ny*float64(b.Dy()))
	var out [3]float64
	for x := x0; x < x0+n; x++ {
		c := rtRGBA(img, x, y)
		out[0] += float64(c.R)
		out[1] += float64(c.G)
		out[2] += float64(c.B)
	}
	for i := range out {
		out[i] /= float64(n)
	}
	return out
}

func TestApply_SkinFromEdit(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	red := color.RGBA{R: 200, G: 60, B: 60, A: 255}
	stripeLo := color.RGBA{R: 70, G: 70, B: 70, A: 255}
	stripeHi := color.RGBA{R: 130, G: 130, B: 130, A: 255}
	cheek := [2]float64{0.30, 0.52}

	tests := []struct {
		name  string
		scale int
	}{
		{name: "SK1 same size", scale: 1},
		{name: "SK1 orig twice the size: result has edit size", scale: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orig := fgStripes(tt.scale*fgW, tt.scale*fgH, 2*tt.scale, stripeLo, stripeHi)
			edited := fgSolid(fgW, fgH, red)

			out, err := Apply(orig, edited, pts, pts)

			require.NoError(t, err)
			assert.Equal(t, image.Rect(0, 0, fgW, fgH), out.Bounds(), "result has the edit's size")
			_, c := fgRowStats(out, cheek[0], cheek[1], 8)
			assert.Less(t, c, 8.0, "cheek has no original structure")
			fgNear(t, red, fgAt(out, cheek[0], cheek[1]), 12, "cheek centre is the edit")
			for _, p := range [][2]float64{{0.38, fgEyeY}, {0.62, fgEyeY}, {0.5, 0.48}} {
				_, c := fgRowStats(out, p[0], p[1], 6)
				assert.Greater(t, c, 30.0, "original structure at %v", p)
				m := skRowMean(out, p[0], p[1], 6)
				assert.InDelta(t, float64(red.R), m[0], 20, "red channel at %v follows the edit", p)
				assert.InDelta(t, float64(red.G), m[1], 20, "green channel at %v follows the edit", p)
				assert.Greater(t, m[0], m[1]+80, "edit tone at %v", p)
			}
		})
	}
}

func TestApply_SmallDetails(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	dark := color.RGBA{R: 30, G: 30, B: 30, A: 255}
	nose := [2]float64{0.5, 0.48}
	cheek := [2]float64{0.30, 0.52}
	orig := skGray(fgW, fgH, 100)
	skDot(orig, nose[0]*fgW, nose[1]*fgH, 4, dark)
	edited := fgSolid(fgW, fgH, fgEditFlat)
	skDot(edited, cheek[0]*fgW, cheek[1]*fgH, 4, dark)
	flat := fgLum(fgEditFlat)

	out, err := Apply(orig, edited, pts, pts)

	require.NoError(t, err)
	assert.Less(t, fgLum(fgAt(out, nose[0], nose[1])), flat-20, "SK2 dark spot on the original nose stays")
	assert.Less(t, fgLum(fgAt(out, cheek[0], cheek[1])), flat-20, "SK2 dark spot on the edited cheek stays")
	assert.InDelta(t, flat, fgLum(fgAt(out, 0.5, 0.15)), 3, "far from the spots the edit is unchanged")
}

func TestApply_PointsMismatchIsError(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	edit := append([]Point(nil), pts...)
	edit[48].X += 0.08 * (pts[16].X - pts[0].X)

	_, err := Apply(skGray(fgW, fgH, 100), fgSolid(fgW, fgH, fgEditFlat), pts, edit)

	require.Error(t, err, "SK3 points that do not fit must not be pasted")
}
