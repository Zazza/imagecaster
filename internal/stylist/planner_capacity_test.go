package stylist

import (
	"errors"
	"math"
	"testing"

	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const capacityGiB = 1024 * 1024 * 1024
const capacityCaption = "hf.co/GitMylo/nsfwcaption-qwen3-vl-8b-v3-gguf:Q4_K_M"
const capacitySmall = "qwen3:4b"
const capacityRecommended = "qwen3:8b"
const capacityLarge = "huihui_ai/qwen2.5-abliterate:14b"

func TestPlannerCapacityPreservesConfiguredTextWithoutProbing(t *testing.T) {
	const configured = "custom-text-model"
	got, err := ResolvePlannerModelWithCapacity(configured, func() ([]llm.LLMModel, error) {
		t.Fatal("ordinary configured models must not list models")
		return nil, nil
	}, func() (PlannerCapacity, error) {
		t.Fatal("ordinary configured models must not probe capacity")
		return PlannerCapacity{}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, configured, got)
}

func TestPlannerCapacityOnlySelectsInstalledModelsThatFit(t *testing.T) {
	cases := []struct {
		name              string
		installed         []string
		freeVRAM, freeRAM float64
		loaded            []llm.LoadedModel
		probeErr          error
		want              string
	}{
		{name: "verified installed model wins", installed: []string{capacitySmall, capacityLarge, capacityRecommended}, freeVRAM: 12 * capacityGiB, freeRAM: 4 * capacityGiB, want: capacityLarge},
		{name: "broken small model never substitutes when verified model does not fit", installed: []string{capacityLarge, capacitySmall}, freeVRAM: 5 * capacityGiB, freeRAM: 4 * capacityGiB},
		{name: "eligible recommended substitutes when verified model does not fit", installed: []string{capacityLarge, capacitySmall, capacityRecommended}, freeVRAM: 8 * capacityGiB, freeRAM: 4 * capacityGiB, want: capacityRecommended},
		{name: "broken small model excluded even with ample capacity", installed: []string{capacitySmall}, freeVRAM: 32 * capacityGiB, freeRAM: 4 * capacityGiB},
		{name: "gpu reserve required", installed: []string{capacityRecommended}, freeVRAM: 4 * capacityGiB, freeRAM: 4 * capacityGiB},
		{name: "host reserve required", installed: []string{capacityRecommended}, freeVRAM: 12 * capacityGiB, freeRAM: capacityGiB},
		{name: "gpu unknown", installed: []string{capacityRecommended}, freeRAM: 4 * capacityGiB},
		{name: "ram unknown", installed: []string{capacityRecommended}, freeVRAM: 12 * capacityGiB},
		{name: "gpu nonfinite", installed: []string{capacityRecommended}, freeVRAM: math.NaN(), freeRAM: 4 * capacityGiB},
		{name: "ram nonfinite", installed: []string{capacityRecommended}, freeVRAM: 12 * capacityGiB, freeRAM: math.Inf(1)},
		{name: "probe failed", installed: []string{capacityRecommended}, probeErr: errors.New("memory unavailable")},
		{name: "same loaded candidate credited", installed: []string{capacityLarge}, freeVRAM: 3 * capacityGiB, freeRAM: 4 * capacityGiB, loaded: []llm.LoadedModel{{Name: capacityLarge, SizeVRAM: 8 * capacityGiB}}, want: capacityLarge},
		{name: "other loaded model not credited", installed: []string{capacityLarge}, freeVRAM: 3 * capacityGiB, freeRAM: 4 * capacityGiB, loaded: []llm.LoadedModel{{Name: capacityCaption, SizeVRAM: 8 * capacityGiB}}},
		{name: "host allocation not gpu credit", installed: []string{capacityLarge}, freeVRAM: 3 * capacityGiB, freeRAM: 4 * capacityGiB, loaded: []llm.LoadedModel{{Name: capacityLarge, Size: 8 * capacityGiB}}},
		{name: "uninstalled text not selected", installed: []string{capacityCaption, "unknown-text"}, freeVRAM: 12 * capacityGiB, freeRAM: 4 * capacityGiB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			models := make([]llm.LLMModel, 0, len(tc.installed))
			for _, name := range tc.installed {
				models = append(models, llm.LLMModel{ID: name})
			}
			got, err := ResolvePlannerModelWithCapacity(capacityCaption, func() ([]llm.LLMModel, error) { return models, nil }, func() (PlannerCapacity, error) {
				return PlannerCapacity{FreeVRAM: tc.freeVRAM, FreeRAM: tc.freeRAM, Loaded: tc.loaded}, tc.probeErr
			})
			if tc.want == "" {
				require.Error(t, err)
				assert.NotEmpty(t, err.Error())
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestPlannerCapacityReplacesExplicitBrokenModel(t *testing.T) {
	for _, installed := range [][]string{{capacitySmall, capacityLarge, capacityRecommended}, {capacitySmall}} {
		t.Run(installed[len(installed)-1], func(t *testing.T) {
			got, err := ResolvePlannerModelWithCapacity(capacitySmall, func() ([]llm.LLMModel, error) {
				var models []llm.LLMModel
				for _, name := range installed {
					models = append(models, llm.LLMModel{ID: name})
				}
				return models, nil
			}, func() (PlannerCapacity, error) {
				return PlannerCapacity{FreeVRAM: 32 * capacityGiB, FreeRAM: 4 * capacityGiB}, nil
			})
			if len(installed) == 1 {
				require.Error(t, err)
				assert.NotEmpty(t, err.Error())
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, capacityLarge, got)
			}
		})
	}
}
