package importexport

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/finish"
)

func efPNGBase64(t *testing.T, w, h int, c color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func efDecode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return img
}

func efOff(v float64) *float64 { return &v }

func TestProcessExportImage_Finish(t *testing.T) {
	t.Parallel()
	dark := color.NRGBA{R: 60, G: 60, B: 60, A: 255}

	tests := []struct {
		name   string
		params ExportImageParams
		check  func(t *testing.T, w, h int)
	}{
		{
			name:   "no finish keeps size",
			params: ExportImageParams{Format: "png"},
			check: func(t *testing.T, w, h int) {
				assert.Equal(t, 1000, w)
				assert.Equal(t, 1000, h)
			},
		},
		{
			name:   "neutral finish keeps size",
			params: ExportImageParams{Format: "png", Finish: &finish.Params{}},
			check: func(t *testing.T, w, h int) {
				assert.Equal(t, 1000, w)
				assert.Equal(t, 1000, h)
			},
		},
		{
			name:   "crop 4:5 png",
			params: ExportImageParams{Format: "png", Finish: &finish.Params{Ratio: "4:5", Fit: "crop", Offset: efOff(0.5)}},
			check: func(t *testing.T, w, h int) {
				assert.Equal(t, 800, w)
				assert.Equal(t, 1000, h)
			},
		},
		{
			name:   "crop 4:5 jpeg",
			params: ExportImageParams{Format: "jpeg", Quality: 90, Finish: &finish.Params{Ratio: "4:5", Fit: "crop"}},
			check: func(t *testing.T, w, h int) {
				assert.Equal(t, 800, w)
				assert.Equal(t, 1000, h)
			},
		},
		{
			name:   "fit 9:16 white",
			params: ExportImageParams{Format: "png", Finish: &finish.Params{Ratio: "9:16", Fit: "white"}},
			check: func(t *testing.T, w, h int) {
				assert.InDelta(t, 9.0/16.0, float64(w)/float64(h), 0.01, "%dx%d", w, h)
			},
		},
		{
			name:   "resize applies to the photo before format",
			params: ExportImageParams{Format: "png", Width: 500, Height: 500, Finish: &finish.Params{Ratio: "4:5", Fit: "crop"}},
			check: func(t *testing.T, w, h int) {
				assert.Equal(t, 400, w)
				assert.Equal(t, 500, h)
			},
		},
		{
			name:   "resize applies to the photo before frame",
			params: ExportImageParams{Format: "png", Width: 500, Height: 500, Finish: &finish.Params{Frame: "white"}},
			check: func(t *testing.T, w, h int) {
				assert.Greater(t, w, 500, "frame is added on top of the resized photo")
				assert.Greater(t, h, 500, "frame is added on top of the resized photo")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(t, &mockSDService{})
			p := tt.params
			p.ImageBase64 = efPNGBase64(t, 1000, 1000, dark)

			out, err := svc.ProcessExportImage(p)

			require.NoError(t, err)
			require.NotNil(t, out)
			img := efDecode(t, out.Data)
			tt.check(t, img.Bounds().Dx(), img.Bounds().Dy())
		})
	}
}

func TestProcessExportImage_FinishFrameKeepsResizedPhoto(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, &mockSDService{})
	dark := color.NRGBA{R: 60, G: 60, B: 60, A: 255}

	out, err := svc.ProcessExportImage(ExportImageParams{
		ImageBase64: efPNGBase64(t, 1000, 1000, dark),
		Format:      "png",
		Width:       500,
		Height:      500,
		Finish:      &finish.Params{Frame: "black"},
	})

	require.NoError(t, err)
	img := efDecode(t, out.Data)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	photo := 0
	for x := 0; x < w; x++ {
		c := color.NRGBAModel.Convert(img.At(x, h/2)).(color.NRGBA)
		if c.R > 40 && c.R < 80 {
			photo++
		}
	}
	assert.InDelta(t, 500, photo, 2, "the photo inside the frame must be 500 px wide")
}

func TestProcessExportImage_InvalidFinish(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p    finish.Params
	}{
		{name: "bad ratio", p: finish.Params{Ratio: "7:3", Fit: "crop"}},
		{name: "bad frame", p: finish.Params{Frame: "gold"}},
		{name: "offset out of range", p: finish.Params{Ratio: "1:1", Fit: "crop", Offset: efOff(2)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(t, &mockSDService{})
			p := tt.p

			_, err := svc.ProcessExportImage(ExportImageParams{
				ImageBase64: efPNGBase64(t, 200, 200, color.NRGBA{R: 1, G: 2, B: 3, A: 255}),
				Format:      "png",
				Finish:      &p,
			})

			assert.Error(t, err)
		})
	}
}
