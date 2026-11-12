package serverclient

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadModelStream(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantErr   string
		wantLost  bool
		wantLines []string
	}{
		{"done with progress", 200, "data: {\"downloaded\":1,\"total\":2}\n\ndata: already downloaded\n\ndata: [DONE]\n\n", "", false, []string{`{"downloaded":1,"total":2}`, "already downloaded"}},
		{"server error event", 200, "data: {\"downloaded\":1}\n\ndata: [ERROR] download: 404\n\n", "download: 404", false, []string{`{"downloaded":1}`}},
		{"stream cut before done", 200, "data: {\"downloaded\":1}\n\n", "", true, []string{`{"downloaded":1}`}},
		{"bad request json", 400, `{"error":"invalid type"}`, "server: invalid type", false, nil},
		{"bad request text", 502, "bad gateway", "server returned status 502: bad gateway", false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotQuery map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/server/models/download/stream", r.URL.Path)
				gotQuery = map[string]string{"type": r.URL.Query().Get("type"), "url": r.URL.Query().Get("url"), "filename": r.URL.Query().Get("filename")}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			c := NewClient()
			c.SetBaseURL(srv.URL + "/")

			var lines []string
			err := c.DownloadModelStream("sd", "https://h.example/a b.safetensors?x=1&y=2", "a b.safetensors", func(s string) { lines = append(lines, s) })

			assert.Equal(t, map[string]string{"type": "sd", "url": "https://h.example/a b.safetensors?x=1&y=2", "filename": "a b.safetensors"}, gotQuery)
			assert.Equal(t, tc.wantLines, lines)
			switch {
			case tc.wantLost:
				assert.True(t, errors.Is(err, ErrStreamLost), "got %v", err)
			case tc.wantErr != "":
				require.Error(t, err)
				assert.Equal(t, tc.wantErr, err.Error())
			default:
				assert.NoError(t, err)
			}
		})
	}
}

func TestDownloadModelStream_RejectsPathInFilename(t *testing.T) {
	c := NewClient()
	c.SetBaseURL("http://127.0.0.1:1")
	assert.Error(t, c.DownloadModelStream("sd", "https://h.example/x", "../x.safetensors", nil))
}

func TestDownloadModelStream_ServerDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := NewClient()
	c.SetBaseURL(url)
	err := c.DownloadModelStream("sd", "https://h.example/x", "x.safetensors", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server unreachable")
}

func TestPullLLMModelStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/server/models/llm/pull", r.URL.Path)
		require.Equal(t, "hf.co/a/b:Q4_K_M", r.URL.Query().Get("name"))
		fmt.Fprint(w, "data: pulling manifest\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	c := NewClient()
	c.SetBaseURL(srv.URL)
	var lines []string
	require.NoError(t, c.PullLLMModelStream("hf.co/a/b:Q4_K_M", func(s string) { lines = append(lines, s) }))
	assert.Equal(t, []string{"pulling manifest"}, lines)
}
