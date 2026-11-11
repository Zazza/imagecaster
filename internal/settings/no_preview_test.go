package settings

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var previewKeys = []string{"preview_mode", "preview_width", "preview_height"}

func TestGetSettings_NoPreviewDefaults(t *testing.T) {
	t.Parallel()
	svc, _ := testService(t, nil, nil)

	settings, err := svc.GetSettings()
	require.NoError(t, err)
	require.NotEmpty(t, settings)
	for _, k := range previewKeys {
		_, ok := settings[k]
		assert.False(t, ok, "GetSettings must not return default for %s", k)
	}
}

func TestAllowedSettings_RejectsPreviewKeys(t *testing.T) {
	t.Parallel()
	for _, k := range previewKeys {
		assert.False(t, config.AllowedSettings[k], "%s must not be allowed", k)
	}
	assert.True(t, config.AllowedSettings["analyze_describe_prompt"], "sanity: regular prompt key stays allowed")
	assert.True(t, config.AllowedSettings["edit_garment_prompt"], "garment prompt is saved from settings")
}

func TestUpdateSettings_DoesNotStorePreviewKeys(t *testing.T) {
	t.Parallel()
	svc, db := testService(t, nil, nil)

	err := svc.UpdateSettings(map[string]string{
		"preview_mode":        "true",
		"preview_width":       "256",
		"preview_height":      "256",
		"edit_garment_prompt": "garment custom",
	})
	require.NoError(t, err)

	for _, k := range previewKeys {
		v, err := db.GetSetting(k)
		require.NoError(t, err)
		assert.Empty(t, v, "%s must not be stored", k)
	}
	v, err := db.GetSetting("edit_garment_prompt")
	require.NoError(t, err)
	assert.Equal(t, "garment custom", v)
}
