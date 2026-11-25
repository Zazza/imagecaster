package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/promptutil"
)

type PresetShotParams struct {
	PresetID            int64
	Model               string
	Sampler             string
	Steps               int
	CfgScale            float64
	ScenePrompt         string
	ExtraNegativePrompt string
	Overrides           *PresetOverrides
	PersonLora          string
	Width               int
	Height              int
	Seed                *int64
}

type PresetShotResult struct {
	FaceSettings *FaceRefineParams `json:"-"`
	Image        string
	Info         json.RawMessage
	Seed         int64
	Model        string
}

func (s *Service) presetForShot(params PresetShotParams) (*preset.Preset, error) {
	if params.PresetID > 0 {
		p, err := s.db.Get(params.PresetID)
		if err != nil {
			return nil, fmt.Errorf("пресет не найден: %w", err)
		}
		return p, nil
	}
	if params.Model == "" {
		return nil, errors.New("нет ни пресета, ни модели")
	}
	return &preset.Preset{ModelName: params.Model, Sampler: params.Sampler, Steps: params.Steps, CfgScale: params.CfgScale}, nil
}

func (s *Service) GeneratePresetShot(params PresetShotParams) (*PresetShotResult, error) {
	p, err := s.presetForShot(params)
	if err != nil {
		return nil, err
	}
	if err := s.sd.HealthCheck(); err != nil {
		return nil, fmt.Errorf("SD is not available: %w", err)
	}
	s.beginProgress()
	defer s.endProgress()
	ov := params.Overrides
	if ov != nil {
		c := *ov
		c.ModelName, c.Person = nil, nil
		ApplyPresetOverrides(p, &c)
	}
	if params.PersonLora != "" {
		p.Loras = addLoraOnce(p.Loras, params.PersonLora)
	}
	if p.Sampler == "" {
		p.Sampler = "euler_a"
	}
	if p.Steps == 0 {
		p.Steps = 20
	}
	if p.CfgScale == 0 {
		p.CfgScale = 7.0
	}
	scene, err := s.kids.FilterInput(strings.TrimSpace(params.ScenePrompt))
	if err != nil {
		return nil, err
	}
	prose := promptutil.ProseBase(ModelBase(p.ModelName))
	bp := s.buildStepPrompts(prose, p.Prompt, p.NegativePrompt, p.Loras, scene, strings.TrimSpace(params.ExtraNegativePrompt))
	spec := s.prepareSDContext(p, "stylist preset")
	width, height := params.Width, params.Height
	if width <= 0 || height <= 0 {
		width, height = s.resolveResolution(p, nil)
	}
	seed := params.Seed
	if seed == nil {
		seed = p.Seed
	}
	run := s.presetTxt2Img(p, spec, bp.Prompt, bp.NegativePrompt, width, height, seed, nil, false)
	run.req.DenoisingStrength = nil
	run.logPrefix = "stylist preset"
	out, err := s.runPresetTxt2Img(run)
	if err != nil {
		return nil, err
	}
	res := &PresetShotResult{FaceSettings: &FaceRefineParams{Model: p.ModelName, Sampler: p.Sampler, Steps: p.Steps, CfgScale: p.CfgScale, Prompt: bp.Prompt, NegativePrompt: bp.NegativePrompt}, Image: out.result.Images[0], Info: out.result.Info, Model: p.ModelName}
	if p.Name != "" {
		res.Model = p.Name
	}
	var info struct {
		Seed int64 `json:"seed"`
	}
	if len(out.result.Info) > 0 && json.Unmarshal(out.result.Info, &info) == nil {
		res.Seed = info.Seed
	}
	if res.Seed == 0 && seed != nil {
		res.Seed = *seed
	}
	return res, nil
}
