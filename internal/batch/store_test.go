package batch

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tinyImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for x := 0; x < 4; x++ {
		for y := 0; y < 3; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 80), B: 120, A: 255})
		}
	}
	return img
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, tinyImage()))
	return buf.Bytes()
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, tinyImage(), &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func b64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func dirEntries(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return entries
}

func TestSaveWritesFileWithFormatExtension(t *testing.T) {
	tests := []struct {
		name string
		data func(t *testing.T) []byte
		ext  string
	}{
		{name: "png", data: pngBytes, ext: ".png"},
		{name: "jpeg", data: jpegBytes, ext: ".jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "batch")
			data := tt.data(t)

			path, err := New(dir).Save(b64(data))
			require.NoError(t, err)

			assert.Equal(t, filepath.Clean(dir), filepath.Dir(path))
			assert.Equal(t, tt.ext, filepath.Ext(path))
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, data, got)
		})
	}
}

func TestSaveSameImageTwiceGivesDifferentPaths(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	data := b64(pngBytes(t))

	first, err := store.Save(data)
	require.NoError(t, err)
	second, err := store.Save(data)
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.FileExists(t, first)
	assert.FileExists(t, second)
	assert.Len(t, dirEntries(t, dir), 2)
}

func overLimitPNG(t *testing.T) string {
	t.Helper()
	data := pngBytes(t)
	for len(data)%3 != 0 {
		data = append(data, 0)
	}
	head := b64(data)
	return head + strings.Repeat("A", config.MaxImageBase64Len+4-len(head))
}

func TestSaveRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input func(t *testing.T) string
	}{
		{name: "empty", input: func(t *testing.T) string { return "" }},
		{name: "not base64", input: func(t *testing.T) string { return "!!!not base64 at all???" }},
		{name: "base64 of text", input: func(t *testing.T) string { return b64([]byte("hello, this is plain text, not an image")) }},
		{name: "longer than limit", input: overLimitPNG},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "batch")

			path, err := New(dir).Save(tt.input(t))

			require.Error(t, err)
			assert.Empty(t, path)
			assert.Empty(t, dirEntries(t, dir))
		})
	}
}

func TestSaveAcceptsDataURLPrefix(t *testing.T) {
	dir := t.TempDir()
	data := pngBytes(t)

	path, err := New(dir).Save("data:image/png;base64," + b64(data))
	require.NoError(t, err)

	assert.Equal(t, ".png", filepath.Ext(path))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestResetRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	first, err := store.Save(b64(pngBytes(t)))
	require.NoError(t, err)
	second, err := store.Save(b64(jpegBytes(t)))
	require.NoError(t, err)

	require.NoError(t, store.Reset())

	assert.NoFileExists(t, first)
	assert.NoFileExists(t, second)
	assert.Empty(t, dirEntries(t, dir))
}

func TestResetOnMissingDirIsNil(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")

	assert.NoError(t, New(dir).Reset())
}

func TestSaveWorksAfterReset(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	_, err := store.Save(b64(pngBytes(t)))
	require.NoError(t, err)
	require.NoError(t, store.Reset())

	data := pngBytes(t)
	path, err := store.Save(b64(data))
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)
	assert.Len(t, dirEntries(t, dir), 1)
}
