package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGenModelFamilies_ExactList(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t,
		[]string{"flux", "z-image", "moss", "klein", "qwen-image", "chroma", "hunyuan", "krea"},
		NewGenModelFamilies,
	)
}

func TestIsNewGenModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"flux1-dev-fp8", true},
		{"flux-2-klein-4b", true},
		{"z-image-turbo-fp8-aio", true},
		{"moss_image_sft_bf16", true},
		{"Qwen-Image-Edit", true},
		{"chroma-unlocked-v35", true},
		{"HunyuanImage-2.1", true},
		{"FLUX1-DEV", true},
		{"my_Klein_merge", true},
		{"models/Z-IMAGE.safetensors", true},
		{"juggernautXL_ragnarokBy", false},
		{"epicrealismXL_pureFix", false},
		{"dreamshaper_8", false},
		{"riMixPONYIllustrious_riMixV2", false},
		{"ponyDiffusionV6XL_v6StartWithThisOne", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsNewGenModel(tt.name), "IsNewGenModel(%q)", tt.name)
		})
	}
}

func TestIsNewGenModel_EveryFamilyMatchesItself(t *testing.T) {
	t.Parallel()
	for _, f := range NewGenModelFamilies {
		assert.True(t, IsNewGenModel(f), "family %q", f)
		assert.True(t, IsNewGenModel("x_"+f+"_y"), "family %q inside name", f)
	}
}
