package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
)

const rjLabel = "Ретушь: Портрет в студии (1 из 2)"

const rjPhotoParams = `{"image_base64":"SU1H","size":"4k","label":"Ретушь: фото","index":1,"total":2}`

func TestJobRetouchType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, JobType("retouch"), JobRetouch)
}

func TestStore_RetouchImageStrippedOnFinish(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status JobStatus
		finish func(t *testing.T, store *Store, id int64)
		keep   bool
	}{
		{name: "completed", status: StatusCompleted, finish: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.UpdateCompleted(id, `{}`))
		}},
		{name: "cancelled", status: StatusCancelled, finish: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.UpdateStatus(id, StatusCancelled, ""))
		}},
		{name: "failed keeps the photo for retry", status: StatusFailed, keep: true, finish: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.UpdateStatus(id, StatusFailed, "boom"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := NewStore(openTestDB(t))
			id, err := store.CreateLabeled(JobRetouch, rjPhotoParams, "editor", rjLabel)
			require.NoError(t, err)

			tt.finish(t, store, id)

			job := sfGet(t, store, id)
			assert.Equal(t, tt.status, job.Status)
			p := paramsMap(t, job.Params)
			if tt.keep {
				assert.Contains(t, p, "image_base64")
				return
			}
			assert.NotContains(t, p, "image_base64")
			assert.Equal(t, float64(1), p["index"])
			assert.Equal(t, float64(2), p["total"])
		})
	}
}

func TestService_HandleRetouch(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	rec := &sfRecorder{}
	frame := json.RawMessage(`{"id":601,"width":2560,"height":3840,"upscale":"done"}`)
	svc.Handle(JobRetouch, func(gen *generation.Service, job *Job) (*JobResult, error) {
		rec.add(gen, job)
		return &JobResult{SessionItemID: 601, Frame: frame}, nil
	})
	id, err := store.CreateLabeled(JobRetouch, rjPhotoParams, "editor", rjLabel)
	require.NoError(t, err)

	svc.worker.processNext(context.Background())

	require.Equal(t, []int64{id}, rec.ids())
	assert.Equal(t, JobRetouch, rec.jobs[0].Type)
	job := sfGet(t, store, id)
	assert.Equal(t, StatusCompleted, job.Status)
	assert.Equal(t, rjLabel, sfLabelOf(t, job.ProgressDetail))
	var res JobResult
	require.NoError(t, json.Unmarshal([]byte(job.Result), &res), job.Result)
	assert.Equal(t, int64(601), res.SessionItemID)
	assert.JSONEq(t, string(frame), string(res.Frame))
	assert.NotContains(t, paramsMap(t, job.Params), "image_base64")
}

func TestService_CancelRetouchStripsImage(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	id, err := store.CreateLabeled(JobRetouch, rjPhotoParams, "editor", rjLabel)
	require.NoError(t, err)

	require.NoError(t, svc.CancelJob(id))

	job := sfGet(t, store, id)
	assert.Equal(t, StatusCancelled, job.Status)
	assert.NotContains(t, paramsMap(t, job.Params), "image_base64")
}
