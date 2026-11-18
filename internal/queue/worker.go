package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/session"
)

type Worker struct {
	store     *Store
	processor Processor
	emit      EventEmitter
	notify    chan struct{}

	mu       sync.Mutex
	paused   bool
	current  *Job
	cancelFn context.CancelCauseFunc
	cleanup  func(jobID int64)
	done     chan struct{}
}

func NewWorker(store *Store, processor Processor, emit EventEmitter) *Worker {
	return &Worker{
		store:     store,
		processor: processor,
		emit:      emit,
		notify:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

func (w *Worker) SetCleanup(fn func(jobID int64)) {
	w.mu.Lock()
	w.cleanup = fn
	w.mu.Unlock()
}

type forgetter interface {
	Forget(jobID int64)
}

func (w *Worker) runCleanup(jobID int64) {
	if f, ok := w.processor.(forgetter); ok {
		f.Forget(jobID)
	}
	w.mu.Lock()
	fn := w.cleanup
	w.mu.Unlock()
	if fn != nil {
		fn(jobID)
	}
}

func (w *Worker) Start(ctx context.Context) {
	go w.loop(ctx)
}

func (w *Worker) Done() <-chan struct{} {
	return w.done
}

func (w *Worker) Notify() {
	select {
	case w.notify <- struct{}{}:
	default:
	}
}

func (w *Worker) Pause() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paused = true
}

func (w *Worker) Resume() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paused = false
	w.Notify()
}

func (w *Worker) IsPaused() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.paused
}

func (w *Worker) CancelCurrent() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancelFn != nil {
		w.cancelFn(sd.ErrUserCancel)
	}
}

func (w *Worker) CurrentJob() *Job {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current
}

func (w *Worker) loop(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			if !w.paused {
				w.mu.Unlock()
				w.processNext(ctx)
				w.checkMoreJobs()
			} else {
				w.mu.Unlock()
			}
		case <-w.notify:
			w.mu.Lock()
			if w.paused {
				w.mu.Unlock()
				continue
			}
			w.mu.Unlock()

			w.processNext(ctx)
			w.checkMoreJobs()
		}
	}
}

func (w *Worker) checkMoreJobs() {
	more, err := w.store.HasClaimable()
	if err != nil {
		log.Printf("[queue] check pending jobs: %v", err)
		return
	}
	if more {
		w.Notify()
	}
}

func (w *Worker) processNext(parentCtx context.Context) {
	if parentCtx.Err() != nil {
		return
	}
	job, err := w.store.ClaimNext()
	if err != nil {
		log.Printf("[queue] claim next job: %v", err)
		return
	}
	if job == nil {
		return
	}

	jobCtx, cancel := context.WithCancelCause(parentCtx)
	w.mu.Lock()
	w.current = job
	w.cancelFn = cancel
	w.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[queue] panic in processNext: %v", r)
			w.setStatus(job.ID, StatusFailed, fmt.Sprintf("panic: %v", r))
			w.emit.Emit(EventQueueFailed, EventPayload{JobID: job.ID, Type: job.Type, Error: fmt.Sprintf("panic: %v", r)})
			w.emit.Emit(EventQueueChanged)
			w.runCleanup(job.ID)
		}
		cancel(nil)
		w.mu.Lock()
		w.current = nil
		w.cancelFn = nil
		w.mu.Unlock()
	}()

	w.emit.Emit(EventQueueStarted, EventPayload{JobID: job.ID, Type: job.Type})
	w.emit.Emit(EventQueueChanged)

	result, procErr := w.processor.ProcessJob(jobCtx, job)

	if procErr != nil && parentCtx.Err() != nil {
		if err := w.store.RequeueInterrupted(job.ID); err != nil {
			log.Printf("[queue] job %d: requeue on shutdown: %v", job.ID, err)
		}
		return
	}
	if jobCtx.Err() != nil && parentCtx.Err() == nil {
		current, err := w.store.Get(job.ID)
		if err != nil {
			log.Printf("[queue] job %d: read status after cancel: %v", job.ID, err)
		}
		if current != nil && current.Status == StatusRunning {
			w.setStatus(job.ID, StatusFailed, "cancelled")
		}
		w.emit.Emit(EventQueueFailed, EventPayload{JobID: job.ID, Type: job.Type, Error: "cancelled"})
		w.runCleanup(job.ID)
	} else if procErr != nil {
		errMsg := procErr.Error()
		if IsRetryableError(procErr) && job.RetryCount < job.MaxRetries {
			if err := w.store.IncrementRetry(job.ID, job.RetryCount, job.MaxRetries); err != nil {
				log.Printf("[queue] job %d: schedule retry: %v", job.ID, err)
			}
			w.emit.Emit(EventQueuePaused, EventPayload{
				JobID:      job.ID,
				Error:      errMsg,
				RetryCount: job.RetryCount + 1,
				MaxRetries: job.MaxRetries,
			})
			w.Notify()
		} else {
			w.setStatus(job.ID, StatusFailed, errMsg)
			w.emit.Emit(EventQueueFailed, EventPayload{JobID: job.ID, Type: job.Type, Error: errMsg})
			w.runCleanup(job.ID)
		}
	} else {
		w.complete(job.ID, result)
	}
	w.emit.Emit(EventQueueChanged)
}

func (w *Worker) setStatus(id int64, status JobStatus, errMsg string) {
	if err := w.store.UpdateStatus(id, status, errMsg); err != nil {
		log.Printf("[queue] job %d: set status %s: %v", id, status, err)
	}
}

func (w *Worker) complete(id int64, result *JobResult) {
	defer w.runCleanup(id)
	stored := JobResult{}
	if result != nil {
		stored = *result
		stored.ImageBase64 = ""
	}
	resultJSON, err := json.Marshal(stored)
	if err == nil {
		err = w.store.UpdateCompleted(id, string(resultJSON))
	}
	if err != nil {
		log.Printf("[queue] job %d: save result: %v", id, err)
		msg := "save result: " + err.Error()
		w.setStatus(id, StatusFailed, msg)
		w.emit.Emit(EventQueueFailed, EventPayload{JobID: id, Error: msg})
		return
	}
	w.emit.Emit(EventQueueCompleted, EventPayload{JobID: id, Result: eventResult(result)})
}

func eventResult(result *JobResult) *JobResult {
	if result == nil || result.SessionItemID == 0 || len(result.ImageBase64) > session.MaxStoredImageBase64 {
		return result
	}
	out := *result
	out.ImageBase64 = ""
	return &out
}
