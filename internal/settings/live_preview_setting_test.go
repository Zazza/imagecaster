package settings

import (
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLivePreviewSetting_DefaultAndPersistedFalse(t *testing.T) {
	svc, db := testService(t, nil, nil)
	values, err := svc.GetSettings()
	require.NoError(t, err)
	require.Equal(t, "true", values[config.GenerationLivePreview])
	require.NoError(t, svc.UpdateSettings(map[string]string{config.GenerationLivePreview: "false"}))
	stored, err := db.GetSetting(config.GenerationLivePreview)
	require.NoError(t, err)
	require.Equal(t, "false", stored)
	values, err = svc.GetSettings()
	require.NoError(t, err)
	require.Equal(t, "false", values[config.GenerationLivePreview])
	require.NoError(t, svc.UpdateSettings(map[string]string{config.GenerationLivePreview: "true"}))
	values, err = svc.GetSettings()
	require.NoError(t, err)
	require.Equal(t, "true", values[config.GenerationLivePreview])
}

func TestLivePreviewSetting_RejectsInvalidWithoutReplacingSavedValue(t *testing.T) {
	for _, invalid := range []string{"", "yes", "0", "TRUE"} {
		t.Run(invalid, func(t *testing.T) {
			svc, _ := testService(t, nil, nil)
			require.NoError(t, svc.UpdateSettings(map[string]string{config.GenerationLivePreview: "false"}))
			err := svc.UpdateSettings(map[string]string{config.GenerationLivePreview: invalid})
			require.Error(t, err)
			values, err := svc.GetSettings()
			require.NoError(t, err)
			require.Equal(t, "false", values[config.GenerationLivePreview])
		})
	}
}
