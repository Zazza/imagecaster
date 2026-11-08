package queue

import (
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/generation"
)

func validateJobParams(jobType JobType, params any) error {
	switch jobType {
	case JobTxt2Img:
		if p, ok := params.(generation.GenerateImageParams); ok {
			if p.PresetID <= 0 {
				return fmt.Errorf("preset is required")
			}
			if err := generation.ValidateIdea(p.Idea, p.IdeaNegative); err != nil {
				return err
			}
		}
	case JobCompound:
		if p, ok := params.(generation.GenerateCompoundImageParams); ok {
			if err := generation.ValidateIdea(p.Idea, p.IdeaNegative); err != nil {
				return err
			}
			if len(p.Steps) > 0 {
				if p.CompoundPresetID > 0 {
					return fmt.Errorf("steps and compound_preset_id are mutually exclusive")
				}
				if err := generation.ValidateCustomSteps(p.Steps); err != nil {
					return err
				}
			} else if p.CompoundPresetID <= 0 {
				return fmt.Errorf("compound preset is required")
			}
		}
	case JobLook:
		if p, ok := params.(generation.ApplyLookParams); ok {
			if err := generation.ValidateLook(p); err != nil {
				return err
			}
		}
	case JobEdit:
		if p, ok := params.(generation.EditImageParams); ok {
			if p.ImageBase64 == "" {
				return fmt.Errorf("image is required")
			}
			if strings.TrimSpace(p.Instruction) == "" {
				return fmt.Errorf("instruction is required")
			}
		}
	case JobFromImage:
		if p, ok := params.(generation.GenerateFromImageParams); ok {
			if p.OutputMode != "" && p.OutputMode != "standard" && p.OutputMode != "quality" {
				return fmt.Errorf("output_mode must be standard or quality")
			}
			if p.OutputMode == "quality" && (p.Mode != "img2img" || p.GenMode != "preset" || p.ResolutionID == nil || *p.ResolutionID <= 0) {
				return fmt.Errorf("quality output mode requires img2img preset mode with resolution")
			}
			if p.PreserveIdentity && (p.Mode != "img2img" || p.GenMode != "preset") {
				return fmt.Errorf("preserve identity requires img2img preset mode")
			}
			if p.PreserveIdentity && p.DenoisingStrength > 0.9 {
				return fmt.Errorf("preserve identity requires denoising strength of 0.9 or less")
			}
			if p.Mode != "" && p.Mode != "txt2img" && p.Mode != "img2img" {
				return fmt.Errorf("mode must be txt2img or img2img")
			}
			if p.GenMode == "preset" && p.PresetID <= 0 {
				return fmt.Errorf("preset is required")
			}
			if p.GenMode == "compound" && p.CompoundPresetID <= 0 {
				return fmt.Errorf("compound preset is required")
			}
		}
	}
	return nil
}
