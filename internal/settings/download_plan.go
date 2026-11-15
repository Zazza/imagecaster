package settings

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

type DownloadItem struct {
	Type     string  `json:"type"`
	Filename string  `json:"filename"`
	URL      string  `json:"url"`
	SizeGB   float64 `json:"size_gb"`
	Model    string  `json:"model"`
}

type DownloadPlan struct {
	Items   []DownloadItem `json:"items"`
	TotalGB float64        `json:"total_gb"`
}

type downloadPlanner struct {
	s        *Service
	catalog  *serverclient.Catalog
	present  map[string]map[string]bool
	listErrs map[string]error
	queued   map[string]bool
	vramMB   *int
	plan     *DownloadPlan
}

func (s *Service) newDownloadPlanner(catalog *serverclient.Catalog) *downloadPlanner {
	return &downloadPlanner{
		s:        s,
		catalog:  catalog,
		present:  map[string]map[string]bool{},
		listErrs: map[string]error{},
		queued:   map[string]bool{},
		plan:     &DownloadPlan{Items: []DownloadItem{}},
	}
}

func (p *downloadPlanner) listed(modelType string) map[string]bool {
	if p.present[modelType] == nil {
		p.present[modelType] = map[string]bool{}
		items, err := p.s.GetServerModels(modelType)
		if err != nil {
			p.listErrs[modelType] = err
			p.s.log.Warn("list server %s models: %s", modelType, err)
		}
		for _, it := range items {
			p.present[modelType][it.Name] = true
		}
	}
	return p.present[modelType]
}

func (p *downloadPlanner) listErr() error {
	for modelType, err := range p.listErrs {
		return fmt.Errorf("list server %s models: %w", modelType, err)
	}
	return nil
}

func (p *downloadPlanner) add(item DownloadItem) {
	key := item.Type + "/" + item.Filename
	if p.queued[key] || p.listed(item.Type)[item.Filename] || p.listErrs[item.Type] != nil {
		return
	}
	p.queued[key] = true
	p.plan.Items = append(p.plan.Items, item)
	p.plan.TotalGB += item.SizeGB
}

func (p *downloadPlanner) serverVRAM() int {
	if p.vramMB == nil {
		v := p.s.serverVRAMMB()
		p.vramMB = &v
	}
	return *p.vramMB
}

func (p *downloadPlanner) addModel(name string) bool {
	m := p.catalog.FindSDModel(name)
	if m == nil || m.URL == "" {
		return false
	}
	for _, mod := range m.Modules {
		p.add(DownloadItem{Type: mod.Type, Filename: mod.Filename, URL: mod.URL, SizeGB: mod.SizeGB, Model: m.Name})
	}
	if _, ok := m.InstalledFile(sdFiles(p.listed("sd"))); ok {
		return true
	}
	filename, url, size := mainDownload(*m, p.serverVRAM())
	p.add(DownloadItem{Type: "sd", Filename: filename, URL: url, SizeGB: size, Model: m.Name})
	return true
}

func (p *downloadPlanner) addLoRA(name string) bool {
	for _, l := range p.catalog.LoRA {
		if l.Name != name || l.URL == "" {
			continue
		}
		p.add(DownloadItem{Type: "lora", Filename: name + ".safetensors", URL: l.URL, SizeGB: l.SizeMB / 1024, Model: name})
		return true
	}
	return false
}

func (s *Service) GetDefaultDownloadPlan() (*DownloadPlan, error) {
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return nil, err
	}
	bundled, err := s.bundledSources()
	if err != nil {
		return nil, err
	}
	models, loras, err := preset.SourceDependencies(bundled)
	if err != nil {
		return nil, err
	}
	p := s.newDownloadPlanner(catalog)
	for _, name := range models {
		p.addModel(name)
	}
	for _, name := range loras {
		p.addLoRA(name)
	}
	p.plan.TotalGB = math.Round(p.plan.TotalGB*100) / 100
	return p.plan, nil
}

func (s *Service) InstallPresetDeps(presetID int64) error {
	pr, err := s.db.Get(presetID)
	if err != nil {
		return fmt.Errorf("preset not found: %w", err)
	}
	var loras []preset.LoRAEntry
	if pr.Loras != "" && pr.Loras != "[]" {
		if err := json.Unmarshal([]byte(pr.Loras), &loras); err != nil {
			return fmt.Errorf("preset %q: parse LoRA list: %w", pr.Name, err)
		}
	}
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	p := s.newDownloadPlanner(catalog)
	if pr.ModelName != "" && !p.addModel(pr.ModelName) {
		s.log.Warn("preset %q: model %q has no download link in the catalog", pr.Name, pr.ModelName)
	}
	for _, l := range loras {
		if !p.addLoRA(l.Name) {
			s.log.Warn("preset %q: LoRA %q has no download link in the catalog", pr.Name, l.Name)
		}
	}
	if err := p.listErr(); err != nil {
		return err
	}
	for _, it := range p.plan.Items {
		if err := s.DownloadServerModel(it.Type, it.URL, it.Filename); err != nil {
			return fmt.Errorf("download %s %s: %w", it.Type, it.Filename, err)
		}
	}
	return nil
}

func sdFiles(names map[string]bool) []serverclient.ModelInfo {
	files := make([]serverclient.ModelInfo, 0, len(names))
	for n := range names {
		files = append(files, serverclient.ModelInfo{Name: n})
	}
	return files
}

func mainDownload(m serverclient.CatalogSDModel, vramMB int) (filename, url string, sizeGB float64) {
	if v := m.FitVariant(vramMB); v != nil {
		return v.SaveAs, v.URL, v.SizeGB
	}
	return m.MainFilename(), m.URL, m.SizeGB
}

func (s *Service) serverVRAMMB() int {
	status, err := s.GetServerStatus()
	if err != nil {
		s.log.Warn("server status for VRAM: %s", err)
		return 0
	}
	return status.GPU.MemoryTotal
}
