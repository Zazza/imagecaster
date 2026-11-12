package preset

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
)

type PresetSource struct {
	File string
	Raw  []byte
}

func BundledSources(fsys fs.FS) ([]PresetSource, error) {
	entries, err := fs.ReadDir(fsys, "data/presets")
	if err != nil {
		return nil, fmt.Errorf("read bundled presets: %w", err)
	}
	var out []PresetSource
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join("data/presets", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read bundled presets %s: %w", entry.Name(), err)
		}
		out = append(out, PresetSource{File: entry.Name(), Raw: raw})
	}
	return out, nil
}

func parseSource(src PresetSource) ([]bundledPreset, error) {
	var file bundledPresetsFile
	if err := json.Unmarshal(src.Raw, &file); err != nil {
		return nil, fmt.Errorf("parse presets %s: %w", src.File, err)
	}
	if file.Version != 2 {
		return nil, fmt.Errorf("presets %s: unsupported version %d", src.File, file.Version)
	}
	return file.Presets, nil
}

func SourceDependencies(sources []PresetSource) ([]string, []string, error) {
	modelCount := map[string]int{}
	loraCount := map[string]int{}
	for _, src := range sources {
		presets, err := parseSource(src)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range presets {
			if p.ModelName != "" {
				modelCount[p.ModelName]++
			}
			for _, l := range parseLoRAs(p.Loras) {
				if l.Name != "" {
					loraCount[l.Name]++
				}
			}
		}
	}
	return sortedByCount(modelCount), sortedByCount(loraCount), nil
}

func parseLoRAs(raw string) []LoRAEntry {
	if raw == "" || raw == "[]" {
		return nil
	}
	var loras []LoRAEntry
	if json.Unmarshal([]byte(raw), &loras) != nil {
		return nil
	}
	return loras
}

const autoInstalledPresetsKey = "auto_installed_presets"

var presetSyncMu sync.Mutex

func modelSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[trimModelExt(n)] = true
	}
	return set
}

func loraSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[strings.TrimSuffix(n, ".safetensors")] = true
	}
	return set
}

func (d *DB) presetNames() (map[string]bool, error) {
	rows, err := d.db.Query(`SELECT name FROM presets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names[name] = true
	}
	return names, rows.Err()
}

func (d *DB) loadNameList(key string) (map[string]bool, bool, error) {
	raw, err := d.GetSetting(key)
	if err != nil || raw == "" {
		return map[string]bool{}, false, err
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, true, fmt.Errorf("parse setting %s: %w", key, err)
	}
	set := make(map[string]bool, len(list))
	for _, n := range list {
		set[n] = true
	}
	return set, true, nil
}

func (d *DB) saveNameList(key string, set map[string]bool) error {
	list := make([]string, 0, len(set))
	for n := range set {
		list = append(list, n)
	}
	slices.Sort(list)
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return d.SetSetting(key, string(raw))
}

func (d *DB) SyncModelPresets(sources []PresetSource, sdModels, loraModels []string) (int, error) {
	if sdModels == nil {
		return 0, nil
	}
	presetSyncMu.Lock()
	defer presetSyncMu.Unlock()

	present := modelSet(sdModels)
	remembered, _, err := d.loadNameList(autoInstalledPresetsKey)
	if err != nil {
		return 0, err
	}
	existing, err := d.presetNames()
	if err != nil {
		return 0, err
	}
	var pending []bundledPreset
	changed := false
	for _, src := range sources {
		presets, err := parseSource(src)
		if err != nil {
			return 0, err
		}
		for _, p := range presets {
			if p.Name == "" || remembered[p.Name] {
				continue
			}
			if p.ModelName != "" && !present[trimModelExt(p.ModelName)] {
				continue
			}
			remembered[p.Name] = true
			changed = true
			if existing[p.Name] {
				continue
			}
			existing[p.Name] = true
			pending = append(pending, p)
		}
	}
	if len(pending) > 0 {
		if err := d.insertBundledPresets(pending); err != nil {
			return 0, err
		}
	}
	if changed {
		if err := d.saveNameList(autoInstalledPresetsKey, remembered); err != nil {
			return len(pending), err
		}
	}
	return len(pending), nil
}

func (d *DB) MigratePresetInstallLog(sources []PresetSource) error {
	presetSyncMu.Lock()
	defer presetSyncMu.Unlock()

	if _, exists, err := d.loadNameList(autoInstalledPresetsKey); err != nil || exists {
		return err
	}
	seededFiles, _, err := d.loadNameList(bundledSeededFilesKey)
	if err != nil {
		return err
	}
	existing, err := d.presetNames()
	if err != nil {
		return err
	}
	remembered := map[string]bool{}
	for _, src := range sources {
		presets, err := parseSource(src)
		if err != nil {
			return err
		}
		installed := seededFiles[src.File]
		for _, p := range presets {
			installed = installed || existing[p.Name]
		}
		if !installed {
			continue
		}
		for _, p := range presets {
			if p.Name != "" {
				remembered[p.Name] = true
			}
		}
	}
	return d.saveNameList(autoInstalledPresetsKey, remembered)
}

func (d *DB) PresetAvailability(sdModels, loraModels []string) ([]PresetInstallStatus, error) {
	sdKnown, loraKnown := sdModels != nil, loraModels != nil
	sdPresent, loraPresent := modelSet(sdModels), loraSet(loraModels)

	rows, err := d.db.Query(`SELECT id, name, model_name, loras FROM presets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []PresetInstallStatus{}
	for rows.Next() {
		var id int64
		var name, modelName, lorasJSON string
		if err := rows.Scan(&id, &name, &modelName, &lorasJSON); err != nil {
			return nil, err
		}
		status := PresetInstallStatus{ID: id, Name: name}
		if sdKnown && modelName != "" && !sdPresent[trimModelExt(modelName)] {
			status.MissingSD = append(status.MissingSD, modelName)
		}
		if loraKnown {
			for _, l := range parseLoRAs(lorasJSON) {
				if l.Name != "" && !loraPresent[strings.TrimSuffix(l.Name, ".safetensors")] {
					status.MissingLoRA = append(status.MissingLoRA, l.Name)
				}
			}
		}
		status.Installed = len(status.MissingSD) == 0 && len(status.MissingLoRA) == 0
		result = append(result, status)
	}
	return result, rows.Err()
}

func ModelPresetNames(sources []PresetSource) (map[string][]string, error) {
	out := map[string][]string{}
	for _, src := range sources {
		presets, err := parseSource(src)
		if err != nil {
			return nil, err
		}
		for _, p := range presets {
			if p.ModelName == "" || p.Name == "" {
				continue
			}
			out[p.ModelName] = append(out[p.ModelName], p.Name)
		}
	}
	return out, nil
}
