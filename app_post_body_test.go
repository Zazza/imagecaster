package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/bodyshape"
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/sd"
)

type pbBodies struct {
	calls int
	pts   []bodyshape.Point
	err   error
}

func (b *pbBodies) BodyKeypoints(string) ([]bodyshape.Point, error) {
	b.calls++
	return b.pts, b.err
}

var _ bodyDetector = (*pbBodies)(nil)

func TestPostBodyPoints(t *testing.T) {
	t.Parallel()
	local := &config.Config{SDUrl: "http://192.168.1.184:8080"}
	remote := &config.Config{SDUrl: "http://8.8.8.8:7860"}
	pts := make([]bodyshape.Point, 18)
	tests := []struct {
		name      string
		cfg       *config.Config
		det       *pbBodies
		wantSkip  string
		wantCalls int
		wantPts   int
		wantError bool
	}{
		{name: "local server returns points", cfg: local, det: &pbBodies{pts: pts}, wantCalls: 1, wantPts: 18},
		{name: "remote server is not asked", cfg: remote, det: &pbBodies{pts: pts}, wantSkip: "not_local"},
		{name: "no body", cfg: local, det: &pbBodies{err: sd.ErrNoBody}, wantSkip: "no_body", wantCalls: 1},
		{name: "wrapped no body", cfg: local, det: &pbBodies{err: fmt.Errorf("body: %w", sd.ErrNoBody)}, wantSkip: "no_body", wantCalls: 1},
		{name: "server error is unavailable with a message", cfg: local, det: &pbBodies{err: errors.New("status 500")}, wantSkip: "unavailable", wantCalls: 1, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := postBodyPoints(tc.det, tc.cfg, prImage(t))
			require.NoError(t, err)
			assert.Equal(t, tc.wantSkip, got.Skip)
			assert.Equal(t, tc.wantCalls, tc.det.calls)
			assert.Len(t, got.Points, tc.wantPts)
			assert.NotNil(t, got.Points)
			assert.Equal(t, tc.wantError, got.Error != "")
			if tc.wantError {
				assert.Contains(t, got.Error, "status 500")
			}
		})
	}
}

func TestPostBodyPoints_RejectsEmptyImage(t *testing.T) {
	t.Parallel()
	det := &pbBodies{}
	_, err := postBodyPoints(det, &config.Config{SDUrl: "http://127.0.0.1:1"}, "")
	require.Error(t, err)
	assert.Zero(t, det.calls)
}
