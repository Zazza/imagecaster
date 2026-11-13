package session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddToSession_SamplerFieldXReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		info string
		want string
	}{
		{"new format sampler", `{"prompt":"p","sampler":"euler_a","steps":20}`, "euler_a"},
		{"legacy sampler_name", `{"prompt":"p","sampler_name":"DPM++ 2M","steps":20}`, "DPM++ 2M"},
		{"sampler wins over sampler_name", `{"prompt":"p","sampler":"dpm++2m","sampler_name":"Euler a","steps":20}`, "dpm++2m"},
		{"empty sampler falls back to legacy", `{"prompt":"p","sampler":"","sampler_name":"Euler a","steps":20}`, "Euler a"},
		{"neither present", `{"prompt":"p","steps":20}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := testService(t)
			id := svc.AddToSession(makeImageJPEG(t, 32, 32), json.RawMessage(tt.info), "sd", false, nil)
			require.Greater(t, id, int64(0))
			item, err := svc.SessionItem(id)
			require.NoError(t, err)
			require.NotNil(t, item)
			assert.Equal(t, tt.want, item.Sampler)
		})
	}
}

func TestAddToSession_SamplerFieldWrappedInfoXReview(t *testing.T) {
	t.Parallel()
	svc, _, _ := testService(t)
	raw, err := json.Marshal(`{"prompt":"p","sampler":"lcm"}`)
	require.NoError(t, err)
	id := svc.AddToSession(makeImageJPEG(t, 32, 32), raw, "sd", false, nil)
	require.Greater(t, id, int64(0))
	item, err := svc.SessionItem(id)
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "lcm", item.Sampler)
}
