package stylist

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var proseBases = []string{"Z-Image", "Moss-Image", "Flux.1", "Flux.2"}

func firstSentence(s string) string {
	if i := strings.IndexAny(s, ".!?"); i >= 0 {
		return s[:i]
	}
	return s
}

func TestFramePrompt_SDXLAndUnknownKeepTagForm(t *testing.T) {
	t.Parallel()
	s := Series{Outfit: "a floor-length dress, high heels", Makeup: "red lips", Background: "studio backdrop"}
	tail := "red lips, studio backdrop, " + QualityStudio + " <lora:anna:1>"
	want := "full body shot, facing camera, a woman, a floor-length dress, high heels, " + tail
	for _, base := range []string{"SDXL", "", "SD1.5"} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			got := FramePrompt(base, "anna", "a woman", s, Shot{Prompt: "facing camera", Framing: "full"})
			assert.Equal(t, want, got)
		})
	}
}

func TestFramePrompt_ProseFraming(t *testing.T) {
	t.Parallel()
	s := Series{Outfit: "a floor-length dress, high heels", Hair: "long wavy hair", Makeup: "red lips", Background: "a city street at dusk"}
	tests := []struct {
		framing   string
		words     string
		keepsFull bool
	}{
		{"close", "close-up", false},
		{"waist", "waist-up", false},
		{"knee", "knee-up", true},
		{"full", "full-body", true},
	}
	for _, base := range proseBases {
		for _, tt := range tests {
			t.Run(base+"/"+tt.framing, func(t *testing.T) {
				t.Parallel()
				got := FramePrompt(base, "anna", "a woman", s, Shot{Prompt: "facing camera", Framing: tt.framing})
				assert.NotContains(t, got, "BREAK")
				assert.NotContains(t, got, ", ,")
				assert.True(t, strings.HasPrefix(got, "A "), "%q", got)
				assert.Contains(t, strings.ToLower(firstSentence(got)), tt.words, "%q", got)
				assert.Contains(t, got, "Wearing ")
				if tt.keepsFull {
					assert.Contains(t, got, "heels")
					assert.Contains(t, got, "floor-length")
				} else {
					assert.NotContains(t, got, "heels")
					assert.NotContains(t, got, "floor-length")
					assert.Contains(t, got, "dress")
				}
			})
		}
	}
}

func TestFramePrompt_ProseLabelsAndTail(t *testing.T) {
	t.Parallel()
	candidTail := strings.ToLower(strings.TrimPrefix(Quality, "BREAK, "))
	studioTail := strings.ToLower(strings.TrimPrefix(QualityStudio, "BREAK, "))
	tests := []struct {
		name       string
		lora       string
		series     Series
		wantIn     []string
		wantNotIn  []string
		wantSuffix string
	}{
		{
			name:       "all fields, candid, lora",
			lora:       "anna",
			series:     Series{Outfit: "a red dress", Hair: "long wavy hair", Makeup: "red lips", Background: "a city street at dusk"},
			wantIn:     []string{"Wearing a red dress", "Hair: long wavy hair", "Makeup: red lips", "Background: a city street at dusk", candidTail},
			wantNotIn:  []string{studioTail},
			wantSuffix: " <lora:anna:1>",
		},
		{
			name:       "studio background in any case uses studio tail",
			lora:       "anna",
			series:     Series{Outfit: "a red dress", Background: "a grey STUDIO backdrop"},
			wantIn:     []string{"Background: a grey STUDIO backdrop", studioTail},
			wantNotIn:  []string{candidTail},
			wantSuffix: " <lora:anna:1>",
		},
		{
			name:      "empty hair and makeup produce no labels, no lora",
			series:    Series{Outfit: "a red dress", Background: "a park"},
			wantIn:    []string{"Wearing a red dress", "Background: a park"},
			wantNotIn: []string{"Hair:", "Makeup:", "<lora:"},
		},
		{
			name:      "outfit already starting with wearing is not doubled",
			series:    Series{Outfit: "wearing a red dress", Background: "a park"},
			wantIn:    []string{"a red dress"},
			wantNotIn: []string{"Wearing wearing", "wearing wearing"},
		},
	}
	for _, base := range proseBases {
		for _, tt := range tests {
			t.Run(base+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				got := FramePrompt(base, tt.lora, "a woman", tt.series, Shot{Prompt: "facing camera", Framing: "knee"})
				for _, w := range tt.wantIn {
					assert.Contains(t, strings.ToLower(got), strings.ToLower(w), "%q", got)
				}
				for _, w := range tt.wantNotIn {
					assert.NotContains(t, strings.ToLower(got), strings.ToLower(w), "%q", got)
				}
				assert.NotContains(t, got, "BREAK")
				assert.NotContains(t, got, ", ,")
				noLora := strings.TrimSuffix(got, tt.wantSuffix)
				if tt.wantSuffix != "" {
					require.True(t, strings.HasSuffix(got, tt.wantSuffix), "%q", got)
				}
				if strings.Contains(tt.series.Background, "STUDIO") || tt.series.Background == "a city street at dusk" {
					tail := candidTail
					if strings.Contains(tt.series.Background, "STUDIO") {
						tail = studioTail
					}
					end := strings.ToLower(strings.TrimSpace(noLora))
					assert.True(t, strings.HasSuffix(end, tail) || strings.HasSuffix(end, tail+"."), "%q", got)
				}
			})
		}
	}
}
