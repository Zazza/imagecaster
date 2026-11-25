package stylisttrace

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/Zazza/imagecaster/internal/config"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "golang.org/x/image/webp"
)

const (
	Directory      = "stylist-diagnostics"
	completedLimit = config.DefaultDiagnosticsRetention
	activeLimit    = 4
	imageByteLimit = 16 << 20
	directoryMode  = 0700
	fileMode       = 0600
	metadataFile   = "metadata.json"
)

type Stage string

const (
	Base     Stage = "base"
	Garment  Stage = "garment"
	FacePass Stage = "face-pass"
	FaceSwap Stage = "face-swap"
)

type stageMeta struct {
	Stage  Stage `json:"stage"`
	Width  int   `json:"width"`
	Height int   `json:"height"`
}
type Recorder struct {
	root, dir        string
	stages           []stageMeta
	finished, closed bool
	options          Options
}

var registry = struct {
	sync.Mutex
	active map[string]int
}{active: make(map[string]int)}

type Options struct {
	DebugImages, KeepOriginals bool
	Retention                  int
}

func New(root string) (*Recorder, error) {
	return NewWithOptions(root, Options{DebugImages: true, KeepOriginals: true, Retention: completedLimit})
}
func NewWithOptions(root string, options Options) (*Recorder, error) {
	if !options.DebugImages && !options.KeepOriginals {
		return nil, nil
	}
	if options.Retention < config.MinDiagnosticsRetention || options.Retention > config.MaxDiagnosticsRetention {
		options.Retention = completedLimit
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	registry.Lock()
	defer registry.Unlock()
	if registry.active[root] >= activeLimit {
		return nil, errors.New("stylist diagnostics active limit reached")
	}
	if err := os.MkdirAll(root, directoryMode); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid stylist diagnostics directory")
	}
	if err := os.Chmod(root, directoryMode); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, "frame-")
	if err != nil {
		return nil, err
	}
	registry.active[root]++
	return &Recorder{root: root, dir: dir, options: options}, nil
}
func (r *Recorder) Save(stage Stage, encoded string) error {
	if r == nil {
		return nil
	}
	registry.Lock()
	defer registry.Unlock()
	if r.closed || r.finished {
		return errors.New("stylist diagnostics recorder closed")
	}
	switch stage {
	case Base, Garment, FacePass, FaceSwap:
	default:
		return errors.New("invalid stylist diagnostics stage")
	}
	if !r.options.DebugImages && stage == FaceSwap {
		return nil
	}
	for _, s := range r.stages {
		if s.Stage == stage {
			return errors.New("stylist diagnostics stage already saved")
		}
	}
	if strings.HasPrefix(encoded, "data:image/") {
		_, encoded, _ = strings.Cut(encoded, ",")
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(imageByteLimit) {
		return errors.New("stylist diagnostics image exceeds limit")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) > imageByteLimit {
		return errors.New("invalid stylist diagnostics image encoding")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return errors.New("invalid stylist diagnostics image")
	}
	switch format {
	case "png", "jpeg", "webp":
	default:
		return errors.New("unsupported stylist diagnostics image format")
	}
	if err := os.WriteFile(filepath.Join(r.dir, string(stage)+"."+format), raw, fileMode); err != nil {
		return err
	}
	r.stages = append(r.stages, stageMeta{stage, cfg.Width, cfg.Height})
	return nil
}
func (r *Recorder) Finish(itemID int64) error {
	if r == nil {
		return nil
	}
	registry.Lock()
	defer registry.Unlock()
	if r.closed || r.finished {
		return errors.New("stylist diagnostics recorder closed")
	}
	if itemID <= 0 || len(r.stages) == 0 {
		return errors.New("stylist diagnostics requires saved frame")
	}
	raw, err := json.Marshal(struct {
		FrameID int64       `json:"frame_id"`
		Stages  []stageMeta `json:"stages"`
	}{itemID, r.stages})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.dir, metadataFile), raw, fileMode); err != nil {
		return err
	}
	r.finished = true
	registry.active[r.root]--
	return prune(r.root, r.dir, r.options.Retention)
}
func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	registry.Lock()
	defer registry.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.finished {
		return nil
	}
	registry.active[r.root]--
	return os.RemoveAll(r.dir)
}
func prune(root, newest string, retention int) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type completed struct {
		path string
		info os.FileInfo
	}
	var dirs []completed
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "frame-") {
			continue
		}
		path := filepath.Join(root, e.Name())
		if err := validateFrame(path); err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(path, metadataFile))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		dirs = append(dirs, completed{path, info})
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].path == newest {
			return true
		}
		if dirs[j].path == newest {
			return false
		}
		if dirs[i].info.ModTime().Equal(dirs[j].info.ModTime()) {
			return dirs[i].path < dirs[j].path
		}
		return dirs[i].info.ModTime().After(dirs[j].info.ModTime())
	})
	if len(dirs) <= retention {
		return nil
	}
	for _, d := range dirs[retention:] {
		if err := os.RemoveAll(d.path); err != nil {
			return err
		}
	}
	return nil
}
