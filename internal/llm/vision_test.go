package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type visionReq struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string   `json:"role"`
		Content string   `json:"content"`
		Images  []string `json:"images"`
	} `json:"messages"`
	Options map[string]float64 `json:"options"`
}

func visionServer(t *testing.T, status int, body string) (*Client, *visionReq, *string) {
	t.Helper()
	got := &visionReq{}
	path := new(string)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*path = r.Method + " " + r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	t.Cleanup(ts.Close)
	return New(ts.URL), got, path
}

func TestChatVision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		numCtx      int
		system      string
		status      int
		body        string
		want        string
		wantErr     string
		wantNumCtx  float64
		wantMsgRole []string
	}{
		{name: "num_ctx raised to 8192", numCtx: 2048, system: "sys", status: 200, body: `{"message":{"content":"  a cat  "}}`, want: "a cat", wantNumCtx: 8192, wantMsgRole: []string{"system", "user"}},
		{name: "zero num_ctx raised", numCtx: 0, status: 200, body: `{"message":{"content":"x"}}`, want: "x", wantNumCtx: 8192, wantMsgRole: []string{"user"}},
		{name: "larger num_ctx kept", numCtx: 16384, status: 200, body: `{"message":{"content":"x"}}`, want: "x", wantNumCtx: 16384, wantMsgRole: []string{"user"}},
		{name: "think block stripped", numCtx: 0, status: 200, body: `{"message":{"content":"<think>\nreasoning\n</think>\n red sofa"}}`, want: "red sofa", wantNumCtx: 8192, wantMsgRole: []string{"user"}},
		{name: "error status with body", numCtx: 0, status: 500, body: `model not found`, wantErr: "vision request: status 500: model not found", wantNumCtx: 8192, wantMsgRole: []string{"user"}},
		{name: "bad json", numCtx: 0, status: 200, body: `{`, wantErr: "vision decode", wantNumCtx: 8192, wantMsgRole: []string{"user"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, req, path := visionServer(t, tt.status, tt.body)
			numCtx := tt.numCtx
			c.SetConfigSource(func(string) BackendConfig { return BackendConfig{NumCtx: numCtx} })
			got, err := c.ChatVision("llava", tt.system, "describe", "AAAA", 0.3, 256)

			assert.Equal(t, "POST /api/chat", *path)
			assert.Equal(t, "llava", req.Model)
			assert.False(t, req.Stream)
			roles := make([]string, 0, len(req.Messages))
			for _, m := range req.Messages {
				roles = append(roles, m.Role)
			}
			assert.Equal(t, tt.wantMsgRole, roles)
			user := req.Messages[len(req.Messages)-1]
			assert.Equal(t, "describe", user.Content)
			assert.Equal(t, []string{"AAAA"}, user.Images)
			if tt.system != "" {
				assert.Equal(t, tt.system, req.Messages[0].Content)
				assert.Empty(t, req.Messages[0].Images)
			}
			assert.Equal(t, tt.wantNumCtx, req.Options["num_ctx"])
			assert.InDelta(t, 0.3, req.Options["temperature"], 1e-9)
			assert.Equal(t, float64(256), req.Options["num_predict"])

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

func TestChatVision_Unreachable(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close()
	_, err := New(url).ChatVision("m", "", "x", "AAAA", 0.1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vision request")
}

func TestAnalyzeImage_SendsImageAndCleansTags(t *testing.T) {
	t.Parallel()
	c, req, _ := visionServer(t, 200, `{"message":{"content":"<think>hmm</think>Here are tags: masterpiece, cat, cat, sofa"}}`)
	got, err := c.AnalyzeImage("llava", "tagger", "BBBB", 128)
	require.NoError(t, err)
	assert.Equal(t, "masterpiece, cat, sofa", got)
	require.Len(t, req.Messages, 2)
	assert.Equal(t, "tagger", req.Messages[0].Content)
	assert.Equal(t, []string{"BBBB"}, req.Messages[1].Images)
}

func TestAnalyzeImageDescribe_NoSystemMessage(t *testing.T) {
	t.Parallel()
	c, req, _ := visionServer(t, 200, `{"message":{"content":"A cat on a sofa."}}`)
	got, err := c.AnalyzeImageDescribe("llava", "describe it", "CCCC", 64)
	require.NoError(t, err)
	assert.Equal(t, "A cat on a sofa.", got)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "describe it", req.Messages[0].Content)
	assert.Equal(t, []string{"CCCC"}, req.Messages[0].Images)
}
