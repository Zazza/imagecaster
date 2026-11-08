package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/promptutil"
	"github.com/Zazza/imagecaster/internal/sd"
)

// --- Post style compiler ---

type RecipeStep struct {
	PresetID int64   `json:"preset_id"`
	Name     string  `json:"name"`
	Strength float64 `json:"strength"`
}

type CompiledRecipe struct {
	Steps      []RecipeStep  `json:"steps"`
	Adjust     adjust.Params `json:"adjust"`
	SourceText string        `json:"source_text,omitempty"`
}

const (
	recipeStrengthMin = 0.35
	recipeStrengthMax = 0.65
	recipeMaxSteps    = 3
	recipeTemperature = 0.2
	recipePurposeMax  = 80
)

var postTypeFamilies = []string{"post", "post_material", "post_skin", "post_light", "post_film"}

var adjustRanges = adjust.Ranges

const describeForStylePrompt = "Describe this photo in 1-2 short sentences for a photo style assistant: subject and pose, clothing, lighting, mood, dominant colors. No lists, no camera settings, no advice."

type postEffect struct {
	ID      int64
	Name    string
	Family  string
	Purpose string
}

type recipeLLMResponse struct {
	Effects []struct {
		Name     string  `json:"name"`
		Strength float64 `json:"strength"`
	} `json:"effects"`
	Adjust map[string]float64 `json:"adjust"`
}

func clampFloat(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampAdjust(values map[string]float64) adjust.Params {
	var p adjust.Params
	if len(values) == 0 {
		return p
	}
	set := func(dst *float64, key string) {
		v, ok := values[key]
		if !ok {
			return
		}
		r := adjustRanges[key]
		*dst = clampFloat(v, r[0], r[1])
	}
	set(&p.Brightness, "brightness")
	set(&p.Contrast, "contrast")
	set(&p.BlackPoint, "black_point")
	set(&p.WhitePoint, "white_point")
	set(&p.Gamma, "gamma")
	set(&p.Saturation, "saturation")
	set(&p.Temperature, "temperature")
	set(&p.BalanceR, "balance_r")
	set(&p.BalanceG, "balance_g")
	set(&p.BalanceB, "balance_b")
	set(&p.Sharpen, "sharpen")
	set(&p.Vignette, "vignette")
	return p
}

func parseAndValidate(text string, catalog []postEffect) (*CompiledRecipe, error) {
	var parsed recipeLLMResponse
	if err := json.Unmarshal([]byte(promptutil.ExtractJSON(text)), &parsed); err != nil {
		return nil, fmt.Errorf("parse recipe response: %w", err)
	}

	byName := make(map[string]postEffect, len(catalog))
	for _, e := range catalog {
		key := strings.ToLower(strings.TrimSpace(e.Name))
		if _, ok := byName[key]; !ok {
			byName[key] = e
		}
	}

	recipe := &CompiledRecipe{Steps: []RecipeStep{}}
	seen := make(map[int64]bool, len(parsed.Effects))
	for _, e := range parsed.Effects {
		if len(recipe.Steps) >= recipeMaxSteps {
			break
		}
		effect, ok := byName[strings.ToLower(strings.TrimSpace(e.Name))]
		if !ok || seen[effect.ID] {
			continue
		}
		seen[effect.ID] = true
		recipe.Steps = append(recipe.Steps, RecipeStep{
			PresetID: effect.ID,
			Name:     effect.Name,
			Strength: clampFloat(e.Strength, recipeStrengthMin, recipeStrengthMax),
		})
	}
	recipe.Adjust = clampAdjust(parsed.Adjust)

	if len(recipe.Steps) == 0 && recipe.Adjust.IsNeutral() {
		return nil, fmt.Errorf("could not compile a recipe from this description")
	}
	return recipe, nil
}

func effectPurpose(prompt string) string {
	p := strings.TrimSpace(prompt)
	if i := strings.IndexAny(p, ",.;\n"); i >= 0 {
		p = strings.TrimSpace(p[:i])
	}
	r := []rune(p)
	if len(r) > recipePurposeMax {
		r = r[:recipePurposeMax]
	}
	return string(r)
}

func (s *Service) postEffectCatalog() ([]postEffect, error) {
	types, err := s.db.ListPresetTypes()
	if err != nil {
		return nil, fmt.Errorf("list preset types: %w", err)
	}

	familyByTypeID := make(map[int64]string)
	for _, pt := range types {
		name := strings.ToLower(strings.TrimSpace(pt.Name))
		if slices.Contains(postTypeFamilies, name) {
			familyByTypeID[pt.ID] = name
		}
	}
	if len(familyByTypeID) == 0 {
		return nil, nil
	}

	typeIDs := make([]int64, 0, len(familyByTypeID))
	for id := range familyByTypeID {
		typeIDs = append(typeIDs, id)
	}
	slices.Sort(typeIDs)

	presets, err := s.db.ListByTypeIDs(typeIDs)
	if err != nil {
		return nil, fmt.Errorf("list post effects: %w", err)
	}

	effects := make([]postEffect, 0, len(presets))
	for _, p := range presets {
		if p.TypeID == nil {
			continue
		}
		family, ok := familyByTypeID[*p.TypeID]
		if !ok {
			continue
		}
		effects = append(effects, postEffect{
			ID:      p.ID,
			Name:    p.Name,
			Family:  family,
			Purpose: effectPurpose(p.Prompt),
		})
	}
	return effects, nil
}

func buildRecipeSystemPrompt(catalog []postEffect) string {
	var b strings.Builder
	b.WriteString("You are a post-processing recipe compiler. Convert the user's look description into a recipe.\n\n")
	if len(catalog) == 0 {
		b.WriteString("Available effects: none.\n")
	} else {
		b.WriteString("Available effects grouped by family (name — purpose):\n")
		for _, family := range postTypeFamilies {
			var entries []string
			for _, e := range catalog {
				if e.Family != family {
					continue
				}
				entry := e.Name
				if e.Purpose != "" {
					entry += " — " + e.Purpose
				}
				entries = append(entries, entry)
			}
			if len(entries) == 0 {
				continue
			}
			b.WriteString(family + ": " + strings.Join(entries, "; ") + "\n")
		}
	}
	b.WriteString("\nAvailable adjustments with ranges: brightness -100..100, contrast -100..100, black_point 0..254, white_point 1..255, gamma 0.1..3, saturation -100..100, temperature -100..100, balance_r -100..100, balance_g -100..100, balance_b -100..100, sharpen 0..100, vignette 0..100.\n\n")
	b.WriteString(`Rules: pick 0-3 effects whose names appear EXACTLY in the list; strength 0.35-0.65; adjustments optional, prefer empty when unsure; omit fields you don't change. Reply with ONLY valid JSON: {"effects":[{"name":"","strength":0.5}],"adjust":{"temperature":-15}}`)
	return b.String()
}

func (s *Service) CompilePostStyle(text string) (*CompiledRecipe, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("look description is required")
	}
	filtered, filterErr := s.kids.FilterInput(text)
	if filterErr != nil {
		return nil, filterErr
	}
	if strings.TrimSpace(filtered) == "" {
		return nil, fmt.Errorf("look description is required")
	}

	catalog, err := s.postEffectCatalog()
	if err != nil {
		return nil, err
	}

	systemPrompt := buildRecipeSystemPrompt(catalog)
	userMessage := "LOOK DESCRIPTION:\n" + filtered

	maxTokens := s.getMaxTokens()
	generateModel := s.getGenerateModel()
	s.settings.ApplyLLMConfig("generate")

	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := s.llm.Chat(generateModel, systemPrompt, userMessage, recipeTemperature, maxTokens)
		if err != nil {
			s.emitter.Emit("llm:status", map[string]string{"status": "done"})
			return nil, err
		}
		recipe, err := parseAndValidate(raw, catalog)
		if err == nil {
			s.emitter.Emit("llm:status", map[string]string{"status": "done"})
			recipe.SourceText = text
			return recipe, nil
		}
		lastErr = err
	}
	s.emitter.Emit("llm:status", map[string]string{"status": "done"})
	return nil, lastErr
}

func (s *Service) DescribeForStyle(imageBase64 string) (string, error) {
	if imageBase64 == "" {
		return "", fmt.Errorf("image is required")
	}
	if len(imageBase64) > MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}

	model := s.getAnalyzeModel()
	s.settings.ApplyLLMConfig("analyze")
	maxTokens := s.getMaxTokens()

	prompt := describeForStylePrompt
	if locale, _ := s.db.GetSetting("ui_locale"); locale == "en" {
		prompt += "\n\nRespond in English."
	} else {
		prompt += "\n\nОтвечай на русском языке."
	}

	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	text, err := s.llm.AnalyzeImageDescribe(model, prompt, imageBase64, maxTokens)
	s.emitter.Emit("llm:status", map[string]string{"status": "done"})
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	return s.kids.FilterOutput(text), nil
}

// --- Look direct (klein reference restyle) ---

const (
	kleinLookSteps          = 8
	kleinLookCfgScale       = 1.0
	kleinLookSampler        = "Euler"
	kleinLookScheduler      = "Simple"
	kleinLookTemperature    = 0.3
	kleinLookMaxPromptRunes = 500
	kleinLookLabelMax       = 60
	kleinLookMaxDim         = 2048
	kleinLookAlign          = 16
)

const kleinLookSystemPrompt = "You translate a description of the desired look into a short English prose prompt for an image restyle model. Describe only the visual look: light, color, mood, texture, atmosphere. 1-2 sentences, at most 60 words. Reply with the prompt only."

type ApplyLookParams struct {
	ImageBase64 string  `json:"image_base64"`
	Text        string  `json:"text"`
	Strength    float64 `json:"strength"`
	Seed        *int64  `json:"seed"`
}

func ValidateLook(p ApplyLookParams) error {
	_, err := validateLookParams(p.ImageBase64, p.Text)
	return err
}

func validateLookParams(imageBase64, text string) (string, error) {
	if imageBase64 == "" {
		return "", fmt.Errorf("image is required")
	}
	if len(imageBase64) > MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("look text is required")
	}
	return text, nil
}

func clampLookStrength(strength float64) float64 {
	return clampFloat(strength, recipeStrengthMin, recipeStrengthMax)
}

const kleinImg2ImgMinDim = 1024

func kleinImg2ImgSize(w, h int) (int, int) {
	if w > 0 && h > 0 && max(w, h) < kleinImg2ImgMinDim {
		scale := float64(kleinImg2ImgMinDim) / float64(max(w, h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
	}
	return alignLookSize(w, h)
}

const kleinWorkPixels = 1024 * 1024

func kleinWorkSize(w, h int) (int, int) {
	if w <= 0 || h <= 0 {
		return alignLookSize(w, h)
	}
	scale := math.Sqrt(float64(kleinWorkPixels) / (float64(w) * float64(h)))
	return alignLookSize(int(float64(w)*scale), int(float64(h)*scale))
}

func (s *Service) kleinUpscaleTo(img string, workW, outW, outH int, logPrefix string) string {
	if workW <= 0 || outW <= workW {
		return img
	}
	upscaler := s.resolveQualityUpscaler()
	scale := float64(outW) / float64(workW)
	s.log.Info("%s: upscale %.2fx to %dx%d (%s)", logPrefix, scale, outW, outH, upscaler)
	up, err := s.upscaleWithFallback(img, scale, outW, outH, upscaler)
	if err != nil {
		s.log.Warn("%s: upscale failed, keeping work-size result: %s", logPrefix, err)
		return img
	}
	return up
}

func alignLookSize(w, h int) (int, int) {
	if w > kleinLookMaxDim || h > kleinLookMaxDim {
		scale := float64(kleinLookMaxDim) / float64(max(w, h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
	}
	w = w / kleinLookAlign * kleinLookAlign
	h = h / kleinLookAlign * kleinLookAlign
	if w < 64 {
		w = 64
	}
	if h < 64 {
		h = 64
	}
	return w, h
}

func (s *Service) buildLookPrompt(text string) (string, error) {
	maxTokens := s.getMaxTokens()
	generateModel := s.getGenerateModel()
	s.settings.ApplyLLMConfig("generate")

	userMessage := "LOOK DESCRIPTION:\n" + text

	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	defer s.emitter.Emit("llm:status", map[string]string{"status": "done"})

	for attempt := 0; attempt < 2; attempt++ {
		raw, err := s.llm.Chat(generateModel, kleinLookSystemPrompt, userMessage, kleinLookTemperature, maxTokens)
		if err != nil {
			return "", err
		}
		if prompt := strings.TrimSpace(raw); prompt != "" {
			return truncateRunes(prompt, kleinLookMaxPromptRunes), nil
		}
	}
	return "", fmt.Errorf("empty look prompt response")
}

func (s *Service) ApplyLookDirect(imageBase64, text string, strength float64, seed *int64) (*GenerateImageResult, error) {
	text, err := validateLookParams(imageBase64, text)
	if err != nil {
		return nil, err
	}
	var filterErr error
	text, filterErr = s.kids.FilterInput(text)
	if filterErr != nil {
		return nil, filterErr
	}
	strength = clampLookStrength(strength)

	lookPrompt, err := s.buildLookPrompt(text)
	if err != nil {
		s.log.Warn("look: LLM rewrite failed, using original text: %s", err)
		lookPrompt = truncateRunes(text, kleinLookMaxPromptRunes)
	}

	s.referenceMu.Lock()
	defer s.referenceMu.Unlock()

	if err := s.sd.HealthCheck(); err != nil {
		return nil, fmt.Errorf("SD is not available: %w", err)
	}
	s.StartSDPolling()
	defer s.StopSDPolling()

	s.guard.EnsureHeadroom(kleinLookModel, "look")

	if err := s.setModel(kleinLookModel, "look"); err != nil {
		return nil, fmt.Errorf("%s not available on SD backend: %w", kleinLookModel, err)
	}
	if err := s.sd.SetOption(kleinReferenceOption, true); err != nil {
		if rerr := s.sd.SetOption(kleinReferenceOption, false); rerr != nil {
			s.log.Warn("look: disable klein reference after failed enable: %s", rerr)
		}
		return nil, fmt.Errorf("enable look reference: %w", err)
	}

	var result *GenerateImageResult
	defer func() {
		derr := s.sd.SetOption(kleinReferenceOption, false)
		if derr != nil {
			s.log.Warn("look: disable klein reference: %s", derr)
		}
		if result != nil {
			result.PreserveIdentityResetFailed = derr != nil
		}
	}()

	srcW, srcH := filebrowser.DecodeImageSize(imageBase64)
	if srcW <= 0 || srcH <= 0 {
		return nil, errors.New("decode image size: invalid image")
	}
	outW, outH := alignLookSize(srcW, srcH)
	width, height := kleinWorkSize(outW, outH)

	batchSize := 1
	batchCount := 1
	resp, err := s.sd.Img2Img(sd.Img2ImgRequest{
		InitImages:        []string{imageBase64},
		Prompt:            lookPrompt,
		NegativePrompt:    s.kids.ApplyNegative(""),
		SamplerName:       kleinLookSampler,
		Scheduler:         kleinLookScheduler,
		Steps:             kleinLookSteps,
		CfgScale:          kleinLookCfgScale,
		Width:             width,
		Height:            height,
		Seed:              seed,
		DenoisingStrength: &strength,
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
		return nil, errors.New("no image generated (look)")
	}

	finalImage := s.kleinUpscaleTo(resp.Images[0], width, outW, outH, "look")
	result = &GenerateImageResult{
		Image:           finalImage,
		Info:            resp.Info,
		EffectivePrompt: lookPrompt,
	}
	s.sessions.AddToSession(finalImage, resp.Info, "look", false, &preset.GenerationMeta{
		Label: "Look: " + truncateRunes(text, kleinLookLabelMax),
		Model: kleinLookModel,
	})
	return result, nil
}
