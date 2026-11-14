package serverclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scRequest struct {
	method string
	path   string
	body   string
}

func scServer(t *testing.T, status int, body string) (*Client, *scRequest, *atomic.Int32) {
	t.Helper()
	got := &scRequest{}
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		got.method, got.path, got.body = r.Method, r.URL.EscapedPath(), string(b)
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(ts.Close)
	c := NewClient()
	c.SetBaseURL(ts.URL + "/")
	return c, got, &hits
}

func TestClient_GetModels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		body    string
		want    []ModelInfo
		wantErr string
	}{
		{name: "parses list", status: 200, body: `[{"name":"a.safetensors","size":123,"extension":".safetensors"},{"name":"b.gguf","size":7}]`,
			want: []ModelInfo{{Name: "a.safetensors", Size: 123, Extension: ".safetensors"}, {Name: "b.gguf", Size: 7}}},
		{name: "empty list", status: 200, body: `[]`, want: []ModelInfo{}},
		{name: "status error", status: 500, body: `{"error":"boom"}`, wantErr: "list checkpoints models: status 500"},
		{name: "bad json", status: 200, body: `{`, wantErr: "unexpected EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, req, _ := scServer(t, tt.status, tt.body)
			got, err := c.GetModels("checkpoints")
			assert.Equal(t, http.MethodGet, req.method)
			assert.Equal(t, "/api/server/models/checkpoints", req.path)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClient_GetLLMModelsAndStatus(t *testing.T) {
	t.Parallel()
	t.Run("llm models parsed", func(t *testing.T) {
		t.Parallel()
		c, req, _ := scServer(t, 200, `[{"name":"qwen3:8b","size":"5.2 GB"}]`)
		got, err := c.GetLLMModels()
		require.NoError(t, err)
		assert.Equal(t, "/api/server/models/llm", req.path)
		assert.Equal(t, []LLMModelInfo{{Name: "qwen3:8b", Size: "5.2 GB"}}, got)
	})
	t.Run("llm models status error", func(t *testing.T) {
		t.Parallel()
		c, _, _ := scServer(t, 503, `{"error":"down"}`)
		_, err := c.GetLLMModels()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 503")
	})
	t.Run("status parsed", func(t *testing.T) {
		t.Parallel()
		c, req, _ := scServer(t, 200, `{"processes":{"sdcpp":{"name":"sdcpp","status":"running","pid":42,"restarts":1}},"gpu":{"name":"RTX","memory_total_mb":24000,"available":true},"models":{"sd_checkpoint":"flux"}}`)
		got, err := c.GetStatus()
		require.NoError(t, err)
		assert.Equal(t, "/api/server/status", req.path)
		assert.Equal(t, ProcessStatus{Name: "sdcpp", Status: "running", PID: 42, Restarts: 1}, got.Processes["sdcpp"])
		assert.Equal(t, 24000, got.GPU.MemoryTotal)
		assert.True(t, got.GPU.Available)
		assert.Equal(t, "flux", got.Models.SDCheckpoint)
	})
	t.Run("status error", func(t *testing.T) {
		t.Parallel()
		c, _, _ := scServer(t, 500, `{"error":"x"}`)
		got, err := c.GetStatus()
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "status 500")
	})
}

func TestClient_DownloadModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		filename string
		status   int
		body     string
		wantErr  string
		wantHit  bool
	}{
		{name: "ok", filename: "model.safetensors", status: 200, body: `{}`, wantHit: true},
		{name: "server error text", filename: "model.safetensors", status: 409, body: `{"error":"already downloading"}`, wantErr: "download failed: already downloading", wantHit: true},
		{name: "status without json", filename: "model.safetensors", status: 502, body: `bad gateway`, wantErr: "download failed: status 502", wantHit: true},
		{name: "slash in filename rejected", filename: "../x.safetensors", wantErr: "invalid path segment"},
		{name: "backslash in filename rejected", filename: `a\b`, wantErr: "invalid path segment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, req, hits := scServer(t, tt.status, tt.body)
			err := c.DownloadModel("loras", "https://example.com/m", tt.filename)
			if tt.wantHit {
				assert.Equal(t, int32(1), hits.Load())
				assert.Equal(t, http.MethodPost, req.method)
				assert.Equal(t, "/api/server/models/loras", req.path)
				var sent map[string]string
				require.NoError(t, json.Unmarshal([]byte(req.body), &sent))
				assert.Equal(t, map[string]string{"url": "https://example.com/m", "filename": tt.filename}, sent)
			} else {
				assert.Zero(t, hits.Load())
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestClient_PullLLMModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "ok", status: 200, body: `{}`},
		{name: "server error text", status: 500, body: `{"error":"no space"}`, wantErr: "pull failed: no space"},
		{name: "status without json", status: 504, body: ``, wantErr: "pull failed: status 504"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, req, _ := scServer(t, tt.status, tt.body)
			err := c.PullLLMModel("qwen3:8b")
			assert.Equal(t, http.MethodPost, req.method)
			assert.Equal(t, "/api/server/models/llm", req.path)
			assert.JSONEq(t, `{"name":"qwen3:8b"}`, req.body)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestClient_DeleteModels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		call     func(c *Client) error
		status   int
		body     string
		wantPath string
		wantErr  string
	}{
		{name: "sd model ok", call: func(c *Client) error { return c.DeleteModel("checkpoints", "a.safetensors") }, status: 200, wantPath: "/api/server/models/delete/checkpoints/a.safetensors"},
		{name: "sd model server error", call: func(c *Client) error { return c.DeleteModel("vae", "v.pt") }, status: 404, body: `{"error":"not found"}`, wantPath: "/api/server/models/delete/vae/v.pt", wantErr: "delete failed: not found"},
		{name: "sd model status only", call: func(c *Client) error { return c.DeleteModel("vae", "v.pt") }, status: 500, body: `oops`, wantPath: "/api/server/models/delete/vae/v.pt", wantErr: "delete failed: status 500"},
		{name: "sd model traversal rejected", call: func(c *Client) error { return c.DeleteModel("vae", "../etc") }, wantErr: "invalid path segment"},
		{name: "llm ok", call: func(c *Client) error { return c.DeleteLLMModel("qwen3") }, status: 200, wantPath: "/api/server/models/delete/llm/qwen3"},
		{name: "llm server error", call: func(c *Client) error { return c.DeleteLLMModel("qwen3") }, status: 409, body: `{"error":"in use"}`, wantPath: "/api/server/models/delete/llm/qwen3", wantErr: "delete failed: in use"},
		{name: "llm traversal rejected", call: func(c *Client) error { return c.DeleteLLMModel("a/b") }, wantErr: "invalid path segment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, req, hits := scServer(t, tt.status, tt.body)
			err := tt.call(c)
			if tt.wantPath == "" {
				assert.Zero(t, hits.Load())
			} else {
				assert.Equal(t, http.MethodDelete, req.method)
				assert.Equal(t, tt.wantPath, req.path)
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestClient_Processes(t *testing.T) {
	t.Parallel()
	type op struct {
		verb string
		call func(c *Client, name string) error
	}
	ops := []op{
		{verb: "start", call: (*Client).StartProcess},
		{verb: "stop", call: (*Client).StopProcess},
		{verb: "restart", call: (*Client).RestartProcess},
	}
	cases := []struct {
		name    string
		process string
		status  int
		body    string
		wantErr string
		wantHit bool
	}{
		{name: "ok", process: "sdcpp", status: 200, body: `{"status":"ok"}`, wantHit: true},
		{name: "server error text", process: "sdcpp", status: 500, body: `{"error":"not installed"}`, wantErr: "failed: not installed", wantHit: true},
		{name: "status without json", process: "llm", status: 502, body: `x`, wantErr: "failed: status 502", wantHit: true},
		{name: "traversal rejected", process: "../admin", wantErr: "invalid path segment"},
	}
	for _, o := range ops {
		for _, tt := range cases {
			t.Run(o.verb+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				c, req, hits := scServer(t, tt.status, tt.body)
				err := o.call(c, tt.process)
				if tt.wantHit {
					assert.Equal(t, http.MethodPost, req.method)
					assert.Equal(t, "/api/server/"+o.verb+"/"+tt.process, req.path)
				} else {
					assert.Zero(t, hits.Load())
				}
				if tt.wantErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErr)
					if tt.wantHit {
						assert.Contains(t, err.Error(), o.verb)
					}
					return
				}
				assert.NoError(t, err)
			})
		}
	}
}

func TestClient_Unreachable(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.NotFoundHandler())
	base := ts.URL
	ts.Close()
	c := NewClient()
	c.SetBaseURL(base)
	_, errModels := c.GetModels("checkpoints")
	assert.Error(t, errModels)
	assert.Error(t, c.StartProcess("sdcpp"))
	assert.Error(t, c.DownloadModel("loras", "u", "f"))
	assert.Error(t, c.DeleteModel("loras", "f"))
}
