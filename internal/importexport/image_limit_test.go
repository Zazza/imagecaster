package importexport

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
)

var (
	ieLargePNGOnce sync.Once
	ieLargePNG     string
)

func ieLargeNoisePNGBase64(t *testing.T) string {
	t.Helper()
	ieLargePNGOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 2400, 2200))
		rand.New(rand.NewSource(1)).Read(img.Pix)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		ieLargePNG = base64.StdEncoding.EncodeToString(buf.Bytes())
	})
	require.Greater(t, len(ieLargePNG), 22*1024*1024)
	require.Less(t, len(ieLargePNG), config.MaxImageBase64Len)
	return ieLargePNG
}

func TestProcessExportImage_LargePNGPassesSizeCheck(t *testing.T) {
	svc := newTestService(t, &mockSDService{})

	img, err := svc.ProcessExportImage(ExportImageParams{
		ImageBase64: ieLargeNoisePNGBase64(t),
		Format:      "png",
	})
	if err != nil {
		assert.NotContains(t, err.Error(), "too large")
	}
	require.NoError(t, err)
	require.NotNil(t, img)
	assert.NotEmpty(t, img.Data)
}

func TestProcessExportImage_OverLimitRejected(t *testing.T) {
	svc := newTestService(t, &mockSDService{})

	_, err := svc.ProcessExportImage(ExportImageParams{
		ImageBase64: strings.Repeat("A", config.MaxImageBase64Len+1),
		Format:      "png",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")
	assert.Contains(t, err.Error(), "128 MB")
}

func TestPixelLimitUnchanged(t *testing.T) {
	assert.Equal(t, 8192*8192, MaxImagePixels)

	err := CheckImageDataSize(hugePNGHeader(8193, 8193))
	require.Error(t, err)

	svc := newTestService(t, &mockSDService{})
	_, err = svc.ProcessExportImage(ExportImageParams{
		ImageBase64: base64.StdEncoding.EncodeToString(hugePNGHeader(8193, 8193)),
		Format:      "png",
	})
	require.Error(t, err)
}
