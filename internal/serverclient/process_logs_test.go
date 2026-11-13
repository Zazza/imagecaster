package serverclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessLogs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		process   string
		lines     int
		status    int
		body      string
		wantURI   string
		want      []string
		wantErr   string
		wantAnErr bool
	}{
		{name: "plain name", process: "llm", lines: 50, status: 200, body: `{"process":"llm","lines":2,"logs":["a","b"]}`, wantURI: "/api/server/logs/llm?lines=50", want: []string{"a", "b"}},
		{name: "name escaped", process: "sd cpp/x", lines: 10, status: 200, body: `{"process":"sd cpp/x","lines":1,"logs":["one"]}`, wantURI: "/api/server/logs/sd%20cpp%2Fx?lines=10", want: []string{"one"}},
		{name: "empty logs", process: "sdcpp", lines: 50, status: 200, body: `{"process":"sdcpp","lines":0,"logs":[]}`, wantURI: "/api/server/logs/sdcpp?lines=50", want: []string{}},
		{name: "zero lines param", process: "llm", lines: 0, status: 200, body: `{"process":"llm","lines":0,"logs":[]}`, wantURI: "/api/server/logs/llm?lines=0", want: []string{}},
		{name: "404 with server error text", process: "nope", lines: 50, status: 404, body: `{"error":"unknown process: nope"}`, wantURI: "/api/server/logs/nope?lines=50", wantErr: "unknown process: nope"},
		{name: "500 with server error text", process: "llm", lines: 50, status: 500, body: `{"error":"log file unreadable"}`, wantURI: "/api/server/logs/llm?lines=50", wantErr: "log file unreadable"},
		{name: "non-200 without json", process: "llm", lines: 50, status: 502, body: `<html>bad gateway</html>`, wantURI: "/api/server/logs/llm?lines=50", wantAnErr: true},
		{name: "bad json on 200", process: "llm", lines: 50, status: 200, body: `not json`, wantURI: "/api/server/logs/llm?lines=50", wantAnErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var gotMethod, gotURI string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotURI = r.RequestURI
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(ts.Close)
			c := NewClient()
			c.SetBaseURL(ts.URL + "/")
			got, err := c.ProcessLogs(tt.process, tt.lines)
			assert.Equal(t, http.MethodGet, gotMethod)
			assert.Equal(t, tt.wantURI, gotURI)
			if tt.wantErr != "" || tt.wantAnErr {
				require.Error(t, err)
				if tt.wantErr != "" {
					assert.True(t, strings.Contains(err.Error(), tt.wantErr), "error %q must contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
			assert.Equal(t, len(tt.want), len(got))
			for i := range tt.want {
				assert.Equal(t, tt.want[i], got[i])
			}
		})
	}
}

func TestProcessLogs_Unreachable(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.NotFoundHandler())
	u := ts.URL
	ts.Close()
	c := NewClient()
	c.SetBaseURL(u)
	_, err := c.ProcessLogs("llm", 50)
	assert.Error(t, err)
}
