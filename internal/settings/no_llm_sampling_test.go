package settings

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var deadSamplingKeys = []string{
	"llm_generate_temperature", "llm_generate_num_predict",
	"llm_analyze_temperature", "llm_analyze_num_predict",
	"llm_num_ctx",
}

func TestGetSettings_NoDeadSamplingDefaults(t *testing.T) {
	t.Parallel()
	svc, _ := testService(t, nil, nil)
	settings, err := svc.GetSettings()
	require.NoError(t, err)
	for _, k := range deadSamplingKeys {
		_, ok := settings[k]
		assert.False(t, ok, "GetSettings must not default %s", k)
	}
	assert.Equal(t, "256", settings["llm_max_tokens"])
}

func TestAllowedSettings_RejectsDeadSamplingKeys(t *testing.T) {
	t.Parallel()
	for _, k := range deadSamplingKeys {
		assert.False(t, config.AllowedSettings[k], "%s must not be allowed", k)
	}
	assert.True(t, config.AllowedSettings["llm_max_tokens"])
	assert.True(t, config.AllowedSettings["llm_generate_top_p"])
}

func TestGetSettings_KeepsStoredDeadSamplingRows(t *testing.T) {
	t.Parallel()
	svc, db := testService(t, nil, nil)
	require.NoError(t, db.SetSetting("llm_generate_num_predict", "900"))
	require.NoError(t, svc.UpdateSettings(map[string]string{"llm_generate_num_predict": "1", "llm_max_tokens": "300"}))
	v, err := db.GetSetting("llm_generate_num_predict")
	require.NoError(t, err)
	assert.Equal(t, "900", v)
	v, err = db.GetSetting("llm_max_tokens")
	require.NoError(t, err)
	assert.Equal(t, "300", v)
}
