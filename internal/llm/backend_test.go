package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureServer(t *testing.T) (*Client, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Errorf("decode request: %v", err)
		}
		m["_path"] = r.URL.Path
		mu.Lock()
		bodies = append(bodies, m)
		mu.Unlock()
		if r.URL.Path == "/api/chat" {
			_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(ts.Close)
	return New(ts.URL), func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), bodies...)
	}
}

func TestBackendConfigAppliedToRequests(t *testing.T) {
	t.Parallel()
	full := BackendConfig{KeepAlive: "10m", NumCtx: 4096, NumThread: 6, TopP: 0.85}
	tests := []struct {
		name      string
		cfg       BackendConfig
		call      func(c *Client) (string, error)
		path      string
		wantTop   map[string]any
		wantOpts  map[string]any
		absentTop []string
		absentOpt []string
	}{
		{
			name:     "chat sends top_p and options, explicit max wins",
			cfg:      full,
			call:     func(c *Client) (string, error) { return c.Chat("m", "sys", "hi", 0.5, 128) },
			path:     "/v1/chat/completions",
			wantTop:  map[string]any{"max_tokens": 128.0, "top_p": 0.85, "keep_alive": "10m"},
			wantOpts: map[string]any{"num_ctx": 4096.0, "top_p": 0.85, "num_thread": 6.0},
		},
		{
			name: "chat without explicit max omits max_tokens",
			cfg:  full,
			call: func(c *Client) (string, error) {
				return c.ChatWithMessages("m", []Message{{Role: "user", Content: "x"}}, 0.5, 0)
			},
			path:      "/v1/chat/completions",
			wantTop:   map[string]any{"top_p": 0.85},
			absentTop: []string{"max_tokens"},
		},
		{
			name:      "chat zero config sends nothing extra",
			cfg:       BackendConfig{},
			call:      func(c *Client) (string, error) { return c.Chat("m", "sys", "hi", 0.5, 0) },
			path:      "/v1/chat/completions",
			absentTop: []string{"max_tokens", "top_p", "keep_alive", "options"},
		},
		{
			name:     "vision sends top_p and num_thread in options",
			cfg:      full,
			call:     func(c *Client) (string, error) { return c.ChatVision("v", "", "describe", "AAAA", 0.4, 200) },
			path:     "/api/chat",
			wantOpts: map[string]any{"num_ctx": 8192.0, "top_p": 0.85, "num_thread": 6.0, "num_predict": 200.0},
		},
		{
			name:      "vision without explicit max omits num_predict",
			cfg:       full,
			call:      func(c *Client) (string, error) { return c.ChatVision("v", "", "describe", "AAAA", 0.4, 0) },
			path:      "/api/chat",
			wantOpts:  map[string]any{"num_ctx": 8192.0},
			absentOpt: []string{"num_predict"},
		},
		{
			name:      "vision zero config omits optional options",
			cfg:       BackendConfig{},
			call:      func(c *Client) (string, error) { return c.ChatVision("v", "", "describe", "AAAA", 0.4, 0) },
			path:      "/api/chat",
			wantOpts:  map[string]any{"num_ctx": 8192.0},
			absentOpt: []string{"top_p", "num_thread", "num_predict"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, bodies := captureServer(t)
			cfg := tt.cfg
			c.SetConfigSource(func(string) BackendConfig { return cfg })
			_, err := tt.call(c)
			require.NoError(t, err)
			got := bodies()
			require.Len(t, got, 1)
			body := got[0]
			assert.Equal(t, tt.path, body["_path"])
			for k, v := range tt.wantTop {
				assert.Equal(t, v, body[k], k)
			}
			for _, k := range tt.absentTop {
				assert.NotContains(t, body, k)
			}
			opts, _ := body["options"].(map[string]any)
			for k, v := range tt.wantOpts {
				assert.Equal(t, v, opts[k], "options."+k)
			}
			for _, k := range tt.absentOpt {
				assert.NotContains(t, opts, k)
			}
		})
	}
}

func TestConfigSourceResolvedPerCallUnderConcurrency(t *testing.T) {
	t.Parallel()
	c, bodies := captureServer(t)
	c.SetConfigSource(func(mode string) BackendConfig {
		if mode == ModeAnalyze {
			return BackendConfig{NumCtx: 16384}
		}
		return BackendConfig{NumCtx: 2048}
	})
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, 2*n)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := c.Chat("m", "sys", "hi", 0.5, 64)
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := c.ChatVision("v", "", "describe", "AAAA", 0.4, 64)
			errs <- err
		}()
		if i == n/2 {
			c.SetURL(c.url())
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	got := bodies()
	require.Len(t, got, 2*n)
	for i, b := range got {
		opts, _ := b["options"].(map[string]any)
		want := 2048.0
		if b["_path"] == "/api/chat" {
			want = 16384.0
		}
		assert.Equal(t, want, opts["num_ctx"], fmt.Sprintf("request %d %v", i, b["_path"]))
	}
}
