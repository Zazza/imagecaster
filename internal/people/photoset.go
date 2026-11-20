package people

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/Zazza/imagecaster/internal/imgformat"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

const (
	SetMaxSide       = 1536
	setThumbSide     = 256
	setJPEGQuality   = 92
	setMaxFileBytes  = 60 << 20
	setMaxDecodeSide = 12000
	setMaxPixels     = 60_000_000
	MaxSetPhotos     = MaxPhotos

	MinFaceHeight  = 96
	SmallFace      = 160
	MinSharpness   = 100
	SoftSharpness  = 300
	SameThreshold  = 0.35
	dupHamming     = 6
	frontalYaw     = 20
	halfProfileYaw = 50
)

var (
	ErrSetNoFace    = errors.New("на фото не найдено лицо")
	ErrSetSmall     = errors.New("лицо слишком мелкое: для обучения нужно, чтобы лицо было крупнее")
	ErrSetBlurry    = errors.New("лицо размыто: для обучения нужны чёткие фото")
	ErrSetDuplicate = errors.New("это фото уже есть в наборе")
	ErrSetFull      = fmt.Errorf("в наборе уже %d фото — больше не добавить", MaxSetPhotos)
	ErrSetNoName    = errors.New("не указано Имя человека")
	ErrSetFaceIndex = errors.New("выбранного лица на фото нет")
)

type SetStore interface {
	AddPersonPhoto(p *preset.PersonPhoto) error
	ListPersonPhotos(key string) ([]preset.PersonPhoto, error)
	GetPersonPhoto(id int64) (*preset.PersonPhoto, error)
	DeletePersonPhoto(id int64) error
	PersonPhotoKeys() ([]string, error)
}

type FaceAnalyzer interface {
	AnalyzeFaces(ctx context.Context, imageBase64 string, refs [][]float32) (*serverclient.FacesResult, error)
}

type PhotoSet struct {
	store SetStore
	faces FaceAnalyzer
	dir   string
	local func() bool
}

func NewPhotoSet(store SetStore, faces FaceAnalyzer, dir string, local func() bool) *PhotoSet {
	return &PhotoSet{store: store, faces: faces, dir: dir, local: local}
}

type SetFace struct {
	Box        [4]float64 `json:"box"`
	Similarity *float64   `json:"similarity,omitempty"`
}

type SetAddRequest struct {
	Path      string `json:"path"`
	Key       string `json:"key"`
	FaceIndex *int   `json:"face_index,omitempty"`
	Confirm   bool   `json:"confirm"`
}

type SetAddResult struct {
	Photo    *preset.PersonPhoto `json:"photo,omitempty"`
	Warnings []string            `json:"warnings,omitempty"`
	PickFace bool                `json:"pick_face,omitempty"`
	Confirm  string              `json:"confirm,omitempty"`
	Faces    []SetFace           `json:"faces,omitempty"`
	Preview  string              `json:"preview,omitempty"`
}

func NormalizeSetKey(key string) string {
	return strings.Join(strings.Fields(key), " ")
}

func YawClass(yaw float64) string {
	a := math.Abs(yaw)
	switch {
	case a < frontalYaw:
		return "front"
	case a < halfProfileYaw:
		return "half"
	}
	return "profile"
}

func (s *PhotoSet) Add(ctx context.Context, req SetAddRequest) (*SetAddResult, error) {
	key := NormalizeSetKey(req.Key)
	if key == "" {
		return nil, ErrSetNoName
	}
	if s.local != nil && !s.local() {
		return nil, ErrNotLocal
	}
	key, err := s.canonicalKey(key)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.ListPersonPhotos(key)
	if err != nil {
		return nil, err
	}
	if len(existing) >= MaxSetPhotos {
		return nil, ErrSetFull
	}
	img, err := loadSetImage(req.Path)
	if err != nil {
		return nil, err
	}
	img = fitSide(img, SetMaxSide)
	b64, err := encodeJPEGBase64(img)
	if err != nil {
		return nil, err
	}
	dh := dHash(img)
	for _, p := range existing {
		if bits.OnesCount64(p.DHash^dh) <= dupHamming {
			return nil, ErrSetDuplicate
		}
	}
	refs := make([][]float32, 0, len(existing))
	for _, p := range existing {
		refs = append(refs, p.Embedding)
	}
	res, err := s.faces.AnalyzeFaces(ctx, b64, refs)
	if err != nil {
		return nil, faceErr(err)
	}
	if len(res.Faces) == 0 {
		return nil, ErrSetNoFace
	}
	idx, pick := chooseFace(res.Faces, len(refs) > 0, req.FaceIndex)
	if pick {
		return &SetAddResult{PickFace: true, Faces: setFaces(res.Faces), Preview: b64}, nil
	}
	if idx < 0 || idx >= len(res.Faces) {
		return nil, ErrSetFaceIndex
	}
	f := res.Faces[idx]
	if f.HeightPx < MinFaceHeight {
		return nil, ErrSetSmall
	}
	if f.Sharpness < MinSharpness {
		return nil, ErrSetBlurry
	}
	if f.Similarity != nil && *f.Similarity < SameThreshold && !req.Confirm {
		return &SetAddResult{Confirm: fmt.Sprintf("лицо не похоже на уже собранные фото «%s» — точно это он(а)?", key), Faces: setFaces(res.Faces), Preview: b64}, nil
	}
	var warnings []string
	if f.HeightPx < SmallFace {
		warnings = append(warnings, "лицо небольшое — фото подойдёт, но крупные лучше")
	}
	if f.Sharpness < SoftSharpness {
		warnings = append(warnings, "лицо не очень резкое")
	}
	out := img
	if len(res.Faces) > 1 {
		crop := personCrop(f.Box, img.Bounds())
		out = cropImage(img, crop)
		if othersInside(res.Faces, idx, crop, img.Bounds()) {
			warnings = append(warnings, "рядом с человеком на фото есть кто-то ещё — фото подойдёт хуже")
		}
	}
	photo := &preset.PersonPhoto{
		PersonKey: key, SourceName: filepath.Base(req.Path),
		Width: out.Bounds().Dx(), Height: out.Bounds().Dy(),
		FaceHeight: f.HeightPx, Yaw: f.Yaw, Sharpness: f.Sharpness,
		Embedding: f.Embedding, DHash: dh,
	}
	if err := s.save(photo, out); err != nil {
		return nil, err
	}
	return &SetAddResult{Photo: photo, Warnings: warnings}, nil
}

func (s *PhotoSet) save(p *preset.PersonPhoto, img image.Image) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "p-*.jpg")
	if err != nil {
		return err
	}
	name := tmp.Name()
	werr := jpeg.Encode(tmp, img, &jpeg.Options{Quality: setJPEGQuality})
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(name)
		return werr
	}
	p.FileName = filepath.Base(name)
	if err := s.store.AddPersonPhoto(p); err != nil {
		os.Remove(name)
		return err
	}
	thumb := filepath.Join(s.dir, strconv.FormatInt(p.ID, 10)+".thumb.jpg")
	if err := writeThumb(thumb, img); err != nil {
		os.Remove(name)
		os.Remove(thumb)
		if derr := s.store.DeletePersonPhoto(p.ID); derr != nil {
			return fmt.Errorf("набор фото: %w (и не удалось убрать запись: %v)", err, derr)
		}
		return fmt.Errorf("набор фото: %w", err)
	}
	return nil
}

func (s *PhotoSet) canonicalKey(key string) (string, error) {
	keys, err := s.store.PersonPhotoKeys()
	if err != nil {
		return "", err
	}
	for _, k := range keys {
		if strings.EqualFold(k, key) {
			return k, nil
		}
	}
	return key, nil
}

func (s *PhotoSet) List(key string) ([]preset.PersonPhoto, error) {
	return s.store.ListPersonPhotos(strings.TrimSpace(key))
}

func (s *PhotoSet) Remove(id int64) error {
	p, err := s.store.GetPersonPhoto(id)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if err := s.store.DeletePersonPhoto(id); err != nil {
		return err
	}
	for _, f := range []string{p.FileName, strconv.FormatInt(id, 10) + ".thumb.jpg"} {
		if err := os.Remove(filepath.Join(s.dir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("набор фото: %w", err)
		}
	}
	return nil
}

func (s *PhotoSet) Thumb(id int64) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, strconv.FormatInt(id, 10)+".thumb.jpg"))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (s *PhotoSet) Folder(key string, tmpRoot string) (string, func(), error) {
	dir, _, cleanup, err := s.SplitFolder(key, tmpRoot, nil)
	return dir, cleanup, err
}

func (s *PhotoSet) SplitFolder(key, tmpRoot string, since *time.Time) (string, string, func(), error) {
	photos, err := s.List(key)
	if err != nil {
		return "", "", nil, err
	}
	root, err := os.MkdirTemp(tmpRoot, "set-*")
	if err != nil {
		return "", "", nil, err
	}
	cleanup := func() { os.RemoveAll(root) }
	oldDir, newDir := filepath.Join(root, "old"), ""
	if since != nil {
		newDir = filepath.Join(root, "new")
	}
	for _, d := range []string{oldDir, newDir} {
		if d == "" {
			continue
		}
		if err := os.Mkdir(d, 0o755); err != nil {
			cleanup()
			return "", "", nil, err
		}
	}
	for _, p := range photos {
		dst := oldDir
		if since != nil && AddedAfter(p.AddedAt, *since) {
			dst = newDir
		}
		src := filepath.Join(s.dir, p.FileName)
		to := filepath.Join(dst, p.FileName)
		if err := os.Link(src, to); err != nil {
			if cerr := copyFile(src, to); cerr != nil {
				cleanup()
				return "", "", nil, fmt.Errorf("набор фото: %w", cerr)
			}
		}
	}
	return oldDir, newDir, cleanup, nil
}

func AddedAfter(addedAt string, since time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, addedAt)
	return err == nil && t.After(since)
}

type SetSummary struct {
	Key     string   `json:"key"`
	Count   int      `json:"count"`
	Front   int      `json:"front"`
	Half    int      `json:"half"`
	Profile int      `json:"profile"`
	LastAdd string   `json:"last_added"`
	Added   []string `json:"added"`
}

func (s *PhotoSet) Summary() ([]SetSummary, error) {
	all, err := s.store.ListPersonPhotos("")
	if err != nil {
		return nil, err
	}
	var out []SetSummary
	idx := map[string]int{}
	for _, p := range all {
		i, ok := idx[p.PersonKey]
		if !ok {
			i = len(out)
			idx[p.PersonKey] = i
			out = append(out, SetSummary{Key: p.PersonKey})
		}
		sm := &out[i]
		sm.Count++
		switch YawClass(p.Yaw) {
		case "front":
			sm.Front++
		case "half":
			sm.Half++
		default:
			sm.Profile++
		}
		sm.Added = append(sm.Added, p.AddedAt)
		if p.AddedAt > sm.LastAdd {
			sm.LastAdd = p.AddedAt
		}
	}
	return out, nil
}

func chooseFace(faces []serverclient.FaceInfo, haveRefs bool, picked *int) (int, bool) {
	if picked != nil {
		return *picked, false
	}
	if len(faces) == 1 {
		return 0, false
	}
	if haveRefs {
		best, bestSim := -1, -2.0
		for i, f := range faces {
			if f.Similarity != nil && *f.Similarity > bestSim {
				best, bestSim = i, *f.Similarity
			}
		}
		if best >= 0 && bestSim >= SameThreshold {
			return best, false
		}
	}
	return -1, true
}

func setFaces(faces []serverclient.FaceInfo) []SetFace {
	out := make([]SetFace, len(faces))
	for i, f := range faces {
		out[i] = SetFace{Box: f.Box, Similarity: f.Similarity}
	}
	return out
}

func faceErr(err error) error {
	var apiErr *serverclient.TrainAPIError
	if errors.As(err, &apiErr) && apiErr.Code == "face_models_missing" {
		return ErrNoFaceModels
	}
	return fmt.Errorf("проверка фото на сервере: %w", err)
}

func personCrop(box [4]float64, b image.Rectangle) image.Rectangle {
	w, h := float64(b.Dx()), float64(b.Dy())
	fx0, fy0, fx1, fy1 := box[0]*w, box[1]*h, box[2]*w, box[3]*h
	fw, fh := fx1-fx0, fy1-fy0
	cx := (fx0 + fx1) / 2
	x0 := math.Max(0, cx-1.8*fw)
	x1 := math.Min(w, cx+1.8*fw)
	y0 := math.Max(0, fy0-0.8*fh)
	y1 := math.Min(h, fy0+7*fh)
	return image.Rect(int(x0), int(y0), int(x1), int(y1)).Add(b.Min).Intersect(b)
}

func othersInside(faces []serverclient.FaceInfo, keep int, crop, b image.Rectangle) bool {
	w, h := float64(b.Dx()), float64(b.Dy())
	for i, f := range faces {
		if i == keep {
			continue
		}
		r := image.Rect(int(f.Box[0]*w), int(f.Box[1]*h), int(f.Box[2]*w), int(f.Box[3]*h)).Add(b.Min)
		in := r.Intersect(crop)
		if !in.Empty() && in.Dx()*in.Dy()*2 >= r.Dx()*r.Dy() {
			return true
		}
	}
	return false
}

func cropImage(img image.Image, r image.Rectangle) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	xdraw.Copy(dst, image.Point{}, img, r, xdraw.Src, nil)
	return dst
}

func loadSetImage(path string) (image.Image, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s — не файл", filepath.Base(path))
	}
	if st.Size() > setMaxFileBytes {
		return nil, fmt.Errorf("файл слишком большой: %d МБ", st.Size()>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать фото: %w", err)
	}
	if max(cfg.Width, cfg.Height) > setMaxDecodeSide || cfg.Width*cfg.Height > setMaxPixels {
		return nil, fmt.Errorf("фото слишком большое: %dx%d", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать фото: %w", err)
	}
	return imgformat.ApplyOrientation(img, imgformat.JPEGOrientation(data)), nil
}

func fitSide(img image.Image, side int) image.Image {
	b := img.Bounds()
	long := max(b.Dx(), b.Dy())
	if long <= side {
		return img
	}
	w, h := max(1, b.Dx()*side/long), max(1, b.Dy()*side/long)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

func encodeJPEGBase64(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: setJPEGQuality}); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func writeThumb(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	werr := jpeg.Encode(f, fitSide(img, setThumbSide), &jpeg.Options{Quality: 85})
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

func dHash(img image.Image) uint64 {
	small := image.NewGray(image.Rect(0, 0, 9, 8))
	xdraw.ApproxBiLinear.Scale(small, small.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	var h uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			h <<= 1
			if small.GrayAt(x, y).Y > small.GrayAt(x+1, y).Y {
				h |= 1
			}
		}
	}
	return h
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
