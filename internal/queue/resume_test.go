package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

func TestResetRunningToPending_CrashSpendsRetry(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)
	last, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE job_queue SET status = 'running', started_at = CURRENT_TIMESTAMP, retry_count = 1 WHERE id = ?`, id)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE job_queue SET status = 'running', started_at = CURRENT_TIMESTAMP, retry_count = 2 WHERE id = ?`, last)
	require.NoError(t, err)

	require.NoError(t, store.ResetRunningToPending())

	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, job.Status)
	assert.Equal(t, 2, job.RetryCount)
	job, err = store.Get(last)
	require.NoError(t, err)
	assert.Equal(t, StatusPaused, job.Status, "crash loop stops at max retries")
}

func TestRequeueInterrupted_KeepsRetriesAndGoesFirst(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)

	older, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)
	running, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE job_queue SET created_at = datetime('now', '-1 hour') WHERE id = ?`, older)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE job_queue SET status = 'running', started_at = CURRENT_TIMESTAMP, retry_count = 1 WHERE id = ?`, running)
	require.NoError(t, err)

	require.NoError(t, store.RequeueInterrupted(running))

	job, err := store.Get(running)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, job.Status)
	assert.Equal(t, 1, job.RetryCount)
	assert.Nil(t, job.NextRetry)

	next, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, running, next.ID, "interrupted job resumes before older pending ones")
}

func TestWorker_ShutdownRequeuesJobWithoutRetry(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)

	var cause atomic.Value
	started := make(chan struct{})
	proc := &mockProcessor{processFn: func(ctx context.Context, job *Job) (*JobResult, error) {
		close(started)
		<-ctx.Done()
		if c := context.Cause(ctx); c != nil {
			cause.Store(c)
		}
		return nil, ctx.Err()
	}}
	w := NewWorker(store, proc, &mockEmitter{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.processNext(ctx)
		close(done)
	}()
	<-started
	cancel()
	<-done

	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, job.Status)
	assert.Equal(t, 0, job.RetryCount)
	c, _ := cause.Load().(error)
	assert.False(t, errors.Is(c, sd.ErrUserCancel))
}

func TestWorker_UserCancelHasCause(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)

	causeCh := make(chan error, 1)
	started := make(chan struct{})
	proc := &mockProcessor{processFn: func(ctx context.Context, job *Job) (*JobResult, error) {
		close(started)
		<-ctx.Done()
		causeCh <- context.Cause(ctx)
		return nil, ctx.Err()
	}}
	w := NewWorker(store, proc, &mockEmitter{})
	var cleaned atomic.Int64
	w.SetCleanup(func(jobID int64) { cleaned.Store(jobID) })
	done := make(chan struct{})
	go func() {
		w.processNext(context.Background())
		close(done)
	}()
	<-started
	w.CancelCurrent()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}

	assert.ErrorIs(t, <-causeCh, sd.ErrUserCancel)
	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, job.Status)
	assert.Equal(t, id, cleaned.Load(), "server jobs of a cancelled queue job are cleaned up")
}

func TestWorker_ShutdownAfterSuccessCompletesJob(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	proc := &mockProcessor{processFn: func(context.Context, *Job) (*JobResult, error) {
		cancel()
		return &JobResult{ImageBase64: "img"}, nil
	}}
	w := NewWorker(store, proc, &mockEmitter{})
	var cleaned atomic.Int64
	w.SetCleanup(func(jobID int64) { cleaned.Store(jobID) })
	w.processNext(ctx)

	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status)
	assert.Contains(t, job.Result, "img")
	assert.Equal(t, id, cleaned.Load())
}

func TestService_StopWaitsForRequeue(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)
	started := make(chan struct{})
	proc := &mockProcessor{processFn: func(ctx context.Context, _ *Job) (*JobResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	svc := NewService(store, proc, &mockEmitter{})
	svc.Start(context.Background())
	<-started
	svc.Stop(5 * time.Second)

	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, job.Status)
	assert.Equal(t, 0, job.RetryCount)
}
