package people

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

type fakeFaces struct {
	mu    sync.Mutex
	faces []serverclient.FaceInfo
	err   error
	calls int
	refs  [][][]float32
}

func (f *fakeFaces) AnalyzeFaces(_ context.Context, imageBase64 string, refs [][]float32) (*serverclient.FacesResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.refs = append(f.refs, refs)
	if f.err != nil {
		return nil, f.err
	}
	raw, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	out := make([]serverclient.FaceInfo, len(f.faces))
	for i, fc := range f.faces {
		out[i] = fc
		if len(refs) == 0 {
			out[i].Similarity = nil
		}
	}
	return &serverclient.FacesResult{Faces: out, Width: cfg.Width, Height: cfg.Height}, nil
}

func (f *fakeFaces) set(faces ...serverclient.FaceInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faces = faces
}

func (f *fakeFaces) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeFaces) lastRefs() [][]float32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.refs) == 0 {
		return nil
	}
	return f.refs[len(f.refs)-1]
}

func sim(v float64) *float64 { return &v }

func goodFace(emb ...float32) serverclient.FaceInfo {
	if len(emb) == 0 {
		emb = []float32{0.1, 0.2, 0.3}
	}
	return serverclient.FaceInfo{
		Box:        [4]float64{0.3, 0.2, 0.7, 0.8},
		HeightPx:   400,
		Score:      0.99,
		Yaw:        0,
		Sharpness:  800,
		Embedding:  emb,
		Similarity: sim(0.9),
	}
}

func patternImage(seed int64, w, h int) image.Image {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	const grid = 12
	cells := make([][grid]color.RGBA, grid)
	for y := 0; y < grid; y++ {
		for x := 0; x < grid; x++ {
			cells[y][x] = color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255}
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, cells[y*grid/h][x*grid/w])
		}
	}
	return img
}

func writeJPEG(t *testing.T, dir string, seed int64, w, h int) string {
	t.Helper()
	p := filepath.Join(dir, fmt.Sprintf("src-%d-%dx%d.jpg", seed, w, h))
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(f, patternImage(seed, w, h), &jpeg.Options{Quality: 92}))
	require.NoError(t, f.Close())
	return p
}

func writePNG(t *testing.T, dir string, seed int64, w, h int) string {
	t.Helper()
	p := filepath.Join(dir, fmt.Sprintf("src-%d-%dx%d.png", seed, w, h))
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, patternImage(seed, w, h)))
	require.NoError(t, f.Close())
	return p
}

type setEnv struct {
	set   *PhotoSet
	db    *preset.DB
	faces *fakeFaces
	dir   string
	src   string
	tmp   string
	local bool
}

func newSetEnv(t *testing.T) *setEnv {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	e := &setEnv{db: db, faces: &fakeFaces{}, dir: t.TempDir(), src: t.TempDir(), tmp: t.TempDir(), local: true}
	e.faces.set(goodFace())
	e.set = NewPhotoSet(db, e.faces, e.dir, func() bool { return e.local })
	return e
}

func (e *setEnv) add(t *testing.T, req SetAddRequest) (*SetAddResult, error) {
	t.Helper()
	return e.set.Add(context.Background(), req)
}

func (e *setEnv) mustAdd(t *testing.T, key string, seed int64) *SetAddResult {
	t.Helper()
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, seed, 640, 480), Key: key, Confirm: true})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Photo)
	return res
}

func (e *setEnv) count(t *testing.T, key string) int {
	t.Helper()
	list, err := e.set.List(key)
	require.NoError(t, err)
	return len(list)
}

func regularFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			out = append(out, p)
		}
		return nil
	}))
	return out
}

func decodeFile(t *testing.T, p string) (image.Config, string) {
	t.Helper()
	f, err := os.Open(p)
	require.NoError(t, err)
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	require.NoError(t, err)
	return cfg, format
}

func folderImages(t *testing.T, e *setEnv, key string) []string {
	t.Helper()
	dir, cleanup, err := e.set.Folder(key, e.tmp)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, en := range entries {
		out = append(out, filepath.Join(dir, en.Name()))
	}
	return out
}

func TestPhotoSetCond_EmptyKey(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	for _, key := range []string{"", "   "} {
		_, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 1, 640, 480), Key: key})
		assert.ErrorIs(t, err, ErrSetNoName, "key %q", key)
	}
	all, err := e.set.List("")
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestPhotoSetCond_CopiedDownscaledJPEGWithThumb(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	src := writePNG(t, e.src, 2, 2400, 1200)
	res, err := e.add(t, SetAddRequest{Path: src, Key: "Наташа"})
	require.NoError(t, err)
	require.NotNil(t, res.Photo)
	assert.Equal(t, "Наташа", res.Photo.PersonKey)
	assert.Positive(t, res.Photo.ID)

	require.NoError(t, os.Remove(src))

	imgs := folderImages(t, e, "Наташа")
	require.Len(t, imgs, 1)
	cfg, format := decodeFile(t, imgs[0])
	assert.Equal(t, "jpeg", format)
	assert.LessOrEqual(t, max(cfg.Width, cfg.Height), SetMaxSide)
	assert.Equal(t, SetMaxSide, cfg.Width, "long side downscaled to the limit, not further")
	assert.InDelta(t, 2.0, float64(cfg.Width)/float64(cfg.Height), 0.02, "aspect ratio kept")

	thumb, err := e.set.Thumb(res.Photo.ID)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(thumb)
	require.NoError(t, err, "thumb is base64")
	_, _, err = image.DecodeConfig(bytes.NewReader(raw))
	assert.NoError(t, err, "thumb decodes as an image")
}

func TestPhotoSetCond_SmallSourceNotUpscaled(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 3, 800, 600), Key: "A"})
	require.NoError(t, err)
	require.NotNil(t, res.Photo)
	imgs := folderImages(t, e, "A")
	require.Len(t, imgs, 1)
	cfg, _ := decodeFile(t, imgs[0])
	assert.Equal(t, 800, cfg.Width)
	assert.Equal(t, 600, cfg.Height)
}

func TestPhotoSetCond_Rejections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		faces   []serverclient.FaceInfo
		wantErr error
	}{
		{name: "no faces", faces: nil, wantErr: ErrSetNoFace},
		{name: "face too small", faces: []serverclient.FaceInfo{func() serverclient.FaceInfo { f := goodFace(); f.HeightPx = MinFaceHeight - 1; return f }()}, wantErr: ErrSetSmall},
		{name: "face blurry", faces: []serverclient.FaceInfo{func() serverclient.FaceInfo { f := goodFace(); f.Sharpness = MinSharpness - 1; return f }()}, wantErr: ErrSetBlurry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newSetEnv(t)
			e.faces.set(tt.faces...)
			res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 4, 640, 480), Key: "A", Confirm: true})
			assert.ErrorIs(t, err, tt.wantErr)
			if res != nil {
				assert.Nil(t, res.Photo)
			}
			assert.Equal(t, 0, e.count(t, "A"), "nothing saved")
			assert.Empty(t, regularFiles(t, e.dir), "no files left in the set dir")
		})
	}
}

func TestPhotoSetCond_BoundaryValuesAccepted(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	f := goodFace()
	f.HeightPx = MinFaceHeight
	f.Sharpness = MinSharpness
	e.faces.set(f)
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 5, 640, 480), Key: "A"})
	require.NoError(t, err)
	require.NotNil(t, res.Photo)
}

func TestPhotoSetCond_Warnings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		height   int
		sharp    float64
		wantWarn bool
	}{
		{name: "small face", height: SmallFace - 1, sharp: 800, wantWarn: true},
		{name: "soft face", height: 400, sharp: SoftSharpness - 1, wantWarn: true},
		{name: "good face", height: SmallFace, sharp: SoftSharpness, wantWarn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newSetEnv(t)
			f := goodFace()
			f.HeightPx = tt.height
			f.Sharpness = tt.sharp
			e.faces.set(f)
			res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 6, 640, 480), Key: "A"})
			require.NoError(t, err)
			require.NotNil(t, res.Photo, "photo is added")
			assert.Equal(t, 1, e.count(t, "A"))
			if tt.wantWarn {
				assert.NotEmpty(t, res.Warnings)
			} else {
				assert.Empty(t, res.Warnings)
			}
		})
	}
}

func TestPhotoSetCond_RefsAreThisPersonsEmbeddings(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.faces.set(goodFace(1, 0, 0))
	e.mustAdd(t, "A", 10)
	e.faces.set(goodFace(0, 1, 0))
	e.mustAdd(t, "B", 11)

	e.faces.set(goodFace(0, 0, 1))
	_, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 12, 640, 480), Key: "A", Confirm: true})
	require.NoError(t, err)
	assert.Equal(t, [][]float32{{1, 0, 0}}, e.faces.lastRefs(), "only A's photos are references")
}

func TestPhotoSetCond_FirstPhotoNoRefs(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.mustAdd(t, "B", 13)
	_, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 14, 640, 480), Key: "A"})
	require.NoError(t, err)
	assert.Empty(t, e.faces.lastRefs())
}

func TestPhotoSetCond_DifferentPersonNeedsConfirm(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.mustAdd(t, "A", 20)

	f := goodFace(9, 9, 9)
	f.Similarity = sim(SameThreshold - 0.1)
	e.faces.set(f)
	src := writeJPEG(t, e.src, 21, 640, 480)

	res, err := e.add(t, SetAddRequest{Path: src, Key: "A", Confirm: false})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Confirm, "question asked")
	assert.Nil(t, res.Photo)
	assert.Equal(t, 1, e.count(t, "A"), "nothing saved without confirmation")

	res, err = e.add(t, SetAddRequest{Path: src, Key: "A", Confirm: true})
	require.NoError(t, err)
	require.NotNil(t, res.Photo)
	assert.Empty(t, res.Confirm)
	assert.Equal(t, 2, e.count(t, "A"))
}

func TestPhotoSetCond_SamePersonNoConfirm(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.mustAdd(t, "A", 22)
	f := goodFace()
	f.Similarity = sim(SameThreshold + 0.01)
	e.faces.set(f)
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 23, 640, 480), Key: "A"})
	require.NoError(t, err)
	assert.Empty(t, res.Confirm)
	require.NotNil(t, res.Photo)
}

func TestPhotoSetCond_Duplicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		again func(t *testing.T, e *setEnv) string
	}{
		{name: "same file", again: func(t *testing.T, e *setEnv) string { return writeJPEG(t, e.src, 30, 640, 480) }},
		{name: "same picture re-encoded as png", again: func(t *testing.T, e *setEnv) string { return writePNG(t, e.src, 30, 640, 480) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newSetEnv(t)
			e.mustAdd(t, "A", 30)
			_, err := e.add(t, SetAddRequest{Path: tt.again(t, e), Key: "A", Confirm: true})
			assert.ErrorIs(t, err, ErrSetDuplicate)
			assert.Equal(t, 1, e.count(t, "A"))
		})
	}
}

func TestPhotoSetCond_DifferentPicturesNotDuplicates(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	for seed := int64(40); seed < 45; seed++ {
		e.mustAdd(t, "A", seed)
	}
	assert.Equal(t, 5, e.count(t, "A"))
}

func TestPhotoSetCond_Full(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	for i := 0; i < MaxSetPhotos; i++ {
		require.NoError(t, e.db.AddPersonPhoto(&preset.PersonPhoto{
			PersonKey: "A", FileName: fmt.Sprintf("x%d.jpg", i), Width: 10, Height: 10,
			FaceHeight: 300, Sharpness: 500, Embedding: []float32{0.1, 0.2, 0.3}, DHash: uint64(i*7919 + 1),
		}))
	}
	_, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 50, 640, 480), Key: "A", Confirm: true})
	assert.ErrorIs(t, err, ErrSetFull)
	assert.Equal(t, MaxSetPhotos, e.count(t, "A"))

	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 51, 640, 480), Key: "B", Confirm: true})
	require.NoError(t, err, "limit is per person")
	assert.NotNil(t, res.Photo)
}

func TestPhotoSetCond_NotLocal(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.local = false
	_, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 60, 640, 480), Key: "A", Confirm: true})
	assert.ErrorIs(t, err, ErrNotLocal)
	assert.Equal(t, 0, e.faces.callCount(), "analyzer not called")
	assert.Equal(t, 0, e.count(t, "A"))
}

func twoFaces(simA, simB *float64) []serverclient.FaceInfo {
	a := goodFace(1, 0, 0)
	a.Box = [4]float64{0.05, 0.1, 0.25, 0.45}
	a.Similarity = simA
	a.Yaw = 5
	b := goodFace(0, 1, 0)
	b.Box = [4]float64{0.7, 0.5, 0.9, 0.85}
	b.Similarity = simB
	b.Yaw = 40
	return []serverclient.FaceInfo{a, b}
}

func TestPhotoSetCond_SeveralFaces_AutoPickBySimilarity(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.mustAdd(t, "A", 70)
	e.faces.set(twoFaces(sim(0.2), sim(0.8))...)
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 71, 1200, 800), Key: "A"})
	require.NoError(t, err)
	assert.False(t, res.PickFace)
	require.NotNil(t, res.Photo)
	assert.InDelta(t, 40, res.Photo.Yaw, 0.001, "the most similar face is used")
	assert.Equal(t, []float32{0, 1, 0}, res.Photo.Embedding)
}

func TestPhotoSetCond_SeveralFaces_NoneSimilarEnough(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.mustAdd(t, "A", 72)
	e.faces.set(twoFaces(sim(0.1), sim(0.2))...)
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 73, 1200, 800), Key: "A"})
	require.NoError(t, err)
	assert.Nil(t, res.Photo, "no face is the person: not auto-picked")
	assert.Equal(t, 1, e.count(t, "A"))
}

func TestPhotoSetCond_SeveralFaces_PickThenSave(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.faces.set(twoFaces(nil, nil)...)
	src := writeJPEG(t, e.src, 74, 1200, 800)

	res, err := e.add(t, SetAddRequest{Path: src, Key: "A"})
	require.NoError(t, err)
	assert.True(t, res.PickFace)
	require.Len(t, res.Faces, 2)
	assert.Equal(t, [4]float64{0.05, 0.1, 0.25, 0.45}, res.Faces[0].Box)
	assert.Equal(t, [4]float64{0.7, 0.5, 0.9, 0.85}, res.Faces[1].Box)
	require.NotEmpty(t, res.Preview)
	_, err = base64.StdEncoding.DecodeString(res.Preview)
	assert.NoError(t, err, "preview is base64")
	assert.Nil(t, res.Photo)
	assert.Equal(t, 0, e.count(t, "A"), "nothing saved before the choice")

	idx := 1
	res, err = e.add(t, SetAddRequest{Path: src, Key: "A", FaceIndex: &idx})
	require.NoError(t, err)
	require.NotNil(t, res.Photo)
	assert.False(t, res.PickFace)
	assert.InDelta(t, 40, res.Photo.Yaw, 0.001)
	assert.Empty(t, res.Warnings, "the other face is far from the crop")

	imgs := folderImages(t, e, "A")
	require.Len(t, imgs, 1)
	cfg, _ := decodeFile(t, imgs[0])
	assert.Less(t, cfg.Width*cfg.Height, 1200*800, "cropped around the face")
	assert.True(t, cfg.Width < 1200 || cfg.Height < 800)
	assert.Equal(t, cfg.Width, res.Photo.Width)
	assert.Equal(t, cfg.Height, res.Photo.Height)
}

func TestPhotoSetCond_SeveralFaces_IndexOutOfRange(t *testing.T) {
	t.Parallel()
	for _, idx := range []int{2, -1} {
		t.Run(fmt.Sprint(idx), func(t *testing.T) {
			t.Parallel()
			e := newSetEnv(t)
			e.faces.set(twoFaces(nil, nil)...)
			i := idx
			_, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 75, 1200, 800), Key: "A", FaceIndex: &i})
			assert.ErrorIs(t, err, ErrSetFaceIndex)
			assert.Equal(t, 0, e.count(t, "A"))
		})
	}
}

func TestPhotoSetCond_SeveralFaces_OtherFaceInsideCropWarns(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	a := goodFace(1, 0, 0)
	a.Box = [4]float64{0.40, 0.30, 0.50, 0.50}
	b := goodFace(0, 1, 0)
	b.Box = [4]float64{0.52, 0.32, 0.60, 0.48}
	e.faces.set(a, b)
	idx := 0
	res, err := e.add(t, SetAddRequest{Path: writeJPEG(t, e.src, 76, 1200, 800), Key: "A", FaceIndex: &idx})
	require.NoError(t, err)
	require.NotNil(t, res.Photo)
	assert.NotEmpty(t, res.Warnings)
}

func TestPhotoSetCond_YawClass(t *testing.T) {
	t.Parallel()
	front, half, profile := YawClass(0), YawClass(30), YawClass(70)
	require.NotEqual(t, front, half)
	require.NotEqual(t, half, profile)
	require.NotEqual(t, front, profile)
	tests := []struct {
		yaw  float64
		want string
	}{
		{0, front}, {19.9, front}, {-19.9, front},
		{20, half}, {-20, half}, {49.9, half}, {-49.9, half},
		{50, profile}, {-50, profile}, {90, profile}, {-120, profile},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, YawClass(tt.yaw), "yaw %v", tt.yaw)
	}
}

func TestPhotoSetCond_Summary(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	sums, err := e.set.Summary()
	require.NoError(t, err)
	assert.Empty(t, sums)

	yaws := map[string][]float64{"A": {0, 10, -30, 75}, "B": {-60}}
	seed := int64(80)
	for key, ys := range yaws {
		for _, y := range ys {
			f := goodFace()
			f.Yaw = y
			e.faces.set(f)
			e.mustAdd(t, key, seed)
			seed++
		}
	}
	sums, err = e.set.Summary()
	require.NoError(t, err)
	byKey := map[string]SetSummary{}
	for _, s := range sums {
		byKey[s.Key] = s
	}
	require.Len(t, byKey, 2)
	assert.Equal(t, SetSummary{Key: "A", Count: 4, Front: 2, Half: 1, Profile: 1, LastAdd: byKey["A"].LastAdd, Added: byKey["A"].Added}, byKey["A"])
	assert.Len(t, byKey["A"].Added, 4)
	assert.Equal(t, SetSummary{Key: "B", Count: 1, Front: 0, Half: 0, Profile: 1, LastAdd: byKey["B"].LastAdd, Added: byKey["B"].Added}, byKey["B"])
}

func TestPhotoSetCond_RemoveDeletesRowAndFiles(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	res := e.mustAdd(t, "A", 90)
	keep := e.mustAdd(t, "A", 91)
	require.NotEmpty(t, regularFiles(t, e.dir))
	before := len(regularFiles(t, e.dir))

	require.NoError(t, e.set.Remove(res.Photo.ID))
	got, err := e.db.GetPersonPhoto(res.Photo.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "row deleted")
	after := regularFiles(t, e.dir)
	assert.Less(t, len(after), before, "removed photo files are gone")
	list, err := e.set.List("A")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, keep.Photo.ID, list[0].ID)

	require.NoError(t, e.set.Remove(keep.Photo.ID))
	assert.Empty(t, regularFiles(t, e.dir))
}

func TestPhotoSetCond_FolderHasExactlyPersonsPhotos(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.mustAdd(t, "A", 100)
	e.mustAdd(t, "A", 101)
	e.mustAdd(t, "B", 102)

	dir, cleanup, err := e.set.Folder("A", e.tmp)
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	for _, en := range entries {
		info, err := os.Lstat(filepath.Join(dir, en.Name()))
		require.NoError(t, err)
		assert.True(t, info.Mode().IsRegular(), "%s is a regular file", en.Name())
		_, _ = decodeFile(t, filepath.Join(dir, en.Name()))
	}
	cleanup()
	_, err = os.Stat(dir)
	assert.True(t, os.IsNotExist(err), "cleanup removes the folder")
	assert.Equal(t, 2, e.count(t, "A"), "set itself untouched")
	assert.Len(t, folderImages(t, e, "A"), 2)
}
