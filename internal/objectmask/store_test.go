package objectmask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

const fixtureModel = "verified model fixture"
const concurrentPreparations = 8
const fixtureFileMode = 0600

func fixtureStore(t *testing.T, handler http.HandlerFunc) (*Store, *atomic.Int32) {
	t.Helper()
	count := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); handler(w, r) }))
	t.Cleanup(server.Close)
	digest := sha256.Sum256([]byte(fixtureModel))
	s := New(t.TempDir())
	s.assets = []asset{{name: EncoderName, url: server.URL, sha256: hex.EncodeToString(digest[:])}}
	return s, count
}
func serveFixture(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(fixtureModel)) }
func assertNoPartialFiles(t *testing.T, s *Store) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(s.dir, ".model-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("partial files remain: %v, %v", matches, err)
	}
}
func TestPrepareDownloadsOnceAndReusesVerifiedDiskCache(t *testing.T) {
	s, calls := fixtureStore(t, serveFixture)
	if err := s.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.dir, EncoderName))
	if err != nil || string(data) != fixtureModel {
		t.Fatalf("model = %q, %v", data, err)
	}
	// A fresh instance must verify and reuse the on-disk cache, too.
	again := New(filepath.Dir(s.dir))
	again.assets = s.assets
	if err := again.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("downloads = %d, want 1", calls.Load())
	}
	assertNoPartialFiles(t, s)
}
func TestPrepareReplacesCorruptCache(t *testing.T) {
	s, calls := fixtureStore(t, serveFixture)
	if err := os.MkdirAll(s.dir, cacheDirectoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, EncoderName), []byte("truncated"), fixtureFileMode); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("downloads = %d", calls.Load())
	}
	data, _ := os.ReadFile(filepath.Join(s.dir, EncoderName))
	if string(data) != fixtureModel {
		t.Fatalf("corrupt cache not replaced: %q", data)
	}
}
func TestPrepareRejectsWrongChecksumAndAllowsRetry(t *testing.T) {
	var corrupt atomic.Bool
	corrupt.Store(true)
	s, _ := fixtureStore(t, func(w http.ResponseWriter, r *http.Request) {
		if corrupt.Load() {
			_, _ = w.Write([]byte("incomplete"))
			return
		}
		serveFixture(w, r)
	})
	if err := s.Prepare(context.Background()); err == nil {
		t.Fatal("accepted unverified bytes")
	}
	if _, err := os.Stat(filepath.Join(s.dir, EncoderName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unverified final file exists: %v", err)
	}
	assertNoPartialFiles(t, s)
	corrupt.Store(false)
	if err := s.Prepare(context.Background()); err != nil {
		t.Fatalf("retry: %v", err)
	}
}
func TestPrepareSerializesConcurrentRequests(t *testing.T) {
	s, calls := fixtureStore(t, serveFixture)
	var group sync.WaitGroup
	failures := make(chan error, concurrentPreparations)
	for range concurrentPreparations {
		group.Add(1)
		go func() { defer group.Done(); failures <- s.Prepare(context.Background()) }()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent downloads = %d, want 1", calls.Load())
	}
}
func TestPrepareCancellationLeavesNoFinalOrPartialFile(t *testing.T) {
	started := make(chan struct{})
	s, _ := fixtureStore(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Prepare(ctx) }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, EncoderName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final file exists after cancellation: %v", err)
	}
	assertNoPartialFiles(t, s)
}
func TestPrepareRejectsNonSuccessAndOversizedResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"http error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }},
		{"oversized", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "999999999")
			w.WriteHeader(http.StatusOK)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := fixtureStore(t, tc.handler)
			if err := s.Prepare(context.Background()); err == nil {
				t.Fatal("accepted invalid response")
			}
			assertNoPartialFiles(t, s)
		})
	}
}

func TestCanceledWaiterDoesNotCancelSharedDownload(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	s, calls := fixtureStore(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-release; serveFixture(w, r) })
	leader := make(chan error, 1)
	go func() { leader <- s.Prepare(context.Background()) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Prepare(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("waiter cancellation = %v", err)
	}
	close(release)
	if err := <-leader; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("downloads = %d", calls.Load())
	}
}
