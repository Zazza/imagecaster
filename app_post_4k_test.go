package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/queue"
)

type p4kQueue struct {
	jobs    []queue.JobType
	params  []any
	sources []string
	labels  []string
}

func (q *p4kQueue) EnqueueLabeled(t queue.JobType, params any, source, label string) (int64, error) {
	q.jobs = append(q.jobs, t)
	q.params = append(q.params, params)
	q.sources = append(q.sources, source)
	q.labels = append(q.labels, label)
	return int64(len(q.jobs)) + 40, nil
}

type p4kUpscaler struct {
	calls   int
	got     string
	targets []int
	res     *generation.UpscaleResult
	err     error
}

func (u *p4kUpscaler) UpscaleLong(img string, target int) (*generation.UpscaleResult, error) {
	u.calls++
	u.got = img
	u.targets = append(u.targets, target)
	return u.res, u.err
}

func p4kParamImage(t *testing.T, params any) string {
	t.Helper()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	v, ok := m["image_base64"].(string)
	require.True(t, ok, "params must carry image_base64: %s", raw)
	return v
}

func TestJobUpscale4KType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, queue.JobType("upscale_4k"), queue.JobUpscale4K)
}

func TestEnqueuePost4K(t *testing.T) {
	t.Parallel()
	img := prImage(t)
	tests := []struct {
		name     string
		image    string
		notLocal bool
		wantErr  error
		anyErr   bool
		wantJobs int
	}{
		{name: "local server enqueues an upscale_4k job", image: img, wantJobs: 1},
		{name: "empty image is refused", image: "", anyErr: true},
		{name: "too large image is refused", image: strings.Repeat("A", config.MaxImageBase64Len+1), anyErr: true},
		{name: "remote server is refused before the queue", image: img, notLocal: true, wantErr: errPost4KNotLocal},
		{name: "remote server with empty image enqueues nothing", image: "", notLocal: true, anyErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &p4kQueue{}
			id, err := enqueuePost4K(q, tc.notLocal, tc.image)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.anyErr:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
			require.Len(t, q.jobs, tc.wantJobs)
			if tc.wantJobs == 0 {
				assert.Zero(t, id)
				return
			}
			assert.Equal(t, int64(41), id)
			assert.Equal(t, queue.JobUpscale4K, q.jobs[0])
			assert.Equal(t, "post", q.sources[0])
			assert.NotEmpty(t, strings.TrimSpace(q.labels[0]))
			assert.Equal(t, tc.image, p4kParamImage(t, q.params[0]))
		})
	}
}

func TestRunPost4K(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		res         *generation.UpscaleResult
		wantImage   string
		wantUpscale string
	}{
		{name: "ESRGAN result becomes the image", res: &generation.UpscaleResult{Image: "BIG", Width: 3840, Height: 2560, Upscaled: true}, wantImage: "BIG", wantUpscale: "done"},
		{name: "without ESRGAN a plain resize is reported", res: &generation.UpscaleResult{Image: "BIG", Width: 3840, Height: 2560, Upscaled: true, Resized: true}, wantImage: "BIG", wantUpscale: "resized"},
		{name: "already 4K keeps the source", res: &generation.UpscaleResult{Image: "SRC", Width: 4000, Height: 3000}, wantImage: "SRC", wantUpscale: "skipped"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := &p4kUpscaler{res: tc.res}
			res, err := runPost4K(up, "SRC")
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, 1, up.calls)
			assert.Equal(t, "SRC", up.got)
			assert.Equal(t, []int{3840}, up.targets)
			assert.Equal(t, tc.wantImage, res.ImageBase64)
			var info map[string]any
			require.NoError(t, json.Unmarshal([]byte(res.Info), &info), "info must be JSON: %q", res.Info)
			assert.Equal(t, tc.wantUpscale, info["upscale"])
		})
	}
}

func TestRunPost4K_UpscaleErrorFailsTheJob(t *testing.T) {
	t.Parallel()
	boom := errors.New("server down")
	up := &p4kUpscaler{err: boom}
	res, err := runPost4K(up, "SRC")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Nil(t, res)
	assert.Equal(t, 1, up.calls)
}
