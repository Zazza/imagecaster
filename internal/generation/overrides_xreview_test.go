package generation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Zazza/imagecaster/internal/preset"
)

func xrStr(v string) *string { return &v }

func TestApplyPresetOverridesForgeNamesXReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		ov                     PresetOverrides
		wantSampler, wantSched string
	}{
		{"euler a", PresetOverrides{Sampler: xrStr("Euler a")}, "euler_a", "exponential"},
		{"combined sampler no schedule override", PresetOverrides{Sampler: xrStr("DPM++ 2M Karras")}, "dpm++2m", "karras"},
		{"explicit schedule wins over embedded", PresetOverrides{Sampler: xrStr("DPM++ 2M Karras"), ScheduleType: xrStr("Exponential")}, "dpm++2m", "exponential"},
		{"explicit sdcpp schedule wins over embedded", PresetOverrides{Sampler: xrStr("DPM++ 2M Karras"), ScheduleType: xrStr("sgm_uniform")}, "dpm++2m", "sgm_uniform"},
		{"schedule Karras", PresetOverrides{ScheduleType: xrStr("Karras")}, "heun", "karras"},
		{"sdcpp names unchanged", PresetOverrides{Sampler: xrStr("dpm++2m"), ScheduleType: xrStr("karras")}, "dpm++2m", "karras"},
		{"unknown sampler keeps preset", PresetOverrides{Sampler: xrStr("TotallyUnknownSampler")}, "heun", "exponential"},
		{"unknown scheduler keeps preset", PresetOverrides{ScheduleType: xrStr("TotallyUnknownSchedule")}, "heun", "exponential"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := preset.Preset{Name: "x", Sampler: "heun", ScheduleType: "exponential", Steps: 20, CfgScale: 7}
			ov := tt.ov
			ApplyPresetOverrides(&p, &ov)
			assert.Equal(t, tt.wantSampler, p.Sampler)
			assert.Equal(t, tt.wantSched, p.ScheduleType)
			assert.Equal(t, 20, p.Steps)
		})
	}
}
