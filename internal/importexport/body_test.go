package importexport

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/bodyshape"
)

func bdImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 100, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 100; x++ {
			c := color.RGBA{R: 10, G: uint8(40 + y%60), B: 180, A: 255}
			if y >= 36 && y < 190 && x >= 40 && x < 60 {
				c = color.RGBA{R: 230, G: 20, B: 20, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func bdPoints() []bodyshape.Point {
	pts := make([]bodyshape.Point, 18)
	for i := range pts {
		pts[i] = bodyshape.Point{X: -1, Y: -1}
	}
	pts[0] = bodyshape.Point{X: 0.5, Y: 0.1}
	pts[1] = bodyshape.Point{X: 0.5, Y: 0.18}
	pts[2] = bodyshape.Point{X: 0.42, Y: 0.2}
	pts[5] = bodyshape.Point{X: 0.58, Y: 0.2}
	pts[8] = bodyshape.Point{X: 0.45, Y: 0.5}
	pts[11] = bodyshape.Point{X: 0.55, Y: 0.5}
	pts[9] = bodyshape.Point{X: 0.45, Y: 0.7}
	pts[12] = bodyshape.Point{X: 0.55, Y: 0.7}
	pts[10] = bodyshape.Point{X: 0.45, Y: 0.9}
	pts[13] = bodyshape.Point{X: 0.55, Y: 0.9}
	return pts
}

func bdSize(t *testing.T, data []byte) image.Point {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	return image.Pt(cfg.Width, cfg.Height)
}

func TestProcessExportImage_BodyChangesPixelsKeepsSize(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	src := rtB64(t, bdImage())
	base, err := svc.ProcessExportImage(ExportImageParams{ImageBase64: src, Format: "png"})
	require.NoError(t, err)
	for _, p := range []bodyshape.Params{{Width: -8}, {Width: 8}, {Height: 6}, {Height: -6}, {Width: -5, Height: 5}} {
		got, err := svc.ProcessExportImage(ExportImageParams{ImageBase64: src, Format: "png", Body: &p, BodyPoints: bdPoints()})
		require.NoError(t, err, "%+v", p)
		assert.NotEqual(t, base.Data, got.Data, "%+v", p)
		assert.Equal(t, image.Pt(100, 200), bdSize(t, got.Data), "%+v", p)
	}
}

func TestProcessExportImage_BodyMatchesBodyshape(t *testing.T) {
	t.Parallel()
	img := bdImage()
	p := bodyshape.Params{Width: -6, Height: 4}
	want, err := bodyshape.Apply(img, bdPoints(), p)
	require.NoError(t, err)
	got, err := (&Service{}).ProcessExportImage(ExportImageParams{ImageBase64: rtB64(t, img), Format: "png", Body: &p, BodyPoints: bdPoints()})
	require.NoError(t, err)
	assert.Equal(t, rtPNG(t, want), got.Data)
}

func TestProcessExportImage_BodyThenResize(t *testing.T) {
	t.Parallel()
	p := bodyshape.Params{Width: -6}
	got, err := (&Service{}).ProcessExportImage(ExportImageParams{ImageBase64: rtB64(t, bdImage()), Format: "png", Width: 50, Height: 100, Body: &p, BodyPoints: bdPoints()})
	require.NoError(t, err)
	assert.Equal(t, image.Pt(50, 100), bdSize(t, got.Data))
}

func TestProcessExportImage_BodyWithoutPointsKeepsOutput(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	src := rtB64(t, bdImage())
	base, err := svc.ProcessExportImage(ExportImageParams{ImageBase64: src, Format: "png"})
	require.NoError(t, err)
	noShoulders := bdPoints()
	noShoulders[2] = bodyshape.Point{X: -1, Y: -1}
	noShoulders[5] = bodyshape.Point{X: -1, Y: -1}
	tests := []struct {
		name string
		p    ExportImageParams
	}{
		{name: "no points", p: ExportImageParams{ImageBase64: src, Format: "png", Body: &bodyshape.Params{Width: -8, Height: 5}}},
		{name: "zero body with points", p: ExportImageParams{ImageBase64: src, Format: "png", Body: &bodyshape.Params{}, BodyPoints: bdPoints()}},
		{name: "width without shoulders", p: ExportImageParams{ImageBase64: src, Format: "png", Body: &bodyshape.Params{Width: -8}, BodyPoints: noShoulders}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.ProcessExportImage(tc.p)
			require.NoError(t, err)
			assert.Equal(t, base.Data, got.Data)
		})
	}
}

func TestProcessExportImage_BodyRejectsOutOfRange(t *testing.T) {
	t.Parallel()
	src := rtB64(t, bdImage())
	for _, p := range []bodyshape.Params{{Width: 11}, {Width: -11}, {Height: 8}, {Height: -8}} {
		for _, pts := range [][]bodyshape.Point{bdPoints(), nil} {
			_, err := (&Service{}).ProcessExportImage(ExportImageParams{ImageBase64: src, Format: "png", Body: &p, BodyPoints: pts})
			assert.Error(t, err, "%+v points=%d", p, len(pts))
		}
	}
}
