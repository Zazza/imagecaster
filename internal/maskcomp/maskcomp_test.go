package maskcomp

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const size = 256

var (
	origColor   = color.RGBA{100, 100, 100, 255}
	editedColor = color.RGBA{20, 200, 30, 255}
	maskRect    = image.Rect(96, 96, 160, 160)
)

func solidRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func grayMask(w, h int, r image.Rectangle) *image.Gray {
	m := image.NewGray(image.Rect(0, 0, w, h))
	r = r.Intersect(m.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			m.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	return m
}

func alphaMask(w, h int, r image.Rectangle) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	r = r.Intersect(m.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			m.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	return m
}

func rgbaAt(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func assertColorNear(t *testing.T, want color.RGBA, got color.RGBA, tol int, msgAndArgs ...any) {
	t.Helper()
	assert.InDelta(t, int(want.R), int(got.R), float64(tol), msgAndArgs...)
	assert.InDelta(t, int(want.G), int(got.G), float64(tol), msgAndArgs...)
	assert.InDelta(t, int(want.B), int(got.B), float64(tol), msgAndArgs...)
}

func isRed(c color.RGBA) bool {
	return c.R >= 180 && c.G <= 80 && c.B <= 80
}

func hasRedNear(img image.Image, x0, y0, x1, y1 int) bool {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if isRed(rgbaAt(img, x, y)) {
				return true
			}
		}
	}
	return false
}

func TestOutline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mask image.Image
	}{
		{name: "gray mask", mask: grayMask(size, size, maskRect)},
		{name: "alpha mask", mask: alphaMask(size, size, maskRect)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			img := solidRGBA(size, size, origColor)

			out := Outline(img, tt.mask)
			require.NotNil(t, out)
			assert.Equal(t, img.Bounds().Dx(), out.Bounds().Dx())
			assert.Equal(t, img.Bounds().Dy(), out.Bounds().Dy())

			assert.True(t, hasRedNear(out, 90, 126, 102, 130), "left edge must be outlined red")
			assert.True(t, hasRedNear(out, 154, 126, 166, 130), "right edge must be outlined red")
			assert.True(t, hasRedNear(out, 126, 90, 130, 102), "top edge must be outlined red")
			assert.True(t, hasRedNear(out, 126, 154, 130, 166), "bottom edge must be outlined red")

			for _, p := range []image.Point{{128, 128}, {20, 20}, {230, 230}, {20, 128}, {128, 230}} {
				assert.Equal(t, origColor, rgbaAt(out, p.X, p.Y), "pixel %v far from boundary must be unchanged", p)
			}
		})
	}
}

func TestOutline_IsContourNotFill(t *testing.T) {
	t.Parallel()
	img := solidRGBA(size, size, origColor)

	out := Outline(img, grayMask(size, size, maskRect))

	for y := 120; y <= 136; y++ {
		for x := 120; x <= 136; x++ {
			assert.False(t, isRed(rgbaAt(out, x, y)), "mask interior (%d,%d) must not be filled red", x, y)
		}
	}
}

func TestOutline_EmptyMaskLeavesImageUnchanged(t *testing.T) {
	t.Parallel()
	img := solidRGBA(size, size, origColor)

	out := Outline(img, image.NewGray(image.Rect(0, 0, size, size)))

	require.NotNil(t, out)
	for _, p := range []image.Point{{0, 0}, {96, 96}, {128, 128}, {255, 255}} {
		assert.Equal(t, origColor, rgbaAt(out, p.X, p.Y))
	}
}

func compositeFixture() (orig, edited *image.RGBA) {
	orig = solidRGBA(size, size, origColor)
	edited = solidRGBA(size, size, origColor)
	fillRect(edited, maskRect, editedColor)
	fillRect(edited, image.Rect(96, 110, 225, 146), editedColor)
	fillRect(edited, image.Rect(10, 10, 50, 50), editedColor)
	return orig, edited
}

func TestComposite(t *testing.T) {
	t.Parallel()

	orig, edited := compositeFixture()
	out := Composite(orig, edited, grayMask(size, size, maskRect))

	require.NotNil(t, out)
	assert.Equal(t, size, out.Bounds().Dx())
	assert.Equal(t, size, out.Bounds().Dy())

	tests := []struct {
		name string
		at   image.Point
		want color.RGBA
		tol  int
	}{
		{name: "deep inside mask takes edited", at: image.Point{128, 128}, want: editedColor, tol: 6},
		{name: "unchanged region far from mask stays orig", at: image.Point{30, 220}, want: origColor, tol: 0},
		{name: "unchanged region right of mask outside blob stays orig", at: image.Point{200, 220}, want: origColor, tol: 0},
		{name: "changed blob connected to mask kept outside mask", at: image.Point{200, 128}, want: editedColor, tol: 10},
		{name: "changed blob not connected to mask reverts to orig", at: image.Point{30, 30}, want: origColor, tol: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertColorNear(t, tt.want, rgbaAt(out, tt.at.X, tt.at.Y), tt.tol, "pixel %v", tt.at)
		})
	}
}

func TestComposite_AlphaMask(t *testing.T) {
	t.Parallel()

	orig, edited := compositeFixture()
	out := Composite(orig, edited, alphaMask(size, size, maskRect))

	assertColorNear(t, editedColor, rgbaAt(out, 128, 128), 6)
	assertColorNear(t, origColor, rgbaAt(out, 30, 220), 0)
	assertColorNear(t, origColor, rgbaAt(out, 30, 30), 3)
}

func TestComposite_EditedAtHalfResolution(t *testing.T) {
	t.Parallel()

	orig := solidRGBA(size, size, origColor)
	edited := solidRGBA(size/2, size/2, origColor)
	fillRect(edited, image.Rect(48, 48, 80, 80), editedColor)

	out := Composite(orig, edited, grayMask(size, size, maskRect))

	require.NotNil(t, out)
	assert.Equal(t, size, out.Bounds().Dx())
	assert.Equal(t, size, out.Bounds().Dy())
	assertColorNear(t, editedColor, rgbaAt(out, 128, 128), 10)
	assertColorNear(t, origColor, rgbaAt(out, 30, 220), 3)
}

func TestComposite_MaskAtHalfResolution(t *testing.T) {
	t.Parallel()

	orig, edited := compositeFixture()
	out := Composite(orig, edited, grayMask(size/2, size/2, image.Rect(48, 48, 80, 80)))

	require.NotNil(t, out)
	assert.Equal(t, size, out.Bounds().Dx())
	assert.Equal(t, size, out.Bounds().Dy())
	assertColorNear(t, editedColor, rgbaAt(out, 128, 128), 6)
	assertColorNear(t, origColor, rgbaAt(out, 30, 220), 0)
	assertColorNear(t, origColor, rgbaAt(out, 30, 30), 3)
}

func TestComposite_SoftEdge(t *testing.T) {
	t.Parallel()

	drift := color.RGBA{112, 112, 112, 255}
	orig := solidRGBA(size, size, origColor)
	edited := solidRGBA(size, size, drift)
	fillRect(edited, maskRect, editedColor)
	out := Composite(orig, edited, grayMask(size, size, maskRect))

	lo, hi := int(origColor.G), int(drift.G)
	found := false
	for x := 40; x < 96; x++ {
		g := int(rgbaAt(out, x, 128).G)
		if g > lo && g < hi {
			found = true
			break
		}
	}
	assert.True(t, found, "edited model drift outside the mask must fade into orig gradually, not as a hard step")

	assertColorNear(t, origColor, rgbaAt(out, 20, 128), 0)
	assertColorNear(t, editedColor, rgbaAt(out, 128, 128), 10)
}

func TestComposite_EmptyMaskReturnsOrig(t *testing.T) {
	t.Parallel()

	orig := solidRGBA(size, size, origColor)
	edited := solidRGBA(size, size, editedColor)

	out := Composite(orig, edited, image.NewGray(image.Rect(0, 0, size, size)))

	require.NotNil(t, out)
	for _, p := range []image.Point{{10, 10}, {128, 128}, {245, 245}} {
		assertColorNear(t, origColor, rgbaAt(out, p.X, p.Y), 3, "pixel %v", p)
	}
}

func TestComposite_ThinStrokeKeepsConnectedChange(t *testing.T) {
	t.Parallel()

	orig := solidRGBA(size, size, origColor)
	edited := solidRGBA(size, size, origColor)
	fillRect(edited, image.Rect(60, 60, 200, 200), editedColor)
	mask := grayMask(size, size, image.Rect(128, 100, 131, 160))

	out := Composite(orig, edited, mask)

	assertColorNear(t, editedColor, rgbaAt(out, 80, 80), 10)
	assertColorNear(t, editedColor, rgbaAt(out, 180, 180), 10)
	assertColorNear(t, origColor, rgbaAt(out, 20, 230), 0)
}
