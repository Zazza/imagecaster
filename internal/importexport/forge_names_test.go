package importexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/logger"
)

func TestImportItems_ConvertsForgeNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, sampler, sched   string
		wantSampler, wantSched string
	}{
		{"forge pair", "DPM++ 2M", "Karras", "dpm++2m", "karras"},
		{"composite sampler", "Euler a Karras", "", "euler_a", "karras"},
		{"automatic scheduler", "Euler", "Automatic", "euler", ""},
		{"already sdcpp", "dpm++2m_sde", "sgm_uniform", "dpm++2m_sde", "sgm_uniform"},
		{"unknown forge sampler", "Restart", "Simple", "", "simple"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			svc := New(db, &mockSDService{}, logger.New(nil))
			pd := dedupPresetData("n")
			pd.Name = "forge-" + tt.name
			pd.Sampler = tt.sampler
			pd.ScheduleType = tt.sched

			result, err := svc.ImportItems([]PresetData{pd})
			require.NoError(t, err)
			require.Len(t, result, 1)
			got, err := db.Get(result[0].ID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSampler, got.Sampler)
			assert.Equal(t, tt.wantSched, got.ScheduleType)
		})
	}
}
