package serverclient

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"strings"
)

//go:embed catalog.json
var catalogData []byte

//go:embed catalog_presets/*.json
var catalogPresetFS embed.FS

type CatalogModule struct {
	Type     string  `json:"type"`
	Filename string  `json:"filename"`
	URL      string  `json:"url"`
	SizeGB   float64 `json:"size_gb"`
}

type CatalogModel struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	DescriptionRu string  `json:"description_ru,omitempty"`
	SizeGB        float64 `json:"size_gb"`
	Recommended   bool    `json:"recommended,omitempty"`
}

type CatalogSDModel struct {
	Name          string           `json:"name"`
	Base          string           `json:"base"`
	Category      string           `json:"category"`
	CategoryRu    string           `json:"category_ru,omitempty"`
	Description   string           `json:"description"`
	DescriptionRu string           `json:"description_ru,omitempty"`
	URL           string           `json:"url,omitempty"`
	SizeGB        float64          `json:"size_gb"`
	Recommended   bool             `json:"recommended,omitempty"`
	Filename      string           `json:"filename,omitempty"`
	Modules       []CatalogModule  `json:"modules,omitempty"`
	Presets       string           `json:"presets,omitempty"`
	Variants      []CatalogVariant `json:"variants,omitempty"`
}

type CatalogVariant struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Filename string  `json:"filename"`
	URL      string  `json:"url"`
	SizeGB   float64 `json:"size_gb"`
	Default  bool    `json:"default,omitempty"`
	SaveAs   string  `json:"save_as"`
	VRAMGB   float64 `json:"vram_gb"`
}

type CatalogLoRA struct {
	Name          string  `json:"name"`
	Base          string  `json:"base"`
	Category      string  `json:"category"`
	DefaultWeight float64 `json:"default_weight"`
	CategoryRu    string  `json:"category_ru,omitempty"`
	Description   string  `json:"description"`
	DescriptionRu string  `json:"description_ru,omitempty"`
	URL           string  `json:"url,omitempty"`
	SizeMB        float64 `json:"size_mb"`
}

type Catalog struct {
	LLMGenerate []CatalogModel   `json:"llm_generate"`
	LLMVision   []CatalogModel   `json:"llm_vision"`
	SDModels    []CatalogSDModel `json:"sd_models"`
	LoRA        []CatalogLoRA    `json:"lora"`
}

func LoadCatalog() (*Catalog, error) {
	var catalog Catalog
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	for i := range catalog.SDModels {
		catalog.SDModels[i].fillVariants()
	}
	return &catalog, nil
}

func (m CatalogSDModel) MainFilename() string {
	if m.Filename != "" {
		return m.Filename
	}
	return m.Name + ".safetensors"
}

func modelStem(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if i := strings.Index(base, " ["); i >= 0 {
		base = base[:i]
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".safetensors", ".ckpt", ".gguf":
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.ToLower(base)
}

func (c *Catalog) FindSDModel(name string) *CatalogSDModel {
	stem := modelStem(name)
	if stem == "" {
		return nil
	}
	for i := range c.SDModels {
		m := &c.SDModels[i]
		if modelStem(m.MainFilename()) == stem || strings.ToLower(m.Name) == stem {
			return m
		}
	}
	return nil
}

func LoadCatalogPresets(file string) ([]byte, error) {
	if file == "" || strings.ContainsAny(file, `/\`) || !strings.HasSuffix(file, ".json") {
		return nil, fmt.Errorf("invalid catalog presets file %q", file)
	}
	return catalogPresetFS.ReadFile(path.Join("catalog_presets", file))
}

func (m *CatalogSDModel) fillVariants() {
	stem := strings.TrimSuffix(m.MainFilename(), filepath.Ext(m.MainFilename()))
	for i := range m.Variants {
		v := &m.Variants[i]
		v.SaveAs = stem + strings.ToLower(filepath.Ext(v.Filename))
		v.VRAMGB = m.estimateVRAM(v.SizeGB)
	}
}

func (m CatalogSDModel) estimateVRAM(sizeGB float64) float64 {
	peak, vae := sizeGB, 0.0
	for _, mod := range m.Modules {
		name := strings.ToLower(mod.Filename)
		switch {
		case strings.Contains(name, "mmproj") || strings.HasSuffix(name, ".json"):
		case mod.Type == "vae":
			vae += mod.SizeGB
		default:
			peak = math.Max(peak, mod.SizeGB)
		}
	}
	return math.Round((peak+vae+1.5)*10) / 10
}

func (m CatalogSDModel) DefaultVariant() *CatalogVariant {
	for i := range m.Variants {
		if m.Variants[i].Default {
			return &m.Variants[i]
		}
	}
	if len(m.Variants) > 0 {
		return &m.Variants[0]
	}
	return nil
}

func (m CatalogSDModel) FitVariant(vramMB int) *CatalogVariant {
	if vramMB <= 0 || len(m.Variants) == 0 {
		return m.DefaultVariant()
	}
	var best, smallest *CatalogVariant
	for i := range m.Variants {
		v := &m.Variants[i]
		if smallest == nil || v.SizeGB < smallest.SizeGB {
			smallest = v
		}
		if v.VRAMGB*1024 <= float64(vramMB) && (best == nil || v.SizeGB > best.SizeGB) {
			best = v
		}
	}
	if best != nil {
		return best
	}
	return smallest
}

func (m CatalogSDModel) InstalledFile(files []ModelInfo) (ModelInfo, bool) {
	stem := modelStem(m.MainFilename())
	for _, f := range files {
		if modelStem(f.Name) == stem {
			return f, true
		}
	}
	return ModelInfo{}, false
}

func (m CatalogSDModel) InstalledVariant(files []ModelInfo) (*CatalogVariant, bool) {
	f, ok := m.InstalledFile(files)
	if !ok {
		return nil, false
	}
	for i := range m.Variants {
		want := m.Variants[i].SizeGB * 1e9
		if want > 0 && math.Abs(float64(f.Size)-want) <= want*0.03 {
			return &m.Variants[i], true
		}
	}
	return nil, true
}
