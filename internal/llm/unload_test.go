package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnloadModelNativeRequest(t *testing.T) {
	const target = "hf.co/GitMylo/nsfwcaption-qwen3-vl-8b-v3-gguf:Q4_K_M"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, map[string]any{"model": target, "keep_alive": float64(0)}, payload, "release must neither prompt nor unload unrelated models")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"done":true}`))
	}))
	defer server.Close()
	client := New(server.URL)
	assert.Equal(t, server.URL, client.IdleModelServerURL())
	require.NoError(t, client.UnloadModel(context.Background(), target))
	assert.Equal(t, 1, calls)
}

func TestUnloadModelPropagatesFailure(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusConflict, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("release rejected"))
			}))
			defer server.Close()
			err := New(server.URL).UnloadModel(context.Background(), "caption")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "release rejected")
		})
	}
	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.Error(t, New("http://127.0.0.1:1").UnloadModel(ctx, "caption"))
	})
}

func TestUnloadModelRejectsUnconfirmedRelease(t *testing.T) {
	for _, body := range []string{`{"error":"busy"}`, `{"done":false}`, `not json`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			require.Error(t, New(server.URL).UnloadModel(context.Background(), "caption"))
		})
	}
}

func TestUnloadModelProxyURLAndEmptyTarget(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/api/llm/api/generate", r.URL.Path)
		_, _ = w.Write([]byte(`{"done":true}`))
	}))
	defer server.Close()
	client := New(server.URL + "/api/llm")
	assert.Equal(t, server.URL, client.IdleModelServerURL())
	require.Error(t, client.UnloadModel(context.Background(), "  "))
	assert.Zero(t, calls)
	require.NoError(t, client.UnloadModel(context.Background(), "caption"))
	assert.Equal(t, 1, calls)
}
