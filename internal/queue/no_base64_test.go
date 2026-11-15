package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Zazza/imagecaster/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payloadEmitter struct {
	mu     sync.Mutex
	events []string
	data   []any
}

func (e *payloadEmitter) Emit(event string, data ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
	var d any
	if len(data) > 0 {
		d = data[0]
	}
	e.data = append(e.data, d)
}

func (e *payloadEmitter) payload(event string) (EventPayload, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, ev := range e.events {
		if ev == event {
			p, ok := e.data[i].(EventPayload)
			return p, ok
		}
	}
	return EventPayload{}, false
}

const bigParams = `{"prompt":"a cat","seed":42,"image_base64":"SU1BR0U=","mask_base64":"TUFTSw==","reference_image_base64":"UkVG"}`

func paramsMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

func TestWorker_CompletedResultWithoutBase64(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobFromImage, bigParams, "test")
	require.NoError(t, err)
	emit := &payloadEmitter{}
	proc := &mockProcessor{processFn: func(context.Context, *Job) (*JobResult, error) {
		return &JobResult{ImageBase64: "UkVTVUxU", SessionItemID: 5, Info: "info"}, nil
	}}
	NewWorker(store, proc, emit).processNext(context.Background())

	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status)
	assert.JSONEq(t, `{"session_item_id":5,"info":"info"}`, job.Result)
	assert.Equal(t, map[string]any{"prompt": "a cat", "seed": float64(42)}, paramsMap(t, job.Params))

	p, ok := emit.payload(EventQueueCompleted)
	require.True(t, ok)
	require.NotNil(t, p.Result)
	assert.Empty(t, p.Result.ImageBase64)
	assert.Equal(t, int64(5), p.Result.SessionItemID)
	assert.Equal(t, "info", p.Result.Info)
}

func TestWorker_CompletedEventImageOnlyWithoutSessionItem(t *testing.T) {
	t.Parallel()
	atCap := strings.Repeat("A", session.MaxStoredImageBase64)
	overCap := atCap + "AAAA"
	tests := []struct {
		name      string
		itemID    int64
		image     string
		wantImage string
	}{
		{name: "saved to session: image fetched by id", itemID: 7, image: "UkVTVUxU", wantImage: ""},
		{name: "not saved to session: image in event", itemID: 0, image: "UkVTVUxU", wantImage: "UkVTVUxU"},
		{name: "saved at the history size cap: fetched by id", itemID: 8, image: atCap, wantImage: ""},
		{name: "over the history size cap: history keeps JPEG, original in event", itemID: 9, image: overCap, wantImage: overCap},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := NewStore(openTestDB(t))
			_, err := store.Create(JobLook, bigParams, "test")
			require.NoError(t, err)
			emit := &payloadEmitter{}
			res := &JobResult{ImageBase64: tc.image, SessionItemID: tc.itemID}
			proc := &mockProcessor{processFn: func(context.Context, *Job) (*JobResult, error) {
				return res, nil
			}}
			NewWorker(store, proc, emit).processNext(context.Background())

			p, ok := emit.payload(EventQueueCompleted)
			require.True(t, ok)
			require.NotNil(t, p.Result)
			assert.Equal(t, len(tc.wantImage), len(p.Result.ImageBase64))
			assert.True(t, tc.wantImage == p.Result.ImageBase64)
			assert.Equal(t, tc.itemID, p.Result.SessionItemID)
			assert.Equal(t, len(tc.image), len(res.ImageBase64))
		})
	}
}

func TestWorker_FailedKeepsParamsForRetry(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobFromImage, bigParams, "test")
	require.NoError(t, err)
	var seen []string
	proc := &mockProcessor{processFn: func(_ context.Context, j *Job) (*JobResult, error) {
		seen = append(seen, j.Params)
		if len(seen) == 1 {
			return nil, errors.New("bad prompt")
		}
		return &JobResult{SessionItemID: 1}, nil
	}}
	svc := NewService(store, proc, &mockEmitter{})
	svc.worker.processNext(context.Background())

	job, err := store.Get(id)
	require.NoError(t, err)
	require.Equal(t, StatusFailed, job.Status)
	assert.Equal(t, bigParams, job.Params)

	require.NoError(t, svc.RetryJob(id))
	svc.worker.processNext(context.Background())
	require.Len(t, seen, 2)
	assert.Equal(t, bigParams, seen[1])
	job, err = store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status)
}

func TestService_CancelStripsImageParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cancel func(t *testing.T, svc *Service, id int64)
	}{
		{"cancel one pending job", func(t *testing.T, svc *Service, id int64) { require.NoError(t, svc.CancelJob(id)) }},
		{"cancel whole queue", func(t *testing.T, svc *Service, id int64) { require.NoError(t, svc.CancelQueue()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			store := NewStore(db)
			id, err := store.Create(JobEdit, bigParams, "test")
			require.NoError(t, err)
			plain, err := store.Create(JobTxt2Img, "not json", "test")
			require.NoError(t, err)
			svc := NewService(store, &mockProcessor{}, &mockEmitter{})
			tt.cancel(t, svc, id)
			job, err := store.Get(id)
			require.NoError(t, err)
			assert.Equal(t, StatusCancelled, job.Status)
			assert.Equal(t, map[string]any{"prompt": "a cat", "seed": float64(42)}, paramsMap(t, job.Params))
			if tt.name == "cancel whole queue" {
				other, err := store.Get(plain)
				require.NoError(t, err)
				assert.Equal(t, "not json", other.Params)
			}
		})
	}
}

func TestWorker_SaveResultFailureDoesNotEmitCompleted(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER fail_complete BEFORE UPDATE ON job_queue WHEN NEW.status = 'completed' BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	require.NoError(t, err)
	emit := &payloadEmitter{}
	var cleaned int64
	w := NewWorker(store, &mockProcessor{processFn: func(context.Context, *Job) (*JobResult, error) {
		return &JobResult{SessionItemID: 3}, nil
	}}, emit)
	w.SetCleanup(func(jobID int64) { cleaned = jobID })
	w.processNext(context.Background())

	_, completed := emit.payload(EventQueueCompleted)
	assert.False(t, completed)
	failed, ok := emit.payload(EventQueueFailed)
	require.True(t, ok)
	assert.Contains(t, failed.Error, "disk full")
	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, job.Status)
	assert.Equal(t, id, cleaned)
}

func TestStore_ActiveCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		statuses []JobStatus
		want     int
	}{
		{"empty", nil, 0},
		{"pending and running", []JobStatus{StatusPending, StatusRunning, StatusPending}, 3},
		{"finished and paused not counted", []JobStatus{StatusCompleted, StatusFailed, StatusCancelled, StatusPaused, StatusRunning}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			for _, st := range tt.statuses {
				insertJob(t, db, st)
			}
			svc := NewService(NewStore(db), &mockProcessor{}, &mockEmitter{})
			n, err := svc.ActiveCount()
			require.NoError(t, err)
			assert.Equal(t, tt.want, n)
		})
	}
}

func TestStore_HasClaimable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup string
		want  bool
	}{
		{"empty", ``, false},
		{"pending due", `INSERT INTO job_queue (type, status) VALUES ('txt2img', 'pending')`, true},
		{"pending retry in past", `INSERT INTO job_queue (type, status, next_retry_at) VALUES ('txt2img', 'pending', '2000-01-01 00:00:00')`, true},
		{"pending retry in future", `INSERT INTO job_queue (type, status, next_retry_at) VALUES ('txt2img', 'pending', '2999-01-01 00:00:00')`, false},
		{"only running and paused", `INSERT INTO job_queue (type, status) VALUES ('txt2img', 'running'), ('txt2img', 'paused')`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			if tt.setup != "" {
				_, err := db.Exec(tt.setup)
				require.NoError(t, err)
			}
			got, err := NewStore(db).HasClaimable()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWorker_CheckMoreJobsNotifiesOnlyWhenClaimable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup string
		want  bool
	}{
		{"claimable", `INSERT INTO job_queue (type, status) VALUES ('txt2img', 'pending')`, true},
		{"backoff only", `INSERT INTO job_queue (type, status, next_retry_at) VALUES ('txt2img', 'pending', '2999-01-01 00:00:00')`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			_, err := db.Exec(tt.setup)
			require.NoError(t, err)
			w := NewWorker(NewStore(db), &mockProcessor{}, &mockEmitter{})
			w.checkMoreJobs()
			assert.Equal(t, tt.want, len(w.notify) == 1)
		})
	}
}

func TestStore_ListForPanelSkipsHeavyColumns(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	_, err := db.Exec(`INSERT INTO job_queue (type, status, params, result, error, source, retry_count) VALUES
		('from_image', 'completed', ?, '{"session_item_id":1}', '', 'from-image', 0),
		('edit', 'pending', ?, '', 'boom', 'edit', 2)`, bigParams, bigParams)
	require.NoError(t, err)
	jobs, err := store.ListForPanel(10)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	for _, j := range jobs {
		assert.Empty(t, j.Params)
		assert.Empty(t, j.Result)
		assert.NotZero(t, j.ID)
		assert.NotEmpty(t, j.Type)
		assert.NotEmpty(t, j.Status)
		assert.NotEmpty(t, j.CreatedAt)
	}
	assert.Equal(t, "boom", jobs[0].Error)
	assert.Equal(t, 2, jobs[0].RetryCount)
	assert.Equal(t, "edit", jobs[0].Source)
}
