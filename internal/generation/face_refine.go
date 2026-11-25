package generation

import (
	"errors"
	"fmt"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/sd"
)

type FaceRefineParams struct {
	ImageBase64       string
	Prompt            string
	NegativePrompt    string
	Model             string
	Sampler           string
	Scheduler         string
	Steps             int
	CfgScale          float64
	DenoisingStrength float64
	Seed              *int64
}

func (s *Service) FaceRefine(p FaceRefineParams) (string, error) {
	if p.ImageBase64 == "" {
		return "", errors.New("image is required")
	}
	if len(p.ImageBase64) > MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}
	if err := s.sd.HealthCheck(); err != nil {
		return "", fmt.Errorf("SD is not available: %w", err)
	}
	s.beginProgress()
	defer s.endProgress()
	prompt, err := s.kids.FilterInput(p.Prompt)
	if err != nil {
		return "", err
	}
	w, h := filebrowser.DecodeImageSize(p.ImageBase64)
	if w <= 0 || h <= 0 {
		return "", errors.New("could not determine image size")
	}
	if config.IsFlux2Klein(p.Model) {
		w, h = alignLookSize(w, h)
	} else {
		w, h = alignTo(w, h, 8)
	}
	s.guard.EnsureHeadroom(p.Model, "face refine")
	denoise := p.DenoisingStrength
	resp, err := s.measuredImg2Img(sd.Img2ImgRequest{
		Model:             modelSpec(p.Model),
		InitImages:        []string{p.ImageBase64},
		Prompt:            prompt,
		NegativePrompt:    p.NegativePrompt,
		SamplerName:       p.Sampler,
		Scheduler:         p.Scheduler,
		Steps:             p.Steps,
		CfgScale:          p.CfgScale,
		Width:             w,
		Height:            h,
		Seed:              p.Seed,
		DenoisingStrength: &denoise,
		CacheMode:         s.cacheModeFor(p.Model),
		BatchSize:         intPtr(1),
		BatchCount:        intPtr(1),
	})
	if err != nil {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return "", ierr
		}
		return "", err
	}
	if len(resp.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return "", ierr
		}
		return "", errors.New("no image generated (face refine)")
	}
	return resp.Images[0], nil
}
