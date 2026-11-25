package stylisttrace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type StorageStats struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Bytes   int64  `json:"bytes"`
	Frames  int    `json:"frames"`
	Active  int    `json:"active"`
	Orphans int    `json:"orphans"`
}

func storageRoot(root string) (string, bool, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return root, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false, errors.New("invalid diagnostics directory")
	}
	return root, true, nil
}

func Storage(root string) (*StorageStats, error) {
	registry.Lock()
	defer registry.Unlock()
	root, exists, err := storageRoot(root)
	if err != nil {
		return nil, err
	}
	stats := &StorageStats{Path: root, Exists: exists, Active: registry.active[root]}
	if !exists {
		return stats, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	incomplete := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "frame-") {
			continue
		}
		stats.Frames++
		dir := filepath.Join(root, entry.Name())
		meta, err := os.Lstat(filepath.Join(dir, metadataFile))
		if errors.Is(err, os.ErrNotExist) {
			incomplete++
		} else if err != nil {
			return nil, err
		} else if !meta.Mode().IsRegular() {
			return nil, errors.New("invalid diagnostics metadata")
		}
		err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("diagnostics contains symbolic link")
			}
			if !entry.IsDir() {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				stats.Bytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	stats.Orphans = incomplete - stats.Active
	if stats.Orphans < 0 {
		stats.Orphans = 0
	}
	return stats, nil
}

func ownedDiagnosticFile(name string) bool {
	if name == metadataFile {
		return true
	}
	for _, stage := range []Stage{Base, Garment, FacePass, FaceSwap} {
		for _, extension := range []string{"png", "jpeg", "webp"} {
			if name == string(stage)+"."+extension {
				return true
			}
		}
	}
	return false
}

func Clear(root string, confirmed bool) error {
	if !confirmed {
		return errors.New("diagnostics deletion requires confirmation")
	}
	registry.Lock()
	defer registry.Unlock()
	root, exists, err := storageRoot(root)
	if err != nil {
		return err
	}
	if registry.active[root] > 0 {
		return errors.New("diagnostics recording is active")
	}
	if !exists {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var dirs []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "frame-") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("diagnostics contains symbolic link")
		}
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if err := validateFrame(dir); err != nil {
			return err
		}
		dirs = append(dirs, dir)
	}
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

func ExistingDirectory(root string) (string, error) {
	registry.Lock()
	defer registry.Unlock()
	path, exists, err := storageRoot(root)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", os.ErrNotExist
	}
	return path, nil
}

func validateFrame(dir string) error {
	children, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, child := range children {
		info, err := child.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !ownedDiagnosticFile(child.Name()) {
			return errors.New("diagnostics contains unrecognized files")
		}
	}
	return nil
}
