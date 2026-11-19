package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestShootPrompt_FormByBase(t *testing.T) {
	t.Parallel()
	look := Look{Outfit: "a red dress"}
	tests := []struct {
		name      string
		base      string
		wantBreak bool
	}{
		{"SDXL keeps tag form", "SDXL", true},
		{"empty base is Z-Image prose", "", false},
		{"Z-Image prose", "Z-Image", false},
		{"Moss-Image prose", "Moss-Image", false},
		{"Flux.1 prose", "Flux.1", false},
		{"Flux.2 prose", "Flux.2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shootPromptFor(personBase(shootPerson("anna", tt.base)), shootPerson("anna", tt.base).Lora, "a woman", look, "standing by the window", "a park")
			if tt.wantBreak {
				assert.Contains(t, got, ", BREAK, ")
			} else {
				assert.NotContains(t, got, "BREAK")
				assert.NotContains(t, got, ", ,")
			}
			assert.True(t, strings.HasSuffix(got, "<lora:anna_lora_v1:1>"), "%q", got)
		})
	}
}

func TestShootPrompt_ProseContent(t *testing.T) {
	t.Parallel()
	full := Look{HairCut: "a short bob", HairColor: "copper red hair", Outfit: "a navy suit", Headwear: "a straw hat", Makeup: "smoky eyes", Free: "holding a coffee cup", Glasses: "round glasses"}
	tests := []struct {
		name   string
		look   Look
		wantIn []string
	}{
		{"every look field appears", full, []string{"a short bob", "copper red hair", "a navy suit", "a straw hat", "smoky eyes", "holding a coffee cup", "round glasses"}},
		{"empty outfit uses default", Look{}, []string{"wearing simple casual everyday clothes"}},
		{"glasses none means no glasses", Look{Outfit: "a navy suit", Glasses: "none"}, []string{"no glasses"}},
	}
	const shot = "leaning on a railing"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := serverclient.Person{Lora: "anna", Base: "Z-Image"}
			got := shootPromptFor(personBase(p), p.Lora, "a woman", tt.look, shot, "a park")
			low := strings.ToLower(got)
			for _, w := range tt.wantIn {
				assert.Contains(t, low, strings.ToLower(w), "%q", got)
			}
			assert.Equal(t, 1, strings.Count(got, shot), "%q", got)
			assert.NotContains(t, got, "BREAK")
			assert.True(t, strings.HasSuffix(got, "<lora:anna:1>"), "%q", got)
		})
	}
}

func TestShootPromptFor_NoLora(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"Z-Image", "Flux.2", "SDXL"} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			got := shootPromptFor(base, "", "a woman", Look{Outfit: "a red dress"}, "standing", "a park")
			assert.NotContains(t, got, "<lora:")
			assert.Contains(t, got, "a red dress")
		})
	}
}
