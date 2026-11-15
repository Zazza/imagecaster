package settings

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWiresLLMConfigPerMode(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	opts := map[string]map[string]any{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read: %v", err)
		}
		var body struct {
			Options map[string]any `json:"options"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode: %v", err)
		}
		mu.Lock()
		opts[r.URL.Path] = body.Options
		mu.Unlock()
		if r.URL.Path == "/api/chat" {
			_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(ts.Close)

	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for k, v := range map[string]string{
		"llm_generate_num_ctx":    "3072",
		"llm_generate_top_p":      "0.7",
		"llm_generate_num_thread": "3",
		"llm_analyze_num_ctx":     "12288",
		"llm_analyze_top_p":       "0.9",
	} {
		require.NoError(t, db.SetSetting(k, v))
	}
	client := llm.New(ts.URL)
	New(db, client, &mockSDService{}, &config.Config{}, logger.New(nil), nil)

	_, err = client.Chat("m", "sys", "hi", 0.5, 64)
	require.NoError(t, err)
	_, err = client.ChatVision("v", "", "describe", "AAAA", 0.4, 64)
	require.NoError(t, err)

	tests := []struct {
		path string
		want map[string]any
	}{
		{path: "/v1/chat/completions", want: map[string]any{"num_ctx": 3072.0, "top_p": 0.7, "num_thread": 3.0}},
		{path: "/api/chat", want: map[string]any{"num_ctx": 12288.0, "top_p": 0.9}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			mu.Lock()
			got := opts[tt.path]
			mu.Unlock()
			for k, v := range tt.want {
				assert.Equal(t, v, got[k], k)
			}
		})
	}
}
