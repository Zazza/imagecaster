package generation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/maskcomp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/promptutil"
	"github.com/Zazza/imagecaster/internal/sd"
)

// --- AnalyzeImage ---

const (
	materialPresetType     = "post_material"
	materialKeepColorSigma = 0.02
	materialObjectTemplate = "Change only the material of %s, keep its color exactly the same: make it %s. Keep all other clothes, the face, the background and everything else exactly as they are."
	patternPresetType      = "post_pattern"
	patternObjectTemplate  = "Change only the print pattern of %s, keep its cut and shape: make it %s. Keep all other clothes, the face, the background and everything else exactly as they are."
)

func (s *Service) AnalyzeImage(imageBase64, mode string) (string, error) {
	if imageBase64 == "" {
		return "", fmt.Errorf("image is required")
	}
	if len(imageBase64) > MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}

	model := s.getAnalyzeModel()
	s.settings.ApplyLLMConfig("analyze")

	maxTokens := s.getMaxTokens()

	if mode == "garment" {
		prompt, _ := s.db.GetSetting("edit_garment_prompt")
		if prompt == "" {
			prompt = config.DefaultEditGarmentPrompt
		}
		prompt += s.answerLanguageSuffix()
		s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
		text, err := s.llm.AnalyzeImageDescribe(model, prompt, imageBase64, maxTokens)
		s.emitter.Emit("llm:status", map[string]string{"status": "done"})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(text), nil
	}

	if mode == "describe" {
		prompt, _ := s.db.GetSetting("analyze_describe_prompt")
		if prompt == "" {
			prompt = config.DefaultAnalyzeDescribePrompt
		}
		prompt += s.answerLanguageSuffix()
		s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
		text, err := s.llm.AnalyzeImageDescribe(model, prompt, imageBase64, maxTokens)
		if err != nil {
			s.emitter.Emit("llm:status", map[string]string{"status": "done"})
			return "", err
		}
		s.emitter.Emit("llm:status", map[string]string{"status": "done"})
		text = strings.TrimSpace(text)
		text = s.kids.FilterOutput(text)
		return text, nil
	}

	systemPrompt, _ := s.db.GetSetting("analyze_system_prompt")
	if systemPrompt == "" {
		systemPrompt = config.DefaultAnalyzeSystemPrompt
	}
	prompt, _ := s.db.GetSetting("analyze_prompt")
	if prompt == "" {
		prompt = config.DefaultAnalyzePrompt
	}
	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	tags, err := s.llm.AnalyzeImage(model, systemPrompt+"\n\n"+prompt, imageBase64, maxTokens)
	if err != nil {
		s.emitter.Emit("llm:status", map[string]string{"status": "done"})
		return "", err
	}
	s.emitter.Emit("llm:status", map[string]string{"status": "done"})
	tags = s.kids.FilterOutput(tags)
	return tags, nil
}

func (s *Service) answerLanguageSuffix() string {
	if locale, _ := s.db.GetSetting("ui_locale"); locale == "en" {
		return "\n\nRespond in English."
	}
	return "\n\nОтвечай на русском языке."
}

func (s *Service) GetDefaultAnalyzePrompts() *AnalyzePrompts {
	return &AnalyzePrompts{
		SystemPrompt:   config.DefaultAnalyzeSystemPrompt,
		SinglePrompt:   config.DefaultAnalyzePrompt,
		DescribePrompt: config.DefaultAnalyzeDescribePrompt,
	}
}

// --- isProse ---

var prosePatterns = []string{
	"The image", "This image", "The picture", "This picture",
	"I can see", "I see", "appears to be", "depicts",
	"featuring", "shows a", "there are", "There is",
	"wearing", "holding", "standing", "sitting",
	"background shows", "foreground",
}

func isProse(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, p := range prosePatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	periods := strings.Count(s, ".")
	commas := strings.Count(s, ",")
	if periods > commas {
		return true
	}
	return false
}

// --- generateLLMPromptFromTags ---

func (s *Service) generateLLMPromptFromTags(p *preset.Preset, tags string, mode, extraNegative string) (*GenerateSDPromptResult, error) {
	sdPromptInstruction := s.getSDPromptInstructionFor(p.ModelName)
	systemPrompt := s.kids.ApplySystemPrompt(sdPromptInstruction)
	maxTokens := s.getMaxTokens()
	generateModel := s.getGenerateModel()

	s.settings.ApplyLLMConfig("generate")

	systemPrompt += fmt.Sprintf(`

RESPONSE LENGTH: your response is limited to ~%d tokens. You MUST fit within this limit.`, maxTokens)

	userParts := []string{
		"STYLE REFERENCE (do NOT include in output): " + p.Prompt,
		"STYLE NEGATIVE REFERENCE (do NOT include in output): " + p.NegativePrompt,
		s.userSceneLabel(p.ModelName) + tags,
	}
	if mode == "img2img" {
		userParts = []string{
			"MODE: img2img — user wants to TRANSFORM the image into the scene described below. Ignore what is currently in the image. Generate a NEW scene based on the user's description.",
			"IMPORTANT: the base image already defines the person's look. Do NOT invent appearance — APPEARANCE VARIATION does NOT apply here; use appearance only as explicitly described in the user instruction.",
			"BASE POSITIVE PROMPT (style/quality reference): " + p.Prompt,
			"BASE NEGATIVE PROMPT: " + p.NegativePrompt,
			"USER SCENE DESCRIPTION (THIS IS THE PRIMARY PROMPT — generate exactly this scene): " + tags,
		}
	}
	if extraNegative != "" {
		userParts = append(userParts, "USER NEGATIVE: "+extraNegative)
	}
	userMessage := strings.Join(userParts, "\n\n")

	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	promptResult, raw, err := s.llmGenerateAndParse(systemPrompt, userMessage, p.PresetType, generateModel, maxTokens)
	if err != nil {
		s.emitter.Emit("llm:status", map[string]string{"status": "done"})
		return nil, err
	}
	s.emitter.Emit("llm:status", map[string]string{"status": "done"})

	if promptResult.Prompt == "" && promptResult.NegativePrompt == "" {
		promptResult = GenerateSDPromptResult{
			Prompt:         promptutil.TruncateRepetitive(raw, s.promptTruncateLimit(p.ModelName)),
			NegativePrompt: p.NegativePrompt,
		}
	}

	if promptutil.ContainsCyrillic(promptResult.Prompt) {
		if extracted := promptutil.ExtractTagsFromRaw(raw); extracted != "" {
			promptResult.Prompt = extracted
		}
	}
	if promptutil.ContainsCyrillic(promptResult.NegativePrompt) {
		promptResult.NegativePrompt = promptutil.ExtractNegativeFromRaw(raw)
	}

	extractEmbeddedNegative(&promptResult)
	promptResult.Prompt = promptutil.StripJunk(promptResult.Prompt)
	promptResult.Prompt = promptutil.TruncateRepetitive(promptResult.Prompt, s.promptTruncateLimit(p.ModelName))
	promptResult.Prompt = promptutil.DedupeTags(promptResult.Prompt)
	promptResult.Prompt = promptutil.RemoveTags(promptResult.Prompt, p.Prompt)
	promptResult.Prompt = s.filterInstructionExamples(promptResult.Prompt, sdPromptInstruction, tags+" "+extraNegative)
	promptResult.NegativePrompt = promptutil.StripJunk(promptResult.NegativePrompt)
	promptResult.NegativePrompt = promptutil.TruncateRepetitive(promptResult.NegativePrompt, 500)
	promptResult.NegativePrompt = s.filterInstructionExamples(promptResult.NegativePrompt, sdPromptInstruction, tags+" "+extraNegative)

	promptResult.Prompt = s.kids.FilterOutput(promptResult.Prompt)
	promptResult.NegativePrompt = s.kids.FilterOutput(promptResult.NegativePrompt)

	return &promptResult, nil
}

// --- GenerateFromImage ---

func (s *Service) GenerateFromImage(params GenerateFromImageParams) (*GenerateImageResult, error) {
	s.StartSDPolling()
	defer s.StopSDPolling()
	if params.ImageBase64 == "" {
		return nil, fmt.Errorf("image is required")
	}
	if len(params.ImageBase64) > MaxImageBase64Len {
		return nil, config.ImageTooLarge("image")
	}
	if params.GenMode != "preset" && params.GenMode != "compound" {
		return nil, fmt.Errorf("gen_mode must be preset or compound")
	}
	if params.GenMode == "preset" && params.PresetID <= 0 {
		return nil, fmt.Errorf("preset is required")
	}
	if params.GenMode == "compound" && params.CompoundPresetID <= 0 {
		return nil, fmt.Errorf("compound preset is required")
	}
	if params.Mode != "txt2img" && params.Mode != "img2img" {
		return nil, fmt.Errorf("mode must be txt2img or img2img")
	}
	if params.OutputMode != "" && params.OutputMode != "standard" && params.OutputMode != "quality" {
		return nil, fmt.Errorf("output_mode must be standard or quality")
	}
	if params.OutputMode == "quality" {
		if params.GenMode != "preset" || params.Mode != "img2img" {
			return nil, fmt.Errorf("quality output mode requires img2img preset mode")
		}
		if params.ResolutionID == nil || *params.ResolutionID <= 0 {
			return nil, fmt.Errorf("quality output mode requires resolution")
		}
	}
	if params.PreserveIdentity && (params.GenMode != "preset" || params.Mode != "img2img") {
		return nil, fmt.Errorf("preserve identity requires img2img preset mode")
	}
	if params.PreserveIdentity && params.DenoisingStrength > 0.9 {
		return nil, fmt.Errorf("preserve identity requires denoising strength of 0.9 or less")
	}
	if params.DenoisingStrength <= 0 {
		params.DenoisingStrength = 0.5
	}
	if params.DenoisingStrength > 1.0 {
		params.DenoisingStrength = 1.0
	}
	var effectMask, maskSource image.Image
	if params.MaskBase64 != "" {
		if params.GenMode != "preset" || params.Mode != "img2img" || params.OutputMode == "quality" {
			return nil, fmt.Errorf("mask requires img2img preset mode with standard output")
		}
		if len(params.MaskBase64) > MaxImageBase64Len {
			return nil, config.ImageTooLarge("mask")
		}
		var err error
		if effectMask, err = decodeB64Image(params.MaskBase64, "mask"); err != nil {
			return nil, err
		}
		if !maskcomp.HasPaint(effectMask) {
			return nil, errors.New("mask is empty — paint over the area")
		}
		if maskSource, err = decodeB64Image(params.ImageBase64, "image"); err != nil {
			return nil, err
		}
	}

	var filterErr error
	params.ExtraNegativePrompt, filterErr = s.kids.FilterInput(params.ExtraNegativePrompt)
	if filterErr != nil {
		return nil, filterErr
	}

	tags := params.Tags

	tags, filterErr = s.kids.FilterInput(tags)
	if filterErr != nil {
		return nil, filterErr
	}

	if params.GenMode == "compound" {
		return s.generateFromImageCompound(params, tags)
	}

	p, err := s.db.Get(params.PresetID)
	if err != nil {
		return nil, fmt.Errorf("preset not found: %w", err)
	}
	ApplyPresetOverrides(p, params.Overrides)
	mossIdentity := params.PreserveIdentity && config.ModelMatchesCSV(p.ModelName, "moss")
	if params.PreserveIdentity && !mossIdentity && !config.ModelMatchesCSV(p.ModelName, "klein") {
		return nil, fmt.Errorf("preserve identity requires a klein or moss model, got %q", p.ModelName)
	}
	if mossIdentity {
		p.Sampler = mossEditSampler
		p.ScheduleType = mossEditScheduler
		p.Steps = mossEditSteps
		p.CfgScale = mossEditCfgScale
	}
	refOption, refOn, refOff := identityReference(mossIdentity)

	seed := p.Seed
	if params.Seed != nil {
		seed = params.Seed
	}

	var prompt, negativePrompt string
	if tags == "" {
		bp := s.buildPrompts(p.Prompt, p.NegativePrompt, p.Loras, "", params.ExtraNegativePrompt)
		prompt = bp.Prompt
		negativePrompt = bp.NegativePrompt
	} else {
		promptResult, err := s.generateLLMPromptFromTags(p, tags, params.Mode, params.ExtraNegativePrompt)
		if err != nil {
			return nil, err
		}

		prompt = promptResult.Prompt
		prompt = appendLorasToPrompt(prompt, p.Loras)
		negativePrompt = s.buildPrompts("", promptResult.NegativePrompt, "", "", params.ExtraNegativePrompt).NegativePrompt
	}

	if mossIdentity {
		prompt = editLoRATag + " " + prompt
	}

	if effectMask != nil && (p.PresetType == materialPresetType || p.PresetType == patternPresetType) {
		object, err := s.detectMaskedObject(maskSource, effectMask)
		if err != nil {
			return nil, err
		}
		template := materialObjectTemplate
		if p.PresetType == patternPresetType {
			template = patternObjectTemplate
		}
		prompt = fmt.Sprintf(template, object, prompt)
	}

	if err := s.prepareSDContext(p, "from-image"); err != nil {
		return nil, err
	}

	samplerName := promptutil.BuildSamplerName(p.Sampler, p.ScheduleType)

	clipSkip := 1
	if p.ClipSkip != nil {
		clipSkip = *p.ClipSkip
	}
	batchSize := 1
	batchCount := 1

	if params.Mode == "img2img" {
		var result *GenerateImageResult
		refDisabled := false
		if params.PreserveIdentity {
			s.referenceMu.Lock()
			defer s.referenceMu.Unlock()
			if err := s.sd.SetOption(refOption, refOn); err != nil {
				if rerr := s.sd.SetOption(refOption, refOff); rerr != nil {
					s.log.Warn("from-image: disable identity reference after failed enable: %s", rerr)
				}
				return nil, fmt.Errorf("enable identity reference: %w", err)
			}
			defer func() {
				if refDisabled {
					return
				}
				derr := s.sd.SetOption(refOption, refOff)
				if derr != nil {
					s.log.Warn("from-image: disable identity reference: %s", derr)
				}
				if result != nil {
					result.PreserveIdentityResetFailed = derr != nil
				}
			}()
		}
		if !params.PreserveIdentity && config.ModelMatchesCSV(p.ModelName, "klein") {
			if err := s.sd.SetOption(kleinReferenceOption, false); err != nil {
				s.log.Warn("from-image: clear stale klein reference: %s", err)
			}
		}
		srcW, srcH := filebrowser.DecodeImageSize(params.ImageBase64)
		rawW, rawH := srcW, srcH
		if srcW > 0 && srcH > 0 {
			srcW = srcW / 8 * 8
			srcH = srcH / 8 * 8
		} else {
			srcW = 512
			srcH = 512
		}
		var imgW, imgH int
		if params.OutputMode == "quality" {
			imgW, imgH = s.fitImageToResolutionDownOnly(srcW, srcH, params.ResolutionID)
		} else {
			imgW, imgH = s.fitImageToResolution(srcW, srcH, params.ResolutionID)
		}
		kleinOutW, kleinOutH := 0, 0
		if config.ModelMatchesCSV(p.ModelName, "klein") {
			if params.ResolutionID == nil || *params.ResolutionID <= 0 {
				kleinOutW, kleinOutH = kleinImg2ImgSize(imgW, imgH)
				if rawW > 0 && rawH > 0 {
					imgW, imgH = kleinWorkSize(rawW, rawH)
				} else {
					imgW, imgH = kleinWorkSize(kleinOutW, kleinOutH)
				}
			} else {
				imgW, imgH = alignLookSize(imgW, imgH)
			}
		}
		if mossIdentity {
			imgW, imgH = alignEditSize(imgW, imgH)
		}
		denoising := params.DenoisingStrength
		if denoising <= 0 {
			denoising = 0.5
		}
		resp, err := s.sd.Img2Img(sd.Img2ImgRequest{
			InitImages:        []string{params.ImageBase64},
			Prompt:            prompt,
			NegativePrompt:    negativePrompt,
			SamplerName:       samplerName,
			Scheduler:         p.ScheduleType,
			Steps:             p.Steps,
			CfgScale:          p.CfgScale,
			Width:             imgW,
			Height:            imgH,
			Seed:              seed,
			DenoisingStrength: &denoising,
			ClipSkip:          &clipSkip,
			BatchSize:         &batchSize,
			BatchCount:        &batchCount,
			DoNotSaveImages:   true,
			DoNotSaveGrid:     true,
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Images) == 0 {
			if ierr := s.checkSDInterrupted(); ierr != nil {
				return nil, ierr
			}
			return nil, fmt.Errorf("no image generated (%s)", params.Mode)
		}
		finalImage := s.kleinUpscaleTo(resp.Images[0], imgW, kleinOutW, kleinOutH, "from-image klein")
		finalInfo := resp.Info
		upscaleSkipped := false
		redenoiseSkipped := false
		if params.OutputMode == "quality" {
			qualityReady := true
			if params.PreserveIdentity {
				if err := s.sd.SetOption(refOption, refOff); err != nil {
					s.log.Warn("from-image: disable identity reference before quality failed, skipping quality: %s", err)
					upscaleSkipped = true
					redenoiseSkipped = true
					qualityReady = false
				} else {
					refDisabled = true
				}
			}
			if qualityReady {
				qualityReq := sd.Txt2ImgRequest{
					Prompt:         prompt,
					NegativePrompt: negativePrompt,
					SamplerName:    samplerName,
					Scheduler:      p.ScheduleType,
					Steps:          p.Steps,
					CfgScale:       p.CfgScale,
					Width:          imgW,
					Height:         imgH,
					Seed:           seed,
					ClipSkip:       &clipSkip,
					BatchSize:      &batchSize,
					BatchCount:     &batchCount,
				}
				qImg, qInfo, qUpscaleSkipped, qRedenoiseSkipped, qErr := s.qualityPostProcess(finalImage, qualityReq, imgW, imgH, params.ResolutionID)
				if qErr != nil {
					return nil, qErr
				}
				finalImage = qImg
				if qInfo != nil {
					finalInfo = qInfo
				}
				upscaleSkipped = qUpscaleSkipped
				redenoiseSkipped = qRedenoiseSkipped
			}
		}
		keepColor := params.KeepColor && p.PresetType == materialPresetType && params.OutputMode != "quality"
		if effectMask != nil || keepColor {
			source := maskSource
			if source == nil {
				if source, err = decodeB64ImageAnySize(params.ImageBase64, "image"); err != nil {
					return nil, err
				}
			}
			edited, err := decodeB64ImageAnySize(finalImage, "result")
			if err != nil {
				return nil, err
			}
			out := image.Image(edited)
			if keepColor {
				out = maskcomp.KeepColor(source, out, materialKeepColorSigma)
			}
			if effectMask != nil {
				out = maskcomp.Blend(source, out, effectMask)
			}
			if finalImage, err = encodePNGBase64(out); err != nil {
				return nil, err
			}
		}
		genMeta := &preset.GenerationMeta{
			PresetID: &params.PresetID,
			Label:    p.Name,
			Model:    p.ModelName,
		}
		if params.OutputMode == "quality" {
			genMeta.Chain = buildQualityChain(preset.StepMeta{
				Preset:  p.Name,
				Model:   p.ModelName,
				Loras:   p.Loras,
				Op:      "base",
				Denoise: &denoising,
			}, upscaleSkipped, redenoiseSkipped)
		}
		result = &GenerateImageResult{
			Image:                   finalImage,
			Info:                    finalInfo,
			QualityUpscaleSkipped:   upscaleSkipped,
			QualityRedenoiseSkipped: redenoiseSkipped,
			EffectivePrompt:         prompt,
			EffectiveNegativePrompt: negativePrompt,
		}
		s.sessions.AddToSession(finalImage, finalInfo, "from-image", false, genMeta)
		return result, nil
	}

	width, height := s.resolveResolution(p, params.ResolutionID)

	hiresEnabled, hiresUpscale, hiresDenoising, hiresUpscaler := s.resolveHires(params.HiresProfileID)
	var hiresFix *bool
	if hiresEnabled {
		hiresFix = &hiresEnabled
	}

	result, err := s.sd.Txt2Img(sd.Txt2ImgRequest{
		Prompt:                 prompt,
		NegativePrompt:         negativePrompt,
		SamplerName:            samplerName,
		Scheduler:              p.ScheduleType,
		Steps:                  p.Steps,
		CfgScale:               p.CfgScale,
		Width:                  width,
		Height:                 height,
		Seed:                   seed,
		DenoisingStrength:      hiresDenoising,
		ClipSkip:               &clipSkip,
		BatchSize:              &batchSize,
		BatchCount:             &batchCount,
		HiresFix:               hiresFix,
		HiresUpscale:           hiresUpscale,
		HiresDenoisingStrength: hiresDenoising,
		HiresUpscaler:          hiresUpscaler,
		DoNotSaveImages:        true,
		DoNotSaveGrid:          true,
	})
	if err != nil {
		return nil, err
	}
	if len(result.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, ierr
		}
		return nil, fmt.Errorf("no image generated (txt2img)")
	}

	img := &GenerateImageResult{
		Image:                   result.Images[0],
		Parameters:              result.Parameters,
		Info:                    result.Info,
		EffectivePrompt:         prompt,
		EffectiveNegativePrompt: negativePrompt,
	}
	img.SessionItemID = s.sessions.AddToSession(result.Images[0], result.Info, "generate", false, &preset.GenerationMeta{
		PresetID: &params.PresetID,
		Label:    p.Name,
		Model:    p.ModelName,
	})
	return img, nil
}

// --- generateFromImageCompound ---

func (s *Service) generateFromImageCompound(params GenerateFromImageParams, tags string) (*GenerateImageResult, error) {
	cp, err := s.db.GetCompoundPreset(params.CompoundPresetID)
	if err != nil {
		return nil, fmt.Errorf("compound preset not found: %w", err)
	}
	if len(cp.Steps) == 0 {
		return nil, fmt.Errorf("compound preset has no steps")
	}

	firstPreset, err := s.db.Get(cp.Steps[0].PresetID)
	if err != nil {
		return nil, fmt.Errorf("step 1: preset not found: %w", err)
	}

	promptResult, err := s.generateLLMPromptFromTags(firstPreset, tags, params.Mode, params.ExtraNegativePrompt)
	if err != nil {
		return nil, err
	}

	var lastImage string
	var lastInfo json.RawMessage
	lastStepIdx := len(cp.Steps) - 1
	chain := make([]preset.StepMeta, 0, len(cp.Steps))
	lastModel := ""

	imgW, imgH := filebrowser.DecodeImageSize(params.ImageBase64)
	if imgW > 0 && imgH > 0 {
		imgW = imgW / 8 * 8
		imgH = imgH / 8 * 8
	}

	for stepIdx, step := range cp.Steps {
		p, err := s.db.Get(step.PresetID)
		if err != nil {
			return nil, fmt.Errorf("step %d: preset not found: %w", stepIdx+1, err)
		}
		ApplyPresetOverrides(p, &PresetOverrides{Loras: step.Loras})
		chain = append(chain, stepMeta(stepIdx, p, step))
		lastModel = p.ModelName

		isLastStep := stepIdx == lastStepIdx

		s.emitter.Emit("fromimage:progress", map[string]any{
			"current": stepIdx + 1,
			"total":   len(cp.Steps),
			"status":  "generating",
		})

		bp := s.buildPrompts(p.Prompt, p.NegativePrompt, p.Loras, promptResult.Prompt, promptResult.NegativePrompt)
		prompt := bp.Prompt
		negativePrompt := bp.NegativePrompt

		if err := s.prepareSDContext(p, fmt.Sprintf("from-image compound step %d", stepIdx+1)); err != nil {
			return nil, err
		}
		samplerName := promptutil.BuildSamplerName(p.Sampler, p.ScheduleType)
		width, height := s.resolveStepResolution(step, p, params.ResolutionID)

		clipSkip := 1
		if p.ClipSkip != nil {
			clipSkip = *p.ClipSkip
		}

		if stepIdx == 0 {
			w, h := imgW, imgH
			if w == 0 || h == 0 {
				w, h = width, height
			}
			lastImage, lastInfo, err = s.runFromImageCompoundFirstStep(p, params.ImageBase64, prompt, negativePrompt, samplerName, w, h, clipSkip, params.Mode, params.DenoisingStrength, stepIdx+1)
			if err != nil {
				return nil, err
			}
			if isLastStep {
				if img, info, ok := s.applyHiresOnLastStep(p, lastImage, prompt, negativePrompt, samplerName, w, h, clipSkip, 1, 1, params.HiresProfileID); ok {
					lastImage, lastInfo = img, info
				}
			}
		} else {
			w, h := width, height
			if imgW > 0 && imgH > 0 && params.Mode == "img2img" {
				w, h = imgW, imgH
			}
			lastImage, lastInfo, err = s.runFromImageCompoundStep(step, p, lastImage, prompt, negativePrompt, samplerName, w, h, clipSkip, stepIdx+1)
			if err != nil {
				return nil, err
			}
			if isLastStep {
				if img, info, ok := s.applyHiresOnLastStep(p, lastImage, prompt, negativePrompt, samplerName, w, h, clipSkip, 1, 1, params.HiresProfileID); ok {
					lastImage, lastInfo = img, info
				}
			}
		}
	}

	s.emitter.Emit("fromimage:progress", map[string]any{
		"current": len(cp.Steps),
		"total":   len(cp.Steps),
		"status":  "done",
	})

	img := &GenerateImageResult{
		Image:                   lastImage,
		Info:                    lastInfo,
		EffectivePrompt:         promptResult.Prompt,
		EffectiveNegativePrompt: promptResult.NegativePrompt,
	}
	s.sessions.AddToSession(lastImage, lastInfo, "compound-from-image", false, &preset.GenerationMeta{
		Label: cp.Name,
		Model: lastModel,
		Chain: chain,
	})
	return img, nil
}

// --- GenerateCompoundImage ---

func (s *Service) resolveCompoundSteps(params GenerateCompoundImageParams) ([]preset.CompoundPresetStep, string, error) {
	if len(params.Steps) > 0 {
		if params.CompoundPresetID > 0 {
			return nil, "", fmt.Errorf("steps and compound_preset_id are mutually exclusive")
		}
		if err := ValidateCustomSteps(params.Steps); err != nil {
			return nil, "", err
		}
		steps := make([]preset.CompoundPresetStep, 0, len(params.Steps))
		for i, cs := range params.Steps {
			steps = append(steps, preset.CompoundPresetStep{
				StepOrder:         i + 1,
				PresetID:          cs.PresetID,
				DenoisingStrength: cs.DenoisingStrength,
				Loras:             cs.Loras,
				ResolutionID:      cs.ResolutionID,
			})
		}
		return steps, "", nil
	}
	if params.CompoundPresetID <= 0 {
		return nil, "", fmt.Errorf("compound preset is required")
	}
	cp, err := s.db.GetCompoundPreset(params.CompoundPresetID)
	if err != nil {
		return nil, "", fmt.Errorf("compound preset not found: %w", err)
	}
	if len(cp.Steps) == 0 {
		return nil, "", fmt.Errorf("compound preset has no steps")
	}
	return cp.Steps, cp.Name, nil
}

func (s *Service) GenerateCompoundImage(params GenerateCompoundImageParams) (*GenerateImageResult, error) {
	steps, label, err := s.resolveCompoundSteps(params)
	if err != nil {
		return nil, err
	}
	if err := ValidateIdea(params.Idea, params.IdeaNegative); err != nil {
		return nil, err
	}
	s.StartSDPolling()
	defer s.StopSDPolling()
	if strings.TrimSpace(params.Idea) != "" {
		prompt, negative, err := s.resolveIdea(steps[0].PresetID, nil, params.Idea, params.IdeaNegative)
		if err != nil {
			return nil, err
		}
		params.ExtraPrompt, params.ExtraNegativePrompt = prompt, negative
	}

	var lastImage string
	var lastInfo json.RawMessage
	lastStepIdx := len(steps) - 1
	chain := make([]preset.StepMeta, 0, len(steps))
	lastModel := ""

	var userScenePrompt, userSceneNeg string
	if strings.TrimSpace(params.ExtraPrompt) != "" {
		firstPreset, err := s.db.Get(steps[0].PresetID)
		if err != nil {
			return nil, fmt.Errorf("step 1: preset not found: %w", err)
		}
		llmResult, err := s.generateLLMPromptFromTags(firstPreset, params.ExtraPrompt, "default", params.ExtraNegativePrompt)
		if err != nil {
			return nil, err
		}
		userScenePrompt = llmResult.Prompt
		userSceneNeg = llmResult.NegativePrompt
	}
	if err := s.checkSDInterrupted(); err != nil {
		return nil, err
	}

	for stepIdx, step := range steps {
		p, err := s.db.Get(step.PresetID)
		if err != nil {
			return nil, fmt.Errorf("step %d: preset not found: %w", stepIdx+1, err)
		}
		if params.Seed != nil {
			p.Seed = params.Seed
		}
		ApplyPresetOverrides(p, &PresetOverrides{Loras: step.Loras})
		chain = append(chain, stepMeta(stepIdx, p, step))
		lastModel = p.ModelName

		isLastStep := stepIdx == lastStepIdx

		s.emitter.Emit("compound:progress", map[string]any{
			"current": stepIdx + 1,
			"total":   len(steps),
			"status":  "generating",
			"step":    stepIdx + 1,
		})

		bp := s.buildPrompts(p.Prompt, p.NegativePrompt, p.Loras, userScenePrompt, userSceneNeg)
		prompt := bp.Prompt
		negativePrompt := bp.NegativePrompt

		if err := s.prepareSDContext(p, fmt.Sprintf("compound step %d", stepIdx+1)); err != nil {
			return nil, err
		}
		samplerName := promptutil.BuildSamplerName(p.Sampler, p.ScheduleType)
		width, height := s.resolveStepResolution(step, p, params.ResolutionID)

		clipSkip := 1
		if p.ClipSkip != nil {
			clipSkip = *p.ClipSkip
		}

		if stepIdx == 0 {
			lastImage, lastInfo, err = s.runCompoundFirstStep(step, p, prompt, negativePrompt, samplerName, width, height, clipSkip, isLastStep, params.HiresProfileID, stepIdx+1)
			if err != nil {
				return nil, err
			}
		} else {
			lastImage, lastInfo, err = s.runCompoundStep(step, p, lastImage, prompt, negativePrompt, samplerName, width, height, clipSkip, stepIdx+1)
			if err != nil {
				return nil, err
			}

			if isLastStep {
				lastImage, lastInfo, _ = s.applyHiresOnLastStep(p, lastImage, prompt, negativePrompt, samplerName, width, height, clipSkip, 1, 1, params.HiresProfileID)
			}
		}
	}

	s.emitter.Emit("compound:progress", map[string]any{
		"current": len(steps),
		"total":   len(steps),
		"status":  "done",
	})

	img := &GenerateImageResult{
		Image:                   lastImage,
		Info:                    lastInfo,
		EffectivePrompt:         "",
		EffectiveNegativePrompt: "",
	}
	img.SessionItemID = s.sessions.AddToSession(lastImage, lastInfo, "compound", false, &preset.GenerationMeta{
		Label: label,
		Model: lastModel,
		Chain: chain,
	})
	return img, nil
}

func decodeB64ImageAnySize(b64, what string) (image.Image, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", what, err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", what, err)
	}
	return img, nil
}

func identityReference(moss bool) (string, any, any) {
	if moss {
		return mossReferenceOption, mossReferenceOn, mossReferenceOff
	}
	return kleinReferenceOption, true, false
}
