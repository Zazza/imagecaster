package filebrowser

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jpegOrientation6(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil))
	raw := buf.Bytes()
	var tiff bytes.Buffer
	tiff.WriteString("II*\x00")
	le := binary.LittleEndian
	_ = binary.Write(&tiff, le, uint32(8))
	_ = binary.Write(&tiff, le, uint16(1))
	_ = binary.Write(&tiff, le, uint16(0x0112))
	_ = binary.Write(&tiff, le, uint16(3))
	_ = binary.Write(&tiff, le, uint32(1))
	_ = binary.Write(&tiff, le, uint16(6))
	_ = binary.Write(&tiff, le, uint16(0))
	_ = binary.Write(&tiff, le, uint32(0))
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{}, raw[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, raw[2:]...)
}

func TestReadThumbnail_RotatesByEXIFOrientation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "camera.jpg")
	require.NoError(t, os.WriteFile(path, jpegOrientation6(t, 800, 400), 0o644))
	b64, err := ReadThumbnail(path)
	require.NoError(t, err)
	data, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, 128, cfg.Width)
	assert.Equal(t, 256, cfg.Height)
}
