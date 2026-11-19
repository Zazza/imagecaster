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
	hdSkin = color.RGBA{R: 200, G: 170, B: 150, A: 255}
	hdMark = color.RGBA{R: 20, G: 20, B: 20, A: 255}
)

const (
	hdEyeChin   = (0.78 - fgEyeY) * fgH
	hdHeadRatio = 1.15
)

func hdPlace(w, h int, cx, cy, s float64) ([]Point, float64) {
	src := fgFace()
	out := make([]Point, len(src))
	for i, p := range src {
		x := cx + s*(p.X-0.5)*fgW
		y := cy + s*(p.Y-fgEyeY)*fgH
		out[i] = Point{X: x / float64(w), Y: y / float64(h)}
	}
	return out, s * hdEyeChin
}

func hdGradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(40 + (x*7+y*3)%170), G: uint8(40 + (y*5)%170), B: uint8(40 + (x*3)%170), A: 255})
		}
	}
	return img
}

func hdCopy(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	return out
}

func TestRetouch_HeadValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		r       Retouch
		wantErr bool
	}{
		{name: "head size 0", r: Retouch{HeadSize: 0}},
		{name: "head size 15", r: Retouch{HeadSize: 15}},
		{name: "head size -15", r: Retouch{HeadSize: -15}},
		{name: "head size 7.5", r: Retouch{HeadSize: 7.5}},
		{name: "head size 15.5", r: Retouch{HeadSize: 15.5}, wantErr: true},
		{name: "head size -16", r: Retouch{HeadSize: -16}, wantErr: true},
		{name: "head size 100", r: Retouch{HeadSize: 100}, wantErr: true},
		{name: "head shift 20", r: Retouch{HeadShift: 20}},
		{name: "head shift -20", r: Retouch{HeadShift: -20}},
		{name: "head shift 20.5", r: Retouch{HeadShift: 20.5}, wantErr: true},
		{name: "head shift -21", r: Retouch{HeadShift: -21}, wantErr: true},
		{name: "both at limits", r: Retouch{HeadSize: -15, HeadShift: 20}},
		{name: "size ok shift beyond", r: Retouch{HeadSize: 5, HeadShift: 30}, wantErr: true},
		{name: "head size 20 is beyond its own limit", r: Retouch{HeadSize: 20}, wantErr: true},
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

func TestRetouch_HeadLocal(t *testing.T) {
	t.Parallel()
	for _, r := range []Retouch{{HeadSize: 5}, {HeadSize: -5}, {HeadShift: 10}, {HeadShift: -10}} {
		assert.True(t, r.Local(), "%+v", r)
		assert.False(t, r.IsZero(), "%+v", r)
	}
	assert.False(t, Retouch{HeadSize: 0, HeadShift: 0}.Local())
	assert.True(t, Retouch{HeadSize: 0, HeadShift: 0}.IsZero())
}

func TestRetouch_HeadForPoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   Retouch
		n    int
		want Retouch
	}{
		{name: "no face drops head size", in: Retouch{HeadSize: 5}, n: 0, want: Retouch{}},
		{name: "few points drop head shift", in: Retouch{HeadShift: -10}, n: 67, want: Retouch{}},
		{name: "no face keeps light", in: Retouch{Light: true, HeadSize: 5, HeadShift: 10}, n: 0, want: Retouch{Light: true}},
		{name: "68 points keep both", in: Retouch{HeadSize: 5, HeadShift: 10}, n: 68, want: Retouch{HeadSize: 5, HeadShift: 10}},
		{name: "70 points keep both", in: Retouch{HeadSize: -5, HeadShift: -10, FaceWidth: 3}, n: 70, want: Retouch{HeadSize: -5, HeadShift: -10, FaceWidth: 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.in.ForPoints(tc.n))
		})
	}
}

func TestApplyRetouch_HeadZeroIsIdentity(t *testing.T) {
	t.Parallel()
	img := fgStripes(fgW, fgH, 2, fgStripeA, fgStripeB)
	pts := fgFace()

	out, err := ApplyRetouch(img, pts, Retouch{HeadSize: 0, HeadShift: 0})
	require.NoError(t, err)
	require.Equal(t, img.Bounds().Size(), out.Bounds().Size())
	assert.Equal(t, img.Pix, out.Pix)

	base, err := ApplyRetouch(img, pts, Retouch{FaceWidth: 5})
	require.NoError(t, err)
	with, err := ApplyRetouch(img, pts, Retouch{FaceWidth: 5, HeadSize: 0, HeadShift: 0})
	require.NoError(t, err)
	assert.Equal(t, base.Pix, with.Pix)
}

func TestApplyRetouch_HeadSize(t *testing.T) {
	t.Parallel()
	const w, h = 1000, 1000
	cx, cy := 500.0, 420.0
	pts, d := hdPlace(w, h, cx, cy, 0.6)
	bg := fgLum(hdSkin)
	chin := [2]float64{cx, cy + 0.6*d}
	marked := func() *image.RGBA {
		img := fgSolid(w, h, hdSkin)
		skDot(img, cx, cy, 3, hdMark)
		skDot(img, chin[0], chin[1], 3, hdMark)
		return img
	}

	tests := []struct {
		name string
		size float64
	}{
		{name: "+10 enlarges the head", size: 10},
		{name: "-10 shrinks the head", size: -10},
		{name: "+15 enlarges the head", size: 15},
		{name: "-15 shrinks the head", size: -15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			img := marked()
			out, err := ApplyRetouch(img, pts, Retouch{HeadSize: tt.size})
			require.NoError(t, err)
			require.NotNil(t, out)
			assert.Equal(t, img.Bounds().Size(), out.Bounds().Size())

			mx, my := skCentroid(out, cx, cy, 12, bg)
			require.False(t, math.IsNaN(mx), "center marker must stay visible")
			assert.InDelta(t, cx, mx, 1.5, "center between the eyes must not move")
			assert.InDelta(t, cy, my, 1.5, "center between the eyes must not move")

			x, y := skCentroid(out, chin[0], chin[1], 30, bg)
			require.False(t, math.IsNaN(x), "chin marker must stay visible")
			before := math.Hypot(chin[0]-cx, chin[1]-cy)
			after := math.Hypot(x-cx, y-cy)
			if tt.size > 0 {
				assert.Greater(t, after, before+2, "before %.1f px, after %.1f px", before, after)
			} else {
				assert.Less(t, after, before-2, "before %.1f px, after %.1f px", before, after)
			}
			assert.InDelta(t, 1+tt.size/100, after/before, 0.04, "inside the head the scale is the full value")
			assert.InDelta(t, cx, x, 1.5, "a marker straight below the center moves only vertically")
		})
	}
}

func TestApplyRetouch_HeadShift(t *testing.T) {
	t.Parallel()
	const w, h = 1000, 1000
	cx, cy := 500.0, 420.0
	pts, d := hdPlace(w, h, cx, cy, 0.6)
	bg := fgLum(hdSkin)
	nose := [2]float64{cx, cy + 0.3*d}
	marked := func() *image.RGBA {
		img := fgSolid(w, h, hdSkin)
		skDot(img, nose[0], nose[1], 3, hdMark)
		skDot(img, cx, cy, 3, hdMark)
		return img
	}

	tests := []struct {
		name  string
		shift float64
	}{
		{name: "+10 moves the head up", shift: 10},
		{name: "-10 moves the head down", shift: -10},
		{name: "+20 moves the head up", shift: 20},
		{name: "-20 moves the head down", shift: -20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			img := marked()
			out, err := ApplyRetouch(img, pts, Retouch{HeadShift: tt.shift})
			require.NoError(t, err)
			require.NotNil(t, out)
			assert.Equal(t, img.Bounds().Size(), out.Bounds().Size())

			want := tt.shift / 100 * d
			for _, m := range [][2]float64{nose, {cx, cy}} {
				x, y := skCentroid(out, m[0], m[1]-want, 12, bg)
				require.False(t, math.IsNaN(x), "marker at %.0f,%.0f must be found near its new place", m[0], m[1])
				moved := m[1] - y
				if tt.shift > 0 {
					assert.Greater(t, moved, 2.0, "marker must go up, moved %.1f px", moved)
				} else {
					assert.Less(t, moved, -2.0, "marker must go down, moved %.1f px", moved)
				}
				assert.InDelta(t, want, moved, 0.25*math.Abs(want), "shift is the share of eyes→chin")
				assert.InDelta(t, m[0], x, 1.5, "shift is vertical only")
			}
		})
	}
}

func TestApplyRetouch_HeadFarAwayUnchanged(t *testing.T) {
	t.Parallel()
	const w, h = 1000, 1000
	cx, cy := 500.0, 420.0
	pts, d := hdPlace(w, h, cx, cy, 0.6)
	img := hdGradient(w, h)
	far := 2.1 * hdHeadRatio * d

	for _, r := range []Retouch{{HeadSize: 15}, {HeadSize: -15}, {HeadShift: 20}, {HeadShift: -20}, {HeadSize: 15, HeadShift: -20}} {
		out, err := ApplyRetouch(hdCopy(img), pts, r)
		require.NoError(t, err, "%+v", r)
		require.Equal(t, img.Bounds().Size(), out.Bounds().Size())
		worst, n := 0, 0
		rtEach(img, func(x, y int, _, _ float64) {
			if math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) <= far {
				return
			}
			n++
			if dd := rtMaxDiff(rtRGBA(img, x, y), rtRGBA(out, x, y)); dd > worst {
				worst = dd
			}
		})
		require.Positive(t, n)
		assert.LessOrEqual(t, worst, 1, "%+v: pixels beyond the transition zone must stay", r)

		changed := false
		rtEach(img, func(x, y int, _, _ float64) {
			if !changed && math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) < d && rtMaxDiff(rtRGBA(img, x, y), rtRGBA(out, x, y)) > 1 {
				changed = true
			}
		})
		assert.True(t, changed, "%+v: the head itself must change", r)
	}
}

func TestApplyRetouch_HeadNearEdge(t *testing.T) {
	t.Parallel()
	const w, h = 300, 300
	places := [][2]float64{{10, 10}, {290, 10}, {10, 290}, {290, 295}, {150, 2}, {150, 298}}
	for _, p := range places {
		pts, _ := hdPlace(w, h, p[0], p[1], 0.6)
		for _, r := range []Retouch{{HeadSize: 15}, {HeadSize: -15}, {HeadShift: 20}, {HeadShift: -20}, {HeadSize: -15, HeadShift: 20}} {
			img := fgStripes(w, h, 2, fgStripeA, fgStripeB)
			require.NotPanics(t, func() {
				out, err := ApplyRetouch(img, pts, r)
				require.NoError(t, err, "%v %+v", p, r)
				assert.Equal(t, img.Bounds().Size(), out.Bounds().Size(), "%v %+v", p, r)
			}, "%v %+v", p, r)
		}
	}
}
