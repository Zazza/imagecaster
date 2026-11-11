package serverclient

import (
	"embed"
	"encoding/json"
	"fmt"
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
	Name          string          `json:"name"`
	Base          string          `json:"base"`
	Category      string          `json:"category"`
	CategoryRu    string          `json:"category_ru,omitempty"`
	Description   string          `json:"description"`
	DescriptionRu string          `json:"description_ru,omitempty"`
	URL           string          `json:"url,omitempty"`
	SizeGB        float64         `json:"size_gb"`
	Recommended   bool            `json:"recommended,omitempty"`
	Filename      string          `json:"filename,omitempty"`
	Modules       []CatalogModule `json:"modules,omitempty"`
	Presets       string          `json:"presets,omitempty"`
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
