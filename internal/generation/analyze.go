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
	systemPrompt := s.kids.ApplySystemPrompt(sdPromptInstruction + textInImageRule)
	maxTokens := s.getMaxTokens()
	generateModel := s.getGenerateModel()

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

	if promptutil.ContainsCyrillicOutsideQuotes(promptResult.Prompt) {
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

type fromImageSource struct {
	b64  string
	img  image.Image
	mask image.Image
}

func (f *fromImageSource) decode() (image.Image, error) {
	if f.img == nil {
		img, err := decodeB64ImageAnySize(f.b64, "image")
		if err != nil {
			return nil, err
		}
		f.img = img
	}
	return f.img, nil
}

func (f *fromImageSource) size() (int, int) {
	if f.img != nil {
		b := f.img.Bounds()
		return b.Dx(), b.Dy()
	}
	return filebrowser.DecodeImageSize(f.b64)
}

func (s *Service) GenerateFromImage(params GenerateFromImageParams) (*GenerateImageResult, error) {
	s.beginProgress()
	defer s.endProgress()
	src, err := validateFromImage(&params)
	if err != nil {
		return nil, err
	}
	tags, err := s.filterFromImageInput(&params)
	if err != nil {
		return nil, err
	}
	if params.GenMode == "compound" {
		return s.generateFromImageCompound(params, tags)
	}

	p, err := s.db.Get(params.PresetID)
	if err != nil {
		return nil, fmt.Errorf("preset not found: %w", err)
	}
	ApplyPresetOverrides(p, params.Overrides)
	if params.PreserveIdentity && !config.IsFlux2Klein(p.ModelName) && !config.IsQwenImage(p.ModelName) {
		return nil, fmt.Errorf("preserve identity requires a flux-2-klein or qwen-image model, got %q", p.ModelName)
	}

	prompt, negativePrompt, err := s.fromImagePrompts(p, params, tags, src)
	if err != nil {
		return nil, err
	}
	spec := s.prepareSDContext(p, "from-image")
	seed := p.Seed
	if params.Seed != nil {
		seed = params.Seed
	}
	if params.Mode == "img2img" {
		return s.fromImageImg2Img(p, params, src, spec, prompt, negativePrompt, seed)
	}
	return s.fromImageTxt2Img(p, params, spec, prompt, negativePrompt, seed)
}

func validateFromImage(params *GenerateFromImageParams) (*fromImageSource, error) {
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
	presetImg2Img := params.GenMode == "preset" && params.Mode == "img2img"
	if params.OutputMode == "quality" {
		if !presetImg2Img {
			return nil, fmt.Errorf("quality output mode requires img2img preset mode")
		}
		if params.ResolutionID == nil || *params.ResolutionID <= 0 {
			return nil, fmt.Errorf("quality output mode requires resolution")
		}
	}
	if params.PreserveIdentity && !presetImg2Img {
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
	src := &fromImageSource{b64: params.ImageBase64}
	if params.MaskBase64 == "" {
		return src, nil
	}
	if !presetImg2Img || params.OutputMode == "quality" {
		return nil, fmt.Errorf("mask requires img2img preset mode with standard output")
	}
	if len(params.MaskBase64) > MaxImageBase64Len {
		return nil, config.ImageTooLarge("mask")
	}
	var err error
	if src.mask, err = decodeB64Image(params.MaskBase64, "mask"); err != nil {
		return nil, err
	}
	if !maskcomp.HasPaint(src.mask) {
		return nil, errors.New("mask is empty — paint over the area")
	}
	if src.img, err = decodeB64Image(params.ImageBase64, "image"); err != nil {
		return nil, err
	}
	return src, nil
}

func (s *Service) filterFromImageInput(params *GenerateFromImageParams) (string, error) {
	neg, err := s.kids.FilterInput(params.ExtraNegativePrompt)
	if err != nil {
		return "", err
	}
	params.ExtraNegativePrompt = neg
	return s.kids.FilterInput(params.Tags)
}

func (s *Service) fromImagePrompts(p *preset.Preset, params GenerateFromImageParams, tags string, src *fromImageSource) (string, string, error) {
	var prompt, negativePrompt string
	if tags == "" {
		bp := s.buildPrompts(p.Prompt, p.NegativePrompt, p.Loras, "", params.ExtraNegativePrompt)
		prompt, negativePrompt = bp.Prompt, bp.NegativePrompt
	} else {
		promptResult, err := s.generateLLMPromptFromTags(p, tags, params.Mode, params.ExtraNegativePrompt)
		if err != nil {
			return "", "", err
		}
		prompt = appendLorasToPrompt(promptResult.Prompt, p.Loras)
		negativePrompt = s.buildPrompts("", promptResult.NegativePrompt, "", "", params.ExtraNegativePrompt).NegativePrompt
	}
	if src.mask != nil && (p.PresetType == materialPresetType || p.PresetType == patternPresetType) {
		object, err := s.detectMaskedObject(src.img, src.mask)
		if err != nil {
			return "", "", err
		}
		template := materialObjectTemplate
		if p.PresetType == patternPresetType {
			template = patternObjectTemplate
		}
		prompt = fmt.Sprintf(template, object, prompt)
	}
	return prompt, negativePrompt, nil
}

type fromImageSize struct {
	w, h           int
	kleinW, kleinH int
}

func (s *Service) fromImageWorkSize(p *preset.Preset, params GenerateFromImageParams, rawW, rawH int) fromImageSize {
	srcW, srcH := 512, 512
	if rawW > 0 && rawH > 0 {
		srcW, srcH = rawW/8*8, rawH/8*8
	}
	var sz fromImageSize
	if params.OutputMode == "quality" {
		sz.w, sz.h = s.fitImageToResolutionDownOnly(srcW, srcH, params.ResolutionID)
	} else {
		sz.w, sz.h = s.fitImageToResolution(srcW, srcH, params.ResolutionID)
	}
	qwenRef := params.PreserveIdentity && config.IsQwenImage(p.ModelName)
	if !config.IsFlux2Klein(p.ModelName) && !qwenRef {
		return sz
	}
	if params.ResolutionID == nil || *params.ResolutionID <= 0 {
		sz.kleinW, sz.kleinH = kleinImg2ImgSize(sz.w, sz.h)
		if rawW > 0 && rawH > 0 {
			sz.w, sz.h = refEditWorkSize(p.ModelName, rawW, rawH)
		} else {
			sz.w, sz.h = refEditWorkSize(p.ModelName, sz.kleinW, sz.kleinH)
		}
		return sz
	}
	sz.w, sz.h = alignLookSize(sz.w, sz.h)
	if qwenRef {
		sz.w, sz.h = alignTo(sz.w, sz.h, qwenAlign)
	}
	return sz
}

func (s *Service) fromImageImg2Img(p *preset.Preset, params GenerateFromImageParams, src *fromImageSource, spec sd.ModelSpec, prompt, negativePrompt string, seed *int64) (*GenerateImageResult, error) {
	keepColor := params.KeepColor && p.PresetType == materialPresetType && params.OutputMode != "quality"
	if keepColor {
		if _, err := src.decode(); err != nil {
			return nil, err
		}
	}
	refSpec := spec
	if params.PreserveIdentity {
		refSpec = withReference(spec)
	}
	rawW, rawH := src.size()
	sz := s.fromImageWorkSize(p, params, rawW, rawH)
	denoising := params.DenoisingStrength
	resp, err := s.sd.Img2Img(sd.Img2ImgRequest{
		Model:             refSpec,
		InitImages:        []string{params.ImageBase64},
		Prompt:            prompt,
		NegativePrompt:    negativePrompt,
		SamplerName:       p.Sampler,
		Scheduler:         p.ScheduleType,
		Steps:             p.Steps,
		CfgScale:          p.CfgScale,
		Width:             sz.w,
		Height:            sz.h,
		Seed:              seed,
		DenoisingStrength: &denoising,
		ClipSkip:          presetClipSkip(p),
		BatchSize:         intPtr(1),
		BatchCount:        intPtr(1),
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
	result := &GenerateImageResult{
		Image:                   s.kleinUpscaleTo(resp.Images[0], sz.w, sz.kleinW, sz.kleinH, "from-image klein"),
		Info:                    resp.Info,
		EffectivePrompt:         prompt,
		EffectiveNegativePrompt: negativePrompt,
	}
	if params.OutputMode == "quality" {
		qualityReq := s.presetTxt2Img(p, spec, prompt, negativePrompt, sz.w, sz.h, seed, nil, false).req
		qualityReq.DenoisingStrength = nil
		qImg, qInfo, upscaleSkipped, redenoiseSkipped, err := s.qualityPostProcess(result.Image, qualityReq, sz.w, sz.h, params.ResolutionID)
		if err != nil {
			return nil, err
		}
		result.Image = qImg
		if qInfo != nil {
			result.Info = qInfo
		}
		result.QualityUpscaleSkipped = upscaleSkipped
		result.QualityRedenoiseSkipped = redenoiseSkipped
	}
	if src.mask != nil || keepColor {
		if result.Image, err = blendFromImageResult(src.img, result.Image, src.mask, keepColor); err != nil {
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
		}, result.QualityUpscaleSkipped, result.QualityRedenoiseSkipped)
	}
	result.SessionItemID = s.sessions.AddToSession(result.Image, result.Info, "from-image", genMeta)
	return result, nil
}

func blendFromImageResult(source image.Image, resultB64 string, mask image.Image, keepColor bool) (string, error) {
	edited, err := decodeB64ImageAnySize(resultB64, "result")
	if err != nil {
		return "", err
	}
	out := edited
	if keepColor {
		out = maskcomp.KeepColor(source, out, materialKeepColorSigma)
	}
	if mask != nil {
		out = maskcomp.Blend(source, out, mask)
	}
	return encodePNGBase64(out)
}

func (s *Service) fromImageTxt2Img(p *preset.Preset, params GenerateFromImageParams, spec sd.ModelSpec, prompt, negativePrompt string, seed *int64) (*GenerateImageResult, error) {
	width, height := s.resolveResolution(p, params.ResolutionID)
	run := s.presetTxt2Img(p, spec, prompt, negativePrompt, width, height, seed, params.HiresProfileID, false)
	run.logPrefix = "from-image"
	out, err := s.runPresetTxt2Img(run)
	if err != nil {
		return nil, err
	}
	img := &GenerateImageResult{
		Image:                   out.result.Images[0],
		Parameters:              out.result.Parameters,
		Info:                    out.result.Info,
		HiresFixSkipped:         out.hiresSkipped,
		HiresFixManual:          out.hiresManual,
		EffectivePrompt:         prompt,
		EffectiveNegativePrompt: negativePrompt,
	}
	img.SessionItemID = s.sessions.AddToSession(img.Image, img.Info, "generate", &preset.GenerationMeta{
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

		s.guard.EnsureHeadroom(p.ModelName, fmt.Sprintf("from-image compound step %d", stepIdx+1))
		samplerName := p.Sampler
		width, height := s.resolveStepResolution(step, p, params.ResolutionID)

		clipSkip := *presetClipSkip(p)

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
	s.sessions.AddToSession(lastImage, lastInfo, "compound-from-image", &preset.GenerationMeta{
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
	s.beginProgress()
	defer s.endProgress()
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

		s.guard.EnsureHeadroom(p.ModelName, fmt.Sprintf("compound step %d", stepIdx+1))
		samplerName := p.Sampler
		width, height := s.resolveStepResolution(step, p, params.ResolutionID)

		clipSkip := *presetClipSkip(p)

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
	img.SessionItemID = s.sessions.AddToSession(lastImage, lastInfo, "compound", &preset.GenerationMeta{
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
