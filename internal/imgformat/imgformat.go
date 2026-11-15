package imgformat

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
)

var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	gifMagic  = []byte("GIF8")
)

type Format struct {
	Ext  string
	MIME string
	Name string
}

var (
	PNG  = Format{Ext: ".png", MIME: "image/png", Name: "PNG"}
	JPEG = Format{Ext: ".jpg", MIME: "image/jpeg", Name: "JPEG"}
	WebP = Format{Ext: ".webp", MIME: "image/webp", Name: "WebP"}
	GIF  = Format{Ext: ".gif", MIME: "image/gif", Name: "GIF"}
)

func Detect(data []byte) (Format, bool) {
	switch {
	case bytes.HasPrefix(data, pngMagic):
		return PNG, true
	case bytes.HasPrefix(data, jpegMagic):
		return JPEG, true
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return WebP, true
	case bytes.HasPrefix(data, gifMagic):
		return GIF, true
	}
	return Format{}, false
}

func (f Format) Exts() []string {
	if f == JPEG {
		return []string{".jpg", ".jpeg"}
	}
	return []string{f.Ext}
}

func JPEGOrientation(data []byte) int {
	if !bytes.HasPrefix(data, jpegMagic) {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		if marker == 0xE1 {
			if o := exifOrientation(data[i+4 : i+2+size]); o != 0 {
				return o
			}
		}
		i += 2 + size
	}
	return 1
}

func exifOrientation(seg []byte) int {
	if len(seg) < 14 || string(seg[0:6]) != "Exif\x00\x00" {
		return 0
	}
	tiff := seg[6:]
	var bo binary.ByteOrder
	switch string(tiff[0:4]) {
	case "II*\x00":
		bo = binary.LittleEndian
	case "MM\x00*":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(tiff[4:8]))
	if off < 8 || off+2 > len(tiff) {
		return 0
	}
	n := int(bo.Uint16(tiff[off : off+2]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(tiff) {
			return 0
		}
		if bo.Uint16(tiff[e:e+2]) != 0x0112 {
			continue
		}
		if bo.Uint16(tiff[e+2:e+4]) != 3 {
			return 0
		}
		v := int(bo.Uint16(tiff[e+8 : e+10]))
		if v < 1 || v > 8 {
			return 0
		}
		return v
	}
	return 0
}

func ApplyOrientation(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	src := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			si := sy*src.Stride + sx*4
			di := y*dst.Stride + x*4
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}
