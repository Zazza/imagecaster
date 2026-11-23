package objectmask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	Directory                = "object-mask"
	EncoderName              = "encoder.onnx"
	DecoderName              = "decoder.onnx"
	EncoderURL               = "/api/object-mask/" + EncoderName
	DecoderURL               = "/api/object-mask/" + DecoderName
	downloadTimeout          = 5 * time.Minute
	maxModelBytes      int64 = 128 << 20
	cacheDirectoryMode       = 0700
)

type asset struct {
	name   string
	url    string
	sha256 string
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type preparation struct {
	done chan struct{}
	err  error
}

type Store struct {
	dir     string
	client  httpClient
	assets  []asset
	mu      sync.Mutex
	ready   bool
	pending *preparation
}

func New(dataDir string) *Store {
	return &Store{dir: filepath.Join(dataDir, Directory), client: &http.Client{Timeout: downloadTimeout}, assets: []asset{
		{name: EncoderName, url: "https://huggingface.co/spaces/Akbartus/projects/resolve/e1813cbdd878bf0037e0e81a08c5e93fe31d66e7/mobilesam.encoder.onnx", sha256: "4125037c5e24d6ea58e201b20e8d8fbbbd1135c0b881e34a8074b8c4f07e6918"},
		{name: DecoderName, url: "https://raw.githubusercontent.com/akbartus/MobileSAM-in-the-Browser/fa8e7b9215ccdc45e8a5f5b8a42892ad4b440e1f/models/mobilesam.decoder.onnx", sha256: "b0735abf07c7affddf20fffc3ce750f44af387ee6a7323880e909389ed15d279"},
	}}
}

func (s *Store) Prepare(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.ready {
		s.mu.Unlock()
		return nil
	}
	if s.pending != nil {
		pending := s.pending
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pending.done:
			return pending.err
		}
	}
	s.pending = &preparation{done: make(chan struct{})}
	s.mu.Unlock()
	err := s.prepare(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.err = err
	s.ready = err == nil
	close(s.pending.done)
	s.pending = nil
	return err
}

func (s *Store) prepare(ctx context.Context) error {
	if err := os.MkdirAll(s.dir, cacheDirectoryMode); err != nil {
		return err
	}
	for _, model := range s.assets {
		if err := s.prepareAsset(ctx, model); err != nil {
			return fmt.Errorf("prepare %s: %w", model.name, err)
		}
	}
	return nil
}

func (s *Store) prepareAsset(ctx context.Context, model asset) error {
	digest, err := hex.DecodeString(model.sha256)
	if err != nil || len(digest) != sha256.Size || model.url == "" {
		return errors.New("object mask model manifest is unavailable")
	}
	path := filepath.Join(s.dir, model.name)
	if validAsset(ctx, path, model.sha256) {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, model.url, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model download HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxModelBytes {
		return errors.New("model download exceeds size limit")
	}
	tmp, err := os.CreateTemp(s.dir, ".model-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, maxModelBytes+1))
	if err != nil {
		return err
	}
	if written > maxModelBytes {
		return errors.New("model download exceeds size limit")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != model.sha256 {
		return errors.New("model checksum mismatch")
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func validAsset(ctx context.Context, path, digest string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxModelBytes {
		return false
	}
	hash := sha256.New()
	_, err = io.Copy(hash, io.LimitReader(f, maxModelBytes+1))
	return err == nil && ctx.Err() == nil && hex.EncodeToString(hash.Sum(nil)) == digest
}
