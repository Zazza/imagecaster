package generation

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStrictMaskCompositeRetainsAllPixelsOutsidePaint(t *testing.T) {
	const width, height = 12, 8
	original := image.NewNRGBA(image.Rect(0, 0, width, height))
	mask := image.NewGray(original.Bounds())
	selected := image.Rect(4, 2, 8, 6)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			original.SetNRGBA(x, y, color.NRGBA{R: uint8(x*13 + 1), G: uint8(y*17 + 3), B: 91, A: uint8((x + y) * 9)})
			if image.Pt(x, y).In(selected) {
				mask.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	encoded, err := blendFromImageResultWithMaskStrict(original, solidPNGBase64(t, width, height, emRed), mask, false, true)
	require.NoError(t, err)
	data, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	out, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			got := color.NRGBAModel.Convert(out.At(x, y)).(color.NRGBA)
			if image.Pt(x, y).In(selected) {
				require.Equal(t, emRed, got, "selected pixel %d,%d", x, y)
			} else {
				require.Equal(t, original.NRGBAAt(x, y), got, "outside pixel %d,%d", x, y)
			}
		}
	}
}

func TestGenerateFromImageStrictMaskKeepsBoundaryAndOutsidePixels(t *testing.T) {
	const width, height = 260, 196
	e := emSetup(t, emSolidRed)
	source := makePNGBase64(t, width, height)
	p := e.params(source)
	selected := image.Rect(width/4, height/4, width*3/4, height*3/4)
	p.MaskBase64 = emMaskBase64(t, width, height, selected)
	p.MaskStrict = true
	result, err := e.svc.GenerateFromImage(p)
	require.NoError(t, err)
	original := decodePNGBase64(t, source)
	out := decodePNGBase64(t, result.Image)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if image.Pt(x, y).In(selected) {
				require.Equal(t, emRed, emPixel(out, x, y), "selected %d,%d", x, y)
			} else {
				require.Equal(t, emPixel(original, x, y), emPixel(out, x, y), "outside %d,%d", x, y)
			}
		}
	}
	e.sessions.mu.Lock()
	defer e.sessions.mu.Unlock()
	require.Len(t, e.sessions.items, 1)
	require.Equal(t, result.Image, e.sessions.items[0].imageBase64)
}
