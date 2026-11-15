package imgcodec

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
)

type Options struct {
	Name    string
	MaxDim  int
	DataURL bool
}

func StripDataURL(s string) string {
	if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
		return s[i+1:]
	}
	return s
}

func Decode(s string, o Options) (image.Image, error) {
	name := o.Name
	if name == "" {
		name = "image"
	}
	if o.DataURL {
		s = StripDataURL(s)
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	if o.MaxDim > 0 {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > o.MaxDim || cfg.Height > o.MaxDim {
			return nil, fmt.Errorf("%s too large: %dx%d (max %d per side)", name, cfg.Width, cfg.Height, o.MaxDim)
		}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return img, nil
}

func EncodePNG(img image.Image, level png.CompressionLevel) (string, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: level}
	if err := enc.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode png: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func EncodeJPEG(img image.Image, quality int) (string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return "", fmt.Errorf("encode jpeg: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
