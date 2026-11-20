package stylist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithWear(t *testing.T) {
	tests := []struct {
		name, outfit, wear, want string
	}{
		{"no wear", "a white blouse", "", "a white blouse"},
		{"appended", "a white blouse", "blouse hanging open", "a white blouse, blouse hanging open"},
		{"trims dot and spaces", " a white blouse ", " torn sleeve. ", "a white blouse, torn sleeve"},
		{"no outfit", "", "torn sleeve", "torn sleeve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WithWear(tt.outfit, tt.wear))
		})
	}
}

func TestFramePrompt_WearInOutfit(t *testing.T) {
	s := Series{Outfit: "a white blouse, black heels", Hair: "bun", Background: "a park"}
	tests := []struct {
		name   string
		base   string
		styled bool
		shot   Shot
		want   string
	}{
		{"prose", "Z-Image", false, Shot{Prompt: "sitting", Framing: "full", Wear: "blouse hanging open"}, "Wearing a white blouse, black heels, blouse hanging open."},
		{"prose close drops shoes keeps wear", "Z-Image", false, Shot{Prompt: "smiling", Framing: "close", Wear: "torn collar"}, "Wearing a white blouse, torn collar."},
		{"styled prose", "Z-Image", true, Shot{Prompt: "sitting", Wear: "blouse hanging open"}, "Wearing a white blouse, black heels, blouse hanging open."},
		{"tags", "SDXL", false, Shot{Prompt: "sitting", Wear: "torn sleeve"}, "a white blouse, black heels, torn sleeve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FramePrompt(tt.base, "", "a woman", s, tt.shot)
			if tt.styled {
				got = StyledFramePrompt(tt.base, "", "a woman", s, tt.shot)
			}
			assert.Contains(t, got, tt.want)
			assert.NotContains(t, got, "sitting, blouse")
		})
	}
}
