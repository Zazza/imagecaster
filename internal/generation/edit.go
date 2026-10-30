package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"

	"go-sd/internal/config"
	"go-sd/internal/filebrowser"
	"go-sd/internal/preset"
	"go-sd/internal/sd"
)

const (
	mossEditModel             = "moss_image_sft_bf16"
	mossReferenceOption       = "moss_do_reference"
	mossReferenceOn           = "Moss-Image-Edit"
	mossReferenceOff          = "Disable"
	mossEditSteps             = 30
	mossEditCfgScale          = 4.0
	mossEditSampler           = "Euler"
	mossEditScheduler         = "Simple"
	mossEditDenoise           = 1.0
	mossEditAlign             = 16
	mossEditMaxDim            = 2048
	mossEditLabelMax          = 60
	mossEditMaxInstructionLen = 2000
	mossEditMaxLoras          = 8
	mossEditMaxLoraWeight     = 2.0
	mossEditMaxLoraNameLen    = 200

	editLoRATag      = "<lora:Moss_Edit_LoRA:1.0>"
	editPromptSuffix = ". Keep the person, face, pose, background and composition unchanged."

	mossTEModulePath  = "/mnt/e/SDStudio/stable-diffusion-webui-forge/models/text_encoder/qwen35_9b_int8_convrot.safetensors"
	mossVAEModulePath = "/mnt/e/SDStudio/stable-diffusion-webui-forge/models/VAE/flux2-vae_small.safetensors"
)

var loraTagPrefixRe = regexp.MustCompile(`(?i)<lora:`)

var mossForgeModules = []string{mossTEModulePath, mossVAEModulePath}

func forgeModulesForModel(model string) []string {
	if config.ModelMatchesCSV(model, "moss") {
		return mossForgeModules
	}
	return []string{}
}

type EditImageParams struct {
	ImageBase64   string             `json:"image_base64"`
	Instruction   string             `json:"instruction"`
	StylePresetID int64              `json:"style_preset_id,omitempty"`
	Loras         []preset.LoRAEntry `json:"loras,omitempty"`
	OutputMode    string             `json:"output_mode,omitempty"`
	ResolutionID  *int64             `json:"resolution_id,omitempty"`
	Seed          *int64             `json:"seed,omitempty"`
}

type editStyle struct {
	presetID int64
	tags     string
	loras    []preset.LoRAEntry
}

func buildEditPrompt(instruction string, styleTags string, loras []preset.LoRAEntry) string {
	var b strings.Builder
	b.WriteString(editLoRATag)
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
	if len(loras) > mossEditMaxLoras {
		return nil, fmt.Errorf("too many loras (max %d)", mossEditMaxLoras)
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
		if len([]rune(name)) > mossEditMaxLoraNameLen {
			return nil, fmt.Errorf("lora name too long (max %d characters)", mossEditMaxLoraNameLen)
		}
		w := l.Weight
		if math.IsNaN(w) || math.IsInf(w, 0) {
			w = 0.6
		}
		if w < 0 {
			w = 0
		}
		if w > mossEditMaxLoraWeight {
			w = mossEditMaxLoraWeight
		}
		out = append(out, preset.LoRAEntry{Name: name, Weight: w})
	}
	return out, nil
}

func sanitizeStyleTags(tags string) string {
	tags = loraTagPrefixRe.ReplaceAllString(stripAngleTags(tags), "")
	tags = strings.TrimSpace(strings.Trim(tags, ", \n\r\t"))
	if len([]rune(tags)) > mossEditMaxInstructionLen {
		tags = truncateRunes(tags, mossEditMaxInstructionLen)
	}
	return tags
}

func (s *Service) loadEditStyle(presetID int64) (*editStyle, error) {
	if presetID <= 0 {
		return nil, nil
	}
	p, err := s.db.Get(presetID)
	if err != nil {
		return nil, fmt.Errorf("style preset not found: %w", err)
	}
	if !config.ModelMatchesCSV(p.ModelName, "moss") {
		return nil, fmt.Errorf("style preset %q must use a moss model, got %q", p.Name, p.ModelName)
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
			st.loras = loras
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
	if w > mossEditMaxDim || h > mossEditMaxDim {
		scale := float64(mossEditMaxDim) / float64(max(w, h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
	}
	w = w / mossEditAlign * mossEditAlign
	h = h / mossEditAlign * mossEditAlign
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
	if len(params.ImageBase64) > MaxImageBase64Len {
		return nil, fmt.Errorf("image too large (max 16 MB)")
	}
	instruction := strings.TrimSpace(stripAngleTags(params.Instruction))
	instruction = loraTagPrefixRe.ReplaceAllString(instruction, "")
	if instruction == "" {
		return nil, fmt.Errorf("instruction is required")
	}
	if len([]rune(instruction)) > mossEditMaxInstructionLen {
		return nil, fmt.Errorf("instruction too long (max %d characters)", mossEditMaxInstructionLen)
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
	style, err := s.loadEditStyle(params.StylePresetID)
	if err != nil {
		return nil, err
	}
	combinedLoras := params.Loras
	if style != nil {
		if len(style.loras)+len(params.Loras) > mossEditMaxLoras {
			return nil, fmt.Errorf("too many loras: style %d + own %d > max %d", len(style.loras), len(params.Loras), mossEditMaxLoras)
		}
		combinedLoras = append(append([]preset.LoRAEntry{}, style.loras...), params.Loras...)
	}
	loras, err := sanitizeEditLoras(combinedLoras)
	if err != nil {
		return nil, err
	}

	s.editMu.Lock()
	defer s.editMu.Unlock()

	if err := s.sd.HealthCheck(); err != nil {
		return nil, fmt.Errorf("SD is not available: %w", err)
	}
	s.StartSDPolling()
	defer s.StopSDPolling()

	s.guard.EnsureHeadroom(mossEditModel, "edit")

	if err := s.setModel(mossEditModel, "edit"); err != nil {
		return nil, fmt.Errorf("moss_image_sft_bf16 not available on SD backend: %w", err)
	}
	if err := s.sd.SetVAE("Automatic"); err != nil {
		s.log.Warn("edit: reset vae: %s", err)
	}
	if err := s.sd.SetOption(mossReferenceOption, mossReferenceOn); err != nil {
		if rerr := s.sd.SetOption(mossReferenceOption, mossReferenceOff); rerr != nil {
			s.log.Warn("edit: disable moss reference after failed enable: %s", rerr)
		}
		return nil, fmt.Errorf("enable edit reference: %w", err)
	}

	var result *GenerateImageResult
	defer func() {
		derr := s.sd.SetOption(mossReferenceOption, mossReferenceOff)
		if derr != nil {
			s.log.Warn("edit: disable moss reference: %s", derr)
		}
		if result != nil {
			result.EditReferenceResetFailed = derr != nil
		}
	}()

	srcW, srcH := filebrowser.DecodeImageSize(params.ImageBase64)
	if srcW <= 0 || srcH <= 0 {
		return nil, errors.New("decode image size: invalid image")
	}
	width, height := alignEditSize(srcW, srcH)

	styleTags := ""
	if style != nil {
		styleTags = style.tags
	}
	prompt := buildEditPrompt(instruction, styleTags, loras)
	batchSize := 1
	batchCount := 1
	denoise := mossEditDenoise

	resp, err := s.sd.Img2Img(sd.Img2ImgRequest{
		InitImages:        []string{params.ImageBase64},
		Prompt:            prompt,
		NegativePrompt:    s.kids.ApplyNegative(""),
		SamplerName:       mossEditSampler,
		Scheduler:         mossEditScheduler,
		Steps:             mossEditSteps,
		CfgScale:          mossEditCfgScale,
		Width:             width,
		Height:            height,
		Seed:              params.Seed,
		DenoisingStrength: &denoise,
		BatchSize:         &batchSize,
		BatchCount:        &batchCount,
		DoNotSaveImages:   true,
		DoNotSaveGrid:     true,
	})
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
	finalInfo := resp.Info
	upscaleSkipped := false
	redenoiseSkipped := false
	if params.OutputMode == "quality" {
		if err := s.sd.SetOption(mossReferenceOption, mossReferenceOff); err != nil {
			s.log.Warn("edit: disable moss reference before quality failed, skipping quality: %s", err)
			upscaleSkipped = true
			redenoiseSkipped = true
		} else {
			qualityReq := sd.Txt2ImgRequest{
				Prompt:         prompt,
				NegativePrompt: s.kids.ApplyNegative(""),
				SamplerName:    mossEditSampler,
				Scheduler:      mossEditScheduler,
				Steps:          mossEditSteps,
				CfgScale:       mossEditCfgScale,
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
		Label: "Edit: " + truncateRunes(instruction, mossEditLabelMax),
		Model: mossEditModel,
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
			Model:   mossEditModel,
			Denoise: &denoise,
			Loras:   lorasJSON,
		}, upscaleSkipped, redenoiseSkipped)
	}

	result = &GenerateImageResult{
		Image:                   finalImage,
		Parameters:              resp.Parameters,
		Info:                    finalInfo,
		IsPreview:               false,
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

func (s *Service) ResetMossReference() {
	if err := s.sd.HealthCheck(); err != nil {
		return
	}
	if err := s.sd.SetOption(mossReferenceOption, mossReferenceOff); err != nil {
		s.log.Warn("reset moss reference: %s", err)
	}
}
