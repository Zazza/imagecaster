package stylisttrace

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func ReadFrame(root string, itemID int64, stage string) (string, error) {
	if itemID <= 0 || (stage != "base" && stage != "before_face") {
		return "", errors.New("неверный исходник кадра")
	}
	registry.Lock()
	defer registry.Unlock()
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return "", errors.New("исходник не сохранён или уже удалён")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "frame-") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		raw, err := readPrivateFile(filepath.Join(dir, metadataFile))
		if err != nil {
			continue
		}
		var meta struct {
			FrameID int64       `json:"frame_id"`
			Stages  []stageMeta `json:"stages"`
		}
		if json.Unmarshal(raw, &meta) != nil || meta.FrameID != itemID {
			continue
		}
		candidates := []Stage{Base}
		if stage == "before_face" {
			candidates = []Stage{FacePass, Garment, Base}
		}
		for _, candidate := range candidates {
			for _, saved := range meta.Stages {
				if saved.Stage != candidate {
					continue
				}
				for _, format := range []string{"png", "jpeg", "webp"} {
					data, err := readPrivateFile(filepath.Join(dir, string(candidate)+"."+format))
					if err == nil {
						return base64.StdEncoding.EncodeToString(data), nil
					}
				}
			}
		}
	}
	return "", errors.New("исходник не сохранён или уже удалён; восстановление недоступно")
}
func readPrivateFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) || info.Size() > imageByteLimit {
		return nil, errors.New("invalid diagnostic file")
	}
	return os.ReadFile(path)
}
