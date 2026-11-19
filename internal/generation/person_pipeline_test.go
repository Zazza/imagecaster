package generation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepPersonLora(t *testing.T) {
	t.Parallel()
	natasha := map[string]string{
		"Z-Image": "natasha_curated",
		"Flux.1":  "natasha_curated_flux1",
	}
	tests := []struct {
		name string
		ov   *PersonOverride
		base string
		want string
	}{
		{"map hit first base", &PersonOverride{Name: "natasha_curated", Loras: natasha}, "Z-Image", "natasha_curated"},
		{"map hit other base", &PersonOverride{Name: "natasha_curated", Loras: natasha}, "Flux.1", "natasha_curated_flux1"},
		{"map miss", &PersonOverride{Name: "natasha_curated", Loras: natasha}, "SDXL", ""},
		{"nil override", nil, "Z-Image", ""},
		{"old form without map", &PersonOverride{Name: "natasha_curated"}, "Z-Image", ""},
		{"empty map", &PersonOverride{Name: "natasha_curated", Loras: map[string]string{}}, "Z-Image", ""},
		{"empty base", &PersonOverride{Name: "natasha_curated", Loras: natasha}, "", ""},
		{"invalid lora name uppercase", &PersonOverride{Loras: map[string]string{"Z-Image": "Natasha"}}, "Z-Image", ""},
		{"invalid lora name path", &PersonOverride{Loras: map[string]string{"Z-Image": "../evil"}}, "Z-Image", ""},
		{"invalid lora name too short", &PersonOverride{Loras: map[string]string{"Z-Image": "a"}}, "Z-Image", ""},
		{"invalid lora name with lora tag", &PersonOverride{Loras: map[string]string{"Z-Image": "x> <lora:y:9"}}, "Z-Image", ""},
		{"empty lora name", &PersonOverride{Loras: map[string]string{"Z-Image": ""}}, "Z-Image", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, stepPersonLora(tt.ov, tt.base))
		})
	}
}

func TestPersonOverride_PipelineJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want PersonOverride
	}{
		{
			"new form",
			`{"name":"natasha_curated","loras":{"Z-Image":"natasha_curated","Flux.1":"natasha_curated_flux1"},"face_swap":true,"face_swap_name":"natasha_curated_flux1"}`,
			PersonOverride{
				Name:         "natasha_curated",
				Loras:        map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"},
				FaceSwap:     true,
				FaceSwapName: "natasha_curated_flux1",
			},
		},
		{
			"old form",
			`{"name":"zkv","face_swap":true}`,
			PersonOverride{Name: "zkv", FaceSwap: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got PersonOverride
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &got))
			assert.Equal(t, tt.want, got)

			out, err := json.Marshal(got)
			require.NoError(t, err)
			var back PersonOverride
			require.NoError(t, json.Unmarshal(out, &back))
			assert.Equal(t, tt.want, back)
		})
	}
}

func TestPersonOverride_OldFormOmitsNewFields(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(PersonOverride{Name: "zkv"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "loras")
	assert.NotContains(t, string(out), "face_swap_name")
}

func TestGenerateCompoundImageParams_PersonJSON(t *testing.T) {
	t.Parallel()
	raw := `{"compound_preset_id":7,"extra_prompt":"in a cafe","person":{"name":"natasha_curated","loras":{"Z-Image":"natasha_curated","Flux.1":"natasha_curated_flux1"},"face_swap":true,"face_swap_name":"natasha_curated_flux1"}}`

	var p GenerateCompoundImageParams
	require.NoError(t, json.Unmarshal([]byte(raw), &p))

	assert.Equal(t, int64(7), p.CompoundPresetID)
	require.NotNil(t, p.Person)
	assert.Equal(t, "natasha_curated", p.Person.Name)
	assert.Equal(t, map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"}, p.Person.Loras)
	assert.True(t, p.Person.FaceSwap)
	assert.Equal(t, "natasha_curated_flux1", p.Person.FaceSwapName)

	out, err := json.Marshal(p)
	require.NoError(t, err)
	var back GenerateCompoundImageParams
	require.NoError(t, json.Unmarshal(out, &back))
	assert.Equal(t, p, back)
}

func TestGenerateCompoundImageParams_NoPerson(t *testing.T) {
	t.Parallel()
	var p GenerateCompoundImageParams
	require.NoError(t, json.Unmarshal([]byte(`{"compound_preset_id":7}`), &p))
	assert.Nil(t, p.Person)

	out, err := json.Marshal(GenerateCompoundImageParams{CompoundPresetID: 7})
	require.NoError(t, err)
	assert.NotContains(t, string(out), `"person"`)
}
