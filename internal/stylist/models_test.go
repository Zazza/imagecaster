package stylist

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func oldShootPrompt(lora, appearance, outfit string, lookParts []string, shot, background string) string {
	parts := []string{appearance}
	if strings.TrimSpace(outfit) == "" {
		parts = append(parts, "wearing simple casual everyday clothes")
	}
	parts = append(parts, lookParts...)
	quality := "BREAK, a candid photograph with natural proportions and honest imperfections: realistic skin texture, natural light, documentary framing, no stylization"
	if strings.Contains(strings.ToLower(background), "studio") {
		quality = "BREAK, a professional studio portrait with realistic skin texture and natural pores, sharp focus, straight symmetrical framing, no stylization"
	}
	parts = append(parts, shot, background, quality)
	clean := make([]string, 0, len(parts))
	for _, v := range parts {
		if v = strings.TrimSpace(v); v != "" {
			clean = append(clean, v)
		}
	}
	return strings.Join(clean, ", ") + fmt.Sprintf(" <lora:%s:1>", lora)
}

func TestComposePrompt_GoldenOldShootPrompt(t *testing.T) {
	tests := []struct {
		name       string
		appearance string
		outfit     string
		look       []string
		shot       string
		background string
		want       string
	}{
		{
			name: "studio default outfit", appearance: "a woman", shot: "front", background: DefaultBackground,
			want: "a woman, wearing simple casual everyday clothes, front, plain light grey studio backdrop, soft even studio light, " + QualityStudio + " <lora:anna_zimage:1>",
		},
		{
			name: "candid with look", appearance: " a man ", outfit: "a navy suit", look: []string{"short haircut", "a navy suit", ""}, shot: "standing, waist-up", background: "an evening old city street",
			want: "a man, short haircut, a navy suit, standing, waist-up, an evening old city street, " + Quality + " <lora:anna_zimage:1>",
		},
		{
			name: "empty shot and background", appearance: "a woman", outfit: "x", look: []string{"x"},
			want: "a woman, x, " + Quality + " <lora:anna_zimage:1>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := []string{tt.appearance}
			if strings.TrimSpace(tt.outfit) == "" {
				parts = append(parts, DefaultOutfit)
			}
			parts = append(parts, tt.look...)
			parts = append(parts, tt.shot)
			got := ComposePrompt("anna_zimage", parts, tt.background)
			assert.Equal(t, oldShootPrompt("anna_zimage", tt.appearance, tt.outfit, tt.look, tt.shot, tt.background), got)
			assert.Equal(t, tt.want, got)
		})
	}
}
