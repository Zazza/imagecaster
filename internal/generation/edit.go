package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

const (
	kleinLookModel        = "flux-2-klein-4b.safetensors"
	editSampler           = "euler"
	editScheduler         = "simple"
	editDenoise           = 1.0
	editAlign             = 16
	editMaxDim            = 2048
	vramReleaseModel      = "z-image-turbo-fp8-aio"
	editLabelMax          = 60
	editMaxInstructionLen = 2000
	editMaxLoras          = 8
	editMaxLoraWeight     = 2.0
	editMaxLoraNameLen    = 200
	kleinEditSteps        = 8
	kleinEditCfgScale     = 1.0
	qwenEditModel         = "qwen_image_2.1-Q8_0"
	qwenEditSteps         = 20
	qwenEditCfgScale      = 6.0
	qwenAlign             = 32

	editPromptSuffix = ". Keep the person, face, pose, background and composition unchanged, and keep the original lighting and color palette so the new clothing matches the photo."

	mossTEModule   = "qwen35_9b_int8_convrot.safetensors"
	mossVAEModule  = "flux2-vae_small.safetensors"
	kleinTEModule  = "qwen_3_4b.safetensors"
	kleinVAEModule = "flux2-klein-vae.safetensors"
)

var loraTagPrefixRe = regexp.MustCompile(`(?i)<lora:`)

var mossModules = []string{mossTEModule, mossVAEModule}
var kleinModules = []string{kleinTEModule, kleinVAEModule}

func isVisionModule(filename string) bool {
	return strings.Contains(strings.ToLower(filename), "mmproj")
}

func catalogModules(model string, vision bool) []string {
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return []string{}
	}
	m := catalog.FindSDModel(model)
	if m == nil {
		return []string{}
	}
	files := []string{}
	for _, mod := range m.Modules {
		if isVisionModule(mod.Filename) == vision {
			files = append(files, mod.Filename)
		}
	}
	return files
}

func modulesForModel(model string) []string {
	if config.ModelMatchesCSV(model, "moss") {
		return mossModules
	}
	if config.IsFlux2Klein(model) {
		return kleinModules
	}
	return catalogModules(model, false)
}

func withReference(spec sd.ModelSpec) sd.ModelSpec {
	spec.KleinReference = true
	if vision := catalogModules(spec.Checkpoint, true); len(vision) > 0 {
		spec.Modules = append(append([]string{}, spec.Modules...), vision...)
	}
	return spec
}

func alignTo(w, h, align int) (int, int) {
	w = w / align * align
	h = h / align * align
	if w < 64 {
		w = 64
	}
	if h < 64 {
		h = 64
	}
	return w, h
}

func refEditWorkSize(model string, w, h int) (int, int) {
	w, h = kleinWorkSize(w, h)
	if config.IsQwenImage(model) {
		w, h = alignTo(w, h, qwenAlign)
	}
	return w, h
}

type EditImageParams struct {
	ImageBase64          string             `json:"image_base64"`
	ReferenceImageBase64 string             `json:"reference_image_base64,omitempty"`
	Instruction          string             `json:"instruction"`
	StylePresetID        int64              `json:"style_preset_id,omitempty"`
	StyleTags            bool               `json:"style_tags,omitempty"`
	Loras                []preset.LoRAEntry `json:"loras,omitempty"`
	OutputMode           string             `json:"output_mode,omitempty"`
	ResolutionID         *int64             `json:"resolution_id,omitempty"`
	Seed                 *int64             `json:"seed,omitempty"`
	Model                string             `json:"model,omitempty"`
}

type editModelProfile struct {
	model  string
	family string
	steps  int
	cfg    float64
}

func resolveEditModel(model string) (editModelProfile, error) {
	switch model {
	case "", "klein":
		return editModelProfile{model: kleinLookModel, family: "klein", steps: kleinEditSteps, cfg: kleinEditCfgScale}, nil
	case "qwen":
		return editModelProfile{model: qwenEditModel, family: "qwen-image,qwen_image", steps: qwenEditSteps, cfg: qwenEditCfgScale}, nil
	}
	return editModelProfile{}, fmt.Errorf("model must be klein or qwen, got %q", truncateRunes(model, 40))
}

type editStyle struct {
	presetID int64
	tags     string
	loras    []preset.LoRAEntry
}

func buildEditPrompt(instruction string, styleTags string, loras []preset.LoRAEntry) string {
	var b strings.Builder
	for _, l := range loras {
		b.WriteString(fmt.Sprintf(" <lora:%s:%g>", l.Name, l.Weight))
	}
	if styleTags != "" {
		b.WriteString(" " + styleTags + ". ")
	}
	b.WriteString("Change only " + instruction + editPromptSuffix)
	return b.String()
}

func sanitizeEditLoras(loras []preset.LoRAEntry) ([]preset.LoRAEntry, error) {
	if len(loras) == 0 {
		return nil, nil
	}
	if len(loras) > editMaxLoras {
		return nil, fmt.Errorf("too many loras (max %d)", editMaxLoras)
	}
	out := make([]preset.LoRAEntry, 0, len(loras))
	for _, l := range loras {
		name := strings.TrimSpace(l.Name)
		if name == "" {
			return nil, errors.New("lora name is required")
		}
		if strings.ContainsAny(name, "<>:") || strings.ContainsFunc(name, unicode.IsControl) {
			return nil, fmt.Errorf("invalid lora name %q", truncateRunes(name, 40))
		}
		if len([]rune(name)) > editMaxLoraNameLen {
			return nil, fmt.Errorf("lora name too long (max %d characters)", editMaxLoraNameLen)
		}
		w := l.Weight
		if math.IsNaN(w) || math.IsInf(w, 0) {
			w = 0.6
		}
		if w < 0 {
			w = 0
		}
		if w > editMaxLoraWeight {
			w = editMaxLoraWeight
		}
		out = append(out, preset.LoRAEntry{Name: name, Weight: w})
	}
	return out, nil
}

func sanitizeStyleTags(tags string) string {
	tags = loraTagPrefixRe.ReplaceAllString(stripAngleTags(tags), "")
	tags = strings.TrimSpace(strings.Trim(tags, ", \n\r\t"))
	if len([]rune(tags)) > editMaxInstructionLen {
		tags = truncateRunes(tags, editMaxInstructionLen)
	}
	return tags
}

func (s *Service) loadEditStyle(presetID int64, family string) (*editStyle, error) {
	if presetID <= 0 {
		return nil, nil
	}
	p, err := s.db.Get(presetID)
	if err != nil {
		return nil, fmt.Errorf("style preset not found: %w", err)
	}
	if !config.ModelMatchesCSV(p.ModelName, family) {
		return nil, fmt.Errorf("style preset %q must use a %s model, got %q", p.Name, family, p.ModelName)
	}
	st := &editStyle{
		presetID: p.ID,
		tags:     sanitizeStyleTags(p.Prompt),
	}
	if p.Loras != "" {
		var loras []preset.LoRAEntry
		if err := json.Unmarshal([]byte(p.Loras), &loras); err != nil {
			s.log.Warn("edit style %q: invalid loras json, ignoring: %s", p.Name, err)
		} else {
			for _, l := range loras {
				if strings.Contains(strings.ToLower(l.Name), "dmd") {
					s.log.Info("edit style %q: skip lora %q (distillation loras degrade edit quality)", p.Name, l.Name)
					continue
				}
				st.loras = append(st.loras, l)
			}
		}
	}
	return st, nil
}

func stripAngleTags(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		s = s[i+j+1:]
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func alignEditSize(w, h int) (int, int) {
	return alignEditSizeCapped(w, h, editMaxDim)
}

func alignEditSizeCapped(w, h, maxDim int) (int, int) {
	if w > maxDim || h > maxDim {
		scale := float64(maxDim) / float64(max(w, h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
	}
	w = w / editAlign * editAlign
	h = h / editAlign * editAlign
	if w < 64 {
		w = 64
	}
	if h < 64 {
		h = 64
	}
	return w, h
}

func (s *Service) EditImage(params EditImageParams) (*GenerateImageResult, error) {
	if params.ImageBase64 == "" {
		return nil, fmt.Errorf("image is required")
	}
	prof, err := resolveEditModel(params.Model)
	if err != nil {
		return nil, err
	}
	if len(params.ImageBase64) > MaxImageBase64Len {
		return nil, config.ImageTooLarge("image")
	}
	if len(params.ReferenceImageBase64) > MaxImageBase64Len {
		return nil, config.ImageTooLarge("reference image")
	}
	instruction := strings.TrimSpace(stripAngleTags(params.Instruction))
	instruction = loraTagPrefixRe.ReplaceAllString(instruction, "")
	if instruction == "" {
		return nil, fmt.Errorf("instruction is required")
	}
	if len([]rune(instruction)) > editMaxInstructionLen {
		return nil, fmt.Errorf("instruction too long (max %d characters)", editMaxInstructionLen)
	}
	var filterErr error
	instruction, filterErr = s.kids.FilterInput(instruction)
	if filterErr != nil {
		return nil, filterErr
	}
	if params.OutputMode != "" && params.OutputMode != "standard" && params.OutputMode != "quality" {
		return nil, fmt.Errorf("output_mode must be standard or quality")
	}
	if params.OutputMode == "quality" && (params.ResolutionID == nil || *params.ResolutionID <= 0) {
		return nil, fmt.Errorf("quality output mode requires resolution")
	}
	style, err := s.loadEditStyle(params.StylePresetID, prof.family)
	if err != nil {
		return nil, err
	}
	combinedLoras := params.Loras
	if style != nil {
		if len(style.loras)+len(params.Loras) > editMaxLoras {
			return nil, fmt.Errorf("too many loras: style %d + own %d > max %d", len(style.loras), len(params.Loras), editMaxLoras)
		}
		combinedLoras = append(append([]preset.LoRAEntry{}, style.loras...), params.Loras...)
	}
	loras, err := sanitizeEditLoras(combinedLoras)
	if err != nil {
		return nil, err
	}

	if err := s.sd.HealthCheck(); err != nil {
		return nil, fmt.Errorf("SD is not available: %w", err)
	}
	s.StartSDPolling()
	defer s.StopSDPolling()

	s.guard.EnsureHeadroom(prof.model, "edit")

	spec := modelSpec(prof.model)
	refSpec := withReference(spec)

	srcW, srcH := filebrowser.DecodeImageSize(params.ImageBase64)
	if srcW <= 0 || srcH <= 0 {
		return nil, errors.New("decode image size: invalid image")
	}
	width, height := kleinImg2ImgSize(srcW, srcH)
	workW, workH := refEditWorkSize(prof.model, srcW, srcH)

	styleTags := ""
	if style != nil && params.StyleTags {
		styleTags = style.tags
	}
	prompt := buildEditPrompt(instruction, styleTags, loras)
	batchSize := 1
	batchCount := 1
	denoise := editDenoise

	baseReq := sd.Img2ImgRequest{
		Model:             refSpec,
		InitImages:        []string{params.ImageBase64},
		Prompt:            prompt,
		NegativePrompt:    s.kids.ApplyNegative(""),
		SamplerName:       editSampler,
		Scheduler:         editScheduler,
		Steps:             prof.steps,
		CfgScale:          prof.cfg,
		Width:             workW,
		Height:            workH,
		Seed:              params.Seed,
		DenoisingStrength: &denoise,
		BatchSize:         &batchSize,
		BatchCount:        &batchCount,
	}
	resp, err := s.sd.Img2Img(baseReq)
	if err != nil {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, ierr
		}
		return nil, err
	}
	if len(resp.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, ierr
		}
		return nil, errors.New("no image generated (edit)")
	}

	finalImage := resp.Images[0]
	finalImage = s.kleinUpscaleTo(finalImage, workW, width, height, "edit klein")
	finalInfo := resp.Info
	upscaleSkipped := false
	redenoiseSkipped := false
	if params.OutputMode == "quality" {
		{
			qualityReq := sd.Txt2ImgRequest{
				Model:          spec,
				Prompt:         prompt,
				NegativePrompt: s.kids.ApplyNegative(""),
				SamplerName:    editSampler,
				Scheduler:      editScheduler,
				Steps:          prof.steps,
				CfgScale:       prof.cfg,
				Width:          width,
				Height:         height,
				Seed:           params.Seed,
			}
			qImg, qInfo, qUpSkipped, qRdSkipped, qErr := s.editQualityPostProcess(finalImage, qualityReq, width, height, params.ResolutionID)
			if qErr != nil {
				return nil, qErr
			}
			finalImage = qImg
			if qInfo != nil {
				finalInfo = qInfo
			}
			upscaleSkipped = qUpSkipped
			redenoiseSkipped = qRdSkipped
		}
	}

	genMeta := &preset.GenerationMeta{
		Label: "Edit: " + truncateRunes(instruction, editLabelMax),
		Model: prof.model,
	}
	if style != nil {
		pid := style.presetID
		genMeta.PresetID = &pid
	}
	if params.OutputMode == "quality" {
		var lorasJSON string
		if len(loras) > 0 {
			if b, merr := json.Marshal(loras); merr == nil {
				lorasJSON = string(b)
			}
		}
		genMeta.Chain = buildQualityChain(preset.StepMeta{
			Op:      "base",
			Model:   prof.model,
			Denoise: &denoise,
			Loras:   lorasJSON,
		}, upscaleSkipped, redenoiseSkipped)
	}

	result := &GenerateImageResult{
		Image:                   finalImage,
		Parameters:              resp.Parameters,
		Info:                    finalInfo,
		EffectivePrompt:         prompt,
		QualityUpscaleSkipped:   upscaleSkipped,
		QualityRedenoiseSkipped: redenoiseSkipped,
	}
	s.sessions.AddToSession(finalImage, finalInfo, "edit", false, genMeta)
	return result, nil
}

func (s *Service) editQualityPostProcess(base64Img string, req sd.Txt2ImgRequest, srcW, srcH int, resolutionID *int64) (string, json.RawMessage, bool, bool, error) {
	fitW, fitH := s.fitImageToResolution(srcW, srcH, resolutionID)
	targetW, targetH := alignEditSize(fitW, fitH)
	return s.qualityUpscaleRedenoise(base64Img, req, srcW, srcH, targetW, targetH, "edit quality")
}
