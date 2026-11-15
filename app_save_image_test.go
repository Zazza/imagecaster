package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Zazza/imagecaster/internal/imgformat"
)

func TestSaveImageName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		in     string
		format imgformat.Format
		want   string
	}{
		{"jpeg session file keeps jpg", "123.jpg", imgformat.JPEG, "123.jpg"},
		{"jpeg session file named png fixed", "123.png", imgformat.JPEG, "123.jpg"},
		{"jpeg extension normalized", "photo.JPEG", imgformat.JPEG, "photo.jpg"},
		{"png session file keeps png", "45.png", imgformat.PNG, "45.png"},
		{"png bytes under jpg name", "45.jpg", imgformat.PNG, "45.png"},
		{"no extension gets real one", "history_7", imgformat.JPEG, "history_7.jpg"},
		{"empty name", "", imgformat.PNG, "imagecaster-image.png"},
		{"blank name", "  ", imgformat.JPEG, "imagecaster-image.jpg"},
		{"non-image dot kept", "my.photo", imgformat.PNG, "my.photo.png"},
		{"webp", "x.png", imgformat.WebP, "x.webp"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, saveImageName(tt.in, tt.format), tt.name)
	}
}

func TestSaveImageFilter(t *testing.T) {
	t.Parallel()
	assert.Equal(t, runtime.FileFilter{DisplayName: "JPEG Image", Pattern: "*.jpg;*.jpeg"}, saveImageFilter(imgformat.JPEG))
	assert.Equal(t, runtime.FileFilter{DisplayName: "PNG Image", Pattern: "*.png"}, saveImageFilter(imgformat.PNG))
}

func TestSaveImage_RejectsUnknownBytes(t *testing.T) {
	t.Parallel()
	a := &App{}
	_, err := a.SaveImage("aGVsbG8=", "x.png")
	assert.ErrorContains(t, err, "unsupported image format")
	_, err = a.SaveImage("!!!", "x.png")
	assert.Error(t, err)
	path, err := a.SaveImage("", "x.png")
	assert.NoError(t, err)
	assert.Empty(t, path)
}
