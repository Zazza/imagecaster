package queue

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
)

const sfLabel = "Стилист: anna, front (1 из 3)"

const sfGarmentParams = `{"person":"anna","shot":"front","index":1,"total":3,"garment_base64":"R0FSTUVOVA==","image_base64":"SU1H"}`

type sfRecorder struct {
	mu   sync.Mutex
	jobs []*Job
	gens []*generation.Service
}

func (r *sfRecorder) add(gen *generation.Service, job *Job) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *job
	r.jobs = append(r.jobs, &cp)
	r.gens = append(r.gens, gen)
	return len(r.jobs)
}

func (r *sfRecorder) ids() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int64, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, j.ID)
	}
	return out
}

func sfNewService(t *testing.T) (*Store, *Service) {
	t.Helper()
	store := NewStore(openTestDB(t))
	emit := &mockEmitter{}
	proc := NewProcessor(nil, nil, store, t.TempDir(), emit)
	return store, NewService(store, proc, emit)
}

func sfLabelOf(t *testing.T, detail string) string {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(detail), &m), "progress_detail must be JSON: %q", detail)
	label, _ := m["label"].(string)
	return label
}

func sfGet(t *testing.T, store *Store, id int64) *Job {
	t.Helper()
	job, err := store.Get(id)
	require.NoError(t, err)
	require.NotNil(t, job, "job %d", id)
	return job
}

func TestStore_CreateLabeled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		label string
	}{
		{name: "stylist label", label: sfLabel},
		{name: "quotes and backslash", label: `Стилист: "anna" \ back (2 из 2)`},
		{name: "empty label", label: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := NewStore(openTestDB(t))

			id, err := store.CreateLabeled(JobStylistFrame, `{"shot":"front"}`, "stylist", tt.label)

			require.NoError(t, err)
			require.Positive(t, id)
			job := sfGet(t, store, id)
			assert.Equal(t, JobStylistFrame, job.Type)
			assert.Equal(t, StatusPending, job.Status)
			assert.Equal(t, `{"shot":"front"}`, job.Params)
			assert.Equal(t, "stylist", job.Source)
			var detail map[string]any
			require.NoError(t, json.Unmarshal([]byte(job.ProgressDetail), &detail), job.ProgressDetail)
			assert.Equal(t, map[string]any{"label": tt.label}, detail)
		})
	}
}

func TestStore_CreateLabeled_ShownInPanelAndCounted(t *testing.T) {
	t.Parallel()
	store := NewStore(openTestDB(t))
	var ids []int64
	for i := 0; i < 3; i++ {
		id, err := store.CreateLabeled(JobStylistFrame, `{}`, "stylist", sfLabel)
		require.NoError(t, err)
		ids = append(ids, id)
	}

	n, err := store.ActiveCount()
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	jobs, err := store.ListForPanel(100)
	require.NoError(t, err)
	seen := map[int64]bool{}
	for _, j := range jobs {
		seen[j.ID] = true
		assert.Equal(t, sfLabel, sfLabelOf(t, j.ProgressDetail), "job %d", j.ID)
	}
	for _, id := range ids {
		assert.True(t, seen[id], "job %d must be in the panel", id)
	}
}

func TestStore_LabelSurvivesRequeue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		requeue func(t *testing.T, store *Store, id int64)
	}{
		{name: "retry failed frame", requeue: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.UpdateStatus(id, StatusFailed, "boom"))
			ok, err := store.RetryFailed(id)
			require.NoError(t, err)
			require.True(t, ok)
		}},
		{name: "app restart mid-frame", requeue: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.ResetRunningToPending())
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := NewStore(openTestDB(t))
			id, err := store.CreateLabeled(JobStylistFrame, `{}`, "stylist", sfLabel)
			require.NoError(t, err)
			claimed, err := store.ClaimNext()
			require.NoError(t, err)
			require.NotNil(t, claimed)

			tt.requeue(t, store, id)

			job := sfGet(t, store, id)
			assert.Equal(t, StatusPending, job.Status)
			assert.Equal(t, sfLabel, sfLabelOf(t, job.ProgressDetail))
		})
	}
}

func TestStore_ClaimNext_SameCreatedAtByID(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	var want []int64
	for i := 0; i < 20; i++ {
		var id int64
		var err error
		if i%2 == 0 {
			id, err = store.CreateLabeled(JobStylistFrame, `{}`, "stylist", sfLabel)
		} else {
			id, err = store.Create(JobTxt2Img, `{}`, "test")
		}
		require.NoError(t, err)
		want = append(want, id)
	}
	_, err := db.Exec(`UPDATE job_queue SET created_at = '2026-11-18 10:00:00'`)
	require.NoError(t, err)

	var got []int64
	for {
		job, err := store.ClaimNext()
		require.NoError(t, err)
		if job == nil {
			break
		}
		got = append(got, job.ID)
	}

	assert.Equal(t, want, got)
}

func TestStore_ClaimNext_OlderCreatedAtFirst(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	first, err := store.CreateLabeled(JobStylistFrame, `{}`, "stylist", sfLabel)
	require.NoError(t, err)
	second, err := store.CreateLabeled(JobStylistFrame, `{}`, "stylist", sfLabel)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE job_queue SET created_at = '2026-11-18 10:00:05' WHERE id = ?`, first)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE job_queue SET created_at = '2026-11-18 10:00:00' WHERE id = ?`, second)
	require.NoError(t, err)

	a, err := store.ClaimNext()
	require.NoError(t, err)
	b, err := store.ClaimNext()
	require.NoError(t, err)

	require.NotNil(t, a)
	require.NotNil(t, b)
	assert.Equal(t, []int64{second, first}, []int64{a.ID, b.ID})
}

func TestStore_GarmentStrippedOnFinish(t *testing.T) {
	t.Parallel()
	stripped := map[string]any{"person": "anna", "shot": "front", "index": float64(1), "total": float64(3)}
	tests := []struct {
		name   string
		finish func(t *testing.T, store *Store, id int64)
		status JobStatus
		keep   bool
	}{
		{name: "completed", status: StatusCompleted, finish: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.UpdateCompleted(id, `{}`))
		}},
		{name: "cancelled one", status: StatusCancelled, finish: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.UpdateStatus(id, StatusCancelled, ""))
		}},
		{name: "cancel pending", status: StatusCancelled, finish: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.CancelPending())
		}},
		{name: "failed keeps garment for retry", status: StatusFailed, keep: true, finish: func(t *testing.T, store *Store, id int64) {
			require.NoError(t, store.UpdateStatus(id, StatusFailed, "boom"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := NewStore(openTestDB(t))
			id, err := store.CreateLabeled(JobStylistFrame, sfGarmentParams, "stylist", sfLabel)
			require.NoError(t, err)

			tt.finish(t, store, id)

			job := sfGet(t, store, id)
			assert.Equal(t, tt.status, job.Status)
			if tt.keep {
				assert.Equal(t, sfGarmentParams, job.Params)
				return
			}
			assert.Equal(t, stripped, paramsMap(t, job.Params))
		})
	}
}

func TestService_CancelStripsGarment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cancel func(t *testing.T, svc *Service, id int64)
	}{
		{name: "cancel one pending frame", cancel: func(t *testing.T, svc *Service, id int64) { require.NoError(t, svc.CancelJob(id)) }},
		{name: "cancel whole queue", cancel: func(t *testing.T, svc *Service, id int64) { require.NoError(t, svc.CancelQueue()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store, svc := sfNewService(t)
			id, err := store.CreateLabeled(JobStylistFrame, sfGarmentParams, "stylist", sfLabel)
			require.NoError(t, err)

			tt.cancel(t, svc, id)

			job := sfGet(t, store, id)
			assert.Equal(t, StatusCancelled, job.Status)
			assert.NotContains(t, paramsMap(t, job.Params), "garment_base64")
		})
	}
}

func TestJobResult_FrameJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		res   JobResult
		frame string
	}{
		{name: "with frame", res: JobResult{SessionItemID: 5, Frame: json.RawMessage(`{"id":5,"shot":"front","face_pass":"done"}`)}, frame: `{"id":5,"shot":"front","face_pass":"done"}`},
		{name: "without frame", res: JobResult{SessionItemID: 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(tt.res)

			require.NoError(t, err)
			var m map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &m))
			if tt.frame == "" {
				assert.NotContains(t, m, "frame")
				return
			}
			require.Contains(t, m, "frame")
			assert.JSONEq(t, tt.frame, string(m["frame"]))
		})
	}
}

func TestService_HandleStylistFrame(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	rec := &sfRecorder{}
	frame := json.RawMessage(`{"id":101,"shot":"front","prompt":"p","face_pass":"off"}`)
	svc.Handle(JobStylistFrame, func(gen *generation.Service, job *Job) (*JobResult, error) {
		rec.add(gen, job)
		return &JobResult{SessionItemID: 101, Frame: frame}, nil
	})
	id, err := store.CreateLabeled(JobStylistFrame, sfGarmentParams, "stylist", sfLabel)
	require.NoError(t, err)

	svc.worker.processNext(context.Background())

	require.Equal(t, []int64{id}, rec.ids())
	assert.Equal(t, JobStylistFrame, rec.jobs[0].Type)
	assert.Equal(t, sfGarmentParams, rec.jobs[0].Params, "handler gets the frame params with the garment")
	job := sfGet(t, store, id)
	assert.Equal(t, StatusCompleted, job.Status)
	var res JobResult
	require.NoError(t, json.Unmarshal([]byte(job.Result), &res), job.Result)
	assert.Equal(t, int64(101), res.SessionItemID)
	assert.JSONEq(t, string(frame), string(res.Frame))
	assert.NotContains(t, paramsMap(t, job.Params), "garment_base64")
}

func TestService_StylistFrameWithoutHandler(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	id, err := store.CreateLabeled(JobStylistFrame, `{}`, "stylist", sfLabel)
	require.NoError(t, err)

	svc.worker.processNext(context.Background())

	job := sfGet(t, store, id)
	assert.Equal(t, StatusFailed, job.Status)
	assert.Contains(t, job.Error, "unknown job type")
}

func TestService_HandleDoesNotCatchOtherTypes(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	rec := &sfRecorder{}
	svc.Handle(JobStylistFrame, func(gen *generation.Service, job *Job) (*JobResult, error) {
		rec.add(gen, job)
		return &JobResult{}, nil
	})
	id, err := store.Create(JobType("bogus"), `{}`, "test")
	require.NoError(t, err)

	svc.worker.processNext(context.Background())

	assert.Empty(t, rec.ids())
	job := sfGet(t, store, id)
	assert.Equal(t, StatusFailed, job.Status)
	assert.Contains(t, job.Error, "unknown job type")
}

func TestService_StylistFrameErrorOthersContinue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus JobStatus
	}{
		{name: "server error is retried later", err: errors.New("dial tcp: connection refused"), wantStatus: StatusPending},
		{name: "final error fails the frame", err: errors.New("bad frame params"), wantStatus: StatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store, svc := sfNewService(t)
			rec := &sfRecorder{}
			svc.Handle(JobStylistFrame, func(gen *generation.Service, job *Job) (*JobResult, error) {
				if rec.add(gen, job) == 1 {
					return nil, tt.err
				}
				return &JobResult{SessionItemID: job.ID}, nil
			})
			first, err := store.CreateLabeled(JobStylistFrame, `{"index":1}`, "stylist", sfLabel)
			require.NoError(t, err)
			second, err := store.CreateLabeled(JobStylistFrame, `{"index":2}`, "stylist", sfLabel)
			require.NoError(t, err)

			svc.worker.processNext(context.Background())
			svc.worker.processNext(context.Background())

			assert.Equal(t, []int64{first, second}, rec.ids())
			j1 := sfGet(t, store, first)
			assert.Equal(t, tt.wantStatus, j1.Status)
			if tt.wantStatus == StatusPending {
				assert.Equal(t, 1, j1.RetryCount)
			} else {
				assert.Contains(t, j1.Error, "bad frame params")
			}
			assert.Equal(t, StatusCompleted, sfGet(t, store, second).Status)
		})
	}
}

func TestService_CancelCurrentFrame(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	var interrupts int
	var mu sync.Mutex
	svc.SetInterruptFn(func() {
		mu.Lock()
		interrupts++
		mu.Unlock()
	})
	rec := &sfRecorder{}
	svc.Handle(JobStylistFrame, func(gen *generation.Service, job *Job) (*JobResult, error) {
		if rec.add(gen, job) == 1 {
			require.NoError(t, svc.CancelJob(job.ID))
			return nil, generation.ErrInterrupted
		}
		return &JobResult{SessionItemID: 7}, nil
	})
	first, err := store.CreateLabeled(JobStylistFrame, sfGarmentParams, "stylist", sfLabel)
	require.NoError(t, err)
	second, err := store.CreateLabeled(JobStylistFrame, `{"index":2}`, "stylist", sfLabel)
	require.NoError(t, err)

	svc.worker.processNext(context.Background())

	mu.Lock()
	assert.Equal(t, 1, interrupts, "cancel must interrupt generation on the server")
	mu.Unlock()
	j1 := sfGet(t, store, first)
	assert.NotEqual(t, StatusCompleted, j1.Status)
	assert.NotContains(t, j1.Result, "session_item_id")
	assert.Equal(t, StatusPending, sfGet(t, store, second).Status, "only the current frame is cancelled")

	svc.worker.processNext(context.Background())

	assert.Equal(t, []int64{first, second}, rec.ids())
	assert.Equal(t, StatusCompleted, sfGet(t, store, second).Status)
}

func TestService_CancelAllFrames(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	rec := &sfRecorder{}
	svc.Handle(JobStylistFrame, func(gen *generation.Service, job *Job) (*JobResult, error) {
		rec.add(gen, job)
		require.NoError(t, svc.CancelQueue())
		return nil, generation.ErrInterrupted
	})
	var ids []int64
	for i := 0; i < 3; i++ {
		id, err := store.CreateLabeled(JobStylistFrame, sfGarmentParams, "stylist", sfLabel)
		require.NoError(t, err)
		ids = append(ids, id)
	}

	svc.worker.processNext(context.Background())
	svc.worker.processNext(context.Background())

	assert.Equal(t, []int64{ids[0]}, rec.ids())
	assert.NotEqual(t, StatusCompleted, sfGet(t, store, ids[0]).Status)
	for _, id := range ids[1:] {
		job := sfGet(t, store, id)
		assert.Equal(t, StatusCancelled, job.Status, "job %d", id)
		assert.NotContains(t, paramsMap(t, job.Params), "garment_base64", "job %d", id)
	}
	n, err := store.ActiveCount()
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestService_PauseStopsBetweenFrames(t *testing.T) {
	t.Parallel()
	store, svc := sfNewService(t)
	rec := &sfRecorder{}
	svc.Handle(JobStylistFrame, func(gen *generation.Service, job *Job) (*JobResult, error) {
		if rec.add(gen, job) == 1 {
			svc.PauseQueue()
		}
		return &JobResult{SessionItemID: job.ID}, nil
	})
	var ids []int64
	for i := 0; i < 3; i++ {
		id, err := store.CreateLabeled(JobStylistFrame, `{}`, "stylist", sfLabel)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		svc.Stop(2 * time.Second)
	})

	svc.Start(ctx)

	require.Eventually(t, func() bool {
		return sfGet(t, store, ids[0]).Status == StatusCompleted
	}, 3*time.Second, 20*time.Millisecond, "the current frame finishes after pause")
	require.Never(t, func() bool {
		return len(rec.ids()) > 1
	}, 400*time.Millisecond, 20*time.Millisecond, "paused queue must not start the next frame")
	assert.Equal(t, StatusPending, sfGet(t, store, ids[1]).Status)
	assert.Equal(t, StatusPending, sfGet(t, store, ids[2]).Status)

	svc.ResumeQueue()

	require.Eventually(t, func() bool {
		return sfGet(t, store, ids[2]).Status == StatusCompleted
	}, 3*time.Second, 20*time.Millisecond, "resume continues with the next frame")
	assert.Equal(t, ids, rec.ids())
}
