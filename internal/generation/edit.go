package generation

import (
	"errors"
	"fmt"
	"strings"

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

	editLoRATag      = "<lora:Moss_Edit_LoRA:1.0>"
	editPromptSuffix = ". Keep the person, face, pose, background and composition unchanged."

	mossTEModulePath  = "/mnt/e/SDStudio/stable-diffusion-webui-forge/models/text_encoder/qwen35_9b_int8_convrot.safetensors"
	mossVAEModulePath = "/mnt/e/SDStudio/stable-diffusion-webui-forge/models/VAE/flux2-vae_small.safetensors"
)

var mossForgeModules = []string{mossTEModulePath, mossVAEModulePath}

func forgeModulesForModel(model string) []string {
	if config.ModelMatchesCSV(model, "moss") {
		return mossForgeModules
	}
	return []string{}
}

type EditImageParams struct {
	ImageBase64 string `json:"image_base64"`
	Instruction string `json:"instruction"`
	Seed        *int64 `json:"seed,omitempty"`
}

func buildEditPrompt(instruction string) string {
	return editLoRATag + "Change only " + instruction + editPromptSuffix
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
		if err := s.sd.SetOption(mossReferenceOption, mossReferenceOff); err != nil {
			s.log.Warn("edit: disable moss reference: %s", err)
			if result != nil {
				result.EditReferenceResetFailed = true
			}
		}
	}()

	srcW, srcH := filebrowser.DecodeImageSize(params.ImageBase64)
	if srcW <= 0 || srcH <= 0 {
		return nil, errors.New("decode image size: invalid image")
	}
	width, height := alignEditSize(srcW, srcH)

	prompt := buildEditPrompt(instruction)
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

	result = &GenerateImageResult{
		Image:           resp.Images[0],
		Parameters:      resp.Parameters,
		Info:            resp.Info,
		IsPreview:       false,
		EffectivePrompt: prompt,
	}
	s.sessions.AddToSession(resp.Images[0], resp.Info, "edit", false, &preset.GenerationMeta{
		Label: "Edit: " + truncateRunes(instruction, mossEditLabelMax),
		Model: mossEditModel,
	})
	return result, nil
}

func (s *Service) ResetMossReference() {
	if err := s.sd.HealthCheck(); err != nil {
		return
	}
	if err := s.sd.SetOption(mossReferenceOption, mossReferenceOff); err != nil {
		s.log.Warn("reset moss reference: %s", err)
	}
}
