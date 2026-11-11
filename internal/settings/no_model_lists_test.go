package settings

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var modelListKeys = []string{"heavy_models", "prose_models"}

func TestGetSettings_NoModelListDefaults(t *testing.T) {
	t.Parallel()
	svc, _ := testService(t, nil, nil)
	settings, err := svc.GetSettings()
	require.NoError(t, err)
	require.NotEmpty(t, settings)
	for _, k := range modelListKeys {
		_, ok := settings[k]
		assert.False(t, ok, "GetSettings must not return %s", k)
	}
}

func TestAllowedSettings_RejectsModelListKeys(t *testing.T) {
	t.Parallel()
	for _, k := range modelListKeys {
		assert.False(t, config.AllowedSettings[k], "%s must not be allowed", k)
	}
	assert.True(t, config.AllowedSettings["llm_auto_unload"], "llm_auto_unload stays allowed")
}

func TestUpdateSettings_DoesNotStoreModelListKeys(t *testing.T) {
	t.Parallel()
	svc, db := testService(t, nil, nil)
	err := svc.UpdateSettings(map[string]string{
		"heavy_models":    "megatron",
		"prose_models":    "megatron",
		"llm_auto_unload": "false",
	})
	require.NoError(t, err)
	for _, k := range modelListKeys {
		v, err := db.GetSetting(k)
		require.NoError(t, err)
		assert.Empty(t, v, "%s must not be stored", k)
	}
	v, err := db.GetSetting("llm_auto_unload")
	require.NoError(t, err)
	assert.Equal(t, "false", v)
}
