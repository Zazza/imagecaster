package sd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueuePreview_HTTPContractAndStageOwnership(t *testing.T) {
	const queueID int64 = 71
	const frameID int64 = 9007199254740991
	const nativePass = 2
	const previewJSON = `{"id":"native-job","status":"running","preview_available":true,"progress":{"step":3,"steps":8},"preview":{"frame_id":9007199254740991,"pass":2,"step":3,"total_steps":8,"b64_json":"aW1hZ2U="}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			_, _ = io.WriteString(w, previewJSON)
		case strings.HasSuffix(r.URL.Path, "/result"):
			_, _ = io.WriteString(w, `{"images":["result"],"info":{}}`)
		default:
			_, _ = io.WriteString(w, `{"id":"native-job","status":"completed"}`)
		}
	}))
	defer server.Close()
	client := New(server.URL)
	client.PollInterval = time.Millisecond
	var events []JobStatus
	client.SetProgressHandler(func(st JobStatus) { events = append(events, st) })
	scoped := client.ForQueueJob(context.Background(), queueID)
	for range 2 {
		_, err := scoped.Txt2Img(Txt2ImgRequest{})
		require.NoError(t, err)
	}
	require.Len(t, events, 4)
	for i := range 2 {
		st := events[i*2]
		require.Equal(t, queueID, st.QueueJobID)
		require.Equal(t, int64(i+1), st.Stage)
		require.True(t, st.PreviewAvailable)
		require.NotNil(t, st.Preview)
		require.Equal(t, frameID, st.Preview.FrameID)
		require.Equal(t, nativePass, st.Preview.Pass)
		require.Equal(t, 3, st.Preview.Step)
		require.Equal(t, 8, st.Preview.TotalSteps)
		require.Equal(t, "aW1hZ2U=", st.Preview.Base64)
		finished := events[i*2+1]
		require.Equal(t, queueID, finished.QueueJobID)
		require.Equal(t, st.Stage, finished.Stage)
		require.True(t, finished.Terminal())
		require.Nil(t, finished.Preview)
	}
}
