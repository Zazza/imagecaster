package settings

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiagnosticsSettingsPersistAndRejectInvalidAtomically(t *testing.T) {
	svc, db := testService(t, nil, nil)
	values := map[string]string{"diagnostics_verbose_logs": "true", "diagnostics_intermediate_images": "false", "stylist_keep_originals": "false", "diagnostics_retention": "1"}
	require.NoError(t, svc.UpdateSettings(values))
	got, err := svc.GetSettings()
	require.NoError(t, err)
	for key, value := range values {
		require.Equal(t, value, got[key])
	}
	for _, invalid := range []map[string]string{
		{"diagnostics_verbose_logs": "yes"}, {"diagnostics_intermediate_images": "TRUE"}, {"stylist_keep_originals": "0"}, {"diagnostics_retention": "0"}, {"diagnostics_retention": "51"}, {"diagnostics_retention": "1.5"},
	} {
		require.Error(t, svc.UpdateSettings(invalid))
	}
	require.Error(t, svc.UpdateSettings(map[string]string{"diagnostics_verbose_logs": "false", "diagnostics_retention": "-1"}))
	persisted, err := db.GetSetting("diagnostics_verbose_logs")
	require.NoError(t, err)
	require.Equal(t, "true", persisted)
}
