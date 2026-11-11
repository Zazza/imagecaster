package settings

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type planPreset struct {
	model   string
	loras   string
	bundled bool
}

func planService(t *testing.T, onServer map[string][]string, presets ...planPreset) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typ := strings.TrimPrefix(r.URL.Path, "/api/server/models/")
		names, ok := onServer[typ]
		if r.Method != http.MethodGet || !ok {
			http.NotFound(w, r)
			return
		}
		list := make([]serverclient.ModelInfo, 0, len(names))
		for _, n := range names {
			ext := ".safetensors"
			if strings.HasSuffix(n, ".gguf") {
				ext = ".gguf"
			}
			list = append(list, serverclient.ModelInfo{Name: n, Size: 1, Extension: ext})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	}))
	t.Cleanup(srv.Close)

	_, db := testService(t, nil, nil)
	_, err := db.DB().Exec(`DELETE FROM presets`)
	require.NoError(t, err)
	for i, p := range presets {
		bundled := 0
		if p.bundled {
			bundled = 1
		}
		loras := p.loras
		if loras == "" {
			loras = "[]"
		}
		_, err := db.DB().Exec(
			`INSERT INTO presets (name, preset_type, model_name, loras, is_bundled) VALUES (?, 'post', ?, ?, ?)`,
			"plan-"+string(rune('a'+i)), p.model, loras, bundled)
		require.NoError(t, err)
	}

	client := serverclient.NewClient()
	client.SetBaseURL(srv.URL)
	svc, _ := testServiceWithDB(t, db, client)
	return svc
}

func testServiceWithDB(t *testing.T, db *preset.DB, client *serverclient.Client) (*Service, *preset.DB) {
	t.Helper()
	base, _ := testService(t, nil, nil)
	return New(db, base.llm, base.sd, base.cfg, base.log, client), db
}

func emptyServer() map[string][]string {
	return map[string][]string{"sd": {}, "vae": {}, "text_encoder": {}, "lora": {}}
}

func bundled(model string) planPreset { return planPreset{model: model, bundled: true} }

func itemKeys(p *DownloadPlan) []string {
	keys := make([]string, 0, len(p.Items))
	for _, it := range p.Items {
		keys = append(keys, it.Type+"/"+it.Filename)
	}
	return keys
}

func findItem(p *DownloadPlan, typ, filename string) *DownloadItem {
	for i := range p.Items {
		if p.Items[i].Type == typ && p.Items[i].Filename == filename {
			return &p.Items[i]
		}
	}
	return nil
}

func indexOf(keys []string, k string) int {
	for i, v := range keys {
		if v == k {
			return i
		}
	}
	return -1
}

func loadPlan(t *testing.T, svc *Service) *DownloadPlan {
	t.Helper()
	plan, err := svc.GetDefaultDownloadPlan()
	require.NoError(t, err)
	require.NotNil(t, plan)
	return plan
}

func catalogModel(t *testing.T, name string) serverclient.CatalogSDModel {
	t.Helper()
	cat, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	for _, m := range cat.SDModels {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("catalog model %s not found", name)
	return serverclient.CatalogSDModel{}
}

func catalogLoRA(t *testing.T, name string) serverclient.CatalogLoRA {
	t.Helper()
	cat, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	for _, l := range cat.LoRA {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("catalog lora %s not found", name)
	return serverclient.CatalogLoRA{}
}

func TestDownloadPlan_CatalogFacts(t *testing.T) {
	t.Parallel()
	moss := catalogModel(t, "moss_image_sft_bf16")
	assert.NotEmpty(t, moss.URL)
	klein := catalogModel(t, "flux-2-klein-4b")
	assert.NotEmpty(t, klein.URL)
	flux1 := catalogModel(t, "flux1-dev-fp8")
	assert.NotEmpty(t, flux1.URL)
	assert.Empty(t, flux1.Modules)
	mossFiles := []string{}
	for _, m := range moss.Modules {
		mossFiles = append(mossFiles, m.Type+"/"+m.Filename)
	}
	assert.ElementsMatch(t, []string{
		"text_encoder/qwen35_9b_int8_convrot.safetensors",
		"vae/flux2-vae_small.safetensors",
		"text_encoder/moss.tokenizer.json",
	}, mossFiles)
	ds := catalogModel(t, "dreamshaper_8")
	assert.NotEmpty(t, ds.URL)
	assert.Empty(t, ds.Modules)
	assert.NotEmpty(t, catalogLoRA(t, "Moss_Edit_LoRA").URL)
}

func TestDownloadPlan_EmptyDB(t *testing.T) {
	t.Parallel()
	svc := planService(t, emptyServer())
	plan := loadPlan(t, svc)
	assert.NotNil(t, plan.Items)
	assert.Empty(t, plan.Items)
	assert.Equal(t, 0.0, plan.TotalGB)

	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"items":[]`)
	assert.NotContains(t, string(raw), `"manual"`)
}

func TestDownloadPlan_ModulesThenMainFile(t *testing.T) {
	t.Parallel()
	svc := planService(t, emptyServer(), bundled("moss_image_sft_bf16.safetensors"))
	plan := loadPlan(t, svc)
	keys := itemKeys(plan)

	modules := []string{
		"text_encoder/qwen35_9b_int8_convrot.safetensors",
		"vae/flux2-vae_small.safetensors",
		"text_encoder/moss.tokenizer.json",
	}
	main := indexOf(keys, "sd/"+catalogModel(t, "moss_image_sft_bf16").MainFilename())
	require.GreaterOrEqual(t, main, 0, keys)
	for _, m := range modules {
		i := indexOf(keys, m)
		require.GreaterOrEqual(t, i, 0, keys)
		assert.Less(t, i, main, m)
	}
	assert.ElementsMatch(t, append(append([]string{}, modules...),
		"sd/"+catalogModel(t, "moss_image_sft_bf16").MainFilename(),
		"lora/Moss_Edit_LoRA.safetensors"), keys)
	for _, it := range plan.Items {
		assert.NotEmpty(t, it.URL, it.Filename)
	}
}

func TestDownloadPlan_SingleFileModel(t *testing.T) {
	t.Parallel()
	svc := planService(t, emptyServer(), bundled("dreamshaper_8.safetensors"))
	plan := loadPlan(t, svc)
	assert.Equal(t, []string{"sd/" + catalogModel(t, "dreamshaper_8").MainFilename()}, itemKeys(plan))
}

func TestDownloadPlan_FilesOnServerSkipped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		onServer map[string][]string
		want     []string
	}{
		{
			name: "everything present",
			onServer: map[string][]string{
				"sd":           {"flux-2-klein-4b.safetensors"},
				"text_encoder": {"qwen_3_4b.safetensors"},
				"vae":          {"flux2-klein-vae.safetensors"},
				"lora":         {},
			},
			want: []string{},
		},
		{
			name: "only main file missing",
			onServer: map[string][]string{
				"sd":           {},
				"text_encoder": {"qwen_3_4b.safetensors"},
				"vae":          {"flux2-klein-vae.safetensors"},
				"lora":         {},
			},
			want: []string{"sd/flux-2-klein-4b.safetensors"},
		},
		{
			name: "only vae missing",
			onServer: map[string][]string{
				"sd":           {"flux-2-klein-4b.safetensors"},
				"text_encoder": {"qwen_3_4b.safetensors"},
				"vae":          {"other.safetensors"},
				"lora":         {},
			},
			want: []string{"vae/flux2-klein-vae.safetensors"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := planService(t, tt.onServer, bundled("flux-2-klein-4b.safetensors"))
			plan := loadPlan(t, svc)
			assert.Equal(t, tt.want, itemKeys(plan))
			assert.NotNil(t, plan.Items)
		})
	}
}

func TestDownloadPlan_SharedFileOnce(t *testing.T) {
	t.Parallel()
	svc := planService(t, emptyServer(),
		bundled("flux-2-klein-4b.safetensors"),
		bundled("flux3-image-klein-Q8_0.gguf"),
	)
	plan := loadPlan(t, svc)
	seen := map[string]int{}
	for _, k := range itemKeys(plan) {
		seen[k]++
	}
	assert.Equal(t, 1, seen["vae/flux2-klein-vae.safetensors"])
	for k, n := range seen {
		assert.Equal(t, 1, n, k)
	}
	assert.Equal(t, 1, seen["sd/flux-2-klein-4b.safetensors"])
	assert.Equal(t, 1, seen["sd/flux3-image-klein-Q8_0.gguf"])
}

func TestDownloadPlan_Flux1Dev(t *testing.T) {
	t.Parallel()
	t.Run("missing is one sd item", func(t *testing.T) {
		t.Parallel()
		svc := planService(t, emptyServer(), bundled("flux1-dev-fp8.safetensors"))
		plan := loadPlan(t, svc)
		assert.Equal(t, []string{"sd/flux1-dev-fp8.safetensors"}, itemKeys(plan))
		assert.Equal(t, catalogModel(t, "flux1-dev-fp8").URL, plan.Items[0].URL)
	})
	t.Run("on server not planned", func(t *testing.T) {
		t.Parallel()
		srv := emptyServer()
		srv["sd"] = []string{"flux1-dev-fp8.safetensors"}
		svc := planService(t, srv, bundled("flux1-dev-fp8.safetensors"))
		plan := loadPlan(t, svc)
		assert.Empty(t, plan.Items)
	})
}

func TestDownloadPlan_IgnoredModels(t *testing.T) {
	t.Parallel()
	svc := planService(t, emptyServer(),
		bundled("not_in_catalog_xyz.safetensors"),
		planPreset{model: "dreamshaper_8.safetensors", bundled: false},
	)
	plan := loadPlan(t, svc)
	assert.Empty(t, plan.Items)
	assert.Equal(t, 0.0, plan.TotalGB)
}

func TestDownloadPlan_UnusedCatalogModelsNeverPlanned(t *testing.T) {
	t.Parallel()
	svc := planService(t, emptyServer(),
		bundled("moss_image_sft_bf16.safetensors"),
		bundled("flux-2-klein-4b.safetensors"),
		bundled("dreamshaper_8.safetensors"),
		bundled("flux1-dev-fp8.safetensors"),
	)
	plan := loadPlan(t, svc)
	unused := []string{"Krea-2-Turbo-Q4_K_M", "qwen_image_2.1-Q8_0", "flux3-image-klein-Q8_0"}
	for _, name := range unused {
		m := catalogModel(t, name)
		assert.Nil(t, findItem(plan, "sd", m.MainFilename()), name)
		for _, mod := range m.Modules {
			if mod.Filename == "flux2-klein-vae.safetensors" {
				continue
			}
			assert.Nil(t, findItem(plan, mod.Type, mod.Filename), name+" "+mod.Filename)
		}
	}
}

func TestDownloadPlan_MossEditLoRA(t *testing.T) {
	t.Parallel()
	lora := catalogLoRA(t, "Moss_Edit_LoRA")
	t.Run("added with moss", func(t *testing.T) {
		t.Parallel()
		svc := planService(t, emptyServer(), bundled("moss_image_sft_bf16.safetensors"))
		plan := loadPlan(t, svc)
		it := findItem(plan, "lora", "Moss_Edit_LoRA.safetensors")
		require.NotNil(t, it, itemKeys(plan))
		assert.Equal(t, lora.URL, it.URL)
		assert.InDelta(t, lora.SizeMB/1024, it.SizeGB, 1e-9)
	})
	t.Run("not added when on server", func(t *testing.T) {
		t.Parallel()
		srv := emptyServer()
		srv["lora"] = []string{"Moss_Edit_LoRA.safetensors"}
		svc := planService(t, srv, bundled("moss_image_sft_bf16.safetensors"))
		plan := loadPlan(t, svc)
		assert.Nil(t, findItem(plan, "lora", "Moss_Edit_LoRA.safetensors"))
		assert.NotEmpty(t, plan.Items)
	})
	t.Run("added even when moss files are on server", func(t *testing.T) {
		t.Parallel()
		moss := catalogModel(t, "moss_image_sft_bf16")
		srv := emptyServer()
		srv["sd"] = []string{moss.MainFilename()}
		for _, m := range moss.Modules {
			srv[m.Type] = append(srv[m.Type], m.Filename)
		}
		svc := planService(t, srv, bundled("moss_image_sft_bf16.safetensors"))
		plan := loadPlan(t, svc)
		assert.Equal(t, []string{"lora/Moss_Edit_LoRA.safetensors"}, itemKeys(plan))
	})
	t.Run("not added without moss", func(t *testing.T) {
		t.Parallel()
		svc := planService(t, emptyServer(), bundled("dreamshaper_8.safetensors"))
		plan := loadPlan(t, svc)
		assert.Nil(t, findItem(plan, "lora", "Moss_Edit_LoRA.safetensors"))
	})
	t.Run("once even if presets also use it", func(t *testing.T) {
		t.Parallel()
		svc := planService(t, emptyServer(), planPreset{
			model:   "moss_image_sft_bf16.safetensors",
			loras:   `[{"name":"Moss_Edit_LoRA","weight":1}]`,
			bundled: true,
		})
		plan := loadPlan(t, svc)
		n := 0
		for _, k := range itemKeys(plan) {
			if k == "lora/Moss_Edit_LoRA.safetensors" {
				n++
			}
		}
		assert.Equal(t, 1, n)
	})
}

func TestDownloadPlan_PresetLoRAs(t *testing.T) {
	t.Parallel()
	withURL := catalogLoRA(t, "hairdetailer")
	require.NotEmpty(t, withURL.URL)
	hdr := catalogLoRA(t, "HDR_Photography")
	require.NotEmpty(t, hdr.URL)

	t.Run("catalog lora with url is an item", func(t *testing.T) {
		t.Parallel()
		svc := planService(t, emptyServer(), planPreset{
			model:   "not_in_catalog_xyz.safetensors",
			loras:   `[{"name":"hairdetailer","weight":0.8}]`,
			bundled: true,
		})
		plan := loadPlan(t, svc)
		it := findItem(plan, "lora", "hairdetailer.safetensors")
		require.NotNil(t, it, itemKeys(plan))
		assert.Equal(t, withURL.URL, it.URL)
		assert.InDelta(t, withURL.SizeMB/1024, it.SizeGB, 1e-9)
	})
	t.Run("catalog lora on server skipped", func(t *testing.T) {
		t.Parallel()
		srv := emptyServer()
		srv["lora"] = []string{"hairdetailer.safetensors"}
		svc := planService(t, srv, planPreset{
			model:   "not_in_catalog_xyz.safetensors",
			loras:   `[{"name":"hairdetailer","weight":0.8}]`,
			bundled: true,
		})
		plan := loadPlan(t, svc)
		assert.Empty(t, plan.Items)
	})
	t.Run("HDR_Photography is planned like other loras", func(t *testing.T) {
		t.Parallel()
		svc := planService(t, emptyServer(), planPreset{
			model:   "not_in_catalog_xyz.safetensors",
			loras:   `[{"name":"HDR_Photography","weight":1}]`,
			bundled: true,
		})
		plan := loadPlan(t, svc)
		it := findItem(plan, "lora", "HDR_Photography.safetensors")
		require.NotNil(t, it, itemKeys(plan))
		assert.Equal(t, hdr.URL, it.URL)
		assert.InDelta(t, hdr.SizeMB/1024, it.SizeGB, 1e-9)
	})
	t.Run("HDR_Photography on server skipped", func(t *testing.T) {
		t.Parallel()
		srv := emptyServer()
		srv["lora"] = []string{"HDR_Photography.safetensors"}
		svc := planService(t, srv, planPreset{
			model:   "not_in_catalog_xyz.safetensors",
			loras:   `[{"name":"HDR_Photography","weight":1}]`,
			bundled: true,
		})
		plan := loadPlan(t, svc)
		assert.Empty(t, plan.Items)
	})
	t.Run("user preset loras ignored", func(t *testing.T) {
		t.Parallel()
		svc := planService(t, emptyServer(), planPreset{
			model:   "not_in_catalog_xyz.safetensors",
			loras:   `[{"name":"hairdetailer","weight":0.8},{"name":"HDR_Photography","weight":1}]`,
			bundled: false,
		})
		plan := loadPlan(t, svc)
		assert.Empty(t, plan.Items)
	})
}

func TestDownloadPlan_TotalGB(t *testing.T) {
	t.Parallel()
	svc := planService(t, emptyServer(),
		bundled("moss_image_sft_bf16.safetensors"),
		bundled("flux-2-klein-4b.safetensors"),
		planPreset{model: "dreamshaper_8.safetensors", loras: `[{"name":"hairdetailer","weight":1}]`, bundled: true},
	)
	plan := loadPlan(t, svc)
	require.NotEmpty(t, plan.Items)
	sum := 0.0
	for _, it := range plan.Items {
		sum += it.SizeGB
	}
	want := math.Round(sum*100) / 100
	assert.InDelta(t, want, plan.TotalGB, 1e-9)
	assert.InDelta(t, plan.TotalGB, math.Round(plan.TotalGB*100)/100, 1e-9)
}

func TestDownloadPlan_ServerListFailsForType(t *testing.T) {
	t.Parallel()
	srv := map[string][]string{"sd": {}, "vae": {}, "lora": {}}
	svc := planService(t, srv, bundled("flux-2-klein-4b.safetensors"))
	plan, err := svc.GetDefaultDownloadPlan()
	require.NoError(t, err)
	require.NotNil(t, plan)
	for _, it := range plan.Items {
		assert.NotEqual(t, "text_encoder", it.Type, it.Filename)
	}
	assert.Nil(t, findItem(plan, "text_encoder", "qwen_3_4b.safetensors"))
	assert.NotNil(t, findItem(plan, "vae", "flux2-klein-vae.safetensors"), itemKeys(plan))
	assert.NotNil(t, findItem(plan, "sd", "flux-2-klein-4b.safetensors"), itemKeys(plan))
}
