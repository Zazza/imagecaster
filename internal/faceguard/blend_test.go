package faceguard

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func blendSolidImg(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func blendPNGBase64(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, blendSolidImg(w, h, c)))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func blendDecode(t *testing.T, b64 string) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	img, _, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	return img
}

func TestBlendRegion(t *testing.T) {
	t.Parallel()
	orig := blendSolidImg(100, 100, color.RGBA{R: 10, G: 10, B: 10, A: 255})
	region := blendSolidImg(40, 40, color.RGBA{R: 250, G: 250, B: 250, A: 255})
	rect := image.Rect(30, 30, 70, 70)

	hard, err := BlendRegion(orig, region, rect, 0)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 100, 100), hard.Bounds(), "result keeps the original size")

	center := hard.At(50, 50).(color.RGBA)
	assert.Equal(t, uint8(250), center.R, "rect center must be the region color with a hard mask")
	assert.Equal(t, uint8(250), center.G)
	assert.Equal(t, uint8(250), center.B)

	outside := hard.At(5, 5).(color.RGBA)
	assert.Equal(t, uint8(10), outside.R, "point outside the rect keeps the original color")
	assert.Equal(t, uint8(10), outside.G)
	assert.Equal(t, uint8(10), outside.B)

	soft, err := BlendRegion(orig, region, rect, 0.15)
	require.NoError(t, err)
	softCenter := soft.At(50, 50).(color.RGBA)
	assert.InDelta(t, 250, softCenter.R, 3, "feathered center stays at the region color")
	edge := soft.At(33, 49).(color.RGBA)
	for _, v := range []uint8{edge.R, edge.G, edge.B} {
		assert.Greater(t, v, uint8(10), "edge pixel must move towards the region color")
		assert.Less(t, v, uint8(250), "edge pixel must stay short of the region color")
	}

	clamped, err := BlendRegion(orig, region, image.Rect(-10, -10, 50, 50), 0)
	require.NoError(t, err, "rect partially outside must be clamped, not fail")
	require.Equal(t, image.Rect(0, 0, 100, 100), clamped.Bounds())
	centerClamped := clamped.At(25, 25).(color.RGBA)
	assert.Equal(t, uint8(250), centerClamped.R, "center of the clamped rect must be the region color")

	_, err = BlendRegion(orig, region, image.Rect(200, 200, 300, 300), 0.15)
	assert.Error(t, err, "rect fully outside the image must fail")

	_, err = BlendRegion(image.NewRGBA(image.Rect(0, 0, 0, 0)), region, rect, 0.15)
	assert.Error(t, err, "empty original must fail")
}

func TestBlendRegionBase64(t *testing.T) {
	t.Parallel()
	origB64 := blendPNGBase64(t, 60, 40, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	regionB64 := blendPNGBase64(t, 20, 20, color.RGBA{R: 240, G: 240, B: 240, A: 255})

	out, err := BlendRegionBase64(origB64, regionB64, image.Rect(10, 5, 30, 25), 0.15)
	require.NoError(t, err)
	img := blendDecode(t, out)
	assert.Equal(t, image.Rect(0, 0, 60, 40), img.Bounds(), "result must match the original size")

	_, err = BlendRegionBase64("%%%", regionB64, image.Rect(10, 5, 30, 25), 0.15)
	assert.Error(t, err, "broken original base64 must fail")

	_, err = BlendRegionBase64(origB64, regionB64, image.Rect(100, 100, 120, 120), 0.15)
	assert.Error(t, err, "rect outside the image must fail")
}
