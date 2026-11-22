package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/importexport"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/sd"
)

type prFaces struct {
	calls int
	pts   []faceguard.Point
	err   error
}

func (f *prFaces) FaceLandmarks(string) ([]faceguard.Point, error) {
	f.calls++
	return f.pts, f.err
}

type prUpscaler struct {
	calls int
	got   string
	res   *generation.UpscaleResult
	err   error
}

func (u *prUpscaler) UpscaleLong(img string, target int) (*generation.UpscaleResult, error) {
	u.calls++
	u.got = img
	if target != post4KLong {
		return nil, fmt.Errorf("target %d", target)
	}
	return u.res, u.err
}

type prExporter struct {
	got []ExportImageParams
	err error
}

func (e *prExporter) ProcessExportImage(p ExportImageParams) (*importexport.ProcessedImage, error) {
	e.got = append(e.got, p)
	if e.err != nil {
		return nil, e.err
	}
	return &importexport.ProcessedImage{Data: []byte("img"), Filename: p.Filename, Format: p.Format}, nil
}

type prQueue struct {
	jobs   []queue.JobType
	params []ExportImageParams
	labels []string
}

func (q *prQueue) EnqueueLabeled(t queue.JobType, params any, source, label string) (int64, error) {
	q.jobs = append(q.jobs, t)
	q.params = append(q.params, params.(ExportImageParams))
	q.labels = append(q.labels, label)
	return int64(len(q.jobs)), nil
}

func prImage(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(1, 1, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestPostFaceLandmarks(t *testing.T) {
	t.Parallel()
	local := &config.Config{SDUrl: "http://192.168.1.184:8080"}
	remote := &config.Config{SDUrl: "http://8.8.8.8:7860"}
	pts := make([]faceguard.Point, 70)
	tests := []struct {
		name      string
		cfg       *config.Config
		faces     *prFaces
		wantSkip  string
		wantCalls int
		wantPts   int
		wantError bool
	}{
		{name: "local server returns points", cfg: local, faces: &prFaces{pts: pts}, wantCalls: 1, wantPts: 70},
		{name: "remote server is not asked", cfg: remote, faces: &prFaces{pts: pts}, wantSkip: postFaceNotLocal},
		{name: "no face", cfg: local, faces: &prFaces{err: sd.ErrNoFace}, wantSkip: postFaceNoFace, wantCalls: 1},
		{name: "too few points counts as no face", cfg: local, faces: &prFaces{err: fmt.Errorf("%w: 12 points", sd.ErrNoFace)}, wantSkip: postFaceNoFace, wantCalls: 1},
		{name: "server error is unavailable with a message", cfg: local, faces: &prFaces{err: errors.New("status 500")}, wantSkip: postFaceUnavailable, wantCalls: 1, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := postFaceLandmarks(tc.faces, tc.cfg, prImage(t))
			require.NoError(t, err)
			assert.Equal(t, tc.wantSkip, got.Skip)
			assert.Equal(t, tc.wantCalls, tc.faces.calls)
			assert.Len(t, got.Points, tc.wantPts)
			assert.NotNil(t, got.Points)
			assert.Equal(t, tc.wantError, got.Error != "")
		})
	}
}

func TestPostFaceLandmarks_RejectsEmptyImage(t *testing.T) {
	t.Parallel()
	faces := &prFaces{}
	_, err := postFaceLandmarks(faces, &config.Config{SDUrl: "http://127.0.0.1:1"}, "")
	require.Error(t, err)
	assert.Zero(t, faces.calls)
}

func TestEnqueuePostSave(t *testing.T) {
	t.Parallel()
	img := prImage(t)
	tests := []struct {
		name      string
		p         ExportImageParams
		pick      string
		pickErr   error
		wantJobs  int
		wantDir   string
		wantName  string
		wantErr   bool
		wantPicks int
		notLocal  bool
	}{
		{name: "remote server refused before the dialog", p: ExportImageParams{ImageBase64: img, Format: "png", Filename: "a.png"}, pick: "/out/b.png", notLocal: true, wantErr: true},
		{name: "dialog path becomes the target", p: ExportImageParams{ImageBase64: img, Format: "png", Filename: "a.png", Width: 100, Height: 80}, pick: "/out/dir/b.png", wantJobs: 1, wantDir: filepath.FromSlash("/out/dir"), wantName: "b.png", wantPicks: 1},
		{name: "cancelled dialog enqueues nothing", p: ExportImageParams{ImageBase64: img, Format: "png", Filename: "a.png"}, wantPicks: 1},
		{name: "dialog error is returned", p: ExportImageParams{ImageBase64: img, Format: "png", Filename: "a.png"}, pickErr: errors.New("boom"), wantErr: true, wantPicks: 1},
		{name: "batch folder skips the dialog", p: ExportImageParams{ImageBase64: img, Format: "jpeg", Filename: "c.jpeg", OutputDir: "/batch"}, wantJobs: 1, wantDir: "/batch", wantName: "c.jpeg"},
		{name: "empty image", p: ExportImageParams{Format: "png"}, wantErr: true},
		{name: "bad format", p: ExportImageParams{ImageBase64: img, Format: "gif"}, wantErr: true},
		{name: "bad retouch", p: ExportImageParams{ImageBase64: img, Format: "png", OutputDir: "/x", Retouch: &faceguard.Retouch{FaceWidth: 50}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &prQueue{}
			picks := 0
			pick := func(name, format string) (string, error) {
				picks++
				return tc.pick, tc.pickErr
			}
			id, err := enqueuePostSave(q, pick, tc.notLocal, tc.p)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantPicks, picks)
			require.Len(t, q.jobs, tc.wantJobs)
			if tc.wantJobs == 0 {
				assert.Zero(t, id)
				return
			}
			assert.Equal(t, int64(1), id)
			assert.Equal(t, queue.JobPostSave, q.jobs[0])
			got := q.params[0]
			assert.Equal(t, tc.wantDir, got.OutputDir)
			assert.Equal(t, tc.wantName, got.Filename)
			assert.Zero(t, got.Width)
			assert.Zero(t, got.Height)
			assert.Equal(t, postSaveLabel+tc.wantName, q.labels[0])
			assert.Equal(t, tc.wantPicks == 1, got.Overwrite, "a path confirmed in the save dialog is overwritten; a batch folder keeps unique names")
		})
	}
}

func TestRunPostSave(t *testing.T) {
	t.Parallel()
	retouch := &faceguard.Retouch{Teeth: true}
	pts := []faceguard.Point{{X: 0.5, Y: 0.5}}
	tests := []struct {
		name        string
		up          *prUpscaler
		wantImage   string
		wantUpscale string
	}{
		{name: "upscaled image goes to adjust and retouch", up: &prUpscaler{res: &generation.UpscaleResult{Image: "BIG", Upscaled: true}}, wantImage: "BIG", wantUpscale: "done"},
		{name: "no ESRGAN is marked as resized", up: &prUpscaler{res: &generation.UpscaleResult{Image: "BIG", Upscaled: true, Resized: true}}, wantImage: "BIG", wantUpscale: "resized"},
		{name: "already large keeps the source", up: &prUpscaler{res: &generation.UpscaleResult{Image: "SRC"}}, wantImage: "SRC", wantUpscale: "skipped"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ex := &prExporter{}
			adj := &AdjustParams{Brightness: 10, WhitePoint: 255, Gamma: 1}
			p := ExportImageParams{ImageBase64: "SRC", Format: "png", Filename: "out.png", OutputDir: dir, Width: 640, Height: 480, Adjust: adj, Retouch: retouch, FacePoints: pts}
			res, err := runPostSave(tc.up, ex, p, nil)
			require.NoError(t, err)
			assert.Equal(t, "SRC", tc.up.got)
			require.Len(t, ex.got, 1)
			got := ex.got[0]
			assert.Equal(t, tc.wantImage, got.ImageBase64)
			assert.Zero(t, got.Width)
			assert.Zero(t, got.Height)
			assert.Equal(t, adj, got.Adjust)
			assert.Equal(t, retouch, got.Retouch)
			assert.Equal(t, pts, got.FacePoints)
			assert.Equal(t, tc.wantUpscale, res.Upscale)
			assert.Equal(t, filepath.Join(dir, "out.png"), res.Path)
			data, err := os.ReadFile(res.Path)
			require.NoError(t, err)
			assert.Equal(t, "img", string(data))
		})
	}
}

func TestRunPostSave_Failures(t *testing.T) {
	t.Parallel()
	ok := &generation.UpscaleResult{Image: "BIG", Upscaled: true}
	tests := []struct {
		name     string
		p        ExportImageParams
		up       *prUpscaler
		ex       *prExporter
		stopped  bool
		wantUps  int
		wantStop bool
	}{
		{name: "no folder", p: ExportImageParams{ImageBase64: "SRC", Format: "png"}, up: &prUpscaler{res: ok}, ex: &prExporter{}},
		{name: "stopped before upscale", p: ExportImageParams{ImageBase64: "SRC", Format: "png", OutputDir: "/x"}, up: &prUpscaler{res: ok}, ex: &prExporter{}, stopped: true, wantStop: true},
		{name: "upscale error", p: ExportImageParams{ImageBase64: "SRC", Format: "png", OutputDir: "/x"}, up: &prUpscaler{err: errors.New("down")}, ex: &prExporter{}, wantUps: 1},
		{name: "export error", p: ExportImageParams{ImageBase64: "SRC", Format: "png", OutputDir: "/x"}, up: &prUpscaler{res: ok}, ex: &prExporter{err: errors.New("bad")}, wantUps: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runPostSave(tc.up, tc.ex, tc.p, func() bool { return tc.stopped })
			require.Error(t, err)
			assert.Equal(t, tc.wantUps, tc.up.calls)
			assert.Equal(t, tc.wantStop, generation.IsInterrupted(err))
		})
	}
}

func TestPostSaveHandler_NoGeneration(t *testing.T) {
	t.Parallel()
	_, err := postSaveHandler(&prExporter{}, nil)(nil, &queue.Job{Params: "{}"})
	require.Error(t, err)
}

func TestPostSaveHandler_RemoteServerRefused(t *testing.T) {
	t.Parallel()
	ex := &prExporter{}
	cfg := &config.Config{SDUrl: "https://sd.example.com"}
	_, err := postSaveHandler(ex, cfg)(&generation.Service{}, &queue.Job{Params: "{}"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не в локальной сети")
}

func TestRunPostSave_Overwrite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		overwrite bool
		wantName  string
	}{
		{name: "path chosen in the dialog is overwritten", overwrite: true, wantName: "a.png"},
		{name: "batch folder keeps the old file", overwrite: false, wantName: "a_1.png"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a.png"), []byte("old"), 0o644))
			up := &prUpscaler{res: &generation.UpscaleResult{}}
			res, err := runPostSave(up, &prExporter{}, ExportImageParams{ImageBase64: "x", Format: "png", Filename: "a.png", OutputDir: dir, Overwrite: tc.overwrite}, nil)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, tc.wantName), res.Path)
			data, err := os.ReadFile(res.Path)
			require.NoError(t, err)
			assert.Equal(t, "img", string(data))
		})
	}
}
