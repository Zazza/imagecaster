package adjust

import "image"

type pixelFn func(x, y int) (r, g, b, a uint32)

func pixelSource(img image.Image) pixelFn {
	switch m := img.(type) {
	case *image.RGBA:
		return func(x, y int) (uint32, uint32, uint32, uint32) {
			i := m.PixOffset(x, y)
			p := m.Pix[i : i+4 : i+4]
			return uint32(p[0]), uint32(p[1]), uint32(p[2]), uint32(p[3])
		}
	case *image.NRGBA:
		return func(x, y int) (uint32, uint32, uint32, uint32) {
			i := m.PixOffset(x, y)
			p := m.Pix[i : i+4 : i+4]
			a := uint32(p[3])
			return premul8(p[0], a), premul8(p[1], a), premul8(p[2], a), a
		}
	case *image.YCbCr:
		return func(x, y int) (uint32, uint32, uint32, uint32) {
			r, g, b, a := m.YCbCrAt(x, y).RGBA()
			return r >> 8, g >> 8, b >> 8, a >> 8
		}
	default:
		return func(x, y int) (uint32, uint32, uint32, uint32) {
			r, g, b, a := img.At(x, y).RGBA()
			return r >> 8, g >> 8, b >> 8, a >> 8
		}
	}
}

func premul8(c uint8, a uint32) uint32 {
	v := uint32(c)
	v |= v << 8
	v *= a
	v /= 0xff
	return v >> 8
}
