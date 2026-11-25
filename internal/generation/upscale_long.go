package generation

import (
	"errors"
	"image"
	"image/draw"
	"image/png"
	"math"

	xdraw "golang.org/x/image/draw"

	"github.com/Zazza/imagecaster/internal/imgcodec"
)

const upscaleMaxDim = 8192

type UpscaleResult struct {
	Image    string
	Width    int
	Height   int
	Upscaled bool
	Resized  bool
}

func (s *Service) UpscaleLong(imageBase64 string, targetLong int) (*UpscaleResult, error) {
	if imageBase64 == "" {
		return nil, errors.New("image is required")
	}
	if s.Stopped() {
		return nil, ErrInterrupted
	}
	src, err := imgcodec.Decode(imageBase64, imgcodec.Options{Name: "image", DataURL: true, MaxDim: upscaleMaxDim})
	if err != nil {
		return nil, err
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	res := &UpscaleResult{Image: imageBase64, Width: w, Height: h}
	long := max(w, h)
	if targetLong <= 0 || long >= targetLong {
		return res, nil
	}
	tw, th := fitLong(w, h, targetLong)
	img, err := s.neuralUpscale(imageBase64, float64(targetLong)/float64(long))
	if err != nil {
		return nil, err
	}
	if img == nil {
		img, res.Resized = src, true
	}
	b64, err := imgcodec.EncodePNG(resizeExact(img, tw, th), png.BestSpeed)
	if err != nil {
		return nil, err
	}
	res.Image, res.Width, res.Height, res.Upscaled = b64, tw, th, true
	return res, nil
}

func fitLong(w, h, long int) (int, int) {
	if w >= h {
		return long, int(math.Round(float64(h) * float64(long) / float64(w)))
	}
	return int(math.Round(float64(w) * float64(long) / float64(h))), long
}

func resizeExact(img image.Image, w, h int) image.Image {
	if img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	return dst
}

func (s *Service) neuralUpscale(b64 string, scale float64) (image.Image, error) {
	name := s.resolveQualityUpscaler()
	if name == "" {
		return nil, nil
	}
	up, err := s.measuredUpscale(b64, name, scale)
	if err != nil {
		if IsInterrupted(err) {
			return nil, err
		}
		s.log.Warn("upscale: %s failed, resizing: %s", name, err)
		return nil, nil
	}
	img, err := imgcodec.Decode(up, imgcodec.Options{Name: "upscaled", MaxDim: upscaleMaxDim * 2})
	if err != nil {
		s.log.Warn("upscale: %s result unreadable, resizing: %s", name, err)
		return nil, nil
	}
	return img, nil
}
