package bodyshape

import (
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bsW       = 200
	bsH       = 400
	bsBodyX0  = 80
	bsBodyX1  = 120
	bsNeckY   = 72
	bsHipY    = 200
	bsAnkleY  = 360
	bsTorsoY  = 140
	bsTopMark = 100
	bsHipMark = 190
	bsLegTop  = 212
	bsLegLow  = 345
)

var (
	bsRed    = color.RGBA{R: 230, G: 20, B: 20, A: 255}
	bsGreen  = color.RGBA{R: 20, G: 220, B: 20, A: 255}
	bsYellow = color.RGBA{R: 255, G: 255, B: 0, A: 255}
)

func bsBackground(y int) color.RGBA {
	return color.RGBA{R: 10, G: uint8(40 + y%60), B: 180, A: 255}
}

func bsImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, bsW, bsH))
	for y := 0; y < bsH; y++ {
		for x := 0; x < bsW; x++ {
			c := bsBackground(y)
			switch {
			case y >= 15 && y < 60 && x >= 90 && x < 110:
				c = bsGreen
			case y >= bsNeckY && y < 385 && x >= bsBodyX0 && x < bsBodyX1:
				c = bsRed
				for _, m := range []int{bsTopMark, bsHipMark, bsLegTop, bsLegLow} {
					if y >= m && y < m+2 {
						c = bsYellow
					}
				}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func bsPoints() []Point {
	pts := make([]Point, 18)
	for i := range pts {
		pts[i] = Point{X: -1, Y: -1}
	}
	f := func(x, y float64) Point { return Point{X: x / bsW, Y: y / bsH} }
	pts[0] = f(100, 40)
	pts[1] = f(100, bsNeckY)
	pts[2] = f(84, 80)
	pts[5] = f(116, 80)
	pts[8] = f(90, bsHipY)
	pts[11] = f(110, bsHipY)
	pts[9] = f(90, 280)
	pts[12] = f(110, 280)
	pts[10] = f(90, bsAnkleY)
	pts[13] = f(110, bsAnkleY)
	return pts
}

func bsWithout(pts []Point, idx ...int) []Point {
	out := append([]Point(nil), pts...)
	for _, i := range idx {
		out[i] = Point{X: -1, Y: -1}
	}
	return out
}

func bsSame(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1>>8 == r2>>8 && g1>>8 == g2>>8 && b1>>8 == b2>>8 && a1>>8 == a2>>8
}

func bsRowsEqual(t *testing.T, before, after image.Image, y0, y1 int) {
	t.Helper()
	for y := y0; y < y1; y++ {
		for x := 0; x < bsW; x++ {
			if !bsSame(before.At(x, y), after.At(x, y)) {
				t.Fatalf("pixel (%d,%d) changed: %v -> %v", x, y, before.At(x, y), after.At(x, y))
			}
		}
	}
}

func bsIsRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 > 150 && g>>8 < 120 && b>>8 < 120
}

func bsIsYellow(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 > 180 && g>>8 > 180 && b>>8 < 100
}

func bsRedInRow(img image.Image, y int) int {
	n := 0
	for x := 0; x < bsW; x++ {
		if bsIsRed(img.At(x, y)) {
			n++
		}
	}
	return n
}

func bsMarkers(img image.Image) []float64 {
	var out []float64
	sum, n := 0.0, 0
	for y := 0; y < bsH; y++ {
		if bsIsYellow(img.At(100, y)) {
			sum += float64(y)
			n++
			continue
		}
		if n > 0 {
			out = append(out, sum/float64(n))
			sum, n = 0, 0
		}
	}
	if n > 0 {
		out = append(out, sum/float64(n))
	}
	return out
}

func TestApply_ZeroParamsKeepsImage(t *testing.T) {
	t.Parallel()
	src := bsImage()
	for name, pts := range map[string][]Point{"with points": bsPoints(), "no points": nil} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, err := Apply(src, pts, Params{})
			require.NoError(t, err)
			require.Equal(t, src.Bounds().Size(), out.Bounds().Size())
			bsRowsEqual(t, src, out, 0, bsH)
		})
	}
}

func TestApply_KeepsSize(t *testing.T) {
	t.Parallel()
	src := bsImage()
	for _, p := range []Params{{Width: -10}, {Width: 10}, {Height: 7}, {Height: -7}, {Width: 5, Height: -3}, {Width: -4, Height: 6}} {
		out, err := Apply(src, bsPoints(), p)
		require.NoError(t, err, "%+v", p)
		assert.Equal(t, src.Bounds().Size(), out.Bounds().Size(), "%+v", p)
	}
}

func TestApply_WidthChangesTorsoNotHead(t *testing.T) {
	t.Parallel()
	src := bsImage()
	before := bsRedInRow(src, bsTorsoY)
	require.Equal(t, bsBodyX1-bsBodyX0, before)
	tests := []struct {
		name   string
		width  float64
		narrow bool
	}{
		{name: "thinner", width: -10, narrow: true},
		{name: "fuller", width: 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := Apply(src, bsPoints(), Params{Width: tc.width})
			require.NoError(t, err)
			for _, y := range []int{bsTorsoY, 170} {
				got := bsRedInRow(out, y)
				if tc.narrow {
					assert.Less(t, got, before, "row %d", y)
				} else {
					assert.Greater(t, got, before, "row %d", y)
				}
			}
			bsRowsEqual(t, src, out, 0, bsNeckY-4)
		})
	}
}

func TestApply_WidthIsProportional(t *testing.T) {
	t.Parallel()
	src := bsImage()
	small, err := Apply(src, bsPoints(), Params{Width: -3})
	require.NoError(t, err)
	big, err := Apply(src, bsPoints(), Params{Width: -10})
	require.NoError(t, err)
	assert.LessOrEqual(t, bsRedInRow(big, bsTorsoY), bsRedInRow(small, bsTorsoY))
}

func TestApply_HeightStretchesLegsShiftsTorso(t *testing.T) {
	t.Parallel()
	src := bsImage()
	m0 := bsMarkers(src)
	require.Len(t, m0, 4)
	tests := []struct {
		name   string
		height float64
		longer bool
	}{
		{name: "taller", height: 7, longer: true},
		{name: "shorter", height: -7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := Apply(src, bsPoints(), Params{Height: tc.height})
			require.NoError(t, err)
			require.Equal(t, src.Bounds().Size(), out.Bounds().Size())
			m := bsMarkers(out)
			require.Len(t, m, 4, "markers %v", m)

			assert.InDelta(t, m0[1]-m0[0], m[1]-m[0], 1.0, "torso length must stay")
			legs0, legs := m0[3]-m0[2], m[3]-m[2]
			up := m0[0] - m[0]
			down := m[3] - m0[3]
			if tc.longer {
				assert.Greater(t, legs, legs0+4, "legs must get longer")
				assert.Greater(t, up, 1.0, "figure moves up into the background above the head")
				assert.Greater(t, down, 1.0, "feet move down into the background below")
			} else {
				assert.Less(t, legs, legs0-4, "legs must get shorter")
				assert.Less(t, up, -1.0)
				assert.Less(t, down, -1.0)
			}
			assert.InDelta(t, up, down, 3.0, "difference is split equally above the head and below the feet")
		})
	}
}

func TestApply_HeightWithoutAnklesStretchesToBottom(t *testing.T) {
	t.Parallel()
	src := bsImage()
	pts := bsWithout(bsPoints(), 9, 10, 12, 13)
	m0 := bsMarkers(src)
	out, err := Apply(src, pts, Params{Height: 7})
	require.NoError(t, err)
	require.Equal(t, src.Bounds().Size(), out.Bounds().Size())
	bsRowsEqual(t, src, out, 0, bsHipY-12)
	m := bsMarkers(out)
	require.GreaterOrEqual(t, len(m), 3, "markers %v", m)
	if len(m) == 4 {
		assert.Greater(t, m[3]-m[2], m0[3]-m0[2]+4)
	}
	assert.Greater(t, m[len(m)-1], m0[3])
}

func TestApply_MissingPoints(t *testing.T) {
	t.Parallel()
	src := bsImage()
	full := bsPoints()
	tests := []struct {
		name    string
		pts     []Point
		p       Params
		wantErr bool
	}{
		{name: "width without shoulders", pts: bsWithout(full, 2, 5), p: Params{Width: -5}, wantErr: true},
		{name: "width without one shoulder", pts: bsWithout(full, 5), p: Params{Width: -5}, wantErr: true},
		{name: "width without neck", pts: bsWithout(full, 1), p: Params{Width: 5}, wantErr: true},
		{name: "height without hips", pts: bsWithout(full, 8, 11), p: Params{Height: 5}, wantErr: true},
		{name: "width without points", pts: nil, p: Params{Width: 5}, wantErr: true},
		{name: "height with empty points", pts: []Point{}, p: Params{Height: 5}, wantErr: true},
		{name: "height with one hip", pts: bsWithout(full, 8), p: Params{Height: 5}},
		{name: "width needs no hips", pts: bsWithout(full, 8, 11), p: Params{Width: -5}},
		{name: "height needs no shoulders", pts: bsWithout(full, 2, 5), p: Params{Height: 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := Apply(src, tc.pts, tc.p)
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrNoBody), "want ErrNoBody, got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, src.Bounds().Size(), out.Bounds().Size())
		})
	}
}

func TestParams_Validate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 10.0, float64(MaxWidth))
	assert.Equal(t, 7.0, float64(MaxHeight))
	tests := []struct {
		p       Params
		wantErr bool
	}{
		{p: Params{}},
		{p: Params{Width: 10, Height: 7}},
		{p: Params{Width: -10, Height: -7}},
		{p: Params{Width: 10.5}, wantErr: true},
		{p: Params{Width: -10.5}, wantErr: true},
		{p: Params{Height: 7.5}, wantErr: true},
		{p: Params{Height: -7.5}, wantErr: true},
	}
	for _, tc := range tests {
		err := tc.p.Validate()
		if tc.wantErr {
			assert.Error(t, err, "%+v", tc.p)
		} else {
			assert.NoError(t, err, "%+v", tc.p)
		}
	}
}

func TestParams_IsZero(t *testing.T) {
	t.Parallel()
	assert.True(t, Params{}.IsZero())
	assert.False(t, Params{Width: 1}.IsZero())
	assert.False(t, Params{Height: -1}.IsZero())
}
