package preset

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type depsPreset struct {
	model   string
	loras   string
	bundled bool
}

func depsSources(t *testing.T, presets ...depsPreset) []PresetSource {
	t.Helper()
	items := []map[string]any{}
	for i, p := range presets {
		if !p.bundled {
			continue
		}
		items = append(items, map[string]any{"name": "deps-" + string(rune('a'+i)), "preset_type": "post", "model_name": p.model, "loras": p.loras})
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "presets": items})
	require.NoError(t, err)
	return []PresetSource{{File: "deps.json", Raw: raw}}
}

func TestSourceDependencies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		presets    []depsPreset
		wantModels []string
		wantLoras  []string
	}{
		{
			name:       "no presets",
			presets:    nil,
			wantModels: nil,
			wantLoras:  nil,
		},
		{
			name: "only user presets ignored",
			presets: []depsPreset{
				{model: "user_model.safetensors", loras: `[{"name":"user_lora","weight":1}]`, bundled: false},
			},
			wantModels: nil,
			wantLoras:  nil,
		},
		{
			name: "distinct models ordered by usage desc",
			presets: []depsPreset{
				{model: "a.safetensors", bundled: true},
				{model: "b.safetensors", bundled: true},
				{model: "b.safetensors", bundled: true},
				{model: "c.safetensors", bundled: true},
				{model: "c.safetensors", bundled: true},
				{model: "c.safetensors", bundled: true},
			},
			wantModels: []string{"c.safetensors", "b.safetensors", "a.safetensors"},
		},
		{
			name: "ties ordered by name ascending",
			presets: []depsPreset{
				{model: "zeta.safetensors", bundled: true},
				{model: "alpha.safetensors", bundled: true},
				{model: "mid.safetensors", bundled: true},
			},
			wantModels: []string{"alpha.safetensors", "mid.safetensors", "zeta.safetensors"},
		},
		{
			name: "user presets do not affect counts",
			presets: []depsPreset{
				{model: "a.safetensors", bundled: true},
				{model: "a.safetensors", bundled: true},
				{model: "b.safetensors", bundled: true},
				{model: "b.safetensors", bundled: false},
				{model: "b.safetensors", bundled: false},
				{model: "b.safetensors", bundled: false},
			},
			wantModels: []string{"a.safetensors", "b.safetensors"},
		},
		{
			name: "empty model names skipped",
			presets: []depsPreset{
				{model: "", bundled: true},
				{model: "", bundled: true},
				{model: "a.safetensors", bundled: true},
			},
			wantModels: []string{"a.safetensors"},
		},
		{
			name: "loras distinct ordered by usage then name",
			presets: []depsPreset{
				{model: "m.safetensors", loras: `[{"name":"lb","weight":0.5},{"name":"la","weight":1}]`, bundled: true},
				{model: "m.safetensors", loras: `[{"name":"lc","weight":0.5}]`, bundled: true},
				{model: "m.safetensors", loras: `[{"name":"lc","weight":0.7}]`, bundled: true},
				{model: "m.safetensors", loras: `[{"name":"user_only","weight":1}]`, bundled: false},
			},
			wantModels: []string{"m.safetensors"},
			wantLoras:  []string{"lc", "la", "lb"},
		},
		{
			name: "empty lora names skipped",
			presets: []depsPreset{
				{model: "m.safetensors", loras: `[{"name":"","weight":1},{"name":"la","weight":1}]`, bundled: true},
			},
			wantModels: []string{"m.safetensors"},
			wantLoras:  []string{"la"},
		},
		{
			name: "invalid loras json skipped, not an error",
			presets: []depsPreset{
				{model: "m.safetensors", loras: `{not json`, bundled: true},
				{model: "n.safetensors", loras: `[{"name":"la","weight":1}]`, bundled: true},
			},
			wantModels: []string{"m.safetensors", "n.safetensors"},
			wantLoras:  []string{"la"},
		},
		{
			name: "preset without model still contributes loras",
			presets: []depsPreset{
				{model: "", loras: `[{"name":"la","weight":1}]`, bundled: true},
			},
			wantModels: nil,
			wantLoras:  []string{"la"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			models, loras, err := SourceDependencies(depsSources(t, tt.presets...))
			require.NoError(t, err)
			if len(tt.wantModels) == 0 {
				assert.Empty(t, models)
			} else {
				assert.Equal(t, tt.wantModels, models)
			}
			if len(tt.wantLoras) == 0 {
				assert.Empty(t, loras)
			} else {
				assert.Equal(t, tt.wantLoras, loras)
			}
		})
	}
}
