package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	planningTestThreadCap    = 4
	planningTestLargeThreads = 12
	planningTestLowerThreads = 2
	planningTestContext      = 8192
	planningTestSmallContext = 4096
	planningTestLargeContext = 32768
	planningTestTokens       = 321
	planningTestTopP         = 0.83
	planningTestTemperature  = 0.37
	planningTestKeepAliveCap = "30s"
)

type planningWireRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	Think     *bool     `json:"think"`
	KeepAlive string    `json:"keep_alive"`
	Options   struct {
		NumThread   int     `json:"num_thread"`
		NumCtx      int     `json:"num_ctx"`
		TopP        float64 `json:"top_p"`
		Temperature float64 `json:"temperature"`
		NumPredict  int     `json:"num_predict"`
	} `json:"options"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	TopP        float64 `json:"top_p"`
}

func TestForPlanningRequestPolicy(t *testing.T) {
	cases := []struct {
		name          string
		source        bool
		threads       int
		keepAlive     string
		context       int
		wantThreads   int
		wantKeepAlive string
	}{
		{"configured high limits", true, planningTestLargeThreads, "5m", planningTestContext, planningTestThreadCap, planningTestKeepAliveCap},
		{"no config source", false, 0, "", 0, planningTestThreadCap, planningTestKeepAliveCap},
		{"configured defaults", true, 0, "", planningTestContext, planningTestThreadCap, planningTestKeepAliveCap},
		{"explicit lower limits", true, planningTestLowerThreads, "10s", planningTestContext, planningTestLowerThreads, "10s"},
		{"immediate expiry", true, planningTestLowerThreads, "0", planningTestContext, planningTestLowerThreads, "0"},
		{"infinite expiry", true, planningTestLargeThreads, "-1", planningTestContext, planningTestThreadCap, planningTestKeepAliveCap},
		{"small configured context", true, planningTestLargeThreads, "5m", planningTestSmallContext, planningTestThreadCap, planningTestKeepAliveCap},
		{"large configured context", true, planningTestLargeThreads, "5m", planningTestLargeContext, planningTestThreadCap, planningTestKeepAliveCap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan planningWireRequest, 2)
			paths := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || (r.URL.Path != "/v1/chat/completions" && r.URL.Path != "/api/chat") {
					t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected call", http.StatusBadRequest)
					return
				}
				var request planningWireRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				requests <- request
				paths <- r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"message":{"content":"synthetic plan"},"choices":[{"message":{"content":"synthetic plan"}}]}`))
			}))
			defer server.Close()
			source := New(server.URL)
			source.httpClient = server.Client()
			configured := BackendConfig{NumThread: tc.threads, KeepAlive: tc.keepAlive, NumCtx: tc.context, TopP: planningTestTopP}
			modes := []string{}
			if tc.source {
				source.cfgSource = func(mode string) BackendConfig {
					modes = append(modes, mode)
					return configured
				}
			}
			planning := ForPlanning(source)
			require.NotSame(t, source, planning)
			result, err := planning.Chat("synthetic-model", "synthetic system", "synthetic text", planningTestTemperature, planningTestTokens)
			require.NoError(t, err)
			require.Equal(t, "synthetic plan", result)
			limited := <-requests
			require.Equal(t, "/api/chat", <-paths)
			require.Equal(t, planningTestTokens, limited.Options.NumPredict)
			require.Equal(t, planningTestTemperature, limited.Options.Temperature)
			require.False(t, limited.Stream)
			require.NotNil(t, limited.Think, "native request must explicitly disable reasoning")
			require.False(t, *limited.Think)
			require.Equal(t, planningTestContext, limited.Options.NumCtx)
			require.Equal(t, tc.wantThreads, limited.Options.NumThread)
			require.Equal(t, tc.wantKeepAlive, limited.KeepAlive)
			_, err = source.Chat("synthetic-model", "synthetic system", "synthetic text", planningTestTemperature, planningTestTokens)
			require.NoError(t, err)
			original := <-requests
			require.Equal(t, "/v1/chat/completions", <-paths)
			require.Equal(t, planningTestTokens, original.MaxTokens)
			require.Equal(t, planningTestTemperature, original.Temperature)
			require.Equal(t, tc.keepAlive, original.KeepAlive)
			require.Equal(t, tc.context, original.Options.NumCtx)
			if tc.source {
				require.Equal(t, tc.threads, original.Options.NumThread)
				require.Equal(t, planningTestContext, limited.Options.NumCtx)
				require.Equal(t, planningTestTopP, limited.Options.TopP)
				require.Equal(t, []string{ModeGenerate, ModeGenerate}, modes)
			} else {
				require.Zero(t, original.Options.NumThread)
				require.Zero(t, limited.Options.TopP)
			}
			require.Equal(t, original.Model, limited.Model)
			require.Equal(t, original.Messages, limited.Messages)
			require.Equal(t, original.Stream, limited.Stream)
			require.Equal(t, tc.context, configured.NumCtx)
			require.Equal(t, tc.threads, configured.NumThread)
			require.Equal(t, tc.keepAlive, configured.KeepAlive)
			require.Equal(t, server.URL, source.url())
		})
	}
}

type planningCompatibilityFake struct{ Service }

func TestForPlanningServiceCompatibility(t *testing.T) {
	require.Nil(t, ForPlanning(nil))
	fake := &planningCompatibilityFake{}
	require.Same(t, fake, ForPlanning(fake))
}
