package serverclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_TrainFlux1(t *testing.T) {
	c := loadTestCatalog(t)
	b := trainBase(t, c, "Flux.1")
	require.Len(t, b.Files, 1)
	f := b.Files[0]
	assert.Equal(t, "sd", f.Type)
	assert.Equal(t, "flux1-dev-fp8.safetensors", f.Filename)
	assert.Equal(t, "https://huggingface.co/Comfy-Org/Flux-1-Dev/resolve/main/flux1-dev-fp8.safetensors", f.URL)
	assert.InDelta(t, 17.25, f.SizeGB, 0.01)
}

func TestCatalog_TrainBasesIncludeFlux1(t *testing.T) {
	c := loadTestCatalog(t)
	var bases []string
	for _, b := range c.Train {
		bases = append(bases, b.Base)
	}
	for _, want := range []string{"Z-Image", "Moss-Image", "SDXL", "Flux.2", "Flux.1"} {
		assert.Contains(t, bases, want)
	}
}
