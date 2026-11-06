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
	grSkin   = color.RGBA{R: 200, G: 170, B: 150, A: 255}
	grStrand = color.RGBA{R: 50, G: 35, B: 25, A: 255}
)

func grDist(a, b color.RGBA) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

func grFillRect(img *image.RGBA, r [4]float64, c color.RGBA) {
	b := img.Bounds()
	for y := int(r[1] * float64(b.Dy())); y < int(r[3]*float64(b.Dy())); y++ {
		for x := int(r[0] * float64(b.Dx())); x < int(r[2]*float64(b.Dx())); x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func grRGBA(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func grMean(img image.Image, px []image.Point) color.RGBA {
	var r, g, b float64
	for _, p := range px {
		c := grRGBA(img, p.X, p.Y)
		r += float64(c.R)
		g += float64(c.G)
		b += float64(c.B)
	}
	n := float64(len(px))
	return color.RGBA{R: uint8(r / n), G: uint8(g / n), B: uint8(b / n), A: 255}
}

func grSquare(cx, cy float64, r int) []image.Point {
	x, y := int(cx*fgW), int(cy*fgH)
	var out []image.Point
	for yy := y - r; yy <= y+r; yy++ {
		for xx := x - r; xx <= x+r; xx++ {
			out = append(out, image.Point{X: xx, Y: yy})
		}
	}
	return out
}

func grStrandStripes(img *image.RGBA, r [4]float64) {
	b := img.Bounds()
	light := color.RGBA{R: 110, G: 90, B: 70, A: 255}
	for y := int(r[1] * float64(b.Dy())); y < int(r[3]*float64(b.Dy())); y++ {
		for x := int(r[0] * float64(b.Dx())); x < int(r[2]*float64(b.Dx())); x++ {
			if (x/2)%2 == 0 {
				img.SetRGBA(x, y, grStrand)
			} else {
				img.SetRGBA(x, y, light)
			}
		}
	}
}

func TestApply_BrowsCheeksForeheadFromEdit(t *testing.T) {
	t.Parallel()
	pts := fgFace()
	orig := fgSolid(fgW, fgH, grSkin)
	for _, r := range [][4]float64{{0.29, 0.31, 0.46, 0.35}, {0.54, 0.31, 0.71, 0.35}, {0.27, 0.47, 0.34, 0.58}, {0.40, 0.20, 0.60, 0.27}} {
		grStrandStripes(orig, r)
	}
	edited := fgSolid(fgW, fgH, fgEditFlat)

	out, err := Apply(orig, edited, pts, pts)

	require.NoError(t, err)
	for _, c := range []Point{pts[19], pts[24], {X: 0.305, Y: 0.525}, {X: 0.5, Y: 0.235}} {
		_, contrast := fgRowStats(out, c.X, c.Y, 8)
		assert.Less(t, contrast, 8.0, "old strand at %v must not come from orig", c)
		got := grMean(out, grSquare(c.X, c.Y, 2))
		assert.Less(t, grDist(got, fgEditFlat), 15.0, "at %v: got %v, want ≈%v", c, got, fgEditFlat)
	}
}
