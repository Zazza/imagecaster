package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/stylist"
)

func xrUnknownBasePreset(t *testing.T, e *pprEnv) *preset.Preset {
	t.Helper()
	m := stylist.Models["Z-Image"]
	p := &preset.Preset{Name: "custom", PresetType: "realism", Prompt: "photo", ModelName: "my_private_merge_v3.safetensors", Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG}
	require.NoError(t, e.db.Create(p))
	return p
}

func TestTools_GeneratePersonUnknownBaseXR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		person   string
		wantLora map[string]string
		wantErr  bool
	}{
		{"exact lora name keeps old behaviour", "natasha_curated_moss", map[string]string{"natasha_curated_moss": "1"}, false},
		{"lora without display name", "ivan_x", map[string]string{"ivan_x": "1"}, false},
		{"display name needs a known base", "Наташа", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := pprNewEnv(t, pnNatasha(false, false))
			p := xrUnknownBasePreset(t, e)
			args := map[string]any{"preset_id": float64(p.ID), "seed": float64(3), "overrides": map[string]any{"person": map[string]any{"name": tt.person}}}

			out, err := styExec(e.srv, "generate", args)

			if tt.wantErr {
				require.Error(t, err, out)
				assert.Zero(t, e.sd.calls())
				return
			}
			require.NoError(t, err, out)
			reqs := e.sd.txtReqs()
			require.Len(t, reqs, 1)
			assert.Equal(t, tt.wantLora, pprLoras(reqs[0].Prompt), reqs[0].Prompt)
		})
	}
}
