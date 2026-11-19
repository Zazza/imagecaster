package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestStylistShootPipeline_PromptFormByStepBases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		bases []string
		prose bool
	}{
		{"all prose steps give sentences", []string{"Z-Image", "Flux.1"}, true},
		{"mixed steps keep tags", []string{"Z-Image", "SDXL"}, false},
		{"SDXL first keeps tags", []string{"SDXL", "Z-Image"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := pnNewEnv(t, []serverclient.Person{pnPerson("anna", "Z-Image", "Анна", false)}, true)
			id := env.pipeline(t, "Цепочка", tt.bases...)

			out, err := styExec(env.s, "stylist_shoot", pnArgs("Анна", map[string]any{"pipeline": float64(id)}))

			require.NoError(t, err, out)
			_, p := sqJob(t, env.store, pnJobIDs(t, out)[0])
			assert.NotContains(t, p.Prompt, "<lora:")
			if tt.prose {
				assert.NotContains(t, p.Prompt, "BREAK", p.Prompt)
				assert.Regexp(t, `^A photograph of `, p.Prompt)
			} else {
				assert.Contains(t, p.Prompt, ", BREAK, ", p.Prompt)
			}
		})
	}
}
