package session

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func orientedJPEG(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{20, 20, 220, 255}
			if x < w/2 {
				c = color.RGBA{220, 20, 20, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}))
	raw := buf.Bytes()
	if orientation == 0 {
		return raw
	}
	var tiff bytes.Buffer
	tiff.WriteString("MM\x00*")
	be := binary.BigEndian
	_ = binary.Write(&tiff, be, uint32(8))
	_ = binary.Write(&tiff, be, uint16(1))
	_ = binary.Write(&tiff, be, uint16(0x0112))
	_ = binary.Write(&tiff, be, uint16(3))
	_ = binary.Write(&tiff, be, uint32(1))
	_ = binary.Write(&tiff, be, uint16(orientation))
	_ = binary.Write(&tiff, be, uint16(0))
	_ = binary.Write(&tiff, be, uint32(0))
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	be.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{}, raw[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, raw[2:]...)
}

func redAt(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return r>>8 > 150 && g>>8 < 90 && b>>8 < 90
}

func TestAddItem_AppliesEXIFOrientation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		orientation int
		w, h        int
		redX, redY  int
		sameBytes   bool
	}{
		{"no exif kept as is", 0, 320, 160, 10, 80, true},
		{"orientation 1 kept as is", 1, 320, 160, 10, 80, true},
		{"orientation 3 rotated 180", 3, 320, 160, 310, 80, false},
		{"orientation 6 rotated clockwise", 6, 160, 320, 80, 10, false},
		{"orientation 8 rotated counter-clockwise", 8, 160, 320, 80, 310, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, dataDir, _ := serviceWithDB(t)
			orig := orientedJPEG(t, 320, 160, tt.orientation)
			id := svc.AddToSession(base64.StdEncoding.EncodeToString(orig), nil, "sd", nil)
			require.Greater(t, id, int64(0))
			item, err := svc.SessionItem(id)
			require.NoError(t, err)
			idStr := strconv.FormatInt(id, 10)
			assert.Equal(t, idStr+".jpg", item.FileName)
			assert.Equal(t, tt.w, item.Width)
			assert.Equal(t, tt.h, item.Height)

			sid := strconv.FormatInt(item.SessionID, 10)
			written, err := os.ReadFile(filepath.Join(dataDir, "sessions", sid, item.FileName))
			require.NoError(t, err)
			if tt.sameBytes {
				assert.Equal(t, orig, written)
			} else {
				assert.NotContains(t, string(written), "Exif\x00\x00", "orientation tag must not survive")
			}
			img, err := jpeg.Decode(bytes.NewReader(written))
			require.NoError(t, err)
			assert.Equal(t, tt.w, img.Bounds().Dx())
			assert.Equal(t, tt.h, img.Bounds().Dy())
			assert.True(t, redAt(img, tt.redX, tt.redY), "red half at (%d,%d)", tt.redX, tt.redY)

			thumb, err := os.ReadFile(filepath.Join(dataDir, "thumbs", sid, item.ThumbName))
			require.NoError(t, err)
			cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
			require.NoError(t, err)
			assert.Equal(t, tt.w > tt.h, cfg.Width > cfg.Height, "thumbnail follows upright orientation")
		})
	}
}

func TestReencodeJPEG_AppliesEXIFOrientation(t *testing.T) {
	t.Parallel()
	data, err := reencodeJPEG(base64.StdEncoding.EncodeToString(orientedJPEG(t, 64, 32, 6)))
	require.NoError(t, err)
	img, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, 32, img.Bounds().Dx())
	assert.Equal(t, 64, img.Bounds().Dy())
	assert.True(t, redAt(img, 16, 4))
}
