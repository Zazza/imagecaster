package settings

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadPlanLocalVariants_VariantOnServerSkipsMain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		model string
		file  string
	}{
		{"qwen Q4_K", "qwen_image_2.1-Q4_K", "qwen_image_2.1-Q4_K.gguf"},
		{"qwen Q4_K with extension", "qwen_image_2.1-Q4_K.gguf", "qwen_image_2.1-Q4_K.gguf"},
		{"chroma Q4_K_S", "Chroma1-HD-Q4_K_S", "Chroma1-HD-Q4_K_S.gguf"},
		{"krea Q6_K", "Krea-2-Turbo-Q6_K", "Krea-2-Turbo-Q6_K.gguf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := emptyVariantServer()
			srv["sd"] = []serverclient.ModelInfo{{Name: tt.file, Size: 123, Extension: ".gguf"}}
			svc, _ := variantService(t, 16376, srv, tt.model)
			plan := loadPlan(t, svc)
			assert.Empty(t, sdItems(plan), itemKeys(plan))
		})
	}
}

func TestDownloadPlanLocalVariants_VariantOnServerDoesNotCountAsMain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		model string
		file  string
	}{
		{"qwen main, Q4_K on server", "qwen_image_2.1-Q8_0", "qwen_image_2.1-Q4_K.gguf"},
		{"chroma main, Q4_K_S on server", "Chroma1-HD", "Chroma1-HD-Q4_K_S.gguf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := catalogModel(t, tt.model)
			srv := emptyVariantServer()
			srv["sd"] = []serverclient.ModelInfo{{Name: tt.file, Size: 123, Extension: ".gguf"}}
			svc, _ := variantService(t, 16376, srv, m.MainFilename())
			plan := loadPlan(t, svc)
			sd := sdItems(plan)
			require.Len(t, sd, 1, itemKeys(plan))
			assert.NotEqual(t, tt.file, sd[0].Filename)
		})
	}
}

func TestInstallPresetDepsLocalVariants_VariantOnServerNotDownloaded(t *testing.T) {
	t.Parallel()
	srv := emptyVariantServer()
	srv["sd"] = []serverclient.ModelInfo{{Name: "qwen_image_2.1-Q4_K.gguf", Size: 123, Extension: ".gguf"}}
	svc, rec := variantService(t, 16376, srv)
	require.NoError(t, svc.InstallPresetDeps(dbPreset(t, svc, "qwen_image_2.1-Q4_K", "[]")))
	for _, d := range rec.recorded() {
		assert.NotEqual(t, "sd", d.Type, d)
	}
}
