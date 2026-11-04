package generation

import (
	"errors"
	"fmt"
	"strings"
)

const maxIdeaRunes = 4000

func ValidateIdea(idea, negative string) error {
	if len([]rune(idea)) > maxIdeaRunes {
		return fmt.Errorf("idea too long (max %d characters)", maxIdeaRunes)
	}
	if len([]rune(negative)) > maxIdeaRunes {
		return fmt.Errorf("idea negative too long (max %d characters)", maxIdeaRunes)
	}
	return nil
}

func (s *Service) resolveIdea(presetID int64, overrides *PresetOverrides, idea, negative string) (string, string, error) {
	res, err := s.GenerateSDPrompt(GenerateSDPromptParams{
		PresetID:    presetID,
		Description: idea,
		Negative:    negative,
		Overrides:   overrides,
	})
	if err != nil {
		return "", "", fmt.Errorf("idea prompt: %w", err)
	}
	if res == nil || strings.TrimSpace(res.Prompt) == "" {
		return "", "", errors.New("idea prompt: LLM returned an empty prompt")
	}
	return res.Prompt, res.NegativePrompt, nil
}
