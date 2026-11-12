package mcp

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/settings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serverModeCfg(t *testing.T, serverURL string) *config.Config {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.SetSetting("server_url", serverURL))
	require.NoError(t, db.SetSetting("connection_mode", "server"))
	cfg := &config.Config{}
	settings.Restore(db, cfg, llm.New(""), sd.New(""), serverclient.NewClient())
	require.Equal(t, serverURL+"/api/llm", cfg.LLMUrl)
	require.Equal(t, serverURL+"/api/sd", cfg.SDUrl)
	return cfg
}

func TestPhotoTools_ServerModeLocality(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		serverURL string
		wantErr   bool
	}{
		{name: "public server refused", serverURL: "http://8.8.8.8:8080", wantErr: true},
		{name: "public hostname refused", serverURL: "https://imagecaster.example.com", wantErr: true},
		{name: "LAN server allowed", serverURL: "http://192.168.1.10:8080", wantErr: false},
		{name: "localhost server allowed", serverURL: "http://127.0.0.1:8080", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := serverModeCfg(t, tt.serverURL)

			styEng := &fakeStylist{}
			stySess := &fakeSessions{}
			sty := NewServer(Deps{OutDir: t.TempDir(), Stylist: styEng, Sessions: stySess, Cfg: cfg})
			RegisterTools(sty)
			photo, _ := styWritePhoto(t)
			_, errDescribe := styExec(sty, "stylist_describe", map[string]any{"photo_path": photo})
			_, errTry := styExec(sty, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(1)})

			edEng := &fakeEditor{presets: edPresets()}
			edSess := &fakeSessions{}
			ed := edNewServer(t, edEng, edSess, cfg)
			edPhoto := edWritePhoto(t, edColumnsPNG(60, 60))
			_, errEdDescribe := styExec(ed, "editor_describe", map[string]any{"photo_path": edPhoto})
			_, errEdApply := styExec(ed, "editor_apply", map[string]any{"photo_path": edPhoto, "effect": map[string]any{"name": "Golden Hour"}})

			if tt.wantErr {
				assert.Error(t, errDescribe)
				assert.Error(t, errTry)
				assert.Error(t, errEdDescribe)
				assert.Error(t, errEdApply)
				assert.Empty(t, styEng.editCalls())
				assert.Empty(t, styEng.describeCalls())
				assert.Empty(t, edEng.describeCalls())
				assert.Empty(t, edEng.kleinCalls())
				assert.Empty(t, stySess.namedCalls())
				assert.Empty(t, edSess.namedCalls())
				return
			}
			assert.NoError(t, errDescribe)
			assert.NoError(t, errTry)
			assert.NoError(t, errEdDescribe)
			assert.NoError(t, errEdApply)
			assert.Len(t, styEng.editCalls(), 1)
			assert.Len(t, edSess.namedCalls(), 1)
		})
	}
}
