package sd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chainJob struct {
	ID        string `json:"id"`
	Client    string `json:"client"`
	ClientRef string `json:"client_ref"`
	Status    string `json:"status"`
}

type chainServer struct {
	mu      sync.Mutex
	jobs    map[string]*chainJob
	byRef   map[string]string
	created int
	deletes []string
	hold    map[string]bool
}

func (s *chainServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/jobs")
	switch {
	case r.Method == http.MethodPost && path == "":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		ref, _ := body["client_ref"].(string)
		client, _ := body["client"].(string)
		if id, ok := s.byRef[ref]; ok {
			_ = json.NewEncoder(w).Encode(s.jobs[id])
			return
		}
		s.created++
		j := &chainJob{ID: fmt.Sprintf("j%d", s.created), Client: client, ClientRef: ref, Status: JobRunning}
		if !s.hold[strings.SplitN(ref, ":", 4)[2]] {
			j.Status = JobCompleted
		}
		s.jobs[j.ID] = j
		s.byRef[ref] = j.ID
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(j)
	case r.Method == http.MethodGet && path == "":
		out := []*chainJob{}
		for _, j := range s.jobs {
			if j.Client == r.URL.Query().Get("client") {
				out = append(out, j)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jobs": out})
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/result"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/result")
		_ = json.NewEncoder(w).Encode(map[string]any{"images": []string{"img-" + id}, "info": map[string]any{}})
	case r.Method == http.MethodGet:
		j, ok := s.jobs[strings.TrimPrefix(path, "/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(j)
	case r.Method == http.MethodDelete:
		id := strings.TrimPrefix(path, "/")
		s.deletes = append(s.deletes, id)
		if j, ok := s.jobs[id]; ok {
			delete(s.byRef, j.ClientRef)
			delete(s.jobs, id)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func runChain(svc Service) (string, string, error) {
	a, err := svc.Txt2Img(Txt2ImgRequest{Model: ModelSpec{Checkpoint: "m"}, Prompt: "step1"})
	if err != nil {
		return "", "", err
	}
	b, err := svc.Img2Img(Img2ImgRequest{Model: ModelSpec{Checkpoint: "m"}, Prompt: "step2", InitImages: a.Images})
	if err != nil {
		return a.Images[0], "", err
	}
	return a.Images[0], b.Images[0], nil
}

func TestForQueueJob_RestartMidChainReusesStepsAndCleansUp(t *testing.T) {
	t.Parallel()
	srv := &chainServer{jobs: map[string]*chainJob{}, byRef: map[string]string{}, hold: map[string]bool{"s2": true}}
	ts := httptest.NewServer(http.HandlerFunc(srv.serve))
	t.Cleanup(ts.Close)
	c := New(ts.URL)
	c.PollInterval = 2 * time.Millisecond

	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := runChain(c.ForQueueJob(ctx, 5))
		done <- err
	}()
	require.Eventually(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return srv.created == 2
	}, 3*time.Second, 2*time.Millisecond)
	cancel(errors.New("app shutdown"))
	require.Error(t, <-done)

	srv.mu.Lock()
	assert.Empty(t, srv.deletes, "queue steps stay on the server until the queue job ends")
	for _, j := range srv.jobs {
		j.Status = JobCompleted
	}
	srv.hold = map[string]bool{}
	srv.mu.Unlock()

	first, second, err := runChain(c.ForQueueJob(context.Background(), 5))
	require.NoError(t, err)
	assert.Equal(t, "img-j1", first, "step 1 taken from the server, not recomputed")
	assert.Equal(t, "img-j2", second, "step 2 re-attached")
	srv.mu.Lock()
	assert.Equal(t, 2, srv.created)
	srv.mu.Unlock()

	require.NoError(t, c.CleanupQueueJob(context.Background(), 5))
	srv.mu.Lock()
	defer srv.mu.Unlock()
	assert.ElementsMatch(t, []string{"j1", "j2"}, srv.deletes)
	assert.Empty(t, srv.jobs)
}

func TestCleanupQueueJob_OnlyThisQueueJob(t *testing.T) {
	t.Parallel()
	srv := &chainServer{jobs: map[string]*chainJob{
		"a": {ID: "a", Client: DefaultClientName, ClientRef: "inst0001:q5:s1:aaaa", Status: JobCompleted},
		"b": {ID: "b", Client: DefaultClientName, ClientRef: "inst0001:q51:s1:bbbb", Status: JobCompleted},
		"c": {ID: "c", Client: DefaultClientName, ClientRef: "inst0001:d:01", Status: JobRunning},
		"d": {ID: "d", Client: "imagecaster-mcp", ClientRef: "inst0001:q5:s1:dddd", Status: JobCompleted},
		"e": {ID: "e", Client: DefaultClientName, ClientRef: "inst0002:q5:s1:aaaa", Status: JobCompleted},
	}, byRef: map[string]string{}}
	ts := httptest.NewServer(http.HandlerFunc(srv.serve))
	t.Cleanup(ts.Close)

	c := New(ts.URL)
	c.SetInstallID("inst0001")
	require.NoError(t, c.CleanupQueueJob(context.Background(), 5))
	srv.mu.Lock()
	defer srv.mu.Unlock()
	assert.Equal(t, []string{"a"}, srv.deletes)
}

func TestTwoInstalls_SameQueueIDDoNotTouchEachOther(t *testing.T) {
	t.Parallel()
	srv := &chainServer{jobs: map[string]*chainJob{}, byRef: map[string]string{}, hold: map[string]bool{}}
	ts := httptest.NewServer(http.HandlerFunc(srv.serve))
	t.Cleanup(ts.Close)

	pc1 := New(ts.URL)
	pc1.PollInterval = 2 * time.Millisecond
	pc1.SetInstallID("aaaa0001")
	pc2 := New(ts.URL)
	pc2.PollInterval = 2 * time.Millisecond
	pc2.SetInstallID("bbbb0002")

	_, _, err := runChain(pc1.ForQueueJob(context.Background(), 1))
	require.NoError(t, err)
	_, _, err = runChain(pc2.ForQueueJob(context.Background(), 1))
	require.NoError(t, err)

	srv.mu.Lock()
	assert.Equal(t, 4, srv.created, "identical steps of another install are not reused")
	srv.mu.Unlock()

	require.NoError(t, pc1.CleanupQueueJob(context.Background(), 1))
	srv.mu.Lock()
	assert.ElementsMatch(t, []string{"j1", "j2"}, srv.deletes)
	assert.Len(t, srv.jobs, 2)
	for _, j := range srv.jobs {
		assert.True(t, strings.HasPrefix(j.ClientRef, "bbbb0002:"), j.ClientRef)
	}
	srv.mu.Unlock()
}

func TestTwoInstalls_InterruptCancelsOnlyOwnJobs(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var cancelled []string
	jobs := []map[string]any{
		{"id": "own-q", "client": DefaultClientName, "client_ref": "aaaa0001:q1:s1:abcdef01", "status": JobRunning},
		{"id": "own-d", "client": DefaultClientName, "client_ref": "aaaa0001:d:01", "status": JobQueued},
		{"id": "other-q", "client": DefaultClientName, "client_ref": "bbbb0002:q1:s1:abcdef01", "status": JobRunning},
		{"id": "other-d", "client": DefaultClientName, "client_ref": "bbbb0002:d:01", "status": JobQueued},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/jobs":
			_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			mu.Lock()
			cancelled = append(cancelled, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/jobs/"), "/cancel"))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"status": JobCancelled})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	pc1 := New(ts.URL)
	pc1.SetInstallID("aaaa0001")
	require.NoError(t, pc1.Interrupt())
	mu.Lock()
	defer mu.Unlock()
	assert.ElementsMatch(t, []string{"own-q", "own-d"}, cancelled)
}

func TestNew_RandomInstallIDPrefixesDirectRefs(t *testing.T) {
	t.Parallel()
	a, b := New(""), New("")
	assert.Regexp(t, `^[0-9a-f]{8}$`, a.InstallID())
	assert.NotEqual(t, a.InstallID(), b.InstallID())
	assert.Regexp(t, `^`+a.InstallID()+`:d:[0-9a-f]{16}$`, a.directRef())
	a.SetInstallID("")
	assert.NotEmpty(t, a.InstallID())
}
