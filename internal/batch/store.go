package batch

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Zazza/imagecaster/internal/config"
)

var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
)

type Store struct {
	dir string
	mu  sync.Mutex
	seq int
}

func New(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Save(imageBase64 string) (string, error) {
	if i := strings.Index(imageBase64, ","); i >= 0 && strings.HasPrefix(imageBase64, "data:") {
		imageBase64 = imageBase64[i+1:]
	}
	if imageBase64 == "" {
		return "", errors.New("image is required")
	}
	if len(imageBase64) > config.MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}
	data, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}
	var ext string
	switch {
	case bytes.HasPrefix(data, pngMagic):
		ext = ".png"
	case bytes.HasPrefix(data, jpegMagic):
		ext = ".jpg"
	default:
		return "", errors.New("unsupported image format")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	for {
		s.seq++
		path := filepath.Join(s.dir, fmt.Sprintf("%06d%s", s.seq, ext))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			os.Remove(path)
			return "", err
		}
		if err := f.Close(); err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	}
}
