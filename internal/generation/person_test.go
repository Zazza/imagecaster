package generation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

const personStyleLoras = `[{"name":"style-lora","weight":0.8}]`

func personPreset(loras string) *preset.Preset {
	return &preset.Preset{Name: "zimage-style", ModelName: "z-image-turbo", Loras: loras}
}

func decodeLoras(t *testing.T, raw string) []preset.LoRAEntry {
	t.Helper()
	var out []preset.LoRAEntry
	require.NoError(t, json.Unmarshal([]byte(raw), &out), "loras json: %q", raw)
	return out
}

func TestApplyPresetOverrides_PersonAddsLora(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		base string
		ov   *PresetOverrides
		want []preset.LoRAEntry
	}{
		{
			name: "style lora kept, person appended",
			base: personStyleLoras,
			ov:   &PresetOverrides{Person: &PersonOverride{Name: "zkv", Trigger: "zkv_man"}},
			want: []preset.LoRAEntry{
				{Name: "style-lora", Weight: 0.8},
				{Name: "zkv", Weight: 1, Trigger: "zkv_man"},
			},
		},
		{
			name: "style without loras",
			base: "",
			ov:   &PresetOverrides{Person: &PersonOverride{Name: "anna_k", Trigger: "anna_k_woman"}},
			want: []preset.LoRAEntry{{Name: "anna_k", Weight: 1, Trigger: "anna_k_woman"}},
		},
		{
			name: "gender person",
			base: personStyleLoras,
			ov:   &PresetOverrides{Person: &PersonOverride{Name: "kid01", Trigger: "kid01_person"}},
			want: []preset.LoRAEntry{
				{Name: "style-lora", Weight: 0.8},
				{Name: "kid01", Weight: 1, Trigger: "kid01_person"},
			},
		},
		{
			name: "loras override applied first, person appended after",
			base: personStyleLoras,
			ov: &PresetOverrides{
				Loras:  strPtr(`[{"name":"user-lora","weight":0.5}]`),
				Person: &PersonOverride{Name: "zkv", Trigger: "zkv_woman"},
			},
			want: []preset.LoRAEntry{
				{Name: "user-lora", Weight: 0.5},
				{Name: "zkv", Weight: 1, Trigger: "zkv_woman"},
			},
		},
		{
			name: "name at max length",
			base: "",
			ov:   &PresetOverrides{Person: &PersonOverride{Name: strings.Repeat("a", 32), Trigger: strings.Repeat("a", 32) + "_man"}},
			want: []preset.LoRAEntry{{Name: strings.Repeat("a", 32), Weight: 1, Trigger: strings.Repeat("a", 32) + "_man"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := personPreset(tt.base)

			ApplyPresetOverrides(p, tt.ov)

			assert.Equal(t, tt.want, decodeLoras(t, p.Loras))
		})
	}
}

func TestApplyPresetOverrides_InvalidPersonIgnored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		person PersonOverride
	}{
		{name: "lora tag injection in name", person: PersonOverride{Name: "x><lora:evil", Trigger: "x><lora:evil_man"}},
		{name: "unknown gender in trigger", person: PersonOverride{Name: "zkv", Trigger: "zkv_dog"}},
		{name: "trigger for another name", person: PersonOverride{Name: "zkv", Trigger: "other_man"}},
		{name: "trigger with extra suffix", person: PersonOverride{Name: "zkv", Trigger: "zkv_man, nsfw"}},
		{name: "empty", person: PersonOverride{}},
		{name: "empty trigger", person: PersonOverride{Name: "zkv"}},
		{name: "name too short", person: PersonOverride{Name: "z", Trigger: "z_man"}},
		{name: "name too long", person: PersonOverride{Name: strings.Repeat("a", 33), Trigger: strings.Repeat("a", 33) + "_man"}},
		{name: "uppercase name", person: PersonOverride{Name: "ZKV", Trigger: "ZKV_man"}},
		{name: "name with dash", person: PersonOverride{Name: "zk-v", Trigger: "zk-v_man"}},
		{name: "name with space", person: PersonOverride{Name: "zk v", Trigger: "zk v_man"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := personPreset(personStyleLoras)
			person := tt.person

			ApplyPresetOverrides(p, &PresetOverrides{Person: &person})

			assert.Equal(t, personStyleLoras, p.Loras)
		})
	}
}

func TestApplyPresetOverrides_InvalidPersonKeepsLorasOverride(t *testing.T) {
	t.Parallel()
	p := personPreset(personStyleLoras)
	override := `[{"name":"user-lora","weight":0.5}]`

	ApplyPresetOverrides(p, &PresetOverrides{Loras: strPtr(override), Person: &PersonOverride{Name: "zkv", Trigger: "zkv_dog"}})

	assert.Equal(t, override, p.Loras)
}

func TestPresetOverrides_PersonFromJSON(t *testing.T) {
	t.Parallel()
	var ov PresetOverrides
	require.NoError(t, json.Unmarshal([]byte(`{"person":{"name":"zkv","trigger":"zkv_man"}}`), &ov))

	require.NotNil(t, ov.Person)
	assert.Equal(t, "zkv", ov.Person.Name)
	assert.Equal(t, "zkv_man", ov.Person.Trigger)

	out, err := json.Marshal(PresetOverrides{})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "person")
}

func TestLoRAEntry_TriggerJSON(t *testing.T) {
	t.Parallel()
	plain, err := json.Marshal(preset.LoRAEntry{Name: "detail", Weight: 0.8})
	require.NoError(t, err)
	assert.NotContains(t, string(plain), "trigger")

	withTrigger, err := json.Marshal(preset.LoRAEntry{Name: "zkv", Weight: 1, Trigger: "zkv_man"})
	require.NoError(t, err)
	assert.Contains(t, string(withTrigger), `"trigger":"zkv_man"`)
}

func TestAppendLorasToPrompt_Trigger(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		prompt   string
		loras    string
		expected string
	}{
		{
			name:     "person with style lora",
			prompt:   "a man in a cafe",
			loras:    `[{"name":"style-lora","weight":0.8},{"name":"zkv","weight":1,"trigger":"zkv_man"}]`,
			expected: "zkv_man, a man in a cafe <lora:style-lora:0.8> <lora:zkv:1>",
		},
		{
			name:     "person only",
			prompt:   "portrait on the beach",
			loras:    `[{"name":"zkv","weight":1,"trigger":"zkv_woman"}]`,
			expected: "zkv_woman, portrait on the beach <lora:zkv:1>",
		},
		{
			name:     "empty prompt",
			prompt:   "",
			loras:    `[{"name":"zkv","weight":1,"trigger":"zkv_man"}]`,
			expected: "zkv_man <lora:zkv:1>",
		},
		{
			name:     "no trigger unchanged",
			prompt:   "test prompt",
			loras:    `[{"name":"detail","weight":0.8}]`,
			expected: "test prompt <lora:detail:0.8>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, appendLorasToPrompt(tt.prompt, tt.loras))
		})
	}
}

func TestPersonOverride_EndToEndPrompt(t *testing.T) {
	t.Parallel()
	p := personPreset(personStyleLoras)

	ApplyPresetOverrides(p, &PresetOverrides{Person: &PersonOverride{Name: "zkv", Trigger: "zkv_man"}})
	got := appendLorasToPrompt("a man in a cafe", p.Loras)

	assert.True(t, strings.HasPrefix(got, "zkv_man, "), "prompt %q", got)
	assert.Contains(t, got, "<lora:zkv:1>")
	assert.Contains(t, got, "<lora:style-lora:0.8>")
	assert.True(t, strings.HasSuffix(got, ">"), "lora tags must be at the end: %q", got)
}

func TestPersonOverride_InvalidLeavesPromptClean(t *testing.T) {
	t.Parallel()
	p := personPreset(personStyleLoras)

	ApplyPresetOverrides(p, &PresetOverrides{Person: &PersonOverride{Name: "x><lora:evil", Trigger: "x><lora:evil_man"}})
	got := appendLorasToPrompt("a man in a cafe", p.Loras)

	assert.Equal(t, "a man in a cafe <lora:style-lora:0.8>", got)
	assert.NotContains(t, got, "evil")
}
