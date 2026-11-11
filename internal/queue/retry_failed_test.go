package queue

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const retryParams = `{"prompt":"a cat","seed":42}`

func insertDirtyJob(t *testing.T, db *sql.DB, status JobStatus) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO job_queue (type, status, params, source, progress, progress_detail, result, error,
			started_at, completed_at, retry_count, max_retries, next_retry_at)
		 VALUES (?, ?, ?, 'mcp', 0.7, '{"step":7}', '{"image":"x"}', 'sd timeout',
			'2026-11-10 10:00:00', '2026-11-10 10:05:00', 3, 3, '2026-11-10 10:10:00')`,
		JobFromImage, status, retryParams,
	)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

type jobRow struct {
	jobType        string
	status         string
	params         string
	source         string
	progress       float64
	progressDetail string
	result         string
	errMsg         string
	startedAt      sql.NullString
	completedAt    sql.NullString
	retryCount     int
	nextRetryAt    sql.NullString
}

func readJobRow(t *testing.T, db *sql.DB, id int64) jobRow {
	t.Helper()
	var r jobRow
	err := db.QueryRow(
		`SELECT type, status, params, source, progress, progress_detail, result, error,
			started_at, completed_at, retry_count, next_retry_at
		 FROM job_queue WHERE id = ?`, id,
	).Scan(&r.jobType, &r.status, &r.params, &r.source, &r.progress, &r.progressDetail, &r.result,
		&r.errMsg, &r.startedAt, &r.completedAt, &r.retryCount, &r.nextRetryAt)
	require.NoError(t, err)
	return r
}

func TestStore_RetryFailed_ResetsFailedJobToPending(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	id := insertDirtyJob(t, db, StatusFailed)

	updated, err := NewStore(db).RetryFailed(id)
	require.NoError(t, err)
	assert.True(t, updated)

	r := readJobRow(t, db, id)
	assert.Equal(t, string(StatusPending), r.status)
	assert.Equal(t, 0, r.retryCount)
	assert.Equal(t, "", r.errMsg)
	assert.False(t, r.nextRetryAt.Valid, "next_retry_at must be NULL, got %q", r.nextRetryAt.String)
	assert.Equal(t, 0.0, r.progress)
	assert.Equal(t, "", r.progressDetail)
	assert.Equal(t, "", r.result)
	assert.False(t, r.startedAt.Valid, "started_at must be NULL, got %q", r.startedAt.String)
	assert.False(t, r.completedAt.Valid, "completed_at must be NULL, got %q", r.completedAt.String)
}

func TestStore_RetryFailed_KeepsParamsTypeSource(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	id := insertDirtyJob(t, db, StatusFailed)

	_, err := NewStore(db).RetryFailed(id)
	require.NoError(t, err)

	r := readJobRow(t, db, id)
	assert.Equal(t, retryParams, r.params)
	assert.Equal(t, string(JobFromImage), r.jobType)
	assert.Equal(t, "mcp", r.source)
}

func TestStore_RetryFailed_OtherStatusesUntouched(t *testing.T) {
	statuses := []JobStatus{StatusPending, StatusRunning, StatusCompleted, StatusCancelled, StatusPaused}
	for _, st := range statuses {
		t.Run(string(st), func(t *testing.T) {
			db := openTestDB(t)
			defer db.Close()
			id := insertDirtyJob(t, db, st)
			before := readJobRow(t, db, id)

			updated, err := NewStore(db).RetryFailed(id)
			require.NoError(t, err)
			assert.False(t, updated)
			assert.Equal(t, before, readJobRow(t, db, id))
		})
	}
}

func TestStore_RetryFailed_NonexistentID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	updated, err := NewStore(db).RetryFailed(999999)
	require.NoError(t, err)
	assert.False(t, updated)
}

func TestStore_RetryFailed_OnlyTargetJobChanges(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	target := insertDirtyJob(t, db, StatusFailed)
	other := insertDirtyJob(t, db, StatusFailed)
	otherBefore := readJobRow(t, db, other)

	updated, err := NewStore(db).RetryFailed(target)
	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, otherBefore, readJobRow(t, db, other))
}

func TestStore_RetryFailed_SecondCallReturnsFalse(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	id := insertDirtyJob(t, db, StatusFailed)
	store := NewStore(db)

	first, err := store.RetryFailed(id)
	require.NoError(t, err)
	require.True(t, first)

	second, err := store.RetryFailed(id)
	require.NoError(t, err)
	assert.False(t, second)
}

func TestService_RetryJob_FailedJobEmitsQueueChanged(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	id := insertDirtyJob(t, db, StatusFailed)
	emitter := &mockEmitter{}
	svc := NewService(NewStore(db), &mockProcessor{}, emitter)

	require.NoError(t, svc.RetryJob(id))
	assert.Equal(t, StatusPending, getJobStatus(t, db, id))
	assert.True(t, emitter.hasEvent(EventQueueChanged))
}

func TestService_RetryJob_WakesWorker(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	db.SetMaxOpenConns(1)
	id := insertDirtyJob(t, db, StatusFailed)

	processed := make(chan int64, 1)
	proc := &mockProcessor{
		processFn: func(ctx context.Context, job *Job) (*JobResult, error) {
			select {
			case processed <- job.ID:
			default:
			}
			return &JobResult{}, nil
		},
	}
	svc := NewService(NewStore(db), proc, &mockEmitter{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.worker.loop(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	require.NoError(t, svc.RetryJob(id))

	select {
	case got := <-processed:
		assert.Equal(t, id, got)
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not pick up retried job within 2s (worker ticker is 5s, so RetryJob must notify it)")
	}
}

func TestService_RetryJob_NotFailedReturnsErrorWithoutEvent(t *testing.T) {
	statuses := []JobStatus{StatusPending, StatusRunning, StatusCompleted, StatusCancelled, StatusPaused}
	for _, st := range statuses {
		t.Run(string(st), func(t *testing.T) {
			db := openTestDB(t)
			defer db.Close()
			id := insertDirtyJob(t, db, st)
			emitter := &mockEmitter{}
			svc := NewService(NewStore(db), &mockProcessor{}, emitter)

			err := svc.RetryJob(id)
			assert.Error(t, err)
			assert.Equal(t, st, getJobStatus(t, db, id))
			assert.Equal(t, 0, emitter.countEvent(EventQueueChanged))
		})
	}
}

func TestService_RetryJob_NonexistentReturnsErrorWithoutEvent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	emitter := &mockEmitter{}
	svc := NewService(NewStore(db), &mockProcessor{}, emitter)

	assert.Error(t, svc.RetryJob(999999))
	assert.Equal(t, 0, emitter.countEvent(EventQueueChanged))
}
