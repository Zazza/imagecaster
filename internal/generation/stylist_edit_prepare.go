package generation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/config"
)

const FrameEditPreserveSuffix = ". Preserve everything not affected by the requested change, including the person's identity, clothing details, scene, lighting and photographic style."

const (
	stylistEditPrepareMaxRunes     = 2000
	stylistEditPrepareTemperature  = 0.0
	stylistEditPrepareMaxTokens    = 220
	stylistEditPrepareSystemPrompt = "Write one concise English image-edit instruction describing the FINAL visible state after the requested change to this exact photo. Inspect the image. If a body part moves, state its new position AND what replaces its old position; move the existing limb, do not add a new limb. State the position of the other limb only when needed to disambiguate a pose. Resolve unspecified side by choosing one. Use natural full sentences, at most 70 words. Do not describe identity, face, body shape, lighting or background unless requested. No step-by-step motion, no camera directions, no explanations. Return only the edit instruction."
)

func (s *Service) PrepareStylistEdit(imageBase64, instruction string) (string, error) {
	if imageBase64 == "" {
		return "", errors.New("image is required")
	}
	if len(imageBase64) > MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" || len([]rune(instruction)) > stylistEditPrepareMaxRunes {
		return "", fmt.Errorf("edit instruction must contain 1–%d characters", stylistEditPrepareMaxRunes)
	}
	if err := s.visionLocal(); err != nil {
		return "", err
	}
	if s.Stopped() {
		return "", ErrInterrupted
	}
	instruction, err := s.kids.FilterInput(instruction)
	if err != nil {
		return "", err
	}
	s.emitter.Emit("llm:status", map[string]string{"status": "thinking"})
	defer s.emitter.Emit("llm:status", map[string]string{"status": "done"})
	text, err := s.llm.ChatVision(s.getAnalyzeModel(), stylistEditPrepareSystemPrompt, instruction, imageBase64, stylistEditPrepareTemperature, stylistEditPrepareMaxTokens)
	if s.Stopped() {
		return "", ErrInterrupted
	}
	if err != nil {
		return "", fmt.Errorf("prepare edit instruction: %w", err)
	}
	text = strings.TrimSpace(s.kids.FilterOutput(text))
	if text == "" || len([]rune(text)) > stylistEditPrepareMaxRunes {
		return "", errors.New("ИИ вернул пустое или слишком длинное описание изменения")
	}
	return text, nil
}
