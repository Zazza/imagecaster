package sd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type faceSwapServer struct {
	mu        sync.Mutex
	submits   []map[string]any
	submitErr int
	errCode   string
	result    map[string]any
}

func (s *faceSwapServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api/sd/jobs")
	switch {
	case r.Method == http.MethodPost && path == "":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.submits = append(s.submits, body)
		if s.submitErr != 0 {
			w.WriteHeader(s.submitErr)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": s.errCode, "message": "x"}})
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "f1", "kind": KindFaceSwap, "status": JobCompleted, "client_ref": body["client_ref"]})
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/result"):
		_ = json.NewEncoder(w).Encode(s.result)
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "f1", "status": JobCompleted})
	}
}

func newFaceSwapClient(t *testing.T, s *faceSwapServer) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(ts.Close)
	c := New(ts.URL + "/api/sd")
	c.PollInterval = 5 * time.Millisecond
	c.SetInstallID("inst")
	return c
}

func TestFaceSwapResults(t *testing.T) {
	tests := []struct {
		name       string
		result     map[string]any
		wantStatus string
		wantSkip   string
		wantImage  string
		wantFacePx int
		wantYaw    int
	}{
		{
			name:       "done",
			result:     map[string]any{"images": []string{"swapped"}, "info": map[string]any{"face_swap": "done", "skip": "", "face_px": 180, "yaw": 12}},
			wantStatus: FaceSwapDone, wantImage: "swapped", wantFacePx: 180, wantYaw: 12,
		},
		{
			name:       "skipped small face",
			result:     map[string]any{"images": []string{}, "info": map[string]any{"face_swap": "skipped", "skip": "face_small", "face_px": 40, "yaw": 3}},
			wantStatus: FaceSwapSkipped, wantSkip: "face_small", wantFacePx: 40, wantYaw: 3,
		},
		{
			name:       "skipped yaw",
			result:     map[string]any{"images": []string{}, "info": map[string]any{"face_swap": "skipped", "skip": "yaw", "face_px": 200, "yaw": -80}},
			wantStatus: FaceSwapSkipped, wantSkip: "yaw", wantFacePx: 200, wantYaw: -80,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &faceSwapServer{result: tt.result}
			c := newFaceSwapClient(t, srv)
			res, err := c.FaceSwap(context.Background(), "aW1n", "anna", 0.6)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, res.Status)
			assert.Equal(t, tt.wantSkip, res.Skip)
			assert.Equal(t, tt.wantImage, res.Image)
			assert.Equal(t, tt.wantFacePx, res.FacePx)
			assert.Equal(t, tt.wantYaw, res.Yaw)
		})
	}
}

func TestFaceSwapEmptyResultIsError(t *testing.T) {
	srv := &faceSwapServer{result: map[string]any{"images": []string{}, "info": map[string]any{"face_swap": "done"}}}
	_, err := newFaceSwapClient(t, srv).FaceSwap(context.Background(), "aW1n", "anna", 0.6)
	require.Error(t, err)
}

func TestFaceSwapSubmitBody(t *testing.T) {
	srv := &faceSwapServer{result: map[string]any{"images": []string{"x"}, "info": map[string]any{"face_swap": "done"}}}
	c := newFaceSwapClient(t, srv)
	_, err := c.FaceSwap(context.Background(), "aW1n", "anna", 0.6)
	require.NoError(t, err)
	require.Len(t, srv.submits, 1)
	body := srv.submits[0]
	assert.Equal(t, KindFaceSwap, body["kind"])
	assert.NotContains(t, body, "model")
	assert.Equal(t, map[string]any{"image": "aW1n", "person": "anna", "restore_blend": 0.6}, body["params"])
}

func TestFaceSwapQueueClientRef(t *testing.T) {
	srv := &faceSwapServer{result: map[string]any{"images": []string{"x"}, "info": map[string]any{"face_swap": "done"}}}
	c := newFaceSwapClient(t, srv)
	q := c.ForQueueJob(context.Background(), 42)
	_, err := q.FaceSwap(context.Background(), "aW1n", "anna", 0.6)
	require.NoError(t, err)
	require.Len(t, srv.submits, 1)
	want := QueueClientRef("inst", 42, 1, faceSwapJob("aW1n", "anna", 0.6))
	assert.Equal(t, want, srv.submits[0]["client_ref"])
	assert.True(t, strings.HasPrefix(want, "inst:q42:s1:"))
}

func TestFaceSwapErrorCodes(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusNotFound, "person_not_found"},
		{http.StatusConflict, "identity_missing"},
		{http.StatusServiceUnavailable, "face_models_missing"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			srv := &faceSwapServer{submitErr: tt.status, errCode: tt.code}
			_, err := newFaceSwapClient(t, srv).FaceSwap(context.Background(), "aW1n", "anna", 0.6)
			require.Error(t, err)
			assert.Equal(t, tt.code, FaceSwapErrorCode(err))
		})
	}
}

func TestFaceSwapErrorCodeFromFailedJob(t *testing.T) {
	err := &JobError{Code: "face_models_missing", Message: "x"}
	assert.Equal(t, "face_models_missing", FaceSwapErrorCode(err))
	assert.Equal(t, "", FaceSwapErrorCode(context.Canceled))
}
