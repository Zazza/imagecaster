package generation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
)

var newGenCardModels = []struct {
	name   string
	newGen bool
}{
	{"flux1-dev-fp8", true},
	{"flux-2-klein-4b", true},
	{"z-image-turbo-fp8-aio", true},
	{"moss_image_sft_bf16", true},
	{"Qwen-Image-Edit", true},
	{"chroma-unlocked-v35", true},
	{"HunyuanImage-2.1", true},
	{"megatron-xl", false},
	{"juggernautXL_ragnarokBy", false},
	{"epicrealismXL_pureFix", false},
	{"dreamshaper_8", false},
	{"riMixPONYIllustrious_riMixV2", false},
	{"ponyDiffusionV6XL_v6StartWithThisOne", false},
}

func dbWithLegacyModelLists(t *testing.T, value string) *preset.DB {
	t.Helper()
	db := openTestDB(t)
	if value != "" {
		require.NoError(t, db.SetSetting("heavy_models", value))
		require.NoError(t, db.SetSetting("prose_models", value))
	}
	return db
}

// 10 GiB free CUDA: below the heavy-model threshold, above the regular one —
// the guard unloads the LLM only when it treats the model as heavy.
func heavyOnlyDeficit(t *testing.T, db *preset.DB, model string) (bool, int32) {
	t.Helper()
	vram := llm.LoadedModel{Name: "llama3:latest", Size: gibI(12), SizeVRAM: gibI(10)}
	llmSvc := &guardLLM{backend: llm.BackendOllama, listLoadedFn: loadedOnce([]llm.LoadedModel{vram})}
	sdSvc := &guardSD{stats: cudaStats(gib(10), gib(16))}
	g := NewMemoryGuard(llmSvc, sdSvc, db, logger.New(nil))
	got := g.EnsureHeadroom(model, "[test]")
	return got, llmSvc.unloadAllCalls.Load()
}

func TestMemoryGuard_HeavyByNewGenOnly(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{"", "megatron", "dreamshaper,juggernaut"} {
		for _, m := range newGenCardModels {
			t.Run(setting+"/"+m.name, func(t *testing.T) {
				t.Parallel()
				require.Equal(t, m.newGen, config.IsNewGenModel(m.name), "card example")
				db := dbWithLegacyModelLists(t, setting)
				unloaded, calls := heavyOnlyDeficit(t, db, m.name)
				assert.Equal(t, m.newGen, unloaded, "heavy treatment of %s with heavy_models=%q", m.name, setting)
				if m.newGen {
					assert.Equal(t, int32(1), calls)
				} else {
					assert.Zero(t, calls)
				}
			})
		}
	}
}

func TestMemoryGuard_MegatronSettingIgnored(t *testing.T) {
	t.Parallel()
	db := dbWithLegacyModelLists(t, "megatron")

	unloaded, calls := heavyOnlyDeficit(t, db, "megatron-xl")
	assert.False(t, unloaded, "megatron-xl must not be heavy")
	assert.Zero(t, calls)

	db2 := dbWithLegacyModelLists(t, "megatron")
	unloaded, calls = heavyOnlyDeficit(t, db2, "flux1-dev-fp8")
	assert.True(t, unloaded, "flux1-dev-fp8 must be heavy")
	assert.Equal(t, int32(1), calls)
}

func TestSDPromptInstruction_ProseByNewGenOnly(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{"", "megatron"} {
		for _, m := range newGenCardModels {
			t.Run(setting+"/"+m.name, func(t *testing.T) {
				t.Parallel()
				db := dbWithLegacyModelLists(t, setting)
				svc := newTestService(t, db, &mockLLM{}, &mockSD{})
				tags := svc.getSDPromptInstruction()
				require.NotEqual(t, config.DefaultSDPromptInstructionProse, tags, "sanity: tags and prose instructions differ")
				got := svc.getSDPromptInstructionFor(m.name)
				if m.newGen {
					assert.Equal(t, config.DefaultSDPromptInstructionProse, got, "%s must get prose instruction", m.name)
				} else {
					assert.Equal(t, tags, got, "%s must get tags instruction (prose_models=%q)", m.name, setting)
				}
			})
		}
	}
}
