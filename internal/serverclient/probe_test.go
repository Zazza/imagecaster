package serverclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func probeServer(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()
	paths := &[]string{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts, paths
}

func infoJSON(t *testing.T, name string, features []string) string {
	t.Helper()
	m := map[string]any{"name": name, "version": "1.2.3", "status": "ok"}
	if features != nil {
		m["features"] = features
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return string(b)
}

func TestProbe(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		body     string
		slash    bool
		wantErr  bool
		wantVer  string
		wantFeat []string
	}{
		{name: "valid server", status: 200, body: infoJSON(t, ServerName, []string{"sdcpp", "download_stream", "llamacpp"}), wantVer: "1.2.3", wantFeat: []string{"sdcpp", "download_stream", "llamacpp"}},
		{name: "trailing slash in base", status: 200, body: infoJSON(t, ServerName, []string{"sdcpp"}), slash: true, wantVer: "1.2.3", wantFeat: []string{"sdcpp"}},
		{name: "old server without features", status: 200, body: infoJSON(t, ServerName, nil), wantVer: "1.2.3"},
		{name: "non-200", status: 500, body: infoJSON(t, ServerName, nil), wantErr: true},
		{name: "not found status", status: 404, body: "", wantErr: true},
		{name: "bad json", status: 200, body: "<html>hello</html>", wantErr: true},
		{name: "empty body", status: 200, body: "", wantErr: true},
		{name: "wrong name", status: 200, body: infoJSON(t, "Stable Diffusion WebUI", []string{"sdcpp"}), wantErr: true},
		{name: "empty name", status: 200, body: infoJSON(t, "", []string{"sdcpp"}), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts, paths := probeServer(t, tt.status, tt.body)
			base := ts.URL
			if tt.slash {
				base += "/"
			}
			info, err := Probe(base)
			require.NotEmpty(t, *paths)
			assert.Equal(t, "GET /", (*paths)[0])
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, ServerName, info.Name)
			assert.Equal(t, tt.wantVer, info.Version)
			assert.ElementsMatch(t, tt.wantFeat, info.Features)
		})
	}
}

func TestProbe_Unreachable(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close()
	info, err := Probe(url)
	assert.Error(t, err)
	assert.Nil(t, info)
}

func TestServerInfo_MissingFeatures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		features []string
		want     []string
	}{
		{name: "all with llamacpp", features: []string{"sdcpp", "download_stream", "llamacpp"}, want: nil},
		{name: "ollama instead of llamacpp", features: []string{"sdcpp", "download_stream", "ollama"}, want: []string{"llamacpp"}},
		{name: "ollama and llamacpp", features: []string{"sdcpp", "download_stream", "ollama", "llamacpp", "extra"}, want: nil},
		{name: "old server no features", features: nil, want: []string{"sdcpp", "download_stream", "llamacpp"}},
		{name: "empty features", features: []string{}, want: []string{"sdcpp", "download_stream", "llamacpp"}},
		{name: "no llm backend", features: []string{"sdcpp", "download_stream"}, want: []string{"llamacpp"}},
		{name: "no sdcpp", features: []string{"download_stream", "llamacpp"}, want: []string{"sdcpp"}},
		{name: "no sdcpp with ollama only", features: []string{"download_stream", "ollama"}, want: []string{"sdcpp", "llamacpp"}},
		{name: "no download_stream", features: []string{"sdcpp", "llamacpp"}, want: []string{"download_stream"}},
		{name: "only unrelated", features: []string{"forge"}, want: []string{"sdcpp", "download_stream", "llamacpp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info := &ServerInfo{Name: ServerName, Features: tt.features}
			got := info.MissingFeatures()
			if tt.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestRequiredFeatures(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, []string{"sdcpp", "download_stream"}, RequiredFeatures)
}
