package filebrowser

import (
	"image"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
)

func writeLargeNoisePNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2400, 2200))
	rand.New(rand.NewSource(1)).Read(img.Pix)
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())
	st, err := os.Stat(path)
	require.NoError(t, err)
	require.Greater(t, st.Size(), int64(16*1024*1024))
}

func writeOverLimitFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(int64(config.MaxImageBytes)+1))
	require.NoError(t, f.Close())
}

func TestReadFileAsBase64_LargePNGPassesSizeCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.png")
	writeLargeNoisePNG(t, path)

	b64, err := ReadFileAsBase64(path)
	if err != nil {
		assert.NotContains(t, err.Error(), "too large")
	}
	require.NoError(t, err)
	assert.NotEmpty(t, b64)
}

func TestReadThumbnail_LargePNGPassesSizeCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.png")
	writeLargeNoisePNG(t, path)

	thumb, err := ReadThumbnail(path)
	if err != nil {
		assert.NotContains(t, err.Error(), "too large")
	}
	require.NoError(t, err)
	assert.NotEmpty(t, thumb)
}

func TestReadFileAsBase64_OverLimitRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.png")
	writeOverLimitFile(t, path)

	_, err := ReadFileAsBase64(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
	assert.Contains(t, err.Error(), "128 MB")
}

func TestReadThumbnail_OverLimitRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.png")
	writeOverLimitFile(t, path)

	_, err := ReadThumbnail(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
	assert.Contains(t, err.Error(), "128 MB")
}
