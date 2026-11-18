package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/promptutil"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/sdnames"

	xdraw "golang.org/x/image/draw"
)

const (
	MaxImageBase64Len = config.MaxImageBase64Len
	MaxImageBytes     = config.MaxImageBytes
)

const (
	qualityRedenoise = 0.35
	maxCustomSteps   = 8
)

var qualityUpscalerPreferences = []string{"RealESRGAN_x4plus"}

// --- Interfaces ---

type EventEmitter interface {
	Emit(event string, data ...any)
}

type SessionAdder interface {
	AddToSession(imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64
}

func stepMeta(idx int, p *preset.Preset, step preset.CompoundPresetStep) preset.StepMeta {
	sm := preset.StepMeta{
		Step:   idx + 1,
		Preset: p.Name,
		Model:  p.ModelName,
		Loras:  p.Loras,
	}
	if idx > 0 {
		ds := step.DenoisingStrength
		if ds <= 0 {
			ds = 0.5
		}
		sm.Denoise = &ds
	}
	return sm
}

// --- Types ---

type SDProgressEvent struct {
	Progress    float64 `json:"progress"`
	ETARelative float64 `json:"eta_relative"`
	Job         string  `json:"job"`
	JobCount    int     `json:"job_count"`
	JobNo       int     `json:"job_no"`
	Steps       int     `json:"steps"`
	SamplerName string  `json:"sampler_name"`
	Preview     string  `json:"preview,omitempty"`
}

type GenerateSDPromptParams struct {
	PresetID    int64            `json:"preset_id"`
	Description string           `json:"description"`
	Negative    string           `json:"negative"`
	Overrides   *PresetOverrides `json:"overrides,omitempty"`
}

type GenerateSDPromptResult struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
}

type AnalyzePrompts struct {
	SystemPrompt   string `json:"system_prompt"`
	SinglePrompt   string `json:"single_prompt"`
	DescribePrompt string `json:"describe_prompt"`
}

type GenerateImageParams struct {
	PresetID            int64            `json:"preset_id"`
	ExtraPrompt         string           `json:"extra_prompt"`
	ExtraNegativePrompt string           `json:"extra_negative_prompt"`
	ResolutionID        *int64           `json:"resolution_id,omitempty"`
	HiresProfileID      *int64           `json:"hires_profile_id,omitempty"`
	Seed                *int64           `json:"seed,omitempty"`
	Overrides           *PresetOverrides `json:"overrides,omitempty"`
	Idea                string           `json:"idea,omitempty"`
	IdeaNegative        string           `json:"idea_negative,omitempty"`
}

type GenerateImageResult struct {
	Image                   string          `json:"image"`
	Parameters              json.RawMessage `json:"parameters"`
	Info                    json.RawMessage `json:"info"`
	HiresFixSkipped         bool            `json:"hires_fix_skipped"`
	HiresFixManual          bool            `json:"hires_fix_manual"`
	QualityUpscaleSkipped   bool            `json:"quality_upscale_skipped,omitempty"`
	QualityRedenoiseSkipped bool            `json:"quality_redenoise_skipped,omitempty"`
	IdeaPrompt              string          `json:"idea_prompt,omitempty"`
	IdeaNegativePrompt      string          `json:"idea_negative_prompt,omitempty"`
	EffectivePrompt         string          `json:"effective_prompt"`
	EffectiveNegativePrompt string          `json:"effective_negative_prompt"`
	DetectedObject          string          `json:"detected_object,omitempty"`
	SessionItemID           int64           `json:"session_item_id,omitempty"`
	FaceSwap                string          `json:"face_swap,omitempty"`
}

type UpscaleImageParams struct {
	ImageBase64 string `json:"image_base64"`
	GenInfo     string `json:"gen_info"`
	PresetID    int64  `json:"preset_id"`
}

type TestGenerateParams struct {
	Mode           string   `json:"mode"`
	SelectedIDs    []int64  `json:"selected_ids"`
	SelectedModels []string `json:"selected_models"`
	Prompt         string   `json:"prompt"`
	NegativePrompt string   `json:"negative_prompt"`
	Sampler        string   `json:"sampler"`
	ScheduleType   string   `json:"schedule_type"`
	Steps          int      `json:"steps"`
	CfgScale       float64  `json:"cfg_scale"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	Seed           *int64   `json:"seed"`
	ResolutionID   *int64   `json:"resolution_id,omitempty"`
	HiresProfileID *int64   `json:"hires_profile_id,omitempty"`
}

type TestGenerateResultItem struct {
	Name         string          `json:"name"`
	Image        string          `json:"image"`
	Seed         int64           `json:"seed"`
	Error        string          `json:"error,omitempty"`
	Sampler      string          `json:"sampler"`
	ScheduleType string          `json:"schedule_type"`
	CfgScale     float64         `json:"cfg_scale"`
	ModelName    string          `json:"model_name"`
	Info         json.RawMessage `json:"info,omitempty"`
}

type CustomStep struct {
	PresetID          int64   `json:"preset_id"`
	DenoisingStrength float64 `json:"denoising_strength"`
	Loras             *string `json:"loras,omitempty"`
	ResolutionID      *int64  `json:"resolution_id,omitempty"`
}

type GenerateCompoundImageParams struct {
	CompoundPresetID    int64        `json:"compound_preset_id"`
	ExtraPrompt         string       `json:"extra_prompt"`
	ExtraNegativePrompt string       `json:"extra_negative_prompt"`
	ResolutionID        *int64       `json:"resolution_id,omitempty"`
	HiresProfileID      *int64       `json:"hires_profile_id,omitempty"`
	Steps               []CustomStep `json:"steps,omitempty"`
	Seed                *int64       `json:"seed,omitempty"`
	Idea                string       `json:"idea,omitempty"`
	IdeaNegative        string       `json:"idea_negative,omitempty"`
}

func ValidateCustomSteps(steps []CustomStep) error {
	if len(steps) > maxCustomSteps {
		return fmt.Errorf("maximum %d custom steps allowed", maxCustomSteps)
	}
	for i, step := range steps {
		if step.PresetID <= 0 {
			return fmt.Errorf("step %d: preset is required", i+1)
		}
		if step.DenoisingStrength < 0 || step.DenoisingStrength > 1 {
			return fmt.Errorf("step %d: denoising_strength must be between 0 and 1", i+1)
		}
		if i > 0 && step.DenoisingStrength == 0 {
			return fmt.Errorf("step %d: denoising_strength must be greater than 0", i+1)
		}
		if step.Loras != nil && *step.Loras != "" {
			if len(*step.Loras) > preset.MaxStepLorasLen {
				return fmt.Errorf("step %d: loras payload too large", i+1)
			}
			if !json.Valid([]byte(*step.Loras)) {
				return fmt.Errorf("step %d: loras must be valid JSON", i+1)
			}
		}
	}
	return nil
}

type GenerateFromImageParams struct {
	ImageBase64         string           `json:"image_base64"`
	Mode                string           `json:"mode"`
	GenMode             string           `json:"gen_mode"`
	OutputMode          string           `json:"output_mode,omitempty"`
	PresetID            int64            `json:"preset_id"`
	CompoundPresetID    int64            `json:"compound_preset_id"`
	DenoisingStrength   float64          `json:"denoising_strength"`
	Tags                string           `json:"tags"`
	ExtraNegativePrompt string           `json:"extra_negative_prompt"`
	PreserveIdentity    bool             `json:"preserve_identity"`
	ResolutionID        *int64           `json:"resolution_id,omitempty"`
	HiresProfileID      *int64           `json:"hires_profile_id,omitempty"`
	Seed                *int64           `json:"seed,omitempty"`
	Overrides           *PresetOverrides `json:"overrides,omitempty"`
	MaskBase64          string           `json:"mask_base64,omitempty"`
	KeepColor           bool             `json:"keep_color,omitempty"`
}

// --- Service ---

type Service struct {
	db       *preset.DB
	llm      llm.Service
	sd       sd.Service
	cfg      *config.Config
	dataDir  string
	emitter  EventEmitter
	kids     *kids.Manager
	sessions SessionAdder
	log      *logger.Logger
	guard    *MemoryGuard

	ctx      context.Context
	progress *sdProgressState
	ideas    *ideaCache
}

func New(
	db *preset.DB,
	llmSvc llm.Service,
	sdSvc sd.Service,
	cfg *config.Config,
	dataDir string,
	emitter EventEmitter,
	kidsMgr *kids.Manager,
	sessions SessionAdder,
	log *logger.Logger,
) *Service {
	svc := &Service{
		db:       db,
		llm:      llmSvc,
		sd:       sdSvc,
		cfg:      cfg,
		dataDir:  dataDir,
		emitter:  emitter,
		kids:     kidsMgr,
		sessions: sessions,
		log:      log,
		guard:    NewMemoryGuard(llmSvc, sdSvc, db, log),
		ctx:      context.Background(),
		progress: &sdProgressState{},
		ideas:    newIdeaCache(),
	}
	if sdSvc != nil {
		sdSvc.SetProgressHandler(svc.onSDProgress)
	}
	return svc
}

func (s *Service) WithSD(sdSvc sd.Service) *Service {
	c := *s
	c.sd = sdSvc
	return &c
}

func (s *Service) WithSessions(sessions SessionAdder) *Service {
	c := *s
	c.sessions = sessions
	return &c
}

type ctxSessions struct {
	ctx     context.Context
	inner   SessionAdder
	log     *logger.Logger
	dropped atomic.Bool
}

func (g *ctxSessions) AddToSession(imageBase64 string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64 {
	if err := g.ctx.Err(); err != nil {
		g.dropped.Store(true)
		if g.log != nil {
			g.log.Warn("queue job stopped (%s), %s result not saved to history", err, source)
		}
		return 0
	}
	return g.inner.AddToSession(imageBase64, info, source, meta)
}

func (s *Service) Stopped() bool {
	return s.ctx != nil && s.ctx.Err() != nil
}

func (s *Service) LLM() llm.Service {
	return s.llm
}

func (s *Service) ForQueue(ctx context.Context, sdSvc sd.Service, llmSvc llm.Service) (*Service, func() bool) {
	c := s.WithSD(sdSvc)
	c.ctx = ctx
	if llmSvc != nil {
		c.llm = llmSvc
	}
	if s.sessions == nil {
		return c, func() bool { return false }
	}
	g := &ctxSessions{ctx: ctx, inner: s.sessions, log: s.log}
	c.sessions = g
	return c, g.dropped.Load
}

type sdProgressState struct {
	mu          sync.Mutex
	active      bool
	interrupted bool
}

func (s *Service) beginProgress() {
	s.progress.mu.Lock()
	s.progress.interrupted = false
	s.progress.active = true
	s.progress.mu.Unlock()
}

func (s *Service) endProgress() {
	s.progress.mu.Lock()
	s.progress.active = false
	s.progress.mu.Unlock()
}

func (s *Service) onSDProgress(st sd.JobStatus) {
	s.progress.mu.Lock()
	active := s.progress.active
	s.progress.mu.Unlock()
	if !active {
		return
	}
	s.emitter.Emit("sd:progress", sdProgressEvent(st, time.Now()))
}

func sdProgressEvent(st sd.JobStatus, now time.Time) SDProgressEvent {
	ev := SDProgressEvent{Job: st.ID, JobCount: 1, Steps: st.Progress.Steps}
	if st.Progress.Steps > 0 {
		ev.Progress = float64(st.Progress.Step) / float64(st.Progress.Steps)
		if ev.Progress > 1 {
			ev.Progress = 1
		}
	}
	if st.StartedAt != nil && st.Progress.Step > 0 && st.Progress.Step < st.Progress.Steps {
		elapsed := now.Sub(*st.StartedAt).Seconds()
		if elapsed > 0 {
			ev.ETARelative = elapsed / float64(st.Progress.Step) * float64(st.Progress.Steps-st.Progress.Step)
		}
	}
	return ev
}

func (s *Service) InterruptGeneration() error {
	s.progress.mu.Lock()
	s.progress.interrupted = true
	s.progress.mu.Unlock()
	return s.sd.Interrupt()
}

var ErrInterrupted = errors.New("interrupted")

func IsInterrupted(err error) bool {
	return err != nil && (errors.Is(err, ErrInterrupted) ||
		errors.Is(err, sd.ErrUserCancel) || errors.Is(err, sd.ErrJobCancelled))
}

func (s *Service) checkSDInterrupted() error {
	s.progress.mu.Lock()
	interrupted := s.progress.interrupted
	s.progress.mu.Unlock()
	if interrupted {
		return ErrInterrupted
	}
	return nil
}

func (s *Service) upscaleWithFallback(base64Img string, scale float64, targetW, targetH int, upscaler string) (string, error) {
	upscaledB64 := ""
	if upscaler != "" {
		neural, err := s.sd.UpscaleImage(base64Img, upscaler, scale)
		if err != nil {
			s.log.Warn("Neural upscale failed (%s), falling back to CatmullRom: %s", upscaler, err)
		} else {
			upscaledB64 = neural
			s.log.Info("Neural upscale with %s OK", upscaler)
		}
	}

	if upscaledB64 == "" {
		imgData, err := base64.StdEncoding.DecodeString(base64Img)
		if err != nil {
			return "", fmt.Errorf("decode base64: %w", err)
		}
		img, err := png.Decode(bytes.NewReader(imgData))
		if err != nil {
			return "", fmt.Errorf("decode png: %w", err)
		}
		dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		upscaledB64, err = imgcodec.EncodePNG(dst, png.BestSpeed)
		if err != nil {
			return "", err
		}
	}

	return upscaledB64, nil
}

func (s *Service) img2imgRedenoise(base64Img string, req sd.Txt2ImgRequest, targetW, targetH int, denoise float64) (*sd.Txt2ImgResponse, error) {
	i2iReq := sd.Img2ImgRequest{
		Model:             req.Model,
		InitImages:        []string{base64Img},
		CacheMode:         req.CacheMode,
		Prompt:            req.Prompt,
		NegativePrompt:    req.NegativePrompt,
		SamplerName:       req.SamplerName,
		Scheduler:         req.Scheduler,
		Steps:             req.Steps,
		CfgScale:          req.CfgScale,
		Width:             targetW,
		Height:            targetH,
		Seed:              req.Seed,
		DenoisingStrength: &denoise,
		ClipSkip:          req.ClipSkip,
		BatchSize:         req.BatchSize,
		BatchCount:        req.BatchCount,
	}
	return s.sd.Img2Img(i2iReq)
}

func (s *Service) manualHiresUpscale(base64Img string, req sd.Txt2ImgRequest, scale float64, denoiseStrength float64, upscaler string) (*sd.Txt2ImgResponse, error) {
	targetW := int(float64(req.Width) * scale)
	targetH := int(float64(req.Height) * scale)

	upscaledB64, err := s.upscaleWithFallback(base64Img, scale, targetW, targetH, upscaler)
	if err != nil {
		return nil, err
	}

	return s.img2imgRedenoise(upscaledB64, req, targetW, targetH, denoiseStrength)
}

func (s *Service) resolveQualityUpscaler() string {
	upscalers, err := s.sd.GetUpscalers()
	if err != nil {
		s.log.Warn("quality upscaler: list failed, falling back to resize: %s", err)
		return ""
	}
	for _, pref := range qualityUpscalerPreferences {
		for _, u := range upscalers {
			if u.Name == pref {
				return u.Name
			}
		}
	}
	s.log.Warn("quality upscaler: none of %v available, falling back to resize", qualityUpscalerPreferences)
	return ""
}

func (s *Service) qualityPostProcess(base64Img string, req sd.Txt2ImgRequest, srcW, srcH int, resolutionID *int64) (string, json.RawMessage, bool, bool, error) {
	targetW, targetH := s.fitImageToResolution(srcW, srcH, resolutionID)
	return s.qualityUpscaleRedenoise(base64Img, req, srcW, srcH, targetW, targetH, "from-image quality")
}

func (s *Service) qualityUpscaleRedenoise(base64Img string, req sd.Txt2ImgRequest, srcW, srcH, targetW, targetH int, logPrefix string) (string, json.RawMessage, bool, bool, error) {
	scale := 1.0
	if srcW > 0 {
		scale = float64(targetW) / float64(srcW)
	}
	upscaler := s.resolveQualityUpscaler()
	s.log.Info("%s: upscale %.2fx to %dx%d (%s)", logPrefix, scale, targetW, targetH, upscaler)
	upscaledB64, err := s.upscaleWithFallback(base64Img, scale, targetW, targetH, upscaler)
	if err != nil {
		s.log.Warn("%s: upscale failed, using base image: %s", logPrefix, err)
		return base64Img, nil, true, true, nil
	}
	hrResult, err := s.img2imgRedenoise(upscaledB64, req, targetW, targetH, qualityRedenoise)
	if err != nil {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return "", nil, false, false, ierr
		}
		s.log.Warn("%s: redenoise failed, using upscaled image: %s", logPrefix, err)
		return upscaledB64, nil, false, true, nil
	}
	if len(hrResult.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return "", nil, false, false, ierr
		}
		s.log.Warn("%s: redenoise returned no image, using upscaled image", logPrefix)
		return upscaledB64, nil, false, true, nil
	}
	return hrResult.Images[0], hrResult.Info, false, false, nil
}

func buildQualityChain(base preset.StepMeta, upscaleSkipped, redenoiseSkipped bool) []preset.StepMeta {
	chain := []preset.StepMeta{base}
	if !upscaleSkipped {
		chain = append(chain, preset.StepMeta{Op: "upscale"})
	}
	if !redenoiseSkipped {
		rd := qualityRedenoise
		chain = append(chain, preset.StepMeta{Op: "redenoise", Denoise: &rd})
	}
	for i := range chain {
		chain[i].Step = i + 1
	}
	return chain
}

// --- Helpers ---

func (s *Service) getMaxTokens() int {
	maxTokens := 512
	if v, err := s.db.GetSetting("llm_max_tokens"); err == nil && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTokens = n
		}
	}
	return maxTokens
}

func (s *Service) getGenerateModel() string {
	generateModel := s.cfg.SDPromptModel
	if v, err := s.db.GetSetting("llm_generate_model"); err == nil && v != "" {
		generateModel = v
	}
	if generateModel == "" || generateModel == "default" {
		if models, err := s.llm.GetModels(); err == nil && len(models) > 0 {
			return models[0].ID
		}
	}
	return generateModel
}

func (s *Service) getSDPromptInstruction() string {
	sdPromptInstruction := config.DefaultSDPromptInstruction
	if v, err := s.db.GetSetting("sd_prompt_instruction"); err == nil && v != "" {
		sdPromptInstruction = v
	}
	return sdPromptInstruction
}

func (s *Service) isProseModel(modelName string) bool {
	return config.IsNewGenModel(modelName)
}

const textInImageRule = `

TEXT IN THE IMAGE: if the user asks for written text in the image (a sign, label, caption, inscription, poster, title), put that text into the prompt exactly as the user wrote it, in its original language and alphabet, inside double quotes, exactly once. Do NOT translate it, do NOT add a translation or transliteration next to it, do NOT offer alternatives in parentheses or with "or". Everything else is translated to English as usual.`

func (s *Service) getSDPromptInstructionFor(modelName string) string {
	if s.isProseModel(modelName) {
		if v, err := s.db.GetSetting("sd_prompt_instruction_prose"); err == nil && v != "" {
			return v
		}
		return config.DefaultSDPromptInstructionProse
	}
	return s.getSDPromptInstruction()
}

func (s *Service) userSceneLabel(modelName string) string {
	if s.isProseModel(modelName) {
		return "USER SCENE (rewrite this as one connected English paragraph): "
	}
	return "USER SCENE (convert this to SD tags with weights): "
}

func (s *Service) promptTruncateLimit(modelName string) int {
	if s.isProseModel(modelName) {
		return 2000
	}
	return 1000
}

func (s *Service) filterInstructionExamples(prompt, instruction, userInput string) string {
	examples := promptutil.ExtractExampleTags(instruction)
	if examples == "" {
		return prompt
	}
	userLower := strings.ToLower(userInput)
	subtract := make([]string, 0)
	for _, tag := range strings.Split(examples, ", ") {
		key := strings.ToLower(strings.TrimSpace(tag))
		if key == "" {
			continue
		}
		if len(key) > 4 && strings.Contains(userLower, key) {
			continue
		}
		subtract = append(subtract, tag)
	}
	return promptutil.RemoveTags(prompt, strings.Join(subtract, ", "))
}

func (s *Service) getAnalyzeModel() string {
	model := s.cfg.VisionModel
	if v, err := s.db.GetSetting("llm_analyze_model"); err == nil && v != "" {
		model = v
	}
	if model == "" || model == "default" {
		model = s.cfg.SDPromptModel
		if v, err := s.db.GetSetting("llm_generate_model"); err == nil && v != "" {
			model = v
		}
	}
	if model == "" || model == "default" {
		if models, err := s.llm.GetModels(); err == nil && len(models) > 0 {
			return models[0].ID
		}
	}
	return model
}

func intPtr(v int) *int { return &v }

func extractEmbeddedNegative(result *GenerateSDPromptResult) {
	idx := strings.Index(result.Prompt, "negative_prompt")
	if idx <= 0 {
		return
	}
	embeddedNeg := result.Prompt[idx:]
	result.Prompt = strings.TrimRight(result.Prompt[:idx], " ,\n\r\t")
	embeddedNeg = strings.TrimPrefix(embeddedNeg, "negative_prompt")
	embeddedNeg = strings.TrimLeft(embeddedNeg, `: "'`)
	embeddedNeg = strings.TrimRight(embeddedNeg, `"}'`)
	embeddedNeg = strings.Trim(embeddedNeg, " ,\n\r\t")
	if embeddedNeg == "" {
		return
	}
	if result.NegativePrompt != "" {
		result.NegativePrompt = embeddedNeg + ", " + result.NegativePrompt
	} else {
		result.NegativePrompt = embeddedNeg
	}
}

func appendLorasToPrompt(prompt, lorasJSON string) string {
	if lorasJSON == "" {
		return prompt
	}
	var loras []preset.LoRAEntry
	if json.Unmarshal([]byte(lorasJSON), &loras) == nil {
		for _, l := range loras {
			if l.Trigger != "" {
				if prompt == "" {
					prompt = l.Trigger
				} else {
					prompt = l.Trigger + ", " + prompt
				}
			}
		}
		for _, l := range loras {
			prompt += fmt.Sprintf(" <lora:%s:%g>", l.Name, l.Weight)
		}
	}
	return prompt
}

type builtPrompts struct {
	Prompt         string
	NegativePrompt string
}

func (s *Service) buildPrompts(presetPrompt, presetNegative, loras, userPrompt, userNegative string) builtPrompts {
	prompt := presetPrompt
	if userPrompt != "" && prompt != "" {
		prompt = userPrompt + ", BREAK, " + prompt
	} else if userPrompt != "" {
		prompt = userPrompt
	}
	prompt = appendLorasToPrompt(prompt, loras)

	neg := presetNegative
	if userNegative != "" && neg != "" {
		neg = neg + ", " + userNegative
	} else if userNegative != "" {
		neg = userNegative
	}
	neg = s.kids.ApplyNegative(neg)

	return builtPrompts{Prompt: prompt, NegativePrompt: neg}
}

// --- Resolution / Hires resolve ---

func (s *Service) resolveResolution(p *preset.Preset, resolutionID *int64) (width int, height int) {
	if resolutionID == nil || *resolutionID <= 0 {
		return 512, 512
	}
	r, err := s.db.GetResolution(*resolutionID)
	if err != nil {
		s.log.Warn("resolveResolution: failed to load resolution %d: %s", *resolutionID, err)
		return 512, 512
	}
	return r.Width, r.Height
}

func (s *Service) fitImageScale(imgW, imgH int, resolutionID *int64) (float64, bool) {
	if resolutionID == nil || *resolutionID <= 0 || imgW <= 0 || imgH <= 0 {
		return 1, false
	}
	r, err := s.db.GetResolution(*resolutionID)
	if err != nil || r.Width <= 0 || r.Height <= 0 {
		s.log.Warn("fitImageScale: failed to load resolution %d: %s", *resolutionID, err)
		return 1, false
	}
	scale := float64(r.Width) / float64(imgW)
	if h := float64(r.Height) / float64(imgH); h < scale {
		scale = h
	}
	return scale, true
}

func scaleImageSize(imgW, imgH int, scale float64) (int, int, bool) {
	w := int(float64(imgW)*scale) / 8 * 8
	h := int(float64(imgH)*scale) / 8 * 8
	if w < 8 || h < 8 {
		return imgW, imgH, false
	}
	return w, h, true
}

func (s *Service) fitImageToResolution(imgW, imgH int, resolutionID *int64) (int, int) {
	scale, ok := s.fitImageScale(imgW, imgH, resolutionID)
	if !ok {
		return imgW, imgH
	}
	w, h, ok := scaleImageSize(imgW, imgH, scale)
	if !ok {
		return imgW, imgH
	}
	return w, h
}

func (s *Service) fitImageToResolutionDownOnly(imgW, imgH int, resolutionID *int64) (int, int) {
	scale, ok := s.fitImageScale(imgW, imgH, resolutionID)
	if !ok {
		return imgW, imgH
	}
	if scale > 1 {
		scale = 1
	}
	w, h, ok := scaleImageSize(imgW, imgH, scale)
	if !ok {
		return imgW, imgH
	}
	return w, h
}

func (s *Service) resolveHires(hiresProfileID *int64) (enabled bool, upscale *float64, denoising *float64, upscaler string) {
	if hiresProfileID == nil || *hiresProfileID <= 0 {
		return false, nil, nil, ""
	}
	h, err := s.db.GetHiresProfile(*hiresProfileID)
	if err != nil {
		s.log.Warn("resolveHires: failed to load hires profile %d: %s", *hiresProfileID, err)
		return false, nil, nil, ""
	}
	upscaler = h.Upscaler
	if upscaler == "" {
		upscaler = "RealESRGAN_x4plus"
	}
	return true, &h.Upscale, &h.DenoisingStrength, upscaler
}

func modelSpec(modelName string) sd.ModelSpec {
	return sd.ModelSpec{Checkpoint: modelName, Modules: modulesForModel(modelName)}
}

func presetSpec(p *preset.Preset) sd.ModelSpec {
	spec := modelSpec(p.ModelName)
	if p.VAE != "" && p.VAE != "Automatic" && p.VAE != "None" {
		spec.VAE = p.VAE
	}
	return spec
}

func (s *Service) prepareSDContext(p *preset.Preset, logPrefix string) sd.ModelSpec {
	s.guard.EnsureHeadroom(p.ModelName, logPrefix)
	return presetSpec(p)
}

type hiresFallbackResult struct {
	Result       *sd.Txt2ImgResponse
	Err          error
	HiresSkipped bool
	HiresManual  bool
}

func (s *Service) doHiresFallback(req sd.Txt2ImgRequest, originalErr error, hiresUpscale *float64, hiresDenoising *float64, hiresUpscaler string, logPrefix string) hiresFallbackResult {
	if req.HiresFix == nil || !s.hiresRetryable(originalErr) {
		return hiresFallbackResult{Err: originalErr}
	}
	s.log.Warn("%s: SD error with hires fix enabled, retrying with manual upscale: %s", logPrefix, originalErr)
	req.HiresFix = nil
	req.HiresUpscale = nil
	req.HiresDenoisingStrength = nil
	req.HiresUpscaler = ""
	req.HiresResizeX = 0
	req.HiresResizeY = 0
	req.HiresSecondPassSteps = 0
	retryResult, retryErr := s.sd.Txt2Img(req)
	if retryErr != nil {
		return hiresFallbackResult{Err: retryErr, HiresSkipped: true}
	}
	if len(retryResult.Images) == 0 {
		return hiresFallbackResult{Result: retryResult, HiresSkipped: true}
	}
	scale := 2.0
	if hiresUpscale != nil {
		scale = *hiresUpscale
	}
	ds := 0.5
	if hiresDenoising != nil {
		ds = *hiresDenoising
	}
	s.log.Info("%s: manual hires upscale: %.1fx, denoise=%.2f, upscaler=%s", logPrefix, scale, ds, hiresUpscaler)
	hrResult, hrErr := s.manualHiresUpscale(retryResult.Images[0], req, scale, ds, hiresUpscaler)
	if hrErr != nil {
		s.log.Warn("%s: manual hires upscale failed, using base image: %s", logPrefix, hrErr)
		return hiresFallbackResult{Result: retryResult, HiresSkipped: true}
	}
	if len(hrResult.Images) > 0 {
		return hiresFallbackResult{Result: hrResult, HiresManual: true}
	}
	return hiresFallbackResult{Result: retryResult, HiresSkipped: true}
}

var hiresNoRetryCodes = map[string]bool{
	"invalid_params":  true,
	"model_not_found": true,
	"queue_full":      true,
	"cancelled":       true,
}

func (s *Service) hiresRetryable(err error) bool {
	if err == nil {
		return false
	}
	if s.ctx != nil && s.ctx.Err() != nil {
		return false
	}
	if s.checkSDInterrupted() != nil {
		return false
	}
	if errors.Is(err, sd.ErrUserCancel) || errors.Is(err, sd.ErrJobCancelled) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var je *sd.JobError
	if errors.As(err, &je) && hiresNoRetryCodes[je.Code] {
		return false
	}
	var ae *sd.APIError
	if errors.As(err, &ae) {
		if hiresNoRetryCodes[ae.Code] {
			return false
		}
		if ae.Status >= 400 && ae.Status < 500 {
			return false
		}
	}
	return true
}

type txt2imgRun struct {
	req            sd.Txt2ImgRequest
	hiresUpscale   *float64
	hiresDenoising *float64
	hiresUpscaler  string
	logPrefix      string
}

type txt2imgOutcome struct {
	result       *sd.Txt2ImgResponse
	hiresSkipped bool
	hiresManual  bool
}

func presetClipSkip(p *preset.Preset) *int {
	clipSkip := 1
	if p.ClipSkip != nil {
		clipSkip = *p.ClipSkip
	}
	return &clipSkip
}

func (s *Service) presetTxt2Img(p *preset.Preset, spec sd.ModelSpec, prompt, negativePrompt string, width, height int, seed *int64, hiresProfileID *int64, presetBatch bool) txt2imgRun {
	batchSize, batchCount := 1, 1
	if presetBatch {
		if p.BatchSize != nil {
			batchSize = *p.BatchSize
		}
		if p.BatchCount != nil {
			batchCount = *p.BatchCount
		}
	}
	hiresEnabled, hiresUpscale, hiresDenoising, hiresUpscaler := s.resolveHires(hiresProfileID)
	var hiresFix *bool
	denoisingStrength := p.DenoisingStrength
	if hiresEnabled {
		hiresFix = &hiresEnabled
		if denoisingStrength == nil {
			ds := 0.5
			if hiresDenoising != nil {
				ds = *hiresDenoising
			}
			denoisingStrength = &ds
		}
	}
	return txt2imgRun{
		req: sd.Txt2ImgRequest{
			Model:                  spec,
			Prompt:                 prompt,
			CacheMode:              s.cacheModeFor(spec.Checkpoint),
			NegativePrompt:         negativePrompt,
			SamplerName:            p.Sampler,
			Scheduler:              p.ScheduleType,
			Steps:                  p.Steps,
			CfgScale:               p.CfgScale,
			Width:                  width,
			Height:                 height,
			Seed:                   seed,
			DenoisingStrength:      denoisingStrength,
			ClipSkip:               presetClipSkip(p),
			BatchSize:              &batchSize,
			BatchCount:             &batchCount,
			HiresFix:               hiresFix,
			HiresUpscale:           hiresUpscale,
			HiresDenoisingStrength: hiresDenoising,
			HiresUpscaler:          hiresUpscaler,
		},
		hiresUpscale:   hiresUpscale,
		hiresDenoising: hiresDenoising,
		hiresUpscaler:  hiresUpscaler,
	}
}

func (s *Service) txt2imgWithFallback(run txt2imgRun) (*txt2imgOutcome, error) {
	result, err := s.sd.Txt2Img(run.req)
	if err == nil {
		return &txt2imgOutcome{result: result}, nil
	}
	if ierr := s.checkSDInterrupted(); ierr != nil {
		return nil, ierr
	}
	fb := s.doHiresFallback(run.req, err, run.hiresUpscale, run.hiresDenoising, run.hiresUpscaler, run.logPrefix)
	if fb.Err != nil {
		return nil, fb.Err
	}
	return &txt2imgOutcome{result: fb.Result, hiresSkipped: fb.HiresSkipped, hiresManual: fb.HiresManual}, nil
}

func (s *Service) runPresetTxt2Img(run txt2imgRun) (*txt2imgOutcome, error) {
	out, err := s.txt2imgWithFallback(run)
	if err != nil {
		return nil, err
	}
	if len(out.result.Images) > 0 {
		return out, nil
	}
	if ierr := s.checkSDInterrupted(); ierr != nil {
		return nil, ierr
	}
	return nil, fmt.Errorf("no image generated: %s (sampler=%s, scheduler=%s, model=%s)",
		emptyReason(out.result.Info), run.req.SamplerName, run.req.Scheduler, run.req.Model.Checkpoint)
}

func emptyReason(info json.RawMessage) string {
	if len(info) > 0 {
		var v struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(info, &v) == nil && v.Reason != "" {
			return v.Reason
		}
	}
	return "empty response"
}

// --- GenerateSDPrompt ---

type llmReplayer interface {
	ReplaySDPrompt(systemPrompt, userMessage, presetType, model string, maxTokens int, text string) string
}

func (s *Service) replayIdea(key, systemPrompt, userMessage, presetType, model string, maxTokens int) (GenerateSDPromptResult, string, bool) {
	raw, ok := s.ideas.get(key)
	if !ok {
		return GenerateSDPromptResult{}, "", false
	}
	if r, isReplayer := s.llm.(llmReplayer); isReplayer {
		raw = r.ReplaySDPrompt(systemPrompt, userMessage, presetType, model, maxTokens, raw)
	}
	var res GenerateSDPromptResult
	if err := json.Unmarshal([]byte(promptutil.ExtractJSON(raw)), &res); err != nil {
		return GenerateSDPromptResult{}, raw, true
	}
	return res, raw, true
}

func (s *Service) llmGenerateAndParse(systemPrompt, userMessage, presetType, model string, maxTokens int) (GenerateSDPromptResult, string, error) {
	var raw string
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		raw, err = s.llm.GenerateSDPrompt(systemPrompt, userMessage, presetType, model, maxTokens)
		if err != nil {
			return GenerateSDPromptResult{}, "", err
		}
		var res GenerateSDPromptResult
		if err := json.Unmarshal([]byte(promptutil.ExtractJSON(raw)), &res); err == nil {
			return res, raw, nil
		}
	}
	if strings.TrimSpace(raw) == "" {
		return GenerateSDPromptResult{}, raw, fmt.Errorf("LLM returned an empty response — check the model, context length and max_tokens settings")
	}
	return GenerateSDPromptResult{}, raw, nil
}

func (s *Service) GenerateSDPrompt(params GenerateSDPromptParams) (*GenerateSDPromptResult, error) {
	return s.generateSDPrompt(params, false)
}

func (s *Service) generateSDPrompt(params GenerateSDPromptParams, reuse bool) (*GenerateSDPromptResult, error) {
	if params.PresetID <= 0 {
		return nil, fmt.Errorf("preset is required")
	}

	p, err := s.db.Get(params.PresetID)
	if err != nil {
		return nil, fmt.Errorf("preset not found: %w", err)
	}
	ApplyPresetOverrides(p, params.Overrides)

	description := strings.TrimSpace(params.Description)
	negative := strings.TrimSpace(params.Negative)

	if description == "" && negative == "" {
		return &GenerateSDPromptResult{
			Prompt:         p.Prompt,
			NegativePrompt: p.NegativePrompt,
		}, nil
	}

	instruction := s.getSDPromptInstructionFor(p.ModelName)
	systemPrompt := instruction + textInImageRule

	var filterErr error
	description, filterErr = s.kids.FilterInput(description)
	if filterErr != nil {
		return nil, filterErr
	}
	negative, filterErr = s.kids.FilterInput(negative)
	if filterErr != nil {
		return nil, filterErr
	}
	systemPrompt = s.kids.ApplySystemPrompt(systemPrompt)

	maxTokens := s.getMaxTokens()
	generateModel := s.getGenerateModel()

	systemPrompt += fmt.Sprintf(`

RESPONSE LENGTH: you have up to %d tokens. Include ALL visual elements from the user scene.`, maxTokens)

	var userParts []string
	userParts = append(userParts, "STYLE REFERENCE (do NOT include in output): "+p.Prompt)
	userParts = append(userParts, "STYLE NEGATIVE REFERENCE (do NOT include in output): "+p.NegativePrompt)
	if description != "" {
		userParts = append(userParts, s.userSceneLabel(p.ModelName)+description)
	}
	if negative != "" {
		userParts = append(userParts, "USER NEGATIVE: "+negative)
	}
	userMessage := strings.Join(userParts, "\n\n")

	cacheKey := ideaCacheKey(systemPrompt, userMessage, p.PresetType, generateModel, maxTokens)
	result, raw, hit := GenerateSDPromptResult{}, "", false
	if reuse {
		result, raw, hit = s.replayIdea(cacheKey, systemPrompt, userMessage, p.PresetType, generateModel, maxTokens)
	}
	if !hit {
		s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
		var err error
		result, raw, err = s.llmGenerateAndParse(systemPrompt, userMessage, p.PresetType, generateModel, maxTokens)
		s.emitter.Emit("llm:status", map[string]string{"status": "done"})
		if err != nil {
			return nil, err
		}
	}

	if result.Prompt == "" && result.NegativePrompt == "" {
		result = GenerateSDPromptResult{
			Prompt:         promptutil.TruncateRepetitive(raw, s.promptTruncateLimit(p.ModelName)),
			NegativePrompt: p.NegativePrompt,
		}
	}

	if promptutil.ContainsCyrillicOutsideQuotes(result.Prompt) {
		if extracted := promptutil.ExtractTagsFromRaw(raw); extracted != "" {
			result.Prompt = extracted
		}
	}
	if promptutil.ContainsCyrillic(result.NegativePrompt) {
		result.NegativePrompt = promptutil.ExtractNegativeFromRaw(raw)
	}

	extractEmbeddedNegative(&result)

	result.Prompt = promptutil.StripJunk(result.Prompt)
	result.Prompt = promptutil.TruncateRepetitive(result.Prompt, s.promptTruncateLimit(p.ModelName))
	result.Prompt = s.filterInstructionExamples(result.Prompt, instruction, description+" "+negative)
	result.NegativePrompt = promptutil.StripJunk(result.NegativePrompt)
	result.NegativePrompt = promptutil.TruncateRepetitive(result.NegativePrompt, 500)
	result.NegativePrompt = s.filterInstructionExamples(result.NegativePrompt, instruction, description+" "+negative)

	result.Prompt = s.kids.FilterOutput(result.Prompt)
	result.NegativePrompt = s.kids.FilterOutput(result.NegativePrompt)

	if reuse && !hit && strings.TrimSpace(result.Prompt) != "" && json.Valid([]byte(promptutil.ExtractJSON(raw))) {
		s.ideas.put(cacheKey, raw)
	}
	return &result, nil
}

func (s *Service) GetDefaultPromptInstruction() string {
	return config.DefaultSDPromptInstruction
}

func (s *Service) GetDefaultPromptInstructionProse() string {
	return config.DefaultSDPromptInstructionProse
}

// --- GenerateImage ---

func (s *Service) GenerateImage(params GenerateImageParams) (*GenerateImageResult, error) {
	if params.PresetID <= 0 {
		return nil, fmt.Errorf("preset is required")
	}
	s.log.UserAction("Generate image (preset_id=%d, override_model=%q)", params.PresetID, OverrideModelName(params.Overrides))
	if err := ValidateIdea(params.Idea, params.IdeaNegative); err != nil {
		return nil, err
	}
	if err := s.sd.HealthCheck(); err != nil {
		return nil, fmt.Errorf("SD is not available: %w", err)
	}
	s.beginProgress()
	defer s.endProgress()
	ideaResolved := false
	if strings.TrimSpace(params.Idea) != "" {
		prompt, negative, err := s.resolveIdea(params.PresetID, params.Overrides, params.Idea, params.IdeaNegative)
		if err != nil {
			return nil, err
		}
		if err := s.checkSDInterrupted(); err != nil {
			return nil, err
		}
		params.ExtraPrompt, params.ExtraNegativePrompt = prompt, negative
		ideaResolved = true
	}
	p, err := s.db.Get(params.PresetID)
	if err != nil {
		s.log.Error("Generate image: preset not found: %s", err)
		return nil, fmt.Errorf("preset not found: %w", err)
	}
	ApplyPresetOverrides(p, params.Overrides)

	bp := s.buildPrompts(p.Prompt, p.NegativePrompt, p.Loras, params.ExtraPrompt, params.ExtraNegativePrompt)
	prompt := bp.Prompt
	negativePrompt := bp.NegativePrompt

	seed := p.Seed
	if params.Seed != nil {
		seed = params.Seed
	}
	width, height := s.resolveResolution(p, params.ResolutionID)
	run := s.presetTxt2Img(p, s.prepareSDContext(p, "generate"), prompt, negativePrompt, width, height, seed, params.HiresProfileID, true)
	run.logPrefix = "generate"
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
	if ideaResolved {
		img.IdeaPrompt, img.IdeaNegativePrompt = params.ExtraPrompt, params.ExtraNegativePrompt
	}
	img.Image, img.FaceSwap = s.faceSwapOverride(params.Overrides, img.Image)
	if err := s.checkSDInterrupted(); err != nil {
		return nil, err
	}
	img.SessionItemID = s.sessions.AddToSession(img.Image, img.Info, "generate", &preset.GenerationMeta{
		PresetID: &params.PresetID,
		Label:    p.Name,
		Model:    p.ModelName,
	})
	return img, nil
}

// --- TestGenerate ---

func (s *Service) TestGenerate(params TestGenerateParams) ([]TestGenerateResultItem, error) {
	if err := s.sd.HealthCheck(); err != nil {
		return nil, fmt.Errorf("SD is not available: %w", err)
	}
	s.beginProgress()
	defer s.endProgress()
	if params.Mode != "presets" && params.Mode != "models" {
		return nil, fmt.Errorf("mode must be 'presets' or 'models'")
	}
	totalItems := len(params.SelectedIDs)
	if params.Mode == "models" {
		totalItems = len(params.SelectedModels)
	}
	if totalItems == 0 {
		return nil, fmt.Errorf("select at least one item")
	}
	if totalItems > 50 {
		return nil, fmt.Errorf("maximum 50 items at once")
	}
	if params.Prompt == "" && len(params.SelectedIDs) == 0 && len(params.SelectedModels) == 0 {
		return nil, fmt.Errorf("prompt is required when no preset is selected")
	}
	if params.Width > 2048 || params.Height > 2048 {
		return nil, fmt.Errorf("maximum dimension is 2048")
	}
	if params.Steps > 150 {
		return nil, fmt.Errorf("maximum steps is 150")
	}
	filteredPrompt, err := s.kids.FilterInput(params.Prompt)
	if err != nil {
		return nil, fmt.Errorf("generating image: %w", err)
	}

	results := make([]TestGenerateResultItem, 0, totalItems)
	for idx := 0; idx < totalItems; idx++ {
		s.emitter.Emit("test:progress", map[string]any{
			"current": idx + 1,
			"total":   totalItems,
			"status":  "generating",
		})
		item, err := s.testOne(params, filteredPrompt, idx)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
		if item.Error != "" {
			continue
		}
		s.emitter.Emit("test:progress", map[string]any{
			"current": idx + 1,
			"total":   totalItems,
			"status":  "done",
		})
	}
	return results, nil
}

func (s *Service) testOne(params TestGenerateParams, filteredPrompt string, idx int) (TestGenerateResultItem, error) {
	item := TestGenerateResultItem{}
	p := &preset.Preset{}
	var spec sd.ModelSpec
	if params.Mode == "presets" {
		id := params.SelectedIDs[idx]
		loaded, err := s.db.Get(id)
		if err != nil {
			item.Error = fmt.Sprintf("preset not found: %v", err)
			item.Name = fmt.Sprintf("Preset #%d", id)
			return item, nil
		}
		p = loaded
		item.Name = p.Name
		spec = s.prepareSDContext(p, fmt.Sprintf("test preset #%d", idx+1))
	} else {
		modelTitle := params.SelectedModels[idx]
		s.guard.EnsureHeadroom(modelTitle, fmt.Sprintf("test model #%d", idx+1))
		item.Name = modelTitle
		spec = modelSpec(modelTitle)
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

	var prompt, negPrompt string
	if params.Mode == "presets" && filteredPrompt != "" {
		promptResult, llmErr := s.generateLLMPromptFromTags(p, filteredPrompt, "", params.NegativePrompt)
		if llmErr != nil {
			s.log.Warn("LLM prompt generation failed, using raw prompt: %s", llmErr)
			bp := s.buildPrompts(p.Prompt, p.NegativePrompt, p.Loras, filteredPrompt, params.NegativePrompt)
			prompt, negPrompt = bp.Prompt, bp.NegativePrompt
		} else {
			prompt = appendLorasToPrompt(promptResult.Prompt, p.Loras)
			negPrompt = s.buildPrompts("", promptResult.NegativePrompt, "", "", params.NegativePrompt).NegativePrompt
		}
	} else {
		bp := s.buildPrompts(p.Prompt, p.NegativePrompt, p.Loras, filteredPrompt, params.NegativePrompt)
		prompt, negPrompt = bp.Prompt, bp.NegativePrompt
	}

	width, height := s.resolveResolution(p, params.ResolutionID)
	run := s.presetTxt2Img(p, spec, prompt, negPrompt, width, height, p.Seed, params.HiresProfileID, false)
	run.req.DenoisingStrength = nil
	run.logPrefix = fmt.Sprintf("test preset #%d", idx+1)
	if params.Sampler != "" {
		run.req.SamplerName = params.Sampler
	}
	if params.ScheduleType != "" {
		run.req.Scheduler = params.ScheduleType
	}
	if params.Steps > 0 {
		run.req.Steps = params.Steps
	}
	if params.CfgScale > 0 {
		run.req.CfgScale = params.CfgScale
	}
	if params.Width > 0 {
		run.req.Width = params.Width
	}
	if params.Height > 0 {
		run.req.Height = params.Height
	}
	if params.Seed != nil {
		run.req.Seed = params.Seed
	}
	item.Sampler = run.req.SamplerName
	item.ScheduleType = run.req.Scheduler
	item.CfgScale = run.req.CfgScale

	out, err := s.txt2imgWithFallback(run)
	if err != nil {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return item, ierr
		}
		item.Error = err.Error()
		item.ModelName = p.ModelName
		return item, nil
	}
	result := out.result
	if len(result.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return item, ierr
		}
		item.Error = "no image returned"
		return item, nil
	}

	if len(result.Info) > 0 {
		var info struct {
			Seed    int64  `json:"seed"`
			SDModel string `json:"sd_model_name"`
		}
		if json.Unmarshal(result.Info, &info) == nil {
			item.Seed = info.Seed
			item.ModelName = info.SDModel
		}
	}
	item.Image = result.Images[0]
	item.Info = result.Info
	if item.ModelName == "" {
		item.ModelName = p.ModelName
	}

	compareMeta := &preset.GenerationMeta{}
	if params.Mode == "presets" {
		compareMeta.Label = p.Name
		compareMeta.Model = p.ModelName
	} else {
		compareMeta.Model = params.SelectedModels[idx]
	}
	s.sessions.AddToSession(item.Image, result.Info, "compare", compareMeta)
	return item, nil
}

// --- UpscaleImage ---

func (s *Service) UpscaleImage(params UpscaleImageParams) (*GenerateImageResult, error) {
	s.beginProgress()
	defer s.endProgress()
	if params.ImageBase64 == "" {
		return nil, fmt.Errorf("image is required")
	}
	if len(params.ImageBase64) > 67*1024*1024 {
		return nil, fmt.Errorf("image too large (max 50 MB)")
	}

	var info struct {
		Prompt         string  `json:"prompt"`
		NegativePrompt string  `json:"negative_prompt"`
		SamplerName    string  `json:"sampler_name"`
		Scheduler      string  `json:"scheduler"`
		Seed           int64   `json:"seed"`
		Width          int     `json:"width"`
		Height         int     `json:"height"`
		Steps          int     `json:"steps"`
		CfgScale       float64 `json:"cfg_scale"`
		ClipSkip       int     `json:"clip_skip"`
		Sampler        string  `json:"sampler"`
		Checkpoint     string  `json:"checkpoint"`
		SDModelName    string  `json:"sd_model_name"`
	}
	if err := json.Unmarshal([]byte(params.GenInfo), &info); err != nil {
		return nil, fmt.Errorf("parse gen_info: %w", err)
	}

	if info.Width <= 0 || info.Height <= 0 {
		return nil, fmt.Errorf("invalid dimensions in gen_info: %dx%d", info.Width, info.Height)
	}

	const maxDim = 2048
	if info.Width > maxDim || info.Height > maxDim {
		return nil, fmt.Errorf("image is already %dx%d (max %d for upscale)", info.Width, info.Height, maxDim)
	}

	prompt := info.Prompt
	negativePrompt := info.NegativePrompt

	negativePrompt = s.kids.ApplyNegative(negativePrompt)

	infoSampler := info.Sampler
	if infoSampler == "" {
		infoSampler = info.SamplerName
	}
	samplerName, scheduler, _ := sdnames.Pair(infoSampler, info.Scheduler)
	modelName := info.Checkpoint
	if modelName == "" {
		modelName = info.SDModelName
	}
	spec := modelSpec(modelName)
	steps := 30
	if info.Steps > 0 {
		steps = info.Steps
	}
	cfgScale := 7.0
	if info.CfgScale > 0 {
		cfgScale = info.CfgScale
	}
	clipSkip := 1
	if info.ClipSkip > 0 {
		clipSkip = info.ClipSkip
	}

	if params.PresetID > 0 {
		p, err := s.db.Get(params.PresetID)
		if err != nil {
			return nil, fmt.Errorf("preset not found: %w", err)
		}
		if p.Prompt != "" {
			prompt = p.Prompt
		}
		if p.NegativePrompt != "" {
			negativePrompt = p.NegativePrompt
		}
		if p.Sampler != "" {
			samplerName = p.Sampler
		}
		if p.ScheduleType != "" {
			scheduler = p.ScheduleType
		}
		if p.Steps > 0 {
			steps = p.Steps
		}
		if p.CfgScale > 0 {
			cfgScale = p.CfgScale
		}
		if p.ClipSkip != nil {
			clipSkip = *p.ClipSkip
		}
		spec = s.prepareSDContext(p, "upscale")
	}

	denoisingStrength := 0.4
	newWidth := info.Width * 2
	newHeight := info.Height * 2
	if newWidth > maxDim*2 {
		newWidth = maxDim * 2
	}
	if newHeight > maxDim*2 {
		newHeight = maxDim * 2
	}
	seed := info.Seed

	result, err := s.sd.Img2Img(sd.Img2ImgRequest{
		Model:             spec,
		InitImages:        []string{params.ImageBase64},
		Prompt:            prompt,
		NegativePrompt:    negativePrompt,
		SamplerName:       samplerName,
		Scheduler:         scheduler,
		Steps:             steps,
		CfgScale:          cfgScale,
		Width:             newWidth,
		Height:            newHeight,
		Seed:              &seed,
		DenoisingStrength: &denoisingStrength,
		ClipSkip:          &clipSkip,
		BatchSize:         intPtr(1),
		BatchCount:        intPtr(1),
	})
	if err != nil {
		return nil, err
	}

	if len(result.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, ierr
		}
		return nil, fmt.Errorf("no image generated during upscale")
	}

	img := &GenerateImageResult{
		Image:      result.Images[0],
		Parameters: result.Parameters,
		Info:       result.Info,
	}
	upscaleMeta := &preset.GenerationMeta{}
	if params.PresetID > 0 {
		upscaleMeta.PresetID = &params.PresetID
	}
	img.SessionItemID = s.sessions.AddToSession(result.Images[0], result.Info, "upscale", upscaleMeta)
	return img, nil
}
