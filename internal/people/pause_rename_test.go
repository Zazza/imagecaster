package people

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func (f *fakeAPI) PauseTraining(_ context.Context, id string) (*serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pauses = append(f.pauses, id)
	return f.pauseJob, f.pauseErr
}

func (f *fakeAPI) ResumeTraining(_ context.Context, id string) (*serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumes = append(f.resumes, id)
	return f.resumeJob, f.resumeErr
}

func (f *fakeAPI) RenamePerson(_ context.Context, oldName, newName string) (*serverclient.Person, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renames = append(f.renames, [2]string{oldName, newName})
	if f.renameErr != nil {
		return nil, f.renameErr
	}
	return &serverclient.Person{Name: newName, Lora: newName}, nil
}

func newLocalService(api API) *Service {
	return New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })
}

func TestPause_PassesID(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{pauseJob: &serverclient.TrainJob{ID: "j1", Status: "paused"}}

	job, err := newLocalService(api).Pause(context.Background(), "j1")

	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "paused", job.Status)
	assert.Equal(t, []string{"j1"}, api.pauses)
}

func TestResume_PassesID(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{resumeJob: &serverclient.TrainJob{ID: "j1", Status: "queued"}}

	job, err := newLocalService(api).Resume(context.Background(), "j1")

	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "queued", job.Status)
	assert.Equal(t, []string{"j1"}, api.resumes)
}

func TestPauseResume_ErrorReturned(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{
		pauseErr:  &serverclient.TrainAPIError{Status: 409, Code: "not_running", Message: "queued"},
		resumeErr: &serverclient.TrainAPIError{Status: 409, Code: "not_paused", Message: "completed"},
	}
	s := newLocalService(api)

	_, err := s.Pause(context.Background(), "j1")
	var got *serverclient.TrainAPIError
	require.True(t, errors.As(err, &got))
	assert.Equal(t, "not_running", got.Code)

	_, err = s.Resume(context.Background(), "j1")
	require.True(t, errors.As(err, &got))
	assert.Equal(t, "not_paused", got.Code)
}

func TestRename_NormalizesNewName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"maria", "maria"},
		{"  Maria ", "maria"},
		{"NEW_NAME", "new_name"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			api := &fakeAPI{}

			require.NoError(t, newLocalService(api).Rename(context.Background(), "anna", tt.in))

			assert.Equal(t, [][2]string{{"anna", tt.want}}, api.renames)
		})
	}
}

func TestRename_ErrorReturned(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{renameErr: &serverclient.TrainAPIError{Status: 409, Code: "name_taken", Message: "name is already used"}}

	err := newLocalService(api).Rename(context.Background(), "anna", "maria")

	var got *serverclient.TrainAPIError
	require.True(t, errors.As(err, &got))
	assert.Equal(t, "name_taken", got.Code)
}
