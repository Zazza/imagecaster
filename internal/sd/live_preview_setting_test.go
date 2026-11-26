package sd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLivePreviewSetting_NextSubmissionAndInputUnchanged(t *testing.T) {
	for _, kind := range []string{KindTxt2Img, KindImg2Img} {
		t.Run(kind, func(t *testing.T) {
			var received []map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				var body struct {
					Params map[string]any `json:"params"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				received = append(received, body.Params)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"job","status":"queued"}`))
			}))
			defer server.Close()
			client := New(server.URL)
			enabled := true
			client.SetLivePreviewSource(func() (bool, error) { return enabled, nil })
			params := map[string]any{"prompt": "a mug", "live_preview": "original"}
			before := map[string]any{"prompt": "a mug", "live_preview": "original"}
			request := JobRequest{Kind: kind, Params: params}
			_, err := client.SubmitJob(context.Background(), request)
			require.NoError(t, err)
			enabled = false
			_, err = client.SubmitJob(context.Background(), request)
			require.NoError(t, err)
			require.Len(t, received, 2)
			require.Equal(t, true, received[0]["live_preview"])
			require.Equal(t, false, received[1]["live_preview"])
			require.Equal(t, "a mug", received[1]["prompt"])
			require.Equal(t, before, params)
		})
	}
}

func TestLivePreviewSetting_SourceErrorPreventsSubmission(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := New(server.URL)
	sourceErr := errors.New("preference unavailable")
	client.SetLivePreviewSource(func() (bool, error) { return false, sourceErr })
	for _, kind := range []string{KindTxt2Img, KindImg2Img} {
		_, err := client.SubmitJob(context.Background(), JobRequest{Kind: kind, Params: map[string]any{"prompt": "mug"}})
		require.ErrorIs(t, err, sourceErr)
	}
	require.Zero(t, requests)
}

func TestLivePreviewSetting_UpscaleDoesNotReadOrSendPreference(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Params map[string]any `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotContains(t, body.Params, "live_preview")
		require.Equal(t, "image", body.Params["image"])
		_, _ = w.Write([]byte(`{"id":"upscale","status":"queued"}`))
	}))
	defer server.Close()
	client := New(server.URL)
	client.SetLivePreviewSource(func() (bool, error) {
		t.Error("upscale read preview preference")
		return false, errors.New("unexpected preference read")
	})
	_, err := client.SubmitJob(context.Background(), JobRequest{Kind: KindUpscale, Params: UpscaleParams{Image: "image", Upscaler: "model", Scale: 2}})
	require.NoError(t, err)
}
