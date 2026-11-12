package settings

import (
	"encoding/json"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/Zazza/imagecaster/internal/sd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func syncFS(t *testing.T, models ...string) fstest.MapFS {
	t.Helper()
	items := []map[string]any{}
	for _, m := range models {
		items = append(items, map[string]any{"name": "sync-" + m, "preset_type": "post", "model_name": m, "loras": "[]"})
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "presets": items})
	require.NoError(t, err)
	return fstest.MapFS{"data/presets/sync.json": {Data: raw}}
}

func statusByName(t *testing.T, r *PresetSync, name string) bool {
	t.Helper()
	for _, s := range r.Statuses {
		if s.Name == name {
			return s.Installed
		}
	}
	require.Failf(t, "status not found", "%s", name)
	return false
}

func TestSyncPresets_DirectSDWithoutServerInstallsPresentModel(t *testing.T) {
	svc, _ := testService(t, nil, &mockSDService{models: []sd.SDModel{{Name: "m-a"}}})
	svc.SetBundledPresets(syncFS(t, "m-a", "m-b"))

	r, err := svc.SyncPresets()
	require.NoError(t, err)
	assert.Equal(t, 1, r.Added)
	assert.True(t, statusByName(t, r, "sync-m-a"))
}

func TestSyncPresets_DirectSDErrorInstallsNothingHidesNothing(t *testing.T) {
	svc, db := testService(t, nil, &mockSDService{modelsErr: errors.New("down")})
	_, err := db.DB().Exec(`INSERT INTO presets (name, preset_type, model_name, loras) VALUES ('user-x', 'post', 'absent', '[]')`)
	require.NoError(t, err)
	svc.SetBundledPresets(syncFS(t, "m-a"))

	r, err := svc.SyncPresets()
	require.NoError(t, err)
	assert.Equal(t, 0, r.Added)
	assert.True(t, statusByName(t, r, "user-x"))
}

func TestSyncPresets_ServerListsUsedWhenConfigured(t *testing.T) {
	svc := planService(t, map[string][]string{"sd": {"m-b.safetensors"}, "lora": {}})
	svc.SetBundledPresets(syncFS(t, "m-a", "m-b"))

	r, err := svc.SyncPresets()
	require.NoError(t, err)
	assert.Equal(t, 1, r.Added)
	assert.True(t, statusByName(t, r, "sync-m-b"))
}
