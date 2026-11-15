package postpreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"sync"

	xdraw "golang.org/x/image/draw"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/finish"
	"github.com/Zazza/imagecaster/internal/importexport"
)

const (
	DefaultCacheSize = 3
	jpegQuality      = 85
)

type cacheKey struct {
	sum  [sha256.Size]byte
	maxW int
}

type entry struct {
	key cacheKey
	img image.Image
}

type Service struct {
	mu      sync.Mutex
	size    int
	entries []entry
	decodes int
}

func New(size int) *Service {
	return &Service{size: max(1, size)}
}

func (s *Service) Adjust(imageBase64 string, p adjust.Params, maxW int) (string, error) {
	return s.Post(imageBase64, p, finish.Params{}, maxW)
}

func (s *Service) Post(imageBase64 string, p adjust.Params, f finish.Params, maxW int) (string, error) {
	img, err := s.scaled(imageBase64, maxW)
	if err != nil {
		return "", err
	}
	out, err := finish.Apply(adjust.Apply(img, p), f)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return "", fmt.Errorf("encode jpeg: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func (s *Service) Auto(imageBase64 string) (adjust.AutoStats, error) {
	img, err := decode(imageBase64)
	if err != nil {
		return adjust.AutoStats{}, err
	}
	return adjust.AnalyzeAuto(img), nil
}

func (s *Service) scaled(imageBase64 string, maxW int) (image.Image, error) {
	if err := checkInput(imageBase64); err != nil {
		return nil, err
	}
	key := cacheKey{sum: sha256.Sum256([]byte(imageBase64)), maxW: maxW}
	if img := s.lookup(key); img != nil {
		return img, nil
	}
	img, err := decode(imageBase64)
	if err != nil {
		return nil, err
	}
	img = downscale(img, maxW)
	s.store(key, img)
	return img, nil
}

func (s *Service) lookup(key cacheKey) image.Image {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.key == key {
			copy(s.entries[1:i+1], s.entries[:i])
			s.entries[0] = e
			return e.img
		}
	}
	return nil
}

func (s *Service) store(key cacheKey, img image.Image) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decodes++
	for _, e := range s.entries {
		if e.key == key {
			return
		}
	}
	if len(s.entries) < s.size {
		s.entries = append(s.entries, entry{})
	}
	copy(s.entries[1:], s.entries[:len(s.entries)-1])
	s.entries[0] = entry{key: key, img: img}
}

func checkInput(imageBase64 string) error {
	if imageBase64 == "" {
		return fmt.Errorf("no image provided")
	}
	if len(imageBase64) > config.MaxImageBase64Len {
		return config.ImageTooLarge("image")
	}
	return nil
}

func decode(imageBase64 string) (image.Image, error) {
	if err := checkInput(imageBase64); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	if err := importexport.CheckImageDataSize(data); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func downscale(img image.Image, maxW int) image.Image {
	b := img.Bounds()
	if maxW <= 0 || b.Dx() <= maxW {
		return img
	}
	h := max(1, int(float64(b.Dy())*float64(maxW)/float64(b.Dx())))
	dst := image.NewRGBA(image.Rect(0, 0, maxW, h))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
	return dst
}
