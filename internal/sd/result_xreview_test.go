package sd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type xrResultServer struct {
	ts         *httptest.Server
	resultHits atomic.Int64
	submits    atomic.Int64
	result     func(n int64, w http.ResponseWriter)
}

func newXRResultServer(t *testing.T, result func(n int64, w http.ResponseWriter)) *xrResultServer {
	t.Helper()
	s := &xrResultServer{result: result}
	job := func(status string) map[string]any {
		return map[string]any{
			"id": "xr-1", "kind": "upscale", "client": "imagecaster", "client_ref": "xr",
			"status": status, "position": 0, "progress": map[string]any{"step": 10, "steps": 10},
			"error": nil, "created_at": "2026-11-13T10:00:00Z", "started_at": "2026-11-13T10:00:01Z",
			"finished_at": "2026-11-13T10:00:05Z", "expires_at": "2026-11-14T10:00:05Z",
		}
	}
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && p == jobsPrefix:
			s.submits.Add(1)
			writeJSON(w, http.StatusAccepted, job("queued"))
		case r.Method == http.MethodGet && p == jobsPrefix+"/xr-1":
			writeJSON(w, http.StatusOK, job("completed"))
		case r.Method == http.MethodGet && p == jobsPrefix+"/xr-1/result":
			s.result(s.resultHits.Add(1), w)
		case r.Method == http.MethodDelete && p == jobsPrefix+"/xr-1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && p == jobsPrefix+"/xr-1/cancel":
			writeJSON(w, http.StatusOK, job("cancelled"))
		case r.Method == http.MethodGet && p == jobsPrefix:
			writeJSON(w, http.StatusOK, map[string]any{"jobs": []any{}})
		default:
			writeAPIError(w, http.StatusNotFound, "not_found", "no route")
		}
	}))
	t.Cleanup(s.ts.Close)
	return s
}

func (s *xrResultServer) client() *Client {
	c := New(s.ts.URL + "/api/sd")
	c.PollInterval = 5 * time.Millisecond
	return c
}

func TestRunJob_BadResultBodyFailsPromptlyXReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result func(n int64, w http.ResponseWriter)
	}{
		{"oversized body", func(_ int64, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"images":["`))
			_, _ = w.Write([]byte(strings.Repeat("A", maxResponseBodySize+1024)))
			_, _ = w.Write([]byte(`"],"info":{}}`))
		}},
		{"invalid json", func(_ int64, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"images":["abc"`))
		}},
		{"not json at all", func(_ int64, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html>gateway</html>`))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newXRResultServer(t, tt.result)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			start := time.Now()
			res, err := s.client().RunJob(ctx, upscaleReq("xr"), nil)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrBadResponse)
			assert.Nil(t, res)
			assert.NoError(t, ctx.Err(), "must fail before the context deadline")
			assert.Less(t, time.Since(start), 10*time.Second)
			assert.GreaterOrEqual(t, s.resultHits.Load(), int64(1))
			assert.LessOrEqual(t, s.resultHits.Load(), int64(3), "result endpoint must not be re-polled endlessly")
			assert.Equal(t, int64(1), s.submits.Load())
		})
	}
}

func TestRunJob_TransientResultNetworkErrorRetriedXReview(t *testing.T) {
	t.Parallel()
	s := newXRResultServer(t, func(n int64, w http.ResponseWriter) {
		if n <= 2 {
			hangUp(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"images": []string{"img-ok"}, "info": map[string]any{"seed": 1}})
	})

	res, err := s.client().RunJob(runCtx(t), upscaleReq("xr"), nil)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, []string{"img-ok"}, res.Images)
	assert.GreaterOrEqual(t, s.resultHits.Load(), int64(3))
	assert.Equal(t, int64(1), s.submits.Load())
}
