package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

type PostEditorParams struct {
	Model string             `json:"model"`
	Loras []preset.LoRAEntry `json:"loras"`
}

type postEditorProfile struct {
	editModelProfile
	loras []preset.LoRAEntry
}

var postEditorLoraTag = regexp.MustCompile(`(?i)<lora:[^>]*>`)

var errPostEditorModelRequired = errors.New("editor model is required")

func NormalizePostEditor(editor *PostEditorParams) (*PostEditorParams, error) {
	if editor == nil {
		return nil, errPostEditorModelRequired
	}
	prof, err := resolvePostEditor(editor, kleinEditSteps)
	if err != nil {
		return nil, err
	}
	return &PostEditorParams{Model: prof.model, Loras: prof.loras}, nil
}

func resolvePostEditor(editor *PostEditorParams, legacySteps int) (postEditorProfile, error) {
	prof := postEditorProfile{editModelProfile: editModelProfile{
		model: kleinLookModel, family: "klein", steps: legacySteps, cfg: kleinLookCfgScale,
	}}
	if editor == nil {
		return prof, nil
	}
	if strings.TrimSpace(editor.Model) == "" {
		return postEditorProfile{}, errPostEditorModelRequired
	}
	model, err := resolveEditModel(editor.Model)
	if err != nil {
		return postEditorProfile{}, err
	}
	loras, err := sanitizeEditLoras(editor.Loras)
	if err != nil {
		return postEditorProfile{}, err
	}
	return postEditorProfile{editModelProfile: model, loras: loras}, nil
}

func (p postEditorProfile) prompt(prompt string) string {
	for _, lora := range p.loras {
		prompt += fmt.Sprintf(" <lora:%s:%g>", lora.Name, lora.Weight)
	}
	return prompt
}

func catalogEditorName(model string, m *serverclient.CatalogSDModel) bool {
	matches := func(file string) bool {
		return strings.EqualFold(model, file) || strings.EqualFold(model, strings.TrimSuffix(file, filepath.Ext(file)))
	}
	if strings.EqualFold(model, m.Name) || matches(m.MainFilename()) {
		return true
	}
	for _, v := range m.Variants {
		if matches(v.Filename) {
			return true
		}
	}
	return false
}

func isPostReferenceModel(model string) bool {
	return config.IsFlux2Klein(model) || config.IsQwenImage(model) || strings.HasPrefix(strings.ToLower(model), "flux3-image-klein-")
}

func applyPostEditor(p *preset.Preset, editor *PostEditorParams) (*preset.Preset, error) {
	if p.PresetType != "post" && !strings.HasPrefix(p.PresetType, "post_") {
		return nil, fmt.Errorf("editor requires a post effect preset")
	}
	prof, err := resolvePostEditor(editor, kleinEditSteps)
	if err != nil {
		return nil, err
	}
	copyPreset := *p
	copyPreset.ModelName = prof.model
	copyPreset.Sampler, copyPreset.ScheduleType = editSampler, editScheduler
	copyPreset.Steps, copyPreset.CfgScale = prof.steps, prof.cfg
	copyPreset.VAE, copyPreset.ClipSkip = "", nil
	copyPreset.Prompt = postEditorLoraTag.ReplaceAllString(p.Prompt, "")
	copyPreset.NegativePrompt = postEditorLoraTag.ReplaceAllString(p.NegativePrompt, "")
	encoded, err := json.Marshal(prof.loras)
	if err != nil {
		return nil, fmt.Errorf("encode editor loras: %w", err)
	}
	copyPreset.Loras = string(encoded)
	return &copyPreset, nil
}
