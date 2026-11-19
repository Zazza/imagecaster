package stylist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModels_Flux1(t *testing.T) {
	m, ok := Models["Flux.1"]
	require.True(t, ok, "Flux.1 must be a stylist base")
	assert.Equal(t, "flux1-dev-fp8", m.Model)
	assert.Equal(t, "euler", m.Sampler)
	assert.Equal(t, 20, m.Steps)
	assert.InDelta(t, 1.0, m.CFG, 1e-9)
}

func TestModels_Flux1KeepsOtherBases(t *testing.T) {
	want := map[string]string{
		"Z-Image":    "z-image-turbo-fp8-aio",
		"Moss-Image": "moss_image_sft_bf16",
		"SDXL":       "juggernautXL_ragnarokBy",
		"Flux.2":     "flux-2-klein-4b",
	}
	for base, model := range want {
		assert.Equal(t, model, Models[base].Model, base)
	}
}
