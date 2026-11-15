package settings

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dbPreset(t *testing.T, svc *Service, model, loras string) int64 {
	t.Helper()
	p := preset.Preset{Name: "deps-" + t.Name(), PresetType: "post", ModelName: model, Loras: loras}
	require.NoError(t, svc.db.Create(&p))
	return p.ID
}

func TestInstallPresetDeps_DownloadsOnlyMissingLoRAs(t *testing.T) {
	t.Parallel()
	missing := catalogLoRA(t, "add-detail-xl")
	present := catalogLoRA(t, "HDR_Photography")
	srv := emptyVariantServer()
	srv["lora"] = []serverclient.ModelInfo{{Name: present.Name + ".safetensors", Size: 1, Extension: ".safetensors"}}
	svc, rec := variantService(t, 16376, srv)
	id := dbPreset(t, svc, "", `[{"name":"add-detail-xl","weight":0.5},{"name":"HDR_Photography","weight":0.7},{"name":"not-in-catalog","weight":1}]`)

	require.NoError(t, svc.InstallPresetDeps(id))

	assert.Equal(t, []variantDownload{{Type: "lora", Filename: missing.Name + ".safetensors", URL: missing.URL}}, rec.recorded())
}

func TestInstallPresetDeps_LoRAListFailsReturnsErrorWithoutDownloads(t *testing.T) {
	t.Parallel()
	srv := emptyVariantServer()
	delete(srv, "lora")
	svc, rec := variantService(t, 16376, srv)
	id := dbPreset(t, svc, "", `[{"name":"add-detail-xl","weight":0.5}]`)

	err := svc.InstallPresetDeps(id)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "lora")
	assert.Empty(t, rec.recorded())
}

func TestInstallPresetDeps_BrokenLoRAListReturnsError(t *testing.T) {
	t.Parallel()
	svc, rec := variantService(t, 16376, emptyVariantServer())
	id := dbPreset(t, svc, "", `[{"name":`)

	err := svc.InstallPresetDeps(id)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "LoRA")
	assert.Empty(t, rec.recorded())
}

func TestInstallPresetDeps_UnknownPresetReturnsError(t *testing.T) {
	t.Parallel()
	svc, _ := variantService(t, 16376, emptyVariantServer())
	require.Error(t, svc.InstallPresetDeps(999999))
}

func TestInstallPresetDeps_ModelAndLoRAInOneRun(t *testing.T) {
	t.Parallel()
	m := catalogModel(t, "Chroma1-HD")
	l := catalogLoRA(t, "add-detail-xl")
	svc, rec := variantService(t, 16376, emptyVariantServer())
	id := dbPreset(t, svc, m.Name, `[{"name":"add-detail-xl","weight":0.5}]`)

	require.NoError(t, svc.InstallPresetDeps(id))

	got := rec.recorded()
	require.NotEmpty(t, got)
	assert.Equal(t, variantDownload{Type: "lora", Filename: l.Name + ".safetensors", URL: l.URL}, got[len(got)-1])
	sd := 0
	for _, d := range got {
		if d.Type == "sd" {
			sd++
		}
	}
	assert.Equal(t, 1, sd, got)
}
