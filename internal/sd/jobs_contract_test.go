package sd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const jobsPrefix = "/api/sd/jobs"

type jobPlan struct {
	polls   int
	final   string
	errCode string
	errMsg  string
}

type fakeJob struct {
	id     string
	ref    string
	kind   string
	client string
	status string
	polls  int
	plan   jobPlan
}

type fakeSD struct {
	t  *testing.T
	ts *httptest.Server

	mu       sync.Mutex
	paths    []string
	submits  []map[string]any
	jobs     map[string]*fakeJob
	byRef    map[string]string
	nextID   int
	gets     int
	cancels  []string
	deletes  []string
	listQ    []string
	seedList []*fakeJob

	plan        func(n int) jobPlan
	submitFault func(n int) string
	pollFault   func(n int) string
}

func newFakeSD(t *testing.T) *fakeSD {
	t.Helper()
	f := &fakeSD{
		t:     t,
		jobs:  map[string]*fakeJob{},
		byRef: map[string]string{},
		plan:  func(int) jobPlan { return jobPlan{polls: 1, final: "completed"} },
	}
	f.ts = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(func() {
		f.ts.Close()
		for _, p := range f.allPaths() {
			assert.NotContains(t, p, "/sdapi", "A1: request to the A1111 layer")
		}
	})
	return f
}

func (f *fakeSD) client() *Client {
	c := New(f.ts.URL + "/api/sd")
	c.PollInterval = 5 * time.Millisecond
	return c
}

func (f *fakeSD) allPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

func (f *fakeSD) submitBodies() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.submits...)
}

func (f *fakeSD) submitRefs() []string {
	var refs []string
	for _, b := range f.submitBodies() {
		s, _ := b["client_ref"].(string)
		refs = append(refs, s)
	}
	return refs
}

func (f *fakeSD) cancelled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cancels...)
}

func (f *fakeSD) deleted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deletes...)
}

func (f *fakeSD) getCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets
}

func hangUp(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "no hijack", http.StatusInternalServerError)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	_ = conn.Close()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"code": errCode, "message": msg}})
}

func jobJSON(j *fakeJob) map[string]any {
	m := map[string]any{
		"id":          j.id,
		"kind":        j.kind,
		"client":      j.client,
		"client_ref":  j.ref,
		"status":      j.status,
		"position":    0,
		"progress":    map[string]any{"step": j.polls, "steps": 10},
		"error":       nil,
		"created_at":  "2026-11-13T10:00:00Z",
		"started_at":  nil,
		"finished_at": nil,
		"expires_at":  nil,
	}
	if j.status == "queued" {
		m["position"] = 1
		m["progress"] = map[string]any{"step": 0, "steps": 10}
	}
	if j.status == "running" {
		m["started_at"] = "2026-11-13T10:00:01Z"
	}
	switch j.status {
	case "completed", "failed", "cancelled", "lost":
		m["started_at"] = "2026-11-13T10:00:01Z"
		m["finished_at"] = "2026-11-13T10:00:05Z"
	}
	if j.status == "completed" {
		m["expires_at"] = "2026-11-14T10:00:05Z"
	}
	if j.plan.errCode != "" && (j.status == "failed" || j.status == "lost") {
		m["error"] = map[string]any{"code": j.plan.errCode, "message": j.plan.errMsg}
	}
	return m
}

func (f *fakeSD) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	f.mu.Unlock()

	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && p == jobsPrefix:
		f.handleSubmit(w, r)
	case r.Method == http.MethodGet && p == jobsPrefix:
		f.handleList(w, r)
	case strings.HasPrefix(p, jobsPrefix+"/"):
		rest := strings.TrimPrefix(p, jobsPrefix+"/")
		parts := strings.Split(rest, "/")
		id := parts[0]
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			f.handleGet(w, id)
		case len(parts) == 1 && r.Method == http.MethodDelete:
			f.handleDelete(w, id)
		case len(parts) == 2 && parts[1] == "result" && r.Method == http.MethodGet:
			f.handleResult(w, id)
		case len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost:
			f.handleCancel(w, id)
		default:
			writeAPIError(w, http.StatusNotFound, "not_found", "no route")
		}
	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "no route")
	}
}

func (f *fakeSD) handleSubmit(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_params", "bad json")
		return
	}
	f.mu.Lock()
	f.submits = append(f.submits, body)
	n := len(f.submits)
	fault := ""
	if f.submitFault != nil {
		fault = f.submitFault(n)
	}
	f.mu.Unlock()

	switch fault {
	case "netdown":
		hangUp(w)
		return
	case "500":
		writeAPIError(w, http.StatusInternalServerError, "sd_failed", "server exploded")
		return
	case "503":
		writeAPIError(w, http.StatusServiceUnavailable, "not_ready", "server busy")
		return
	case "400":
		writeAPIError(w, http.StatusBadRequest, "invalid_params", "unknown sampler Euler a")
		return
	case "429":
		writeAPIError(w, http.StatusTooManyRequests, "queue_full", "queue is full")
		return
	}

	ref, _ := body["client_ref"].(string)
	kind, _ := body["kind"].(string)
	client, _ := body["client"].(string)

	f.mu.Lock()
	defer f.mu.Unlock()
	if ref != "" {
		if id, ok := f.byRef[ref]; ok {
			if j, ok := f.jobs[id]; ok && j.status != "lost" {
				writeJSON(w, http.StatusOK, jobJSON(j))
				return
			}
		}
	}
	f.nextID++
	j := &fakeJob{
		id:     fmt.Sprintf("job-%d", f.nextID),
		ref:    ref,
		kind:   kind,
		client: client,
		status: "queued",
		plan:   f.plan(n),
	}
	f.jobs[j.id] = j
	if ref != "" {
		f.byRef[ref] = j.id
	}
	writeJSON(w, http.StatusAccepted, jobJSON(j))
}

func (f *fakeSD) handleGet(w http.ResponseWriter, id string) {
	f.mu.Lock()
	f.gets++
	n := f.gets
	fault := ""
	if f.pollFault != nil {
		fault = f.pollFault(n)
	}
	f.mu.Unlock()
	if fault == "netdown" {
		hangUp(w)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	switch j.status {
	case "queued", "running":
		j.polls++
		if j.polls >= j.plan.polls {
			if j.plan.final == "vanish" {
				delete(f.jobs, id)
				writeAPIError(w, http.StatusNotFound, "not_found", "job not found")
				return
			}
			j.status = j.plan.final
		} else {
			j.status = "running"
		}
	}
	writeJSON(w, http.StatusOK, jobJSON(j))
}

func (f *fakeSD) handleResult(w http.ResponseWriter, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	if j.status != "completed" {
		writeAPIError(w, http.StatusConflict, "not_ready", "not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"images": []string{"img-" + j.id},
		"info":   map[string]any{"seed": 123, "prompt": "p-" + j.id},
	})
}

func (f *fakeSD) handleCancel(w http.ResponseWriter, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, id)
	j, ok := f.jobs[id]
	if !ok {
		for _, s := range f.seedList {
			if s.id == id {
				j = s
			}
		}
	}
	if j == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	if j.status == "queued" || j.status == "running" {
		j.status = "cancelled"
	}
	writeJSON(w, http.StatusOK, jobJSON(j))
}

func (f *fakeSD) handleDelete(w http.ResponseWriter, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, id)
	j, ok := f.jobs[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	delete(f.jobs, id)
	if j.ref != "" {
		delete(f.byRef, j.ref)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeSD) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("client")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listQ = append(f.listQ, q)
	out := []map[string]any{}
	for _, j := range f.seedList {
		if q != "" && j.client != q {
			continue
		}
		out = append(out, jobJSON(j))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
}

func waitUntil(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", msg)
}

func upscaleReq(ref string) JobRequest {
	return JobRequest{
		Kind:      "upscale",
		ClientRef: ref,
		Params:    map[string]any{"image": "aW1n", "upscaler": "lanczos", "scale": 2},
	}
}

func runCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSubmitJob_RequestBodyShape(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	c := f.client()

	st, err := c.SubmitJob(runCtx(t), JobRequest{
		Kind:      "txt2img",
		ClientRef: "ref-1",
		Model: ModelSpec{
			Checkpoint:     "flux/flux1-dev.safetensors",
			VAE:            "ae.safetensors",
			Modules:        []string{"clip_l.safetensors", "t5xxl.safetensors"},
			KleinReference: true,
			MossReference:  false,
		},
		Params: map[string]any{"prompt": "a cat", "steps": 20, "sampler": "euler_a"},
	})
	require.NoError(t, err)
	require.NotNil(t, st)

	bodies := f.submitBodies()
	require.Len(t, bodies, 1)
	b := bodies[0]
	assert.Equal(t, "txt2img", b["kind"])
	assert.Equal(t, "imagecaster", b["client"])
	assert.Equal(t, "ref-1", b["client_ref"])

	model, ok := b["model"].(map[string]any)
	require.True(t, ok, "model object: %v", b["model"])
	assert.Equal(t, "flux/flux1-dev.safetensors", model["checkpoint"])
	assert.Equal(t, "ae.safetensors", model["vae"])
	assert.Equal(t, []any{"clip_l.safetensors", "t5xxl.safetensors"}, model["modules"])
	assert.Equal(t, true, model["klein_reference"])
	moss, present := model["moss_reference"]
	assert.True(t, present, "moss_reference must be sent")
	assert.Equal(t, false, moss)

	params, ok := b["params"].(map[string]any)
	require.True(t, ok, "params object: %v", b["params"])
	assert.Equal(t, "a cat", params["prompt"])
	assert.EqualValues(t, 20, params["steps"])
	assert.Equal(t, "euler_a", params["sampler"])

	assert.Equal(t, "job-1", st.ID)
	assert.Equal(t, "queued", st.Status)
	assert.Equal(t, "ref-1", st.ClientRef)
	assert.EqualValues(t, 1, st.Position)
}

func TestSubmitJob_ServerErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fault   string
		wantMsg string
	}{
		{name: "400 invalid_params", fault: "400", wantMsg: "unknown sampler Euler a"},
		{name: "429 queue_full", fault: "429", wantMsg: "queue is full"},
		{name: "500", fault: "500", wantMsg: "server exploded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeSD(t)
			f.submitFault = func(int) string { return tt.fault }
			st, err := f.client().SubmitJob(runCtx(t), upscaleReq("r"))
			require.Error(t, err)
			assert.Nil(t, st)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestGetJob_ParsesStatus(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.plan = func(int) jobPlan {
		return jobPlan{polls: 1, final: "failed", errCode: "model_not_found", errMsg: "checkpoint x.safetensors not found"}
	}
	c := f.client()
	ctx := runCtx(t)
	st, err := c.SubmitJob(ctx, upscaleReq("r-get"))
	require.NoError(t, err)

	got, err := c.GetJob(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, st.ID, got.ID)
	assert.Equal(t, "upscale", got.Kind)
	assert.Equal(t, "imagecaster", got.Client)
	assert.Equal(t, "r-get", got.ClientRef)
	assert.Equal(t, "failed", got.Status)
	assert.EqualValues(t, 0, got.Position)
	assert.EqualValues(t, 1, got.Progress.Step)
	assert.EqualValues(t, 10, got.Progress.Steps)
	require.NotNil(t, got.Error)
	assert.Equal(t, "model_not_found", got.Error.Code)
	assert.Equal(t, "checkpoint x.safetensors not found", got.Error.Message)
}

func TestGetJob_NotFound(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	_, err := f.client().GetJob(runCtx(t), "missing")
	assert.Error(t, err)
}

func TestJobResult_CancelJob_DeleteJob(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	c := f.client()
	ctx := runCtx(t)
	st, err := c.SubmitJob(ctx, upscaleReq("r-res"))
	require.NoError(t, err)

	_, err = c.JobResult(ctx, st.ID)
	assert.Error(t, err, "409 not_ready must be an error")

	_, err = c.GetJob(ctx, st.ID)
	require.NoError(t, err)
	res, err := c.JobResult(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"img-" + st.ID}, res.Images)
	assert.JSONEq(t, `{"seed":123,"prompt":"p-`+st.ID+`"}`, string(res.Info))

	require.NoError(t, c.CancelJob(ctx, st.ID))
	require.NoError(t, c.CancelJob(ctx, st.ID))
	assert.Equal(t, []string{st.ID, st.ID}, f.cancelled())

	require.NoError(t, c.DeleteJob(ctx, st.ID))
	assert.Equal(t, []string{st.ID}, f.deleted())
}

func TestListJobs_PassesClientFilter(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.seedList = []*fakeJob{
		{id: "a", client: "imagecaster", status: "running"},
		{id: "b", client: "other", status: "running"},
	}
	jobs, err := f.client().ListJobs(runCtx(t), "imagecaster")
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "a", jobs[0].ID)
	assert.Equal(t, "running", jobs[0].Status)
}

func TestRunJob_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.plan = func(int) jobPlan { return jobPlan{polls: 3, final: "completed"} }
	c := f.client()

	var mu sync.Mutex
	var progress []JobStatus
	res, err := c.RunJob(runCtx(t), upscaleReq("happy"), func(s JobStatus) {
		mu.Lock()
		progress = append(progress, s)
		mu.Unlock()
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, []string{"img-job-1"}, res.Images)
	assert.JSONEq(t, `{"seed":123,"prompt":"p-job-1"}`, string(res.Info))

	assert.Len(t, f.submitBodies(), 1)
	assert.Equal(t, []string{"job-1"}, f.deleted(), "result must be deleted after it is taken")
	assert.Empty(t, f.cancelled())

	paths := f.allPaths()
	require.NotEmpty(t, paths)
	assert.Equal(t, "POST "+jobsPrefix, paths[0])
	assert.Equal(t, "DELETE "+jobsPrefix+"/job-1", paths[len(paths)-1])
	resultIdx, deleteIdx := -1, -1
	for i, p := range paths {
		switch p {
		case "GET " + jobsPrefix + "/job-1/result":
			resultIdx = i
		case "DELETE " + jobsPrefix + "/job-1":
			deleteIdx = i
		}
	}
	require.NotEqual(t, -1, resultIdx, "result fetched")
	assert.Less(t, resultIdx, deleteIdx, "DELETE after result")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, progress, "onProgress called")
	sawRunning := false
	for _, s := range progress {
		assert.Equal(t, "job-1", s.ID)
		if s.Status == "running" && s.Progress.Steps == 10 && s.Progress.Step > 0 {
			sawRunning = true
		}
	}
	assert.True(t, sawRunning, "progress of running job reported")
}

func TestRunJob_NilProgressCallback(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	res, err := f.client().RunJob(runCtx(t), upscaleReq("nil-cb"), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"img-job-1"}, res.Images)
}

func TestRunJob_NetworkErrorDuringPollDoesNotAbort(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.plan = func(int) jobPlan { return jobPlan{polls: 2, final: "completed"} }
	f.pollFault = func(n int) string {
		if n <= 3 {
			return "netdown"
		}
		return ""
	}
	res, err := f.client().RunJob(runCtx(t), upscaleReq("net-poll"), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"img-job-1"}, res.Images)
	assert.Len(t, f.submitBodies(), 1, "poll network error must not resubmit")
	assert.Empty(t, f.cancelled())
}

func TestRunJob_LostStepResubmittedOnce(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		lostPlan  jobPlan
		lostTimes int
		wantErr   bool
	}{
		{name: "lost then ok", lostPlan: jobPlan{polls: 1, final: "lost", errCode: "server_restarted", errMsg: "server restarted"}, lostTimes: 1},
		{name: "404 then ok", lostPlan: jobPlan{polls: 1, final: "vanish"}, lostTimes: 1},
		{name: "lost twice", lostPlan: jobPlan{polls: 1, final: "lost", errCode: "server_restarted", errMsg: "server restarted"}, lostTimes: 2, wantErr: true},
		{name: "404 twice", lostPlan: jobPlan{polls: 1, final: "vanish"}, lostTimes: 2, wantErr: true},
		{name: "lost then 404", lostPlan: jobPlan{polls: 1, final: "lost", errCode: "server_restarted", errMsg: "server restarted"}, lostTimes: 2, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeSD(t)
			f.plan = func(n int) jobPlan {
				if n <= tt.lostTimes {
					if tt.name == "lost then 404" && n == 2 {
						return jobPlan{polls: 1, final: "vanish"}
					}
					return tt.lostPlan
				}
				return jobPlan{polls: 1, final: "completed"}
			}
			res, err := f.client().RunJob(runCtx(t), upscaleReq("q7:s1:abcdef01"), nil)

			refs := f.submitRefs()
			require.Len(t, refs, 2, "exactly one resubmit")
			assert.Equal(t, []string{"q7:s1:abcdef01", "q7:s1:abcdef01"}, refs, "resubmit with the same ClientRef")
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrJobLost)
				assert.Nil(t, res)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"img-job-2"}, res.Images)
		})
	}
}

func TestRunJob_FailedCarriesServerMessage(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.plan = func(int) jobPlan {
		return jobPlan{polls: 2, final: "failed", errCode: "model_not_found", errMsg: "checkpoint flux/missing.safetensors not found"}
	}
	res, err := f.client().RunJob(runCtx(t), upscaleReq("failed"), nil)
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "checkpoint flux/missing.safetensors not found")
	assert.NotErrorIs(t, err, ErrJobLost)
	assert.Len(t, f.submitBodies(), 1, "failed job is not resubmitted")
}

func TestRunJob_SubmitNetworkErrorRetriedWithSameRef(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.submitFault = func(n int) string {
		if n == 1 {
			return "netdown"
		}
		return ""
	}
	res, err := f.client().RunJob(runCtx(t), upscaleReq("net-submit"), nil)
	require.NoError(t, err)
	require.NotNil(t, res)
	refs := f.submitRefs()
	require.GreaterOrEqual(t, len(refs), 2)
	for _, r := range refs {
		assert.Equal(t, "net-submit", r)
	}
}

func TestRunJob_SubmitNetworkErrorGivesUp(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.submitFault = func(int) string { return "netdown" }
	res, err := f.client().RunJob(runCtx(t), upscaleReq("net-down"), nil)
	require.Error(t, err)
	assert.Nil(t, res)
	refs := f.submitRefs()
	assert.GreaterOrEqual(t, len(refs), 2, "network error on submit is retried")
	assert.LessOrEqual(t, len(refs), 4, "submit retries are bounded (card: up to 3)")
	for _, r := range refs {
		assert.Equal(t, "net-down", r)
	}
}

func TestRunJob_SubmitHTTPErrorsNotRetried(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"400", "429", "500", "503"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			f := newFakeSD(t)
			f.submitFault = func(int) string { return fault }
			res, err := f.client().RunJob(runCtx(t), upscaleReq("http-"+fault), nil)
			require.Error(t, err)
			assert.Nil(t, res)
			assert.Len(t, f.submitBodies(), 1)
		})
	}
}

func TestRunJob_ContextCancel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		cause      error
		wantCancel bool
	}{
		{name: "user cancel", cause: ErrUserCancel, wantCancel: true},
		{name: "app shutdown", cause: errors.New("app shutdown"), wantCancel: false},
		{name: "plain cancel", cause: nil, wantCancel: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeSD(t)
			f.plan = func(int) jobPlan { return jobPlan{polls: 1 << 30, final: "completed"} }
			c := f.client()
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)

			done := make(chan error, 1)
			go func() {
				_, err := c.RunJob(ctx, upscaleReq("cancel-"+tt.name), nil)
				done <- err
			}()
			waitUntil(t, 3*time.Second, func() bool { return f.getCount() >= 2 }, "polling started")
			cancel(tt.cause)

			var err error
			select {
			case err = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("RunJob did not return after ctx cancel")
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, ErrUserCancel), "ctx error expected, got %v", err)

			if tt.wantCancel {
				waitUntil(t, 2*time.Second, func() bool { return len(f.cancelled()) > 0 }, "CancelJob called")
				assert.Equal(t, []string{"job-1"}, f.cancelled())
			} else {
				time.Sleep(100 * time.Millisecond)
				assert.Empty(t, f.cancelled(), "job on the server must not be touched")
				assert.Empty(t, f.deleted(), "job on the server must not be deleted")
			}
		})
	}
}

var queueRefRe = regexp.MustCompile(`^[0-9a-f]{8,}:q(\d+):s(\d+):([0-9a-f]{8})$`)

func parseQueueRef(t *testing.T, ref string) (string, string, string) {
	t.Helper()
	m := queueRefRe.FindStringSubmatch(ref)
	require.NotNil(t, m, "ref %q does not match <install>:q<id>:s<seq>:<hash8>", ref)
	return m[1], m[2], m[3]
}

func TestForQueueJob_ClientRefs(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	c := f.client()
	ctx := runCtx(t)

	s := c.ForQueueJob(ctx, 42)
	out1, err := s.UpscaleImage("aW1nMQ==", "lanczos", 2)
	require.NoError(t, err)
	assert.NotEmpty(t, out1)
	_, err = s.UpscaleImage("aW1nMQ==", "lanczos", 2)
	require.NoError(t, err)

	refs := f.submitRefs()
	require.Len(t, refs, 2)
	id1, seq1, h1 := parseQueueRef(t, refs[0])
	id2, seq2, h2 := parseQueueRef(t, refs[1])
	assert.Equal(t, "42", id1)
	assert.Equal(t, "42", id2)
	assert.Equal(t, "1", seq1)
	assert.Equal(t, "2", seq2)
	assert.Equal(t, h1, h2, "identical requests give identical hash8")

	again := c.ForQueueJob(ctx, 42)
	_, err = again.UpscaleImage("aW1nMQ==", "lanczos", 2)
	require.NoError(t, err)
	_, err = again.UpscaleImage("aW1nMQ==", "lanczos", 2)
	require.NoError(t, err)
	refs = f.submitRefs()
	require.Len(t, refs, 4)
	assert.Equal(t, refs[0], refs[2], "new ForQueueJob with same id restarts seq")
	assert.Equal(t, refs[1], refs[3])

	other := c.ForQueueJob(ctx, 43)
	_, err = other.UpscaleImage("aW1nMQ==", "lanczos", 2)
	require.NoError(t, err)
	refs = f.submitRefs()
	id5, seq5, h5 := parseQueueRef(t, refs[4])
	assert.Equal(t, "43", id5)
	assert.Equal(t, "1", seq5)
	assert.Equal(t, h1, h5, "hash8 depends on the request, not on the queue id")
}

func TestForQueueJob_DifferentParamsDifferentHash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		image    string
		upscaler string
		scale    float64
	}{
		{name: "other scale", image: "aW1nMQ==", upscaler: "lanczos", scale: 4},
		{name: "other upscaler", image: "aW1nMQ==", upscaler: "RealESRGAN_x4plus", scale: 2},
		{name: "other image", image: "aW1nMg==", upscaler: "lanczos", scale: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeSD(t)
			c := f.client()
			ctx := runCtx(t)
			_, err := c.ForQueueJob(ctx, 5).UpscaleImage("aW1nMQ==", "lanczos", 2)
			require.NoError(t, err)
			_, err = c.ForQueueJob(ctx, 5).UpscaleImage(tt.image, tt.upscaler, tt.scale)
			require.NoError(t, err)
			refs := f.submitRefs()
			require.Len(t, refs, 2)
			_, seqA, hA := parseQueueRef(t, refs[0])
			_, seqB, hB := parseQueueRef(t, refs[1])
			assert.Equal(t, "1", seqA)
			assert.Equal(t, "1", seqB)
			assert.NotEqual(t, hA, hB)
		})
	}
}

func TestForQueueJob_RepeatedChainReattachesToFinishedStep(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.plan = func(int) jobPlan { return jobPlan{polls: 1 << 30, final: "completed"} }
	c := f.client()
	ctx, cancel := context.WithCancelCause(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := c.ForQueueJob(ctx, 9).UpscaleImage("aW1nMQ==", "lanczos", 2)
		done <- err
	}()
	waitUntil(t, 3*time.Second, func() bool { return f.getCount() >= 1 }, "polling started")
	cancel(errors.New("app shutdown"))
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("UpscaleImage did not return after ctx cancel")
	}
	assert.Empty(t, f.cancelled(), "closing the app does not cancel the queue job on the server")

	f.mu.Lock()
	for _, j := range f.jobs {
		j.status = "completed"
	}
	f.mu.Unlock()

	out, err := c.ForQueueJob(runCtx(t), 9).UpscaleImage("aW1nMQ==", "lanczos", 2)
	require.NoError(t, err)
	assert.Equal(t, "img-job-1", out, "after restart the finished step is taken, not recomputed")
	refs := f.submitRefs()
	require.Len(t, refs, 2)
	assert.Equal(t, refs[0], refs[1])
}

func TestForQueueJob_UserCancelCancelsOnServer(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.plan = func(int) jobPlan { return jobPlan{polls: 1 << 30, final: "completed"} }
	c := f.client()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	done := make(chan error, 1)
	go func() {
		_, err := c.ForQueueJob(ctx, 11).UpscaleImage("aW1nMQ==", "lanczos", 2)
		done <- err
	}()
	waitUntil(t, 3*time.Second, func() bool { return f.getCount() >= 1 }, "polling started")
	cancel(ErrUserCancel)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("UpscaleImage did not return after user cancel")
	}
	waitUntil(t, 2*time.Second, func() bool { return len(f.cancelled()) > 0 }, "CancelJob called")
	assert.Equal(t, []string{"job-1"}, f.cancelled())
}

func TestDirectUpscale_SubmitAndWait(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	f.plan = func(int) jobPlan { return jobPlan{polls: 2, final: "completed"} }
	c := f.client()

	out, err := c.UpscaleImage("aW1nMQ==", "RealESRGAN_x4plus", 2)
	require.NoError(t, err)
	assert.Equal(t, "img-job-1", out)
	_, err = c.UpscaleImage("aW1nMQ==", "RealESRGAN_x4plus", 2)
	require.NoError(t, err)

	bodies := f.submitBodies()
	require.Len(t, bodies, 2)
	assert.Equal(t, "upscale", bodies[0]["kind"])
	assert.Equal(t, "imagecaster", bodies[0]["client"])
	params, ok := bodies[0]["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "aW1nMQ==", params["image"])
	assert.Equal(t, "RealESRGAN_x4plus", params["upscaler"])
	assert.EqualValues(t, 2, params["scale"])

	refs := f.submitRefs()
	assert.NotEmpty(t, refs[0])
	assert.NotEmpty(t, refs[1])
	assert.NotEqual(t, refs[0], refs[1], "direct calls use a random ClientRef")
	assert.False(t, queueRefRe.MatchString(refs[0]), "direct call ref is not a queue ref")
	assert.ElementsMatch(t, []string{"job-1", "job-2"}, f.deleted())
}

func TestInterrupt_CancelsUnfinishedImagecasterJobs(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	c := f.client()
	c.SetInstallID("aaaa1111")
	f.seedList = []*fakeJob{
		{id: "q", client: "imagecaster", ref: "aaaa1111:q1:s1:abcdef01", status: "queued"},
		{id: "r", client: "imagecaster", ref: "aaaa1111:d:01", status: "running"},
		{id: "c", client: "imagecaster", ref: "aaaa1111:d:02", status: "completed"},
		{id: "f", client: "imagecaster", ref: "aaaa1111:d:03", status: "failed"},
		{id: "x", client: "imagecaster", ref: "aaaa1111:d:04", status: "cancelled"},
		{id: "l", client: "imagecaster", ref: "aaaa1111:d:05", status: "lost"},
		{id: "o", client: "other-app", ref: "aaaa1111:d:06", status: "running"},
	}
	require.NoError(t, c.Interrupt())
	assert.ElementsMatch(t, []string{"q", "r"}, f.cancelled())
}

func TestInterrupt_NoJobs(t *testing.T) {
	t.Parallel()
	f := newFakeSD(t)
	require.NoError(t, f.client().Interrupt())
	assert.Empty(t, f.cancelled())
}

func TestNoSdapiInAppCode(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	skipDirs := map[string]bool{"node_modules": true, ".git": true, "dist": true, "build": true, "__tests__": true, ".claude": true, "docs": true}
	exts := map[string]bool{".go": true, ".js": true, ".ts": true, ".vue": true}
	var hits []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !exts[filepath.Ext(name)] || strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "/sdapi") {
			rel, _ := filepath.Rel(root, path)
			hits = append(hits, rel)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, hits, "A1: /sdapi must not appear in app code")
}
