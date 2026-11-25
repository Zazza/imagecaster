package sd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnloadIdleModelUsesAtomicServerEndpoint(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusConflict, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/unload", r.URL.Path)
				w.WriteHeader(status)
			}))
			defer server.Close()
			err := New(server.URL).UnloadIdleModel(context.Background())
			assert.Equal(t, 1, calls, "must never restart, cancel jobs, or retry conflict")
			if status == http.StatusNoContent {
				require.NoError(t, err)
			} else {
				var apiErr *APIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, status, apiErr.Status)
			}
		})
	}
}

func TestUnloadIdleModelHonorsCancelledContext(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, New(server.URL).UnloadIdleModel(ctx), context.Canceled)
	assert.Zero(t, calls)
}
