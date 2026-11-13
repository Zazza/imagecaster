package serverclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type showRequest struct {
	method string
	path   string
	body   map[string]any
}

func showServer(t *testing.T, status int, body string) (*httptest.Server, func() []showRequest) {
	t.Helper()
	var mu sync.Mutex
	var reqs []showRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		mu.Lock()
		reqs = append(reqs, showRequest{method: r.Method, path: r.URL.Path, body: m})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts, func() []showRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]showRequest(nil), reqs...)
	}
}

func TestLLMCapabilities(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		model   string
		status  int
		body    string
		want    []string
		wantErr bool
	}{
		{name: "vision model", model: "qwen3-vl:8b", status: 200, body: `{"capabilities":["completion","vision"]}`, want: []string{"completion", "vision"}},
		{name: "text model", model: "qwen3:8b", status: 200, body: `{"capabilities":["completion"]}`, want: []string{"completion"}},
		{name: "embedding model", model: "bge-m3", status: 200, body: `{"capabilities":["embedding"],"details":{"family":"bert"}}`, want: []string{"embedding"}},
		{name: "server error", model: "qwen3:8b", status: 500, body: `{"error":"boom"}`, wantErr: true},
		{name: "not found", model: "missing:1b", status: 404, body: `{"error":"model not found"}`, wantErr: true},
		{name: "bad json", model: "qwen3:8b", status: 200, body: `<html>oops</html>`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts, reqs := showServer(t, tt.status, tt.body)
			c := NewClient()
			c.SetBaseURL(ts.URL)

			got, err := c.LLMCapabilities(tt.model)

			r := reqs()
			require.Len(t, r, 1)
			assert.Equal(t, http.MethodPost, r[0].method)
			assert.Equal(t, "/api/llm/api/show", r[0].path)
			assert.Equal(t, tt.model, r[0].body["model"])
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
