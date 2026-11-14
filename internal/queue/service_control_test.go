package queue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func drainNotify(t *testing.T, w *Worker) {
	t.Helper()
	for range 2 {
		w.Notify()
		require.Eventually(t, func() bool { return len(w.notify) == 0 }, 5*time.Second, time.Millisecond)
	}
}

func startService(t *testing.T, svc *Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		<-svc.worker.Done()
	})
	svc.Start(ctx)
}

func TestService_PauseQueueKeepsJobsPendingUntilResume(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	emitter := &mockEmitter{}
	svc := NewService(NewStore(db), &mockProcessor{}, emitter)
	svc.PauseQueue()
	require.True(t, svc.IsPaused())
	startService(t, svc)

	id := insertJob(t, db, StatusPending)
	drainNotify(t, svc.worker)
	assert.Equal(t, StatusPending, getJobStatus(t, db, id))
	assert.False(t, emitter.hasEvent(EventQueueStarted))

	svc.ResumeQueue()
	require.False(t, svc.IsPaused())
	waitEvent(t, emitter, EventQueueCompleted)
	assert.Equal(t, StatusCompleted, getJobStatus(t, db, id))
}

func TestService_RemoveJob(t *testing.T) {
	tests := []struct {
		status      JobStatus
		wantDeleted bool
	}{
		{StatusPending, true},
		{StatusFailed, true},
		{StatusCancelled, true},
		{StatusPaused, true},
		{StatusRunning, false},
		{StatusCompleted, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			db := openTestDB(t)
			defer db.Close()
			emitter := &mockEmitter{}
			svc := NewService(NewStore(db), &mockProcessor{}, emitter)
			var cleaned []int64
			svc.SetJobCleanup(func(id int64) { cleaned = append(cleaned, id) })

			id := insertJob(t, db, tt.status)
			other := insertJob(t, db, tt.status)

			require.NoError(t, svc.RemoveJob(id))

			job, err := svc.store.Get(id)
			require.NoError(t, err)
			if tt.wantDeleted {
				assert.Nil(t, job)
				assert.Equal(t, []int64{id}, cleaned)
				assert.Equal(t, 1, emitter.countEvent(EventQueueChanged))
			} else {
				require.NotNil(t, job)
				assert.Equal(t, tt.status, job.Status)
				assert.Empty(t, cleaned)
				assert.Equal(t, 0, emitter.countEvent(EventQueueChanged))
			}
			assert.Equal(t, tt.status, getJobStatus(t, db, other))
		})
	}
}

func TestService_ClearCompletedRemovesOnlyFinishedSuccessfullyOrCancelled(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	emitter := &mockEmitter{}
	svc := NewService(NewStore(db), &mockProcessor{}, emitter)

	ids := map[JobStatus]int64{}
	for _, st := range []JobStatus{StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusCancelled, StatusPaused} {
		ids[st] = insertJob(t, db, st)
	}

	require.NoError(t, svc.ClearCompleted())

	for st, id := range ids {
		job, err := svc.store.Get(id)
		require.NoError(t, err)
		if st == StatusCompleted || st == StatusCancelled {
			assert.Nil(t, job, "status %s must be removed", st)
			continue
		}
		require.NotNil(t, job, "status %s must stay", st)
		assert.Equal(t, st, job.Status)
	}
	assert.Equal(t, 1, emitter.countEvent(EventQueueChanged))
}

func TestService_PausedJobs(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	emitter := &mockEmitter{}
	svc := NewService(NewStore(db), &mockProcessor{}, emitter)

	has, err := svc.HasPausedJobs()
	require.NoError(t, err)
	assert.False(t, has)

	n, err := svc.ResumePausedJobs()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, emitter.countEvent(EventQueueChanged))

	failed := insertJob(t, db, StatusFailed)
	var paused []int64
	for range 2 {
		id := insertJob(t, db, StatusPaused)
		_, err := db.Exec(`UPDATE job_queue SET retry_count = 3, error = 'paused: max retries reached', next_retry_at = '2099-01-01 00:00:00' WHERE id = ?`, id)
		require.NoError(t, err)
		paused = append(paused, id)
	}

	has, err = svc.HasPausedJobs()
	require.NoError(t, err)
	assert.True(t, has)

	n, err = svc.ResumePausedJobs()
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 1, emitter.countEvent(EventQueueChanged))

	for _, id := range paused {
		r := readJobRow(t, db, id)
		assert.Equal(t, string(StatusPending), r.status)
		assert.Equal(t, 0, r.retryCount)
		assert.Empty(t, r.errMsg)
		assert.False(t, r.nextRetryAt.Valid)
	}
	assert.Equal(t, StatusFailed, getJobStatus(t, db, failed))

	has, err = svc.HasPausedJobs()
	require.NoError(t, err)
	assert.False(t, has)
}

func TestStore_ClaimNextRespectsNextRetryAt(t *testing.T) {
	tests := []struct {
		name      string
		nextRetry any
		wantClaim bool
	}{
		{"no retry time", nil, true},
		{"retry time passed", time.Now().Add(-time.Minute).UTC().Format("2006-01-02 15:04:05"), true},
		{"retry time in future", time.Now().Add(time.Hour).UTC().Format("2006-01-02 15:04:05"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			defer db.Close()
			store := NewStore(db)
			id := insertJob(t, db, StatusPending)
			_, err := db.Exec(`UPDATE job_queue SET next_retry_at = ? WHERE id = ?`, tt.nextRetry, id)
			require.NoError(t, err)

			job, err := store.ClaimNext()
			require.NoError(t, err)
			if !tt.wantClaim {
				assert.Nil(t, job)
				assert.Equal(t, StatusPending, getJobStatus(t, db, id))
				return
			}
			require.NotNil(t, job)
			assert.Equal(t, id, job.ID)
			assert.Equal(t, StatusRunning, getJobStatus(t, db, id))
		})
	}
}

func TestStore_ClaimNextSkipsDelayedJobForReadyOne(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := NewStore(db)
	delayed := insertJob(t, db, StatusPending)
	_, err := db.Exec(`UPDATE job_queue SET next_retry_at = ? WHERE id = ?`, time.Now().Add(time.Hour).UTC().Format("2006-01-02 15:04:05"), delayed)
	require.NoError(t, err)
	ready := insertJob(t, db, StatusPending)

	job, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, ready, job.ID)
	assert.Equal(t, StatusPending, getJobStatus(t, db, delayed))
}
