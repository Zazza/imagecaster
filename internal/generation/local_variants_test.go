package generation

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

var localVariantCases = []struct {
	file  string
	main  string
	cache string
}{
	{"qwen_image_2.1-Q4_K.gguf", "qwen_image_2.1-Q8_0", "easycache"},
	{"qwen_image_2.1-Q4_K", "qwen_image_2.1-Q8_0", "easycache"},
	{"Chroma1-HD-Q4_K_S.gguf", "Chroma1-HD", "dbcache"},
	{"Krea-2-Turbo-Q6_K.gguf", "Krea-2-Turbo-Q4_K_M", ""},
	{"MOSS-Image-SFT-Q4_K.gguf", "moss_image_sft_bf16", "dbcache"},
	{"flux-2-klein-4b-Q4_0.gguf", "flux-2-klein-4b", ""},
}

func TestLocalVariantModules_SameAsMainModel(t *testing.T) {
	t.Parallel()
	for _, tt := range localVariantCases {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			want := modulesForModel(tt.main)
			require.NotEmpty(t, want, tt.main)
			assert.ElementsMatch(t, want, modulesForModel(tt.file))
			assert.ElementsMatch(t, want, modelSpec(tt.file).Modules)
		})
	}
}

func TestLocalVariantModules_QwenCatalogModules(t *testing.T) {
	t.Parallel()
	want := []string{}
	for _, f := range catalogModuleFilenames(t, qwenTestModel) {
		if !strings.Contains(f, "mmproj") {
			want = append(want, f)
		}
	}
	for _, file := range []string{"qwen_image_2.1-Q4_K.gguf", "qwen_image_2.1-Q6_K.gguf", "qwen_image_2.1-Q4_K"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			assert.ElementsMatch(t, want, modulesForModel(file))
		})
	}
}

func TestLocalVariantSpec_QwenReferenceCarriesVision(t *testing.T) {
	t.Parallel()
	plain := modelSpec("qwen_image_2.1-Q4_K.gguf")
	assert.False(t, hasMMProj(plain.Modules), "%v", plain.Modules)
	ref := withReference(plain)
	assert.Contains(t, ref.Modules, qwenTestVision)
	assert.Equal(t, "qwen_image_2.1-Q4_K.gguf", ref.Checkpoint)
}

func TestLocalVariantCacheMode(t *testing.T) {
	t.Parallel()
	for _, global := range []bool{true, false} {
		for _, tt := range localVariantCases {
			name := map[bool]string{true: "global=true", false: "global=false"}[global] + "/" + tt.file
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				db := openTestDB(t)
				g := global
				setFastGlobal(t, db, &g)
				svc := newTestService(t, db, &mockLLM{}, &mockSD{})
				want := ""
				if global {
					want = tt.cache
				}
				assert.Equal(t, want, svc.cacheModeFor(tt.file))
			})
		}
	}
}

func TestGenerateImage_LocalVariantFile(t *testing.T) {
	t.Parallel()
	for _, tt := range localVariantCases {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			setFastGlobal(t, db, boolp(true))
			p := makeTestPreset(t, db, &preset.Preset{Name: "lv", PresetType: "txt2img", Prompt: "cat", Sampler: "euler", Steps: 10, CfgScale: 5, ModelName: tt.file})
			var mu sync.Mutex
			var got []sd.Txt2ImgRequest
			mock := &mockSD{txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
				mu.Lock()
				got = append(got, req)
				mu.Unlock()
				return &sd.Txt2ImgResponse{Images: []string{"img"}, Info: json.RawMessage(`{"seed":1}`), Parameters: json.RawMessage(`{}`)}, nil
			}}
			svc := newTestService(t, db, &mockLLM{}, mock)
			svc.ctx = context.Background()
			_, err := svc.GenerateImage(GenerateImageParams{PresetID: p.ID})
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			require.NotEmpty(t, got)
			want := modulesForModel(tt.main)
			for i, req := range got {
				assert.Equal(t, tt.cache, req.CacheMode, "request %d", i)
				assert.Equal(t, tt.file, req.Model.Checkpoint, "request %d", i)
				assert.ElementsMatch(t, want, req.Model.Modules, "request %d", i)
			}
		})
	}
}
