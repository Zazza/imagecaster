package imgformat

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	red  = color.RGBA{220, 20, 20, 255}
	blue = color.RGBA{20, 20, 220, 255}
)

func halfImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.Set(x, y, red)
			} else {
				img.Set(x, y, blue)
			}
		}
	}
	return img
}

func exifSegment(bo binary.ByteOrder, orientation int) []byte {
	var tiff bytes.Buffer
	if bo == binary.LittleEndian {
		tiff.WriteString("II*\x00")
	} else {
		tiff.WriteString("MM\x00*")
	}
	_ = binary.Write(&tiff, bo, uint32(8))
	_ = binary.Write(&tiff, bo, uint16(2))
	_ = binary.Write(&tiff, bo, uint16(0x010F))
	_ = binary.Write(&tiff, bo, uint16(2))
	_ = binary.Write(&tiff, bo, uint32(1))
	_ = binary.Write(&tiff, bo, uint32(0))
	_ = binary.Write(&tiff, bo, uint16(0x0112))
	_ = binary.Write(&tiff, bo, uint16(3))
	_ = binary.Write(&tiff, bo, uint32(1))
	_ = binary.Write(&tiff, bo, uint16(orientation))
	_ = binary.Write(&tiff, bo, uint16(0))
	_ = binary.Write(&tiff, bo, uint32(0))
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	return append(seg, payload...)
}

func jpegWithOrientation(t *testing.T, img image.Image, bo binary.ByteOrder, orientation int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}))
	raw := buf.Bytes()
	if orientation == 0 {
		return raw
	}
	out := append([]byte{}, raw[:2]...)
	out = append(out, exifSegment(bo, orientation)...)
	return append(out, raw[2:]...)
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 > 150 && g>>8 < 90 && b>>8 < 90
}

func TestGoJPEGDecoderIgnoresEXIFOrientation(t *testing.T) {
	t.Parallel()
	data := jpegWithOrientation(t, halfImage(32, 16), binary.BigEndian, 6)
	img, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, 32, img.Bounds().Dx(), "decoder returns stored, unrotated width")
	assert.Equal(t, 16, img.Bounds().Dy())
	assert.True(t, isRed(img.At(2, 8)), "left half still red: no rotation applied")
}

func TestJPEGOrientation(t *testing.T) {
	t.Parallel()
	src := halfImage(32, 16)
	for _, o := range []int{1, 2, 3, 4, 5, 6, 7, 8} {
		for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			assert.Equal(t, o, JPEGOrientation(jpegWithOrientation(t, src, bo, o)), "orientation %d %v", o, bo)
		}
	}
	assert.Equal(t, 1, JPEGOrientation(jpegWithOrientation(t, src, binary.BigEndian, 0)), "no exif")
	assert.Equal(t, 1, JPEGOrientation(jpegWithOrientation(t, src, binary.BigEndian, 9)), "out of range")
	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, src))
	assert.Equal(t, 1, JPEGOrientation(pngBuf.Bytes()), "png")
	assert.Equal(t, 1, JPEGOrientation(nil))
	truncated := jpegWithOrientation(t, src, binary.BigEndian, 6)[:20]
	assert.Equal(t, 1, JPEGOrientation(truncated))
}

func TestApplyOrientation(t *testing.T) {
	t.Parallel()
	src := halfImage(4, 2)
	tests := []struct {
		o          int
		w, h       int
		redAt      image.Point
		blueAt     image.Point
		sameObject bool
	}{
		{1, 4, 2, image.Pt(0, 0), image.Pt(3, 0), true},
		{2, 4, 2, image.Pt(3, 0), image.Pt(0, 0), false},
		{3, 4, 2, image.Pt(3, 1), image.Pt(0, 1), false},
		{4, 4, 2, image.Pt(0, 0), image.Pt(3, 0), false},
		{5, 2, 4, image.Pt(0, 0), image.Pt(0, 3), false},
		{6, 2, 4, image.Pt(0, 0), image.Pt(0, 3), false},
		{7, 2, 4, image.Pt(0, 3), image.Pt(0, 0), false},
		{8, 2, 4, image.Pt(0, 3), image.Pt(0, 0), false},
	}
	for _, tt := range tests {
		got := ApplyOrientation(src, tt.o)
		assert.Equal(t, tt.w, got.Bounds().Dx(), "o=%d", tt.o)
		assert.Equal(t, tt.h, got.Bounds().Dy(), "o=%d", tt.o)
		assert.Equal(t, red, color.RGBAModel.Convert(got.At(tt.redAt.X, tt.redAt.Y)), "o=%d red", tt.o)
		assert.Equal(t, blue, color.RGBAModel.Convert(got.At(tt.blueAt.X, tt.blueAt.Y)), "o=%d blue", tt.o)
		if tt.sameObject {
			assert.Same(t, src, got.(*image.RGBA))
		}
	}
}

func TestApplyOrientation_Rotate90Exact(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := range src.Pix {
		src.Pix[i] = uint8(i)
	}
	cw := ApplyOrientation(src, 6).(*image.RGBA)
	ccw := ApplyOrientation(src, 8).(*image.RGBA)
	assert.Equal(t, src.RGBAAt(0, 1), cw.RGBAAt(0, 0), "bottom-left goes to top-left on CW")
	assert.Equal(t, src.RGBAAt(0, 0), cw.RGBAAt(1, 0), "top-left goes to top-right on CW")
	assert.Equal(t, src.RGBAAt(2, 0), ccw.RGBAAt(0, 0), "top-right goes to top-left on CCW")
	assert.Equal(t, src.RGBAAt(0, 0), ccw.RGBAAt(0, 2), "top-left goes to bottom-left on CCW")
}

func TestDetect(t *testing.T) {
	t.Parallel()
	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, halfImage(2, 2)))
	tests := []struct {
		name string
		data []byte
		want Format
		ok   bool
	}{
		{"png", pngBuf.Bytes(), PNG, true},
		{"jpeg", jpegWithOrientation(t, halfImage(2, 2), binary.BigEndian, 0), JPEG, true},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), WebP, true},
		{"gif", []byte("GIF89a......"), GIF, true},
		{"text", []byte("hello"), Format{}, false},
		{"empty", nil, Format{}, false},
	}
	for _, tt := range tests {
		got, ok := Detect(tt.data)
		assert.Equal(t, tt.ok, ok, tt.name)
		assert.Equal(t, tt.want, got, tt.name)
	}
	assert.Equal(t, []string{".jpg", ".jpeg"}, JPEG.Exts())
	assert.Equal(t, []string{".png"}, PNG.Exts())
}
