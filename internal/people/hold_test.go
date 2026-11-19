package people

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestTrain_HoldPassedToAPI(t *testing.T) {
	tests := []struct {
		name string
		hold bool
	}{
		{"hold", true},
		{"start now", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			dir := t.TempDir()
			makePhotos(t, dir, 12)
			api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1", Status: "paused"}}
			s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

			job, err := s.Train(context.Background(), dir, "Anna", "", "woman", "Z-Image", tt.hold)

			require.NoError(t, err)
			require.NotNil(t, job)
			require.Len(t, api.submits, 1)
			req := api.submits[0]
			assert.Equal(t, tt.hold, req.Hold)
			assert.Equal(t, "anna", req.Name)
			assert.Equal(t, "Z-Image", req.Base)
			assert.Equal(t, "inst-1", req.Client)
			assert.Equal(t, "inst-1:train:anna", req.ClientRef)
			assert.Len(t, req.Photos, 12)
		})
	}
}

func TestTrain_HoldStillRequiresLocalServer(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 12)
	api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}
	s := New(api, func() string { return "http://8.8.8.8:8080" }, func() string { return "inst-1" })

	job, err := s.Train(context.Background(), dir, "anna", "", "woman", "Z-Image", true)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotLocal), "want ErrNotLocal, got %v", err)
	assert.Nil(t, job)
	assert.Zero(t, api.totalCalls())
}

func TestTrain_HoldStillChecksPhotoCount(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 9)
	api := &fakeAPI{}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	_, err := s.Train(context.Background(), dir, "anna", "", "woman", "Z-Image", true)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPhotoCount), "want ErrPhotoCount, got %v", err)
	assert.Zero(t, api.totalCalls())
}
