package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/stretchr/testify/require"
)

const targetCancelWait = 5 * time.Second

type targetRun struct {
	id     int64
	ctx    context.Context
	finish chan struct{}
}

func targetService(t *testing.T) (*Service, <-chan targetRun) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	started := make(chan targetRun)
	proc := &mockProcessor{processFn: func(ctx context.Context, job *Job) (*JobResult, error) {
		run := targetRun{id: job.ID, ctx: ctx, finish: make(chan struct{})}
		select {
		case started <- run:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case <-run.finish:
			return &JobResult{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	svc := NewService(NewStore(db), proc, &mockEmitter{})
	startService(t, svc)
	return svc, started
}

func receiveTargetRun(t *testing.T, started <-chan targetRun, id int64) targetRun {
	t.Helper()
	select {
	case run := <-started:
		require.Equal(t, id, run.id)
		return run
	case <-time.After(targetCancelWait):
		t.Fatal("queued job did not start")
		return targetRun{}
	}
}

func TestTargetCancel_ServiceCancelsOnlyRequestedContextAndRunsNext(t *testing.T) {
	svc, started := targetService(t)
	first := insertJob(t, svc.store.db, StatusPending)
	second := insertJob(t, svc.store.db, StatusPending)
	var interrupts atomic.Int32
	svc.SetInterruptFn(func() { interrupts.Add(1) })
	svc.worker.Notify()
	a := receiveTargetRun(t, started, first)
	require.NoError(t, svc.CancelJob(first))
	select {
	case <-a.ctx.Done():
		require.ErrorIs(t, context.Cause(a.ctx), sd.ErrUserCancel)
	case <-time.After(targetCancelWait):
		t.Fatal("target job context was not cancelled")
	}
	b := receiveTargetRun(t, started, second)
	require.NoError(t, b.ctx.Err(), "next job must start with a live context")
	require.Zero(t, interrupts.Load(), "single-job cancellation must not use global interrupt")
	close(b.finish)
	require.Eventually(t, func() bool {
		job, err := svc.store.Get(second)
		return err == nil && job.Status == StatusCompleted
	}, targetCancelWait, time.Millisecond)
}

func TestTargetCancel_WorkerRejectsFinishedTargetAfterNextJobStarts(t *testing.T) {
	svc, started := targetService(t)
	first := insertJob(t, svc.store.db, StatusPending)
	second := insertJob(t, svc.store.db, StatusPending)
	svc.worker.Notify()
	a := receiveTargetRun(t, started, first)
	close(a.finish)
	b := receiveTargetRun(t, started, second)
	require.False(t, svc.worker.CancelJob(first), "finished A must not match current B")
	require.NoError(t, b.ctx.Err(), "late cancellation of A must leave B alive")
	require.Equal(t, StatusCompleted, getJobStatus(t, svc.store.db, first))
	require.Error(t, svc.CancelJob(first), "completed job remains non-cancellable")
	require.Equal(t, StatusCompleted, getJobStatus(t, svc.store.db, first))
	require.NoError(t, b.ctx.Err())
	require.True(t, svc.worker.CancelJob(second), "active target must match")
	select {
	case <-b.ctx.Done():
		require.ErrorIs(t, context.Cause(b.ctx), sd.ErrUserCancel)
	case <-time.After(targetCancelWait):
		t.Fatal("matched active job was not cancelled")
	}
}
