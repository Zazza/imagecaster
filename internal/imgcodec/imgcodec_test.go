package imgcodec

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	return img
}

func pngB64(t *testing.T, w, h int) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, testImage(w, h)))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestStripDataURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no prefix", in: "AAAA", want: "AAAA"},
		{name: "png prefix", in: "data:image/png;base64,AAAA", want: "AAAA"},
		{name: "jpeg prefix", in: "data:image/jpeg;base64,BBBB", want: "BBBB"},
		{name: "comma without data scheme", in: "abc,AAAA", want: "abc,AAAA"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, StripDataURL(tt.in))
		})
	}
}

func TestDecode(t *testing.T) {
	raw := pngB64(t, 8, 6)
	tests := []struct {
		name    string
		in      string
		opts    Options
		wantErr string
		wantW   int
		wantH   int
	}{
		{name: "plain base64", in: raw, wantW: 8, wantH: 6},
		{name: "data url accepted", in: "data:image/png;base64," + raw, opts: Options{DataURL: true}, wantW: 8, wantH: 6},
		{name: "data url rejected without option", in: "data:image/png;base64," + raw, opts: Options{Name: "mask"}, wantErr: "decode mask:"},
		{name: "invalid base64", in: "!!!not base64", opts: Options{Name: "image"}, wantErr: "decode image:"},
		{name: "default name", in: "!!!", wantErr: "decode image:"},
		{name: "not an image", in: base64.StdEncoding.EncodeToString([]byte("hello world")), opts: Options{Name: "result"}, wantErr: "parse result:"},
		{name: "not an image with limit", in: base64.StdEncoding.EncodeToString([]byte("hello world")), opts: Options{Name: "mask", MaxDim: 16}, wantErr: "parse mask:"},
		{name: "within limit", in: raw, opts: Options{MaxDim: 8}, wantW: 8, wantH: 6},
		{name: "too large", in: raw, opts: Options{Name: "image", MaxDim: 7}, wantErr: "image too large: 8x6 (max 7 per side)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := Decode(tt.in, tt.opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, img)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantW, img.Bounds().Dx())
			assert.Equal(t, tt.wantH, img.Bounds().Dy())
		})
	}
}

func TestEncodePNGRoundTrip(t *testing.T) {
	src := testImage(10, 7)
	for _, level := range []png.CompressionLevel{png.DefaultCompression, png.BestSpeed, png.BestCompression} {
		b64, err := EncodePNG(src, level)
		require.NoError(t, err)
		data, err := base64.StdEncoding.DecodeString(b64)
		require.NoError(t, err)
		assert.True(t, bytes.HasPrefix(data, []byte("\x89PNG")))
		got, err := Decode(b64, Options{})
		require.NoError(t, err)
		require.Equal(t, src.Bounds(), got.Bounds())
		for y := 0; y < 7; y++ {
			for x := 0; x < 10; x++ {
				assert.Equal(t, color.RGBAModel.Convert(src.At(x, y)), color.RGBAModel.Convert(got.At(x, y)))
			}
		}
	}
}

func TestEncodeJPEGRoundTrip(t *testing.T) {
	src := testImage(16, 9)
	b64, err := EncodeJPEG(src, 90)
	require.NoError(t, err)
	data, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	_, err = jpeg.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	got, err := Decode("data:image/jpeg;base64,"+b64, Options{DataURL: true})
	require.NoError(t, err)
	assert.Equal(t, src.Bounds(), got.Bounds())

	low, err := EncodeJPEG(src, 10)
	require.NoError(t, err)
	assert.Less(t, len(low), len(b64))
}
