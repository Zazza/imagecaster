package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var variantPlanModels = []string{
	"z-image-turbo-fp8-aio",
	"moss_image_sft_bf16",
	"flux-2-klein-4b",
	"flux3-image-klein-Q8_0",
	"Krea-2-Turbo-Q4_K_M",
	"qwen_image_2.1-Q8_0",
	"flux1-dev-fp8",
	"hidream_o1_image_dev_fp8_scaled",
	"Chroma1-HD",
}

type variantDownload struct {
	Type     string
	Filename string
	URL      string
}

type variantServer struct {
	mu        sync.Mutex
	downloads []variantDownload
}

func (v *variantServer) recorded() []variantDownload {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]variantDownload(nil), v.downloads...)
}

func variantService(t *testing.T, vramMB int, onServer map[string][]serverclient.ModelInfo, models ...string) (*Service, *variantServer) {
	t.Helper()
	rec := &variantServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server/status" {
			if vramMB < 0 {
				http.Error(w, "down", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"gpu": map[string]any{"memory_total_mb": vramMB, "available": true}})
			return
		}
		typ := strings.TrimPrefix(r.URL.Path, "/api/server/models/")
		if typ == r.URL.Path {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			list, ok := onServer[typ]
			if !ok {
				http.NotFound(w, r)
				return
			}
			if list == nil {
				list = []serverclient.ModelInfo{}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(list)
		case http.MethodPost:
			var body struct {
				URL      string `json:"url"`
				Filename string `json:"filename"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.mu.Lock()
			rec.downloads = append(rec.downloads, variantDownload{Type: typ, Filename: body.Filename, URL: body.URL})
			rec.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	_, db := testService(t, nil, nil)
	items := []map[string]any{}
	for i, m := range models {
		items = append(items, map[string]any{"name": "variant-" + string(rune('a'+i)), "preset_type": "post", "model_name": m, "loras": "[]"})
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "presets": items})
	require.NoError(t, err)

	client := serverclient.NewClient()
	client.SetBaseURL(srv.URL)
	svc, _ := testServiceWithDB(t, db, client)
	svc.SetBundledPresets(fstest.MapFS{"data/presets/variants.json": {Data: raw}})
	return svc, rec
}

func emptyVariantServer() map[string][]serverclient.ModelInfo {
	return map[string][]serverclient.ModelInfo{"sd": {}, "vae": {}, "text_encoder": {}, "lora": {}}
}

func expectedVariant(t *testing.T, m serverclient.CatalogSDModel, vramMB int) serverclient.CatalogVariant {
	t.Helper()
	require.NotEmpty(t, m.Variants, m.Name)
	var best, smallest, def *serverclient.CatalogVariant
	for i := range m.Variants {
		v := &m.Variants[i]
		if v.Default {
			def = v
		}
		if smallest == nil || v.SizeGB < smallest.SizeGB {
			smallest = v
		}
		if vramMB > 0 && v.VRAMGB*1024 <= float64(vramMB) && (best == nil || v.SizeGB > best.SizeGB) {
			best = v
		}
	}
	switch {
	case vramMB <= 0:
		require.NotNil(t, def, m.Name)
		return *def
	case best != nil:
		return *best
	default:
		return *smallest
	}
}

func sdItems(p *DownloadPlan) []DownloadItem {
	out := []DownloadItem{}
	for _, it := range p.Items {
		if it.Type == "sd" {
			out = append(out, it)
		}
	}
	return out
}

func TestDownloadPlanVariants_PicksVariantByServerVRAM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		vramMB   int
		statusMB int
	}{
		{"16 GB card", 16376, 16376},
		{"8 GB card", 8192, 8192},
		{"tiny card picks smallest", 2000, 2000},
		{"24 GB card", 24564, 24564},
		{"status fails picks default", 0, -1},
	}
	for _, tt := range tests {
		for _, name := range variantPlanModels {
			t.Run(tt.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				m := catalogModel(t, name)
				want := expectedVariant(t, m, tt.vramMB)
				svc, _ := variantService(t, tt.statusMB, emptyVariantServer(), m.MainFilename())
				plan := loadPlan(t, svc)
				sd := sdItems(plan)
				require.Len(t, sd, 1, itemKeys(plan))
				assert.Equal(t, want.SaveAs, sd[0].Filename)
				assert.Equal(t, want.URL, sd[0].URL)
				assert.InDelta(t, want.SizeGB, sd[0].SizeGB, 1e-9)
				assert.Equal(t, strings.ToLower(strings.TrimSuffix(m.MainFilename(), filepath.Ext(m.MainFilename()))),
					strings.ToLower(strings.TrimSuffix(sd[0].Filename, filepath.Ext(sd[0].Filename))))
			})
		}
	}
}

func TestDownloadPlanVariants_ModulesInPlan(t *testing.T) {
	t.Parallel()
	for _, name := range variantPlanModels {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := catalogModel(t, name)
			svc, _ := variantService(t, 16376, emptyVariantServer(), m.MainFilename())
			plan := loadPlan(t, svc)
			keys := itemKeys(plan)
			sd := sdItems(plan)
			require.Len(t, sd, 1, keys)
			main := indexOf(keys, "sd/"+sd[0].Filename)
			for _, mod := range m.Modules {
				i := indexOf(keys, mod.Type+"/"+mod.Filename)
				require.GreaterOrEqual(t, i, 0, "%s missing in %v", mod.Filename, keys)
				assert.Less(t, i, main, mod.Filename)
			}
		})
	}
}

func TestDownloadPlanVariants_ModelPresentUnderAnyExtension(t *testing.T) {
	t.Parallel()
	for _, name := range variantPlanModels {
		for _, ext := range []string{".safetensors", ".gguf"} {
			t.Run(name+ext, func(t *testing.T) {
				t.Parallel()
				m := catalogModel(t, name)
				stem := strings.TrimSuffix(m.MainFilename(), filepath.Ext(m.MainFilename()))
				srv := emptyVariantServer()
				srv["sd"] = []serverclient.ModelInfo{{Name: stem + ext, Size: 123, Extension: ext}}
				svc, _ := variantService(t, 16376, srv, m.MainFilename())
				plan := loadPlan(t, svc)
				assert.Empty(t, sdItems(plan), itemKeys(plan))
			})
		}
	}
}

func TestDownloadPlanVariants_OtherModelOnServerDoesNotCount(t *testing.T) {
	t.Parallel()
	m := catalogModel(t, "Chroma1-HD")
	srv := emptyVariantServer()
	srv["sd"] = []serverclient.ModelInfo{{Name: "Chroma1-HD-old.gguf", Size: 123, Extension: ".gguf"}}
	svc, _ := variantService(t, 16376, srv, m.MainFilename())
	plan := loadPlan(t, svc)
	assert.Len(t, sdItems(plan), 1, itemKeys(plan))
}

func TestDownloadCatalogSDModelVariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		vramMB   int
		statusMB int
	}{
		{"16 GB card", 16376, 16376},
		{"tiny card", 2000, 2000},
		{"status fails", 0, -1},
	}
	for _, tt := range tests {
		for _, name := range variantPlanModels {
			t.Run(tt.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				m := catalogModel(t, name)
				want := expectedVariant(t, m, tt.vramMB)
				svc, rec := variantService(t, tt.statusMB, emptyVariantServer())
				require.NoError(t, svc.DownloadCatalogSDModel(m.Name))
				got := rec.recorded()
				require.NotEmpty(t, got)
				last := got[len(got)-1]
				assert.Equal(t, variantDownload{Type: "sd", Filename: want.SaveAs, URL: want.URL}, last)
				sdCount := 0
				for _, d := range got {
					if d.Type == "sd" {
						sdCount++
					}
				}
				assert.Equal(t, 1, sdCount, got)
				for _, mod := range m.Modules {
					assert.Contains(t, got, variantDownload{Type: mod.Type, Filename: mod.Filename, URL: mod.URL})
				}
			})
		}
	}
}

func TestDownloadCatalogSDModelVariants_PresentModelNotDownloaded(t *testing.T) {
	t.Parallel()
	m := catalogModel(t, "qwen_image_2.1-Q8_0")
	stem := strings.TrimSuffix(m.MainFilename(), filepath.Ext(m.MainFilename()))
	srv := emptyVariantServer()
	srv["sd"] = []serverclient.ModelInfo{{Name: stem + ".safetensors", Size: 5, Extension: ".safetensors"}}
	svc, rec := variantService(t, 16376, srv)
	require.NoError(t, svc.DownloadCatalogSDModel(m.Name))
	for _, d := range rec.recorded() {
		assert.NotEqual(t, "sd", d.Type, d)
	}
}
