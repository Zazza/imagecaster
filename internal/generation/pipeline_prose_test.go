package generation

import (
	"context"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeBasesPipeline(t *testing.T, db *preset.DB, bases ...string) int64 {
	t.Helper()
	cp := &preset.CompoundPreset{Name: "bases"}
	for i, base := range bases {
		m := stylist.Models[base]
		p := makeTestPreset(t, db, &preset.Preset{Name: base, PresetType: "txt2img", Prompt: "photo", Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG, ModelName: m.Model})
		st := preset.CompoundPresetStep{StepOrder: i + 1, PresetID: p.ID}
		if i > 0 {
			st.DenoisingStrength = 0.5
		}
		cp.Steps = append(cp.Steps, st)
	}
	require.NoError(t, db.CreateCompoundPreset(cp))
	return cp.ID
}

func TestGenerateCompoundImage_SceneStyleJoinByBases(t *testing.T) {
	t.Parallel()
	const scene = "woman in a red coat on a pier"
	tests := []struct {
		name  string
		bases []string
		prose bool
	}{
		{"all prose models join as sentences", []string{"Z-Image", "Flux.1", "Moss-Image"}, true},
		{"single prose model joins as sentences", []string{"Flux.2"}, true},
		{"mixed pipeline keeps BREAK on every step", []string{"Z-Image", "SDXL", "Flux.1"}, false},
		{"SDXL only keeps BREAK", []string{"SDXL"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			cpID := makeBasesPipeline(t, db, tt.bases...)
			run := &pipelineRun{}
			svc := newTestService(t, db, &mockLLM{}, run.sd())
			svc.ctx = context.Background()
			svc.cfg.SDUrl = "http://192.168.1.10:7861"

			_, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{CompoundPresetID: cpID, ScenePrompt: scene})
			require.NoError(t, err)
			require.Len(t, run.prompts, len(tt.bases))
			for i, p := range run.prompts {
				if tt.prose {
					assert.NotContains(t, p, "BREAK", "step %d: %q", i+1, p)
					assert.Contains(t, p, scene+". photo", "step %d: %q", i+1, p)
				} else {
					assert.Contains(t, p, scene+", BREAK, photo", "step %d: %q", i+1, p)
				}
			}
		})
	}
}

func TestProseModels(t *testing.T) {
	t.Parallel()
	m := func(b string) string { return stylist.Models[b].Model }
	tests := []struct {
		name   string
		models []string
		want   bool
	}{
		{"empty", nil, false},
		{"all prose", []string{m("Z-Image"), m("Moss-Image"), m("Flux.1"), m("Flux.2")}, true},
		{"one SDXL step", []string{m("Z-Image"), m("SDXL")}, false},
		{"unknown model", []string{m("Z-Image"), "no-such-model"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ProseModels(tt.models))
		})
	}
}
