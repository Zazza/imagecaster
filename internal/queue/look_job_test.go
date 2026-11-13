package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
)

func countJobs(t *testing.T, svc *Service) int {
	t.Helper()
	var n int
	if err := svc.store.db.QueryRow(`SELECT COUNT(*) FROM job_queue`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	return n
}

func newLookTestService(t *testing.T) (*Service, *mockEmitter) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	emitter := &mockEmitter{}
	return NewService(NewStore(db), &mockProcessor{}, emitter), emitter
}

func TestEnqueueLook_RejectsEmptyFields(t *testing.T) {
	cases := []struct {
		name   string
		params generation.ApplyLookParams
	}{
		{name: "empty image", params: generation.ApplyLookParams{ImageBase64: "", Text: "neon night look", Strength: 0.5}},
		{name: "empty text", params: generation.ApplyLookParams{ImageBase64: "aGVsbG8=", Text: "", Strength: 0.5}},
		{name: "both empty", params: generation.ApplyLookParams{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, emitter := newLookTestService(t)

			_, err := svc.Enqueue(JobLook, tc.params, "look")

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if n := countJobs(t, svc); n != 0 {
				t.Errorf("job count = %d, want 0", n)
			}
			if emitter.hasEvent(EventQueueChanged) {
				t.Error("expected no EventQueueChanged for rejected job")
			}
		})
	}
}

func TestEnqueueLook_ValidParamsCreateLookJob(t *testing.T) {
	svc, _ := newLookTestService(t)
	seed := int64(42)

	id, err := svc.Enqueue(JobLook, generation.ApplyLookParams{ImageBase64: "aGVsbG8=", Text: "neon night look", Strength: 0.5, Seed: &seed}, "look")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.Type != JobLook {
		t.Errorf("type = %q, want %q", job.Type, JobLook)
	}
	for _, want := range []string{`"text":"neon night look"`, `"image_base64":"aGVsbG8="`, `"seed":42`} {
		if !strings.Contains(job.Params, want) {
			t.Errorf("params = %s, want it to contain %s", job.Params, want)
		}
	}
}

func TestProcessJob_LookIsDispatched(t *testing.T) {
	p := NewProcessor(nil, nil, nil, t.TempDir(), &mockEmitter{})
	job := &Job{Type: JobLook, Params: "not json"}

	var res *JobResult
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ProcessJob panicked on look job: %v", r)
			}
		}()
		res, err = p.ProcessJob(context.Background(), job)
	}()

	if err == nil {
		t.Fatal("expected error for invalid look params, got nil")
	}
	if strings.Contains(err.Error(), "unknown job type") {
		t.Errorf("error = %q: look job is not dispatched", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid params") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "invalid params")
	}
	if res != nil {
		t.Errorf("result = %+v, want nil", res)
	}
}
