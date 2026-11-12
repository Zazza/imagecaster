package settings

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func imagecasterHandler(t *testing.T, name string, features []string, hits *int32) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			atomic.AddInt32(hits, 1)
		}
		m := map[string]any{"name": name, "version": "2.0.0", "status": "ok"}
		if features != nil {
			m["features"] = features
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	}
}

func fullFeatures() []string {
	return []string{"sdcpp", "download_stream", "llamacpp"}
}

func restoreDB(t *testing.T, settings map[string]string) *preset.DB {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for k, v := range settings {
		require.NoError(t, db.SetSetting(k, v))
	}
	return db
}

func setting(t *testing.T, db *preset.DB, key string) string {
	t.Helper()
	v, err := db.GetSetting(key)
	require.NoError(t, err)
	return v
}

func listenOn8080(t *testing.T, host string, h http.Handler) {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "8080"))
	if err != nil {
		t.Skipf("cannot listen on %s:8080: %v", host, err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

func requireNothingOn8080(t *testing.T, host string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, "8080"), 300*time.Millisecond)
	if err == nil {
		c.Close()
		t.Skipf("something listens on %s:8080", host)
	}
}

func TestRestore_AppliesSavedServerURL(t *testing.T) {
	t.Parallel()
	var hits int32
	ts := httptest.NewServer(imagecasterHandler(t, serverclient.ServerName, fullFeatures(), &hits))
	t.Cleanup(ts.Close)

	tests := []struct {
		name  string
		extra map[string]string
	}{
		{name: "server only", extra: map[string]string{}},
		{name: "stale direct urls are not used", extra: map[string]string{"sd_url": "http://10.0.0.5:7860", "llm_url": "http://10.0.0.5:1234"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := map[string]string{"server_url": ts.URL, "connection_mode": "server"}
			for k, v := range tt.extra {
				s[k] = v
			}
			db := restoreDB(t, s)
			cfg := &config.Config{SDUrl: "http://old-sd:7860", LLMUrl: "http://old-llm:1234"}
			llmSvc := &mockLLMService{}
			sdSvc := &mockSDService{}
			srv := serverclient.NewClient()

			Restore(db, cfg, llmSvc, sdSvc, srv)

			assert.Equal(t, ts.URL+"/api/sd", cfg.SDUrl)
			assert.Equal(t, ts.URL+"/api/llm", cfg.LLMUrl)
			assert.True(t, sdSvc.setURLCalled)
			assert.Equal(t, ts.URL+"/api/sd", sdSvc.setURLVal)
			assert.True(t, llmSvc.setURLCalled)
			assert.Equal(t, ts.URL+"/api/llm", llmSvc.setURLVal)
			assert.Equal(t, ts.URL, srv.GetBaseURL())
		})
	}
}

func TestRestore_ConnectionModeServerDoesNotProbe(t *testing.T) {
	t.Parallel()
	var hits int32
	ts := httptest.NewServer(imagecasterHandler(t, serverclient.ServerName, fullFeatures(), &hits))
	t.Cleanup(ts.Close)

	db := restoreDB(t, map[string]string{"connection_mode": "server", "sd_url": ts.URL, "llm_url": ts.URL})
	Restore(db, &config.Config{}, &mockLLMService{}, &mockSDService{}, serverclient.NewClient())

	assert.Equal(t, int32(0), atomic.LoadInt32(&hits))
	assert.Equal(t, "", setting(t, db, "server_url"))
	assert.Equal(t, "server", setting(t, db, "connection_mode"))
}

func TestRestore_MigratesDirectSettingsToFoundServer(t *testing.T) {
	const host = "127.0.0.2"
	var hits int32
	listenOn8080(t, host, imagecasterHandler(t, serverclient.ServerName, fullFeatures(), &hits))
	want := "http://" + host + ":8080"

	tests := []struct {
		name     string
		settings map[string]string
	}{
		{name: "direct mode with sd_url", settings: map[string]string{"connection_mode": "direct", "sd_url": "http://" + host + ":7860"}},
		{name: "no mode with sd_url", settings: map[string]string{"sd_url": "http://" + host + ":7860"}},
		{name: "direct mode with llm_url only", settings: map[string]string{"connection_mode": "direct", "llm_url": "http://" + host + ":1234/v1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atomic.StoreInt32(&hits, 0)
			db := restoreDB(t, tt.settings)
			cfg := &config.Config{}
			llmSvc := &mockLLMService{}
			sdSvc := &mockSDService{}
			srv := serverclient.NewClient()

			Restore(db, cfg, llmSvc, sdSvc, srv)

			assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
			assert.Equal(t, want, setting(t, db, "server_url"))
			assert.Equal(t, "server", setting(t, db, "connection_mode"))
			assert.Equal(t, want+"/api/sd", cfg.SDUrl)
			assert.Equal(t, want+"/api/llm", cfg.LLMUrl)
			assert.Equal(t, want+"/api/sd", sdSvc.setURLVal)
			assert.Equal(t, want+"/api/llm", llmSvc.setURLVal)
			assert.Equal(t, want, srv.GetBaseURL())

			Restore(db, &config.Config{}, &mockLLMService{}, &mockSDService{}, serverclient.NewClient())
			assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
			assert.Equal(t, want, setting(t, db, "server_url"))
		})
	}
}

func TestRestore_MigrationIgnoresForeignServerOn8080(t *testing.T) {
	const host = "127.0.0.4"
	var hits int32
	listenOn8080(t, host, imagecasterHandler(t, "Stable Diffusion WebUI", nil, &hits))

	db := restoreDB(t, map[string]string{"connection_mode": "direct", "sd_url": "http://" + host + ":7860"})
	Restore(db, &config.Config{}, &mockLLMService{}, &mockSDService{}, serverclient.NewClient())

	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	assert.Equal(t, "", setting(t, db, "server_url"))
	assert.Equal(t, "server", setting(t, db, "connection_mode"))
}

func TestRestore_MigrationServerNotFound(t *testing.T) {
	const host = "127.0.0.3"
	requireNothingOn8080(t, host)

	db := restoreDB(t, map[string]string{"connection_mode": "direct", "sd_url": "http://" + host + ":1"})
	srv := serverclient.NewClient()
	Restore(db, &config.Config{}, &mockLLMService{}, &mockSDService{}, srv)

	assert.Equal(t, "", setting(t, db, "server_url"))
	assert.Equal(t, "server", setting(t, db, "connection_mode"))
	assert.Equal(t, "", srv.GetBaseURL())

	var hits int32
	listenOn8080(t, host, imagecasterHandler(t, serverclient.ServerName, fullFeatures(), &hits))
	Restore(db, &config.Config{}, &mockLLMService{}, &mockSDService{}, serverclient.NewClient())

	assert.Equal(t, int32(0), atomic.LoadInt32(&hits))
	assert.Equal(t, "", setting(t, db, "server_url"))
}

func TestRestore_NoOldURLsNoServer(t *testing.T) {
	t.Parallel()
	db := restoreDB(t, map[string]string{})
	srv := serverclient.NewClient()
	Restore(db, &config.Config{}, &mockLLMService{}, &mockSDService{}, srv)

	assert.Equal(t, "", setting(t, db, "server_url"))
	assert.Equal(t, "", srv.GetBaseURL())
}

func TestCheckServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		srvName     string
		features    []string
		wantState   string
		wantMissing []string
		wantReason  string
	}{
		{name: "ok with llamacpp", srvName: serverclient.ServerName, features: fullFeatures(), wantState: ServerOK},
		{name: "ok with ollama", srvName: serverclient.ServerName, features: []string{"sdcpp", "download_stream", "ollama"}, wantState: ServerOK},
		{name: "old server without features", srvName: serverclient.ServerName, features: nil, wantState: ServerOutdated, wantMissing: []string{"sdcpp", "download_stream", "llamacpp"}, wantReason: "old_version"},
		{name: "forge server without sdcpp", srvName: serverclient.ServerName, features: []string{"forge", "llamacpp", "download_stream"}, wantState: ServerOutdated, wantMissing: []string{"sdcpp"}, wantReason: "no_sdcpp"},
		{name: "forge server without sdcpp and download_stream", srvName: serverclient.ServerName, features: []string{"forge", "llamacpp"}, wantState: ServerOutdated, wantMissing: []string{"sdcpp", "download_stream"}, wantReason: "no_sdcpp"},
		{name: "missing download_stream", srvName: serverclient.ServerName, features: []string{"sdcpp", "llamacpp"}, wantState: ServerOutdated, wantMissing: []string{"download_stream"}, wantReason: "missing_features"},
		{name: "missing llm backend", srvName: serverclient.ServerName, features: []string{"sdcpp", "download_stream"}, wantState: ServerOutdated, wantMissing: []string{"llamacpp"}, wantReason: "missing_features"},
		{name: "foreign server", srvName: "Something Else", features: fullFeatures(), wantState: ServerNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var hits int32
			ts := httptest.NewServer(imagecasterHandler(t, tt.srvName, tt.features, &hits))
			t.Cleanup(ts.Close)
			svc, _ := testService(t, nil, nil)
			require.NoError(t, svc.UpdateSettings(map[string]string{"server_url": ts.URL}))

			got := svc.CheckServer()

			assert.Equal(t, tt.wantState, got.State)
			assert.Equal(t, ts.URL, got.URL)
			assert.Equal(t, tt.wantReason, got.Reason)
			switch tt.wantState {
			case ServerOK:
				assert.Empty(t, got.Missing)
				assert.Empty(t, got.Error)
			case ServerOutdated:
				assert.ElementsMatch(t, tt.wantMissing, got.Missing)
				assert.Equal(t, "2.0.0", got.Version)
			case ServerNotFound:
				assert.NotEmpty(t, got.Error)
			}
		})
	}
}

func TestCheckServer_NotConfigured(t *testing.T) {
	t.Parallel()
	svc, _ := testService(t, nil, nil)
	got := svc.CheckServer()
	assert.Equal(t, ServerNotConfigured, got.State)
	assert.Empty(t, got.Reason)
}

func TestCheckServer_Unreachable(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close()
	svc, _ := testService(t, nil, nil)
	require.NoError(t, svc.UpdateSettings(map[string]string{"server_url": url}))

	got := svc.CheckServer()

	assert.Equal(t, ServerNotFound, got.State)
	assert.Equal(t, url, got.URL)
	assert.NotEmpty(t, got.Error)
	assert.Empty(t, got.Reason)
}

func TestUpdateSettings_ServerURLAppliedImmediately(t *testing.T) {
	t.Parallel()
	llmSvc := &mockLLMService{}
	sdSvc := &mockSDService{}
	svc, db := testService(t, llmSvc, sdSvc)
	const server = "http://192.168.1.10:8080"

	require.NoError(t, svc.UpdateSettings(map[string]string{"server_url": server}))

	assert.Equal(t, server, setting(t, db, "server_url"))
	assert.True(t, llmSvc.setURLCalled)
	assert.Equal(t, server+"/api/llm", llmSvc.setURLVal)
	assert.True(t, sdSvc.setURLCalled)
	assert.Equal(t, server+"/api/sd", sdSvc.setURLVal)
}

func TestUpdateSettings_IgnoresDirectModeKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key   string
		value string
	}{
		{key: "llm_url", value: "http://10.1.1.1:1234"},
		{key: "sd_url", value: "http://10.1.1.1:7860"},
		{key: "llm_backend", value: "ollama"},
		{key: "connection_mode", value: "direct"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			llmSvc := &mockLLMService{}
			sdSvc := &mockSDService{}
			svc, db := testService(t, llmSvc, sdSvc)

			_ = svc.UpdateSettings(map[string]string{tt.key: tt.value})

			assert.Equal(t, "", setting(t, db, tt.key))
			assert.NotEqual(t, tt.value, llmSvc.setURLVal)
			assert.NotEqual(t, tt.value, sdSvc.setURLVal)
		})
	}
}

func TestServerCheck_ReasonJSON(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(ServerCheck{State: ServerOutdated, Reason: "no_sdcpp"})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	assert.Equal(t, "no_sdcpp", m["reason"])
}
