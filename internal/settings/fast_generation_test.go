package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettings_FastGenerationPersists(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"true", "false"} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			svc, db := testService(t, nil, nil)
			require.NoError(t, svc.UpdateSettings(map[string]string{"fast_generation": v}))
			got, err := db.GetSetting("fast_generation")
			require.NoError(t, err)
			assert.Equal(t, v, got)
			settings, err := svc.GetSettings()
			require.NoError(t, err)
			assert.Equal(t, v, settings["fast_generation"])
		})
	}
}
