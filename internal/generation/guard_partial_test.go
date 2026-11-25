package generation

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/stretchr/testify/assert"
)

func TestMemoryGuard_PartialRelief(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		stats  *sd.MemoryStats
		loaded []llm.LoadedModel
		want   bool
	}{
		{name: "observed vram shortage still unloads 5.8 GiB", stats: cudaStats(gib(1.1), gib(16)), loaded: []llm.LoadedModel{{Size: int64(gib(5.8)), SizeVRAM: int64(gib(5.8))}}, want: true},
		{name: "one GiB CPU model helps RAM below ten GiB", stats: cudaStats(gib(16), gib(8)), loaded: []llm.LoadedModel{{Size: gibI(1)}}, want: true},
		{name: "CPU model cannot help sole VRAM deficit", stats: cudaStats(gib(1.1), gib(16)), loaded: []llm.LoadedModel{{Size: gibI(6)}}, want: false},
		{name: "GPU model cannot help sole RAM deficit", stats: cudaStats(gib(16), gib(8)), loaded: []llm.LoadedModel{{Size: gibI(6), SizeVRAM: gibI(6)}}, want: false},
		{name: "zero sized model cannot relieve either deficit", stats: cudaStats(gib(1.1), gib(8)), loaded: []llm.LoadedModel{{Name: "unknown-size"}}, want: false},
		{name: "CPU relief counts even with unresolved VRAM deficit", stats: cudaStats(gib(1.1), gib(8)), loaded: []llm.LoadedModel{{Size: gibI(1)}}, want: true},
		{name: "enough memory keeps model loaded", stats: cudaStats(gib(12), gib(10)), loaded: []llm.LoadedModel{{Size: gibI(6), SizeVRAM: gibI(6)}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			llmSvc := &guardLLM{listLoadedFn: loadedOnce(tt.loaded)}
			sdSvc := &guardSD{stats: tt.stats}
			guard := NewMemoryGuard(llmSvc, sdSvc, openTestDB(t), logger.New(nil))
			assert.Equal(t, tt.want, guard.EnsureHeadroom("flux1-dev-fp8", "[partial-test]"))
			expectedUnloadCalls := int32(0)
			expectedListCalls := int32(1)
			if tt.want {
				expectedUnloadCalls = 1
				expectedListCalls++
			}
			assert.Equal(t, expectedUnloadCalls, llmSvc.unloadAllCalls.Load())
			assert.Equal(t, expectedListCalls, llmSvc.listLoadedCalls.Load())
			assert.EqualValues(t, 1, sdSvc.memoryInfoCalls.Load())
		})
	}
}
