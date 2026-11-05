package generation

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"strings"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/sd"
)

func (s *Service) kleinReferenceImg2Img(logPrefix string, onStart func(), req sd.Img2ImgRequest) (resp *sd.Txt2ImgResponse, resetFailed bool, err error) {
	s.referenceMu.Lock()
	defer s.referenceMu.Unlock()

	if err := s.sd.HealthCheck(); err != nil {
		return nil, false, fmt.Errorf("SD is not available: %w", err)
	}
	s.StartSDPolling()
	defer s.StopSDPolling()
	if onStart != nil {
		onStart()
	}

	s.guard.EnsureHeadroom(kleinLookModel, logPrefix)
	if err := s.setModel(kleinLookModel, logPrefix); err != nil {
		return nil, false, fmt.Errorf("%s not available on SD backend: %w", kleinLookModel, err)
	}
	if err := s.sd.SetVAE("Automatic"); err != nil {
		s.log.Warn("%s: reset vae: %s", logPrefix, err)
	}
	if err := s.sd.SetOption(kleinReferenceOption, true); err != nil {
		if rerr := s.sd.SetOption(kleinReferenceOption, false); rerr != nil {
			s.log.Warn("%s: disable klein reference after failed enable: %s", logPrefix, rerr)
		}
		return nil, false, fmt.Errorf("enable %s reference: %w", logPrefix, err)
	}
	defer func() {
		if derr := s.sd.SetOption(kleinReferenceOption, false); derr != nil {
			s.log.Warn("%s: disable klein reference: %s", logPrefix, derr)
			resetFailed = true
		}
	}()

	denoise := objectDenoise
	batchSize, batchCount := 1, 1
	req.SamplerName = kleinLookSampler
	req.Scheduler = kleinLookScheduler
	req.Steps = objectSteps
	req.CfgScale = kleinLookCfgScale
	req.DenoisingStrength = &denoise
	req.BatchSize = &batchSize
	req.BatchCount = &batchCount
	req.DoNotSaveImages = true
	req.DoNotSaveGrid = true

	resp, err = s.sd.Img2Img(req)
	if err != nil {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, resetFailed, ierr
		}
		return nil, resetFailed, err
	}
	if resp == nil || len(resp.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, resetFailed, ierr
		}
		return nil, resetFailed, fmt.Errorf("no image generated (%s)", logPrefix)
	}
	return resp, resetFailed, nil
}

func (s *Service) KleinEdit(imageBase64, instruction string, seed *int64) (*GenerateImageResult, error) {
	if imageBase64 == "" {
		return nil, errors.New("image is required")
	}
	if len(imageBase64) > MaxImageBase64Len {
		return nil, config.ImageTooLarge("image")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return nil, errors.New("instruction is required")
	}
	instruction, err := s.kids.FilterInput(instruction)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse image: %w", err)
	}
	width, height := alignLookSize(cfg.Width, cfg.Height)

	resp, resetFailed, err := s.kleinReferenceImg2Img("stylist", nil, sd.Img2ImgRequest{
		InitImages:     []string{imageBase64},
		Prompt:         instruction,
		NegativePrompt: s.kids.ApplyNegative(""),
		Width:          width,
		Height:         height,
		Seed:           seed,
	})
	if err != nil {
		return nil, err
	}
	return &GenerateImageResult{
		Image:                       resp.Images[0],
		Info:                        resp.Info,
		EffectivePrompt:             instruction,
		PreserveIdentityResetFailed: resetFailed,
	}, nil
}

func (s *Service) DescribeImage(imageBase64, prompt string) (string, error) {
	if imageBase64 == "" {
		return "", errors.New("image is required")
	}
	if len(imageBase64) > MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}
	model := s.getAnalyzeModel()
	s.settings.ApplyLLMConfig("analyze")
	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	text, err := s.llm.AnalyzeImageDescribe(model, prompt, imageBase64, s.getMaxTokens())
	s.emitter.Emit("llm:status", map[string]string{"status": "done"})
	if err != nil {
		return "", err
	}
	return s.kids.FilterOutput(strings.TrimSpace(text)), nil
}

func (s *Service) FaceLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	det, ok := s.sd.(sd.FaceDetector)
	if !ok {
		return nil, errors.New("face landmarks are not supported by the SD client")
	}
	pts, err := det.FaceLandmarks(imageBase64)
	if err != nil {
		return nil, err
	}
	out := make([]faceguard.Point, len(pts))
	for i, p := range pts {
		out[i] = faceguard.Point{X: p.X, Y: p.Y}
	}
	return out, nil
}
