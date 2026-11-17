package people

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

type fakeAPI struct {
	mu          sync.Mutex
	submits     []serverclient.TrainRequest
	jobsClients []string
	cancels     []string
	peopleCalls int
	deletes     []string

	submitJob *serverclient.TrainJob
	submitErr error
	jobs      []serverclient.TrainJob
	people    []serverclient.Person
	cancelJob *serverclient.TrainJob
	deleteErr error

	basesCalls int
	bases      []string
	basesErr   error
}

func (f *fakeAPI) SubmitTraining(_ context.Context, req serverclient.TrainRequest) (*serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	req.Photos = append([]string(nil), req.Photos...)
	f.submits = append(f.submits, req)
	return f.submitJob, f.submitErr
}

func (f *fakeAPI) TrainingJobs(_ context.Context, client string) ([]serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobsClients = append(f.jobsClients, client)
	return f.jobs, nil
}

func (f *fakeAPI) CancelTraining(_ context.Context, id string) (*serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, id)
	return f.cancelJob, nil
}

func (f *fakeAPI) People(_ context.Context) ([]serverclient.Person, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peopleCalls++
	return f.people, nil
}

func (f *fakeAPI) DeletePerson(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, name)
	return f.deleteErr
}

func (f *fakeAPI) TrainingBases(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.basesCalls++
	return f.bases, f.basesErr
}

func (f *fakeAPI) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.submits) + len(f.jobsClients) + len(f.cancels) + f.peopleCalls + len(f.deletes) + f.basesCalls
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy", "REQUEST_METHOD"} {
		t.Setenv(k, "")
	}
}

func testImage(seed int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: uint8(seed * 11), G: uint8(x * 50), B: uint8(y * 50), A: 255})
		}
	}
	return img
}

func pngBytes(t *testing.T, seed int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, testImage(seed)))
	return buf.Bytes()
}

func jpegBytes(t *testing.T, seed int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, testImage(seed), nil))
	return buf.Bytes()
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func makePhotos(t *testing.T, dir string, n int) []string {
	t.Helper()
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		var p string
		if i%2 == 0 {
			p = filepath.Join(dir, fmt.Sprintf("p%03d.jpg", i))
			writeFile(t, p, jpegBytes(t, i))
		} else {
			p = filepath.Join(dir, fmt.Sprintf("p%03d.png", i))
			writeFile(t, p, pngBytes(t, i))
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func TestCollectPhotos_Count(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		n       int
		wantErr bool
	}{
		{name: "empty folder", n: 0, wantErr: true},
		{name: "9 photos", n: 9, wantErr: true},
		{name: "10 photos", n: 10},
		{name: "50 photos", n: 50},
		{name: "51 photos", n: 51, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			want := makePhotos(t, dir, tt.n)

			got, err := CollectPhotos(dir)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrPhotoCount), "want ErrPhotoCount, got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestCollectPhotos_FiltersAndSorts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var want []string
	for i := 11; i >= 0; i-- {
		var p string
		switch {
		case i == 3:
			p = filepath.Join(dir, fmt.Sprintf("p%02d.JPG", i))
			writeFile(t, p, jpegBytes(t, i))
		case i%2 == 0:
			p = filepath.Join(dir, fmt.Sprintf("p%02d.jpg", i))
			writeFile(t, p, jpegBytes(t, i))
		default:
			p = filepath.Join(dir, fmt.Sprintf("p%02d.png", i))
			writeFile(t, p, pngBytes(t, i))
		}
		want = append(want, p)
	}
	sort.Strings(want)
	writeFile(t, filepath.Join(dir, "notes.txt"), []byte("caption"))
	writeFile(t, filepath.Join(dir, "p05.txt"), []byte("caption"))
	for i := 0; i < 5; i++ {
		writeFile(t, filepath.Join(dir, "sub", fmt.Sprintf("s%02d.jpg", i)), jpegBytes(t, 100+i))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "folder.jpg"), 0o755))

	got, err := CollectPhotos(dir)

	require.NoError(t, err)
	require.Len(t, got, 12)
	assert.Equal(t, want, got)
	assert.Contains(t, got, filepath.Join(dir, "p03.JPG"))
	for _, p := range got {
		assert.NotContains(t, p, string(filepath.Separator)+"sub"+string(filepath.Separator))
	}
}

func TestCollectPhotos_AllExtensionsAnyCase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exts := []string{".jpg", ".JPG", ".jpeg", ".JPEG", ".Jpeg", ".png", ".PNG", ".webp", ".WEBP", ".WebP"}
	var want []string
	for i, ext := range exts {
		p := filepath.Join(dir, fmt.Sprintf("p%02d%s", i, ext))
		switch strings.ToLower(ext) {
		case ".png", ".webp":
			writeFile(t, p, pngBytes(t, i))
		default:
			writeFile(t, p, jpegBytes(t, i))
		}
		want = append(want, p)
	}
	writeFile(t, filepath.Join(dir, "p99.gif"), []byte("GIF89a"))
	writeFile(t, filepath.Join(dir, "p98.jpg.txt"), []byte("x"))
	sort.Strings(want)

	got, err := CollectPhotos(dir)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestCollectPhotos_SubfolderPhotosNotCounted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makePhotos(t, dir, 9)
	makePhotos(t, filepath.Join(dir, "more"), 5)

	_, err := CollectPhotos(dir)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPhotoCount), "want ErrPhotoCount, got %v", err)
}

func TestCollectPhotos_MissingFolder(t *testing.T) {
	t.Parallel()

	got, err := CollectPhotos(filepath.Join(t.TempDir(), "absent"))

	require.Error(t, err)
	assert.Empty(t, got)
}

func TestTrain_NotLocalServer(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		httpProxy string
	}{
		{name: "public address", serverURL: "http://8.8.8.8:8080"},
		{name: "empty address", serverURL: ""},
		{name: "not http", serverURL: "ftp://192.168.1.184"},
		{name: "garbage", serverURL: "not a url"},
		{name: "local address via proxy", serverURL: "http://192.168.1.184:8080", httpProxy: "http://10.0.0.1:3128"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			if tt.httpProxy != "" {
				t.Setenv("HTTP_PROXY", tt.httpProxy)
				t.Setenv("http_proxy", tt.httpProxy)
			}
			dir := t.TempDir()
			makePhotos(t, dir, 12)
			api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}
			s := New(api, func() string { return tt.serverURL }, func() string { return "inst-1" })

			job, err := s.Train(context.Background(), dir, "anna", "woman", "Z-Image")

			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrNotLocal), "want ErrNotLocal, got %v", err)
			assert.Nil(t, job)
			assert.Zero(t, api.totalCalls())
		})
	}
}

func TestTrain_NotLocalCheckedBeforePhotos(t *testing.T) {
	clearProxyEnv(t)
	api := &fakeAPI{}
	s := New(api, func() string { return "http://8.8.8.8:8080" }, func() string { return "inst-1" })

	_, err := s.Train(context.Background(), filepath.Join(t.TempDir(), "absent"), "anna", "woman", "Z-Image")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotLocal), "want ErrNotLocal, got %v", err)
	assert.Zero(t, api.totalCalls())
}

func TestTrain_TooFewPhotos(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 9)
	api := &fakeAPI{}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	job, err := s.Train(context.Background(), dir, "anna", "woman", "Z-Image")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPhotoCount), "want ErrPhotoCount, got %v", err)
	assert.Nil(t, job)
	assert.Zero(t, api.totalCalls())
}

func TestTrain_SubmitsRequest(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	want := makePhotos(t, dir, 12)
	writeFile(t, filepath.Join(dir, "readme.txt"), []byte("x"))
	api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1", Status: "queued"}}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	job, err := s.Train(context.Background(), dir, "anna", "woman", "Z-Image")

	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "j1", job.ID)
	require.Len(t, api.submits, 1)
	req := api.submits[0]
	assert.Equal(t, "anna", req.Name)
	assert.Equal(t, "woman", req.Gender)
	assert.Equal(t, "inst-1", req.Client)
	assert.Equal(t, "inst-1:train:anna", req.ClientRef)
	assert.Equal(t, want, req.Photos)
}

func TestTrain_ClientRefStableAndLowercase(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 12)
	api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}
	s := New(api, func() string { return "http://localhost:8080" }, func() string { return "inst-7" })

	_, err := s.Train(context.Background(), dir, "Anna", "woman", "Z-Image")
	require.NoError(t, err)
	_, err = s.Train(context.Background(), dir, "Anna", "woman", "Z-Image")
	require.NoError(t, err)
	_, err = s.Train(context.Background(), dir, "anna", "woman", "Z-Image")
	require.NoError(t, err)

	require.Len(t, api.submits, 3)
	for _, req := range api.submits {
		assert.Equal(t, "inst-7:train:anna", req.ClientRef)
		assert.Equal(t, "inst-7", req.Client)
	}
}

func TestTrain_DifferentNamesDifferentClientRef(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 12)
	api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}
	s := New(api, func() string { return "http://127.0.0.1:8080" }, func() string { return "inst-1" })

	_, err := s.Train(context.Background(), dir, "anna", "woman", "Z-Image")
	require.NoError(t, err)
	_, err = s.Train(context.Background(), dir, "boris", "man", "Z-Image")
	require.NoError(t, err)

	require.Len(t, api.submits, 2)
	assert.Equal(t, "inst-1:train:anna", api.submits[0].ClientRef)
	assert.Equal(t, "inst-1:train:boris", api.submits[1].ClientRef)
	assert.Equal(t, "man", api.submits[1].Gender)
}

func TestTrain_SubmitErrorReturned(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 12)
	apiErr := &serverclient.TrainAPIError{Status: 409, Code: "name_taken", Message: "taken"}
	api := &fakeAPI{submitErr: apiErr}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	job, err := s.Train(context.Background(), dir, "anna", "woman", "Z-Image")

	require.Error(t, err)
	assert.Nil(t, job)
	var got *serverclient.TrainAPIError
	require.True(t, errors.As(err, &got))
	assert.Equal(t, "name_taken", got.Code)
}

func TestJobs_UsesInstallID(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{jobs: []serverclient.TrainJob{{ID: "j1", Status: "running", Progress: serverclient.TrainProgress{Step: 5, Steps: 10}}}}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-42" })

	jobs, err := s.Jobs(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"inst-42"}, api.jobsClients)
	require.Len(t, jobs, 1)
	assert.Equal(t, "j1", jobs[0].ID)
	assert.Equal(t, 5, jobs[0].Progress.Step)
}

func TestJobs_FreshServiceSeesServerJobs(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{jobs: []serverclient.TrainJob{{ID: "j9", Status: "running"}}}
	first := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-42" })
	_, err := first.Jobs(context.Background())
	require.NoError(t, err)

	second := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-42" })
	jobs, err := second.Jobs(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"inst-42", "inst-42"}, api.jobsClients)
	require.Len(t, jobs, 1)
	assert.Equal(t, "j9", jobs[0].ID)
}

func TestPassThrough(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{
		people:    []serverclient.Person{{Name: "anna", Trigger: "anna_woman", Base: "Z-Image", Lora: "anna"}},
		cancelJob: &serverclient.TrainJob{ID: "j1", Status: "cancelled"},
	}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	job, err := s.Cancel(context.Background(), "j1")
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "cancelled", job.Status)
	assert.Equal(t, []string{"j1"}, api.cancels)

	people, err := s.List(context.Background())
	require.NoError(t, err)
	require.Len(t, people, 1)
	assert.Equal(t, "anna_woman", people[0].Trigger)

	require.NoError(t, s.Delete(context.Background(), "anna"))
	assert.Equal(t, []string{"anna"}, api.deletes)
}

func TestDelete_ErrorReturned(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{deleteErr: &serverclient.TrainAPIError{Status: 409, Code: "busy", Message: "training"}}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	err := s.Delete(context.Background(), "anna")

	var got *serverclient.TrainAPIError
	require.True(t, errors.As(err, &got))
	assert.Equal(t, "busy", got.Code)
}
