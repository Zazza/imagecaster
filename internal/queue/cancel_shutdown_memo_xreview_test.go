package queue

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

func xrMemoFile(dataDir string, id int64) string {
	return filepath.Join(dataDir, "queue-llm", strconv.FormatInt(id, 10)+".json")
}

func xrExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCancelJob_RunningWithoutInterruptFnXReview(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	store := NewStore(db)
	id, err := store.Create(JobTxt2Img, "{}", "test")
	require.NoError(t, err)

	started := make(chan struct{})
	release := make(chan struct{})
	causeCh := make(chan error, 1)
	proc := &mockProcessor{processFn: func(ctx context.Context, _ *Job) (*JobResult, error) {
		close(started)
		<-ctx.Done()
		causeCh <- context.Cause(ctx)
		<-release
		return nil, ctx.Err()
	}}
	svc := NewService(store, proc, &mockEmitter{})
	svc.Start(context.Background())
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("job did not start")
	}

	require.NoError(t, svc.CancelJob(id))

	select {
	case c := <-causeCh:
		assert.ErrorIs(t, c, sd.ErrUserCancel)
	case <-time.After(5 * time.Second):
		t.Fatal("job context was not cancelled")
	}
	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, job.Status)
	assert.Equal(t, "cancelled by user", job.Error)

	close(release)
	svc.Stop(5 * time.Second)
	job, err = store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, job.Status)
}

func TestProcessJob_ShutdownDropsResultXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	sessions := &xrSessions{}
	fsd := &xrSD{}
	proc, _ := env.processor(t, &xrLLM{variant: "A"}, fsd, sessions)
	id := env.ideaJob(t)
	job, err := env.store.Get(id)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fsd.onTxt = cancel

	res, err := proc.ProcessJob(ctx, job)

	require.Len(t, fsd.txtPrompts(), 1)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 0, sessions.count(), "result must not be saved after shutdown")
}

func TestWorker_ShutdownDuringGenerationRequeuesXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	sessions := &xrSessions{}
	fsd := &xrSD{}
	l := &xrLLM{variant: "A"}
	proc, scoper := env.processor(t, l, fsd, sessions)
	id := env.ideaJob(t)

	w := NewWorker(env.store, proc, &mockEmitter{})
	var cleaned atomic.Int64
	w.SetCleanup(func(jobID int64) { cleaned.Store(jobID) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fsd.onTxt = cancel

	w.processNext(ctx)

	require.Len(t, fsd.txtPrompts(), 1)
	require.GreaterOrEqual(t, l.count(), 1)
	job, err := env.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, job.Status)
	assert.Equal(t, 0, job.RetryCount)
	assert.Equal(t, int64(0), cleaned.Load(), "cleanup must not run on shutdown requeue")
	scoper.mu.Lock()
	assert.Empty(t, scoper.cleaned)
	scoper.mu.Unlock()
	assert.Equal(t, 0, sessions.count())
	assert.True(t, xrExists(xrMemoFile(env.dataDir, id)), "llm memo kept on shutdown requeue")
}

func TestWorker_UserCancelDuringGenerationFailsXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	fsd := &xrSD{}
	proc, _ := env.processor(t, &xrLLM{variant: "A"}, fsd, &xrSessions{})
	id := env.ideaJob(t)

	w := NewWorker(env.store, proc, &mockEmitter{})
	var cleaned atomic.Int64
	w.SetCleanup(func(jobID int64) { cleaned.Store(jobID) })
	var memoDuring atomic.Bool
	fsd.onTxt = func() {
		memoDuring.Store(xrExists(xrMemoFile(env.dataDir, id)))
		w.CancelCurrent()
	}

	w.processNext(context.Background())

	require.Len(t, fsd.txtPrompts(), 1)
	job, err := env.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, job.Status)
	assert.Contains(t, job.Error, "cancelled")
	assert.Equal(t, id, cleaned.Load())
	assert.True(t, memoDuring.Load(), "llm memo exists while the job runs")
	assert.False(t, xrExists(xrMemoFile(env.dataDir, id)), "llm memo removed after cancel")
}

func TestLLMMemo_RestartReusesAnswersXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	id := env.ideaJob(t)

	sdA := &xrSD{}
	llmA := &xrLLM{variant: "A"}
	procA, _ := env.processor(t, llmA, sdA, &xrSessions{})
	wA := NewWorker(env.store, procA, &mockEmitter{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sdA.onTxt = cancel
	wA.processNext(ctx)

	promptsA := sdA.txtPrompts()
	require.Len(t, promptsA, 1)
	require.GreaterOrEqual(t, llmA.count(), 1)
	require.Contains(t, promptsA[0], "A-tags-1")
	job, err := env.store.Get(id)
	require.NoError(t, err)
	require.Equal(t, StatusPending, job.Status)

	sdB := &xrSD{}
	llmB := &xrLLM{variant: "B"}
	sessionsB := &xrSessions{}
	procB, _ := env.processor(t, llmB, sdB, sessionsB)
	wB := NewWorker(env.store, procB, &mockEmitter{})
	wB.processNext(context.Background())

	assert.Equal(t, promptsA, sdB.txtPrompts(), "same prompts after restart")
	assert.Equal(t, 0, llmB.count(), "llm answers reused from memo")
	job, err = env.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status)
	assert.Equal(t, 1, sessionsB.count())
	assert.False(t, xrExists(xrMemoFile(env.dataDir, id)), "llm memo removed on completion")
}

func TestLLMMemo_RepeatedCallsAreSeparateEntriesXReview(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := xrMemoFile(dir, 7)

	first := &xrLLM{variant: "A"}
	m := newLLMMemo(first, path)
	r1, err := m.GenerateSDPrompt("sys", "same description", "anime", "model", 100)
	require.NoError(t, err)
	r2, err := m.GenerateSDPrompt("sys", "same description", "anime", "model", 100)
	require.NoError(t, err)
	assert.NotEqual(t, r1, r2, "second identical call must not be served from the first answer")
	assert.Equal(t, 2, first.count())
	assert.True(t, xrExists(path))

	second := &xrLLM{variant: "B"}
	m2 := newLLMMemo(second, path)
	g1, err := m2.GenerateSDPrompt("sys", "same description", "anime", "model", 100)
	require.NoError(t, err)
	g2, err := m2.GenerateSDPrompt("sys", "same description", "anime", "model", 100)
	require.NoError(t, err)
	assert.Equal(t, r1, g1)
	assert.Equal(t, r2, g2)
	assert.Equal(t, 0, second.count())

	g3, err := m2.GenerateSDPrompt("sys", "same description", "anime", "model", 100)
	require.NoError(t, err)
	assert.Equal(t, 1, second.count(), "a call beyond the memo goes to the llm")
	assert.Contains(t, g3, "B-tags-1")
}

func TestLLMMemo_RemovalOnJobEndXReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		txtErr     error
		wantStatus JobStatus
		wantKept   bool
	}{
		{"failed removes memo", errors.New("invalid request: model rejected the prompt"), StatusFailed, false},
		{"retry keeps memo", errors.New("dial tcp 127.0.0.1:7860: connect: connection refused"), StatusPending, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.wantKept, IsRetryableError(tt.txtErr))
			env := newXREnv(t)
			fsd := &xrSD{txtErr: tt.txtErr}
			proc, _ := env.processor(t, &xrLLM{variant: "A"}, fsd, &xrSessions{})
			id := env.ideaJob(t)
			var memoDuring atomic.Bool
			fsd.onTxt = func() { memoDuring.Store(xrExists(xrMemoFile(env.dataDir, id))) }

			w := NewWorker(env.store, proc, &mockEmitter{})
			w.processNext(context.Background())

			require.Len(t, fsd.txtPrompts(), 1)
			job, err := env.store.Get(id)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, job.Status)
			assert.True(t, memoDuring.Load())
			assert.Equal(t, tt.wantKept, xrExists(xrMemoFile(env.dataDir, id)))
		})
	}
}

func TestLLMMemo_ServiceCancelPendingRemovesMemoXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	proc, _ := env.processor(t, &xrLLM{variant: "A"}, &xrSD{}, &xrSessions{})
	id := env.ideaJob(t)
	path := xrMemoFile(env.dataDir, id)
	_, err := newLLMMemo(&xrLLM{variant: "A"}, path).GenerateSDPrompt("sys", "d", "anime", "m", 10)
	require.NoError(t, err)
	require.True(t, xrExists(path))

	svc := NewService(env.store, proc, &mockEmitter{})
	require.NoError(t, svc.CancelJob(id))

	job, err := env.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, job.Status)
	assert.False(t, xrExists(path), "llm memo removed when a pending job is cancelled")
}
