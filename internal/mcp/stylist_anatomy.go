package mcp

import (
	"math"

	"github.com/Zazza/imagecaster/internal/generation"
)

const (
	shootAnatomyAttempts = 3
	anatomyBaseDefect    = "base_defect"
	anatomyGarmentDefect = "garment_defect"
	anatomyUncertain     = "uncertain"
	anatomyUnavailable   = "unavailable"
	anatomyBaseSkipped   = "анатомия исходного кадра: примерка пропущена"
)

type StylistAnatomyChecker interface {
	CheckStylistAnatomy(imageBase64, prompt string) (generation.StylistAnatomyResult, error)
}

func shootStopped(eng StylistEngine, stopped func() bool) bool {
	if stopped != nil && stopped() {
		return true
	}
	if s, ok := eng.(interface{ Stopped() bool }); ok {
		return s.Stopped()
	}
	return false
}

func checkShootAnatomy(eng StylistEngine, img, prompt, defect string, stopped func() bool) (string, error) {
	if shootStopped(eng, stopped) {
		return "", generation.ErrInterrupted
	}
	checker, ok := eng.(StylistAnatomyChecker)
	if !ok {
		return "", nil
	}
	result, err := checker.CheckStylistAnatomy(img, prompt)
	if generation.IsInterrupted(err) || shootStopped(eng, stopped) {
		return "", generation.ErrInterrupted
	}
	if err != nil {
		return anatomyUnavailable, nil
	}
	switch result.Verdict {
	case generation.StylistAnatomyOK:
		return "", nil
	case generation.StylistAnatomyDefect:
		return defect, nil
	default:
		return anatomyUncertain, nil
	}
}

func copyShootSeed(seed *int64) *int64 {
	if seed == nil {
		return nil
	}
	v := *seed
	return &v
}

func nextShootSeed(actual int64, previous *int64) *int64 {
	next := actual
	if next < 0 || next == math.MaxInt64 {
		next = 0
	} else {
		next++
	}
	if previous != nil && next == *previous {
		if next == math.MaxInt64 {
			next = 0
		} else {
			next++
		}
	}
	return &next
}
