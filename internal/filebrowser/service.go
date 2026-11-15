package filebrowser

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/imgformat"
)

var imageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
}

type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func BrowseDirectory(dirPath string) ([]FileEntry, error) {
	if dirPath == "" {
		return []FileEntry{}, nil
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}
	result := []FileEntry{}
	for _, e := range entries {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if e.IsDir() {
			entry := FileEntry{
				Name:  name,
				Path:  filepath.Join(dirPath, name),
				IsDir: true,
			}
			if info, err := e.Info(); err == nil {
				entry.ModTime = info.ModTime().Format("2006-01-02 15:04")
			}
			result = append(result, entry)
			continue
		}
		if !imageExts[ext] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, FileEntry{
			Name:    name,
			Path:    filepath.Join(dirPath, name),
			IsDir:   false,
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04"),
		})
	}
	return result, nil
}

func ReadFileAsBase64(filePath string) (string, error) {
	if filePath == "" {
		return "", nil
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if !imageExts[ext] {
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if info.Size() > config.MaxImageBytes {
		return "", config.ImageTooLarge("image")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func ReadThumbnail(filePath string) (string, error) {
	return (&ThumbCache{}).Read(filePath)
}

type ThumbCache struct {
	dir string
}

func NewThumbCache(dir string) *ThumbCache {
	return &ThumbCache{dir: dir}
}

func (c *ThumbCache) Read(filePath string) (string, error) {
	if filePath == "" {
		return "", nil
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if !imageExts[ext] {
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if info.Size() > config.MaxImageBytes {
		return "", config.ImageTooLarge("image")
	}
	cachePath, cacheGlob := c.entry(filePath, info, ext)
	if cachePath != "" {
		if data, err := os.ReadFile(cachePath); err == nil {
			return base64.StdEncoding.EncodeToString(data), nil
		} else if !os.IsNotExist(err) {
			log.Printf("[filebrowser] read thumb cache %s: %v", cachePath, err)
		}
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	thumb, resized, err := makeThumbnail(data, ext)
	if err != nil {
		return "", err
	}
	if cachePath != "" && resized {
		c.store(cachePath, cacheGlob, thumb)
	}
	return base64.StdEncoding.EncodeToString(thumb), nil
}

func (c *ThumbCache) entry(filePath string, info os.FileInfo, ext string) (string, string) {
	if c == nil || c.dir == "" {
		return "", ""
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		log.Printf("[filebrowser] thumb cache path %s: %v", filePath, err)
		return "", ""
	}
	pathSum := sha256.Sum256([]byte(abs))
	prefix := hex.EncodeToString(pathSum[:12])
	stamp := strconv.FormatInt(info.Size(), 36) + "-" + strconv.FormatInt(info.ModTime().UnixNano(), 36)
	outExt := ".png"
	if ext == ".jpg" || ext == ".jpeg" {
		outExt = ".jpg"
	}
	return filepath.Join(c.dir, prefix+"_"+stamp+outExt), filepath.Join(c.dir, prefix+"_*")
}

func (c *ThumbCache) store(cachePath, cacheGlob string, data []byte) {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		log.Printf("[filebrowser] thumb cache dir: %v", err)
		return
	}
	if stale, err := filepath.Glob(cacheGlob); err == nil {
		for _, old := range stale {
			if old == cachePath {
				continue
			}
			if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
				log.Printf("[filebrowser] remove stale thumb %s: %v", old, err)
			}
		}
	}
	tmp, err := os.CreateTemp(c.dir, ".thumb-*")
	if err != nil {
		log.Printf("[filebrowser] thumb cache temp: %v", err)
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), cachePath)
	}
	if werr != nil {
		log.Printf("[filebrowser] thumb cache write %s: %v", cachePath, werr)
		if err := os.Remove(tmp.Name()); err != nil && !os.IsNotExist(err) {
			log.Printf("[filebrowser] remove thumb temp: %v", err)
		}
	}
}

func makeThumbnail(data []byte, ext string) ([]byte, bool, error) {
	const thumbSize = 256
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, false, fmt.Errorf("decode image: %w", err)
	}
	if cfg.Width <= thumbSize && cfg.Height <= thumbSize {
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return nil, false, fmt.Errorf("decode image: %w", err)
		}
		return data, false, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, fmt.Errorf("decode image: %w", err)
	}
	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()
	ratio := math.Min(float64(thumbSize)/float64(origW), float64(thumbSize)/float64(origH))
	tw := int(float64(origW) * ratio)
	th := int(float64(origH) * ratio)
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
	thumb := imgformat.ApplyOrientation(dst, imgformat.JPEGOrientation(data))
	var buf bytes.Buffer
	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 80})
	default:
		err = png.Encode(&buf, thumb)
	}
	if err != nil {
		return nil, false, fmt.Errorf("encode thumbnail: %w", err)
	}
	return buf.Bytes(), true, nil
}

func DecodeImageSize(b64 string) (int, int) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func SanitizeFilename(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	result := r.Replace(b.String())
	if len(result) > 200 {
		result = result[:200]
		for !utf8.ValidString(result) {
			result = result[:len(result)-1]
		}
	}
	return result
}
