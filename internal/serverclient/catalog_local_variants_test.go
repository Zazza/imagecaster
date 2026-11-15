package serverclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_FindSDModelOrVariant(t *testing.T) {
	t.Parallel()
	c := loadTestCatalog(t)
	tests := []struct {
		name string
		file string
		want string
	}{
		{"qwen variant file", "qwen_image_2.1-Q4_K.gguf", "qwen_image_2.1-Q8_0"},
		{"qwen variant without extension", "qwen_image_2.1-Q4_K", "qwen_image_2.1-Q8_0"},
		{"qwen variant other extension", "qwen_image_2.1-Q4_K.safetensors", "qwen_image_2.1-Q8_0"},
		{"qwen variant other case", "QWEN_IMAGE_2.1-q4_k.GGUF", "qwen_image_2.1-Q8_0"},
		{"qwen variant with path", "models/sd/qwen_image_2.1-Q4_K.gguf", "qwen_image_2.1-Q8_0"},
		{"qwen Q6_K variant", "qwen_image_2.1-Q6_K.gguf", "qwen_image_2.1-Q8_0"},
		{"chroma variant", "Chroma1-HD-Q4_K_S.gguf", "Chroma1-HD"},
		{"krea variant", "Krea-2-Turbo-Q6_K.gguf", "Krea-2-Turbo-Q4_K_M"},
		{"klein variant of model without filename", "flux-2-klein-4b-Q4_0.gguf", "flux-2-klein-4b"},
		{"moss variant", "MOSS-Image-SFT-Q4_K.gguf", "moss_image_sft_bf16"},
		{"flux variant", "flux1-dev-Q4_K_S.gguf", "flux1-dev-fp8"},
		{"main by name", "qwen_image_2.1-Q8_0", "qwen_image_2.1-Q8_0"},
		{"main by file", "Chroma1-HD.gguf", "Chroma1-HD"},
		{"plain model by name", "dreamshaper_8", "dreamshaper_8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := c.FindSDModelOrVariant(tt.file)
			require.NotNil(t, got, tt.file)
			assert.Equal(t, tt.want, got.Name)
		})
	}
}

func TestCatalog_FindSDModelOrVariant_Unknown(t *testing.T) {
	t.Parallel()
	c := loadTestCatalog(t)
	for _, file := range []string{
		"",
		"unknown-model-xyz.gguf",
		"qwen_image_2.1-Q4_K-old.gguf",
		"old-qwen_image_2.1-Q4_K.gguf",
		"Chroma1-HD-Q2_K.gguf",
	} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, c.FindSDModelOrVariant(file))
		})
	}
}

func TestCatalog_FindSDModel_IgnoresVariantFiles(t *testing.T) {
	t.Parallel()
	c := loadTestCatalog(t)
	for _, file := range []string{
		"qwen_image_2.1-Q4_K.gguf",
		"qwen_image_2.1-Q4_K",
		"Chroma1-HD-Q4_K_S.gguf",
		"Krea-2-Turbo-Q6_K.gguf",
		"flux-2-klein-4b-Q4_0.gguf",
	} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, c.FindSDModel(file))
		})
	}
}

func TestCatalog_FindSDModel_MainUnchanged(t *testing.T) {
	t.Parallel()
	c := loadTestCatalog(t)
	for _, tt := range []struct{ file, want string }{
		{"qwen_image_2.1-Q8_0", "qwen_image_2.1-Q8_0"},
		{"qwen_image_2.1-Q8_0.gguf", "qwen_image_2.1-Q8_0"},
		{"Chroma1-HD.gguf", "Chroma1-HD"},
		{"Krea-2-Turbo-Q4_K_M", "Krea-2-Turbo-Q4_K_M"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			got := c.FindSDModel(tt.file)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.Name)
		})
	}
}

func TestCatalog_FastCacheMode_LocalVariants(t *testing.T) {
	t.Parallel()
	c := loadTestCatalog(t)
	tests := []struct {
		file string
		want string
	}{
		{"qwen_image_2.1-Q4_K.gguf", "easycache"},
		{"qwen_image_2.1-Q4_K", "easycache"},
		{"QWEN_IMAGE_2.1-Q6_K.GGUF", "easycache"},
		{"Chroma1-HD-Q4_K_S.gguf", "dbcache"},
		{"Chroma1-HD-Q8_0.gguf", "dbcache"},
		{"MOSS-Image-SFT-Q4_K.gguf", "dbcache"},
		{"flux1-dev-Q4_K_S.gguf", "easycache"},
		{"Krea-2-Turbo-Q6_K.gguf", ""},
		{"flux-2-klein-4b-Q4_0.gguf", ""},
		{"unknown-model-xyz.gguf", ""},
		{"qwen_image_2.1-Q4_K-old.gguf", ""},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, c.FastCacheMode(tt.file))
		})
	}
}

func TestCatalog_FindSDModelOrVariant_EveryVariantFile(t *testing.T) {
	t.Parallel()
	c := loadTestCatalog(t)
	for _, m := range c.SDModels {
		for _, v := range m.Variants {
			t.Run(v.Filename, func(t *testing.T) {
				t.Parallel()
				got := c.FindSDModelOrVariant(v.Filename)
				require.NotNil(t, got, v.Filename)
				assert.Equal(t, m.Name, got.Name)
				assert.Equal(t, c.FastCacheMode(m.Name), c.FastCacheMode(v.Filename))
			})
		}
	}
}
