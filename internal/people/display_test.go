package people

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p    serverclient.Person
		want string
	}{
		{"display name", serverclient.Person{Name: "natasha_curated", DisplayName: "Наташа"}, "Наташа"},
		{"no display name", serverclient.Person{Name: "anna"}, "anna"},
		{"trimmed", serverclient.Person{Name: "natasha_curated", DisplayName: "  Наташа \t"}, "Наташа"},
		{"spaces only", serverclient.Person{Name: "anna", DisplayName: "   "}, "anna"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Key(tt.p))
		})
	}
}

func displayPeople() []serverclient.Person {
	return []serverclient.Person{
		{Name: "natasha_curated", DisplayName: "Наташа", Base: ""},
		{Name: "natasha_curated_moss", DisplayName: " Наташа ", Base: "Moss-Image", Identity: true},
		{Name: "natasha_curated_flux1", DisplayName: "Наташа", Base: "Flux.1", Identity: true},
		{Name: "anna", Base: "Z-Image", Identity: true},
		{Name: "olga_sdxl", DisplayName: "Ольга", Base: "SDXL"},
	}
}

func TestLorasByBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		list []serverclient.Person
		key  string
		want map[string]string
	}{
		{"several loras, empty base is Z-Image, trimmed", displayPeople(), "Наташа", map[string]string{
			"Z-Image":    "natasha_curated",
			"Moss-Image": "natasha_curated_moss",
			"Flux.1":     "natasha_curated_flux1",
		}},
		{"lora without name keyed by its name", displayPeople(), "anna", map[string]string{"Z-Image": "anna"}},
		{"lora name not selectable when it has display name", displayPeople(), "natasha_curated", nil},
		{"single", displayPeople(), "Ольга", map[string]string{"SDXL": "olga_sdxl"}},
		{"no match", displayPeople(), "Мария", nil},
		{"empty list", nil, "Наташа", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := LorasByBase(tt.list, tt.key)
			if tt.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFaceSwapName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		list  []serverclient.Person
		key   string
		bases []string
		want  string
	}{
		{"first base has no identity, second has", displayPeople(), "Наташа", []string{"Z-Image", "SDXL", "Flux.1"}, "natasha_curated_flux1"},
		{"order of bases decides", displayPeople(), "Наташа", []string{"Flux.1", "Moss-Image"}, "natasha_curated_flux1"},
		{"order of bases decides reversed", displayPeople(), "Наташа", []string{"Moss-Image", "Flux.1"}, "natasha_curated_moss"},
		{"no identity on matching bases", displayPeople(), "Наташа", []string{"Z-Image", "SDXL"}, ""},
		{"lora without name", displayPeople(), "anna", []string{"Z-Image"}, "anna"},
		{"identity lora but base not in steps", displayPeople(), "anna", []string{"SDXL"}, ""},
		{"no identity at all", displayPeople(), "Ольга", []string{"SDXL"}, ""},
		{"no match", displayPeople(), "Мария", []string{"Z-Image"}, ""},
		{"no bases", displayPeople(), "Наташа", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, FaceSwapName(tt.list, tt.key, tt.bases))
		})
	}
}
