package generation

import (
	"bytes"
	"encoding/base64"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func detailImage(t *testing.T, pixels ...color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, len(pixels), 1))
	for x, p := range pixels {
		img.SetNRGBA(x, 0, p)
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
func TestRestoreStylistDetailsPreservesSelectedPixelsExactly(t *testing.T) {
	sourcePixels := []color.NRGBA{{R: 73, G: 29, B: 192, A: 255}, {R: 15, G: 201, B: 94, A: 255}}
	targetPixels := []color.NRGBA{{R: 9, G: 85, B: 16, A: 255}, {R: 189, G: 35, B: 206, A: 255}}
	source := detailImage(t, sourcePixels...)
	target := detailImage(t, targetPixels...)
	mask := detailImage(t, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{A: 255})
	result, err := RestoreStylistDetails(target, source, mask)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(result)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	assert.Equal(t, sourcePixels[0], color.NRGBAModel.Convert(img.At(0, 0)))
	assert.Equal(t, targetPixels[1], color.NRGBAModel.Convert(img.At(1, 0)))
}
func TestRestoreStylistDetailsRejectsUnsafeMask(t *testing.T) {
	target := detailImage(t, color.NRGBA{R: 13, A: 255}, color.NRGBA{B: 19, A: 255})
	emptyMask := detailImage(t, color.NRGBA{A: 255}, color.NRGBA{A: 255})
	short := detailImage(t, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	for _, tc := range []struct{ source, mask string }{{target, emptyMask}, {target, short}, {short, target}, {target, "invalid"}, {"invalid", target}} {
		_, err := RestoreStylistDetails(target, tc.source, tc.mask)
		require.Error(t, err)
	}
}
