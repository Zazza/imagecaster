package people

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestTrain_PassesBase(t *testing.T) {
	for _, base := range []string{"Moss-Image", "Z-Image"} {
		t.Run(base, func(t *testing.T) {
			clearProxyEnv(t)
			dir := t.TempDir()
			want := makePhotos(t, dir, 12)
			api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1", Status: "queued", Base: base}}
			s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

			job, err := s.Train(context.Background(), dir, "anna", "woman", base)

			require.NoError(t, err)
			require.NotNil(t, job)
			assert.Equal(t, base, job.Base)
			require.Len(t, api.submits, 1)
			req := api.submits[0]
			assert.Equal(t, base, req.Base)
			assert.Equal(t, "anna", req.Name)
			assert.Equal(t, "woman", req.Gender)
			assert.Equal(t, "inst-1:train:anna", req.ClientRef)
			assert.Equal(t, want, req.Photos)
		})
	}
}

func TestTrain_NotLocalWithMossBase(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 12)
	api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}
	s := New(api, func() string { return "http://8.8.8.8:8080" }, func() string { return "inst-1" })

	job, err := s.Train(context.Background(), dir, "anna", "woman", "Moss-Image")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotLocal), "want ErrNotLocal, got %v", err)
	assert.Nil(t, job)
	assert.Zero(t, api.totalCalls())
}

func TestTrain_BaseUnavailableReturned(t *testing.T) {
	clearProxyEnv(t)
	dir := t.TempDir()
	makePhotos(t, dir, 12)
	api := &fakeAPI{submitErr: &serverclient.TrainAPIError{Status: 400, Code: "base_unavailable", Message: "Moss-Image files are missing"}}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	job, err := s.Train(context.Background(), dir, "anna", "woman", "Moss-Image")

	require.Error(t, err)
	assert.Nil(t, job)
	var got *serverclient.TrainAPIError
	require.True(t, errors.As(err, &got))
	assert.Equal(t, "base_unavailable", got.Code)
}

func TestBases_ProxiesAPI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		bases []string
	}{
		{name: "z-image and moss", bases: []string{"Z-Image", "Moss-Image"}},
		{name: "only z-image", bases: []string{"Z-Image"}},
		{name: "empty", bases: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := &fakeAPI{bases: tt.bases}
			s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

			got, err := s.Bases(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.bases, got)
			assert.Equal(t, 1, api.basesCalls)
		})
	}
}

func TestBases_ErrorReturned(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{basesErr: &serverclient.TrainAPIError{Status: 503, Code: "not_available", Message: "trainer is not installed"}}
	s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

	_, err := s.Bases(context.Background())

	var got *serverclient.TrainAPIError
	require.True(t, errors.As(err, &got), "want *TrainAPIError, got %T: %v", err, err)
	assert.Equal(t, "not_available", got.Code)
}
