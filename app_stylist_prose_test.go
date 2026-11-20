package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/stylist"
)

type proseSpPipes struct{ models []string }

func (p proseSpPipes) PipelineModels(int64) (string, []string, error) {
	return "Цепочка", p.models, nil
}

func TestStylistShoot_PipelinePromptFormByStepBases(t *testing.T) {
	m := func(b string) string { return stylist.Models[b].Model }
	tests := []struct {
		name   string
		models []string
		prose  bool
	}{
		{"all prose steps give sentences", []string{m("Z-Image"), m("Flux.1")}, true},
		{"mixed steps keep tags", []string{m("Z-Image"), m("SDXL")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			req := StylistShootRequest{Person: "anna", Pipeline: 7, Appearance: "170 cm, slim",
				Series: []stylist.Series{spSeries(stylist.Shot{Prompt: "facing camera", Framing: "knee"})}}
			_, err := stylistShoot(context.Background(), spPeopleList(), proseSpPipes{tt.models}, q, false, req)
			require.NoError(t, err)
			require.Len(t, q.jobs, 1)
			p := q.jobs[0].params.Prompt
			assert.NotContains(t, p, "<lora:")
			if tt.prose {
				assert.NotContains(t, p, "BREAK", p)
				assert.Contains(t, p, "A knee-up image of a woman, 170 cm, slim, facing camera.", p)
			} else {
				assert.Contains(t, p, "knee-up, facing camera, 170 cm, slim, ", p)
			}
			assert.NotContains(t, p, "photograph", "the workflow's styles decide the look")
		})
	}
}
