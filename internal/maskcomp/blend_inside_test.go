package maskcomp

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlendStrictPreservesEveryOutsidePixelIncludingAlpha(t *testing.T) {
	const width, height = 9, 7
	original := image.NewNRGBA(image.Rect(0, 0, width, height))
	edited := image.NewNRGBA(original.Bounds())
	mask := image.NewGray(original.Bounds())
	selected := image.Rect(3, 2, 6, 5)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			original.SetNRGBA(x, y, color.NRGBA{R: uint8(x*17 + 1), G: uint8(y*23 + 2), B: 93, A: uint8((x + y) * 13)})
			edited.SetNRGBA(x, y, color.NRGBA{R: 245, G: 11, B: 27, A: 255})
			if image.Pt(x, y).In(selected) {
				mask.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	out := BlendStrict(original, edited, mask)
	require.Equal(t, original.Bounds(), out.Bounds())
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if image.Pt(x, y).In(selected) {
				require.Equal(t, edited.NRGBAAt(x, y), out.NRGBAAt(x, y), "selected pixel %d,%d", x, y)
			} else {
				require.Equal(t, original.NRGBAAt(x, y), out.NRGBAAt(x, y), "outside pixel %d,%d", x, y)
			}
		}
	}
}
