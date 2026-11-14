package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
	for _, k := range []string{"LLM_URL", "SD_URL", "LLM_MODEL", "SD_PROMPT_MODEL", "DB_PATH"} {
		t.Setenv(k, "")
	}

	cfg := Load()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"LLMUrl", cfg.LLMUrl, "http://localhost:1234"},
		{"SDUrl", cfg.SDUrl, "http://localhost:7860"},
		{"LLMModel", cfg.LLMModel, "openai/gpt-oss-20b"},
		{"SDPromptModel", cfg.SDPromptModel, "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
	require.True(t, filepath.IsAbs(cfg.DBPath))
	require.Equal(t, filepath.Join("data", "presets.db"), filepath.Join(filepath.Base(filepath.Dir(cfg.DBPath)), filepath.Base(cfg.DBPath)))
}

func TestLoad_DefaultPromptContent(t *testing.T) {
	t.Parallel()

	cfg := Load()

	if !strings.Contains(cfg.SystemPrompt, "SD ONLY understands concrete visual tags") {
		t.Error("SystemPrompt should contain SD tag instructions")
	}
	if !strings.Contains(cfg.SystemPrompt, "JSON") {
		t.Error("SystemPrompt should reference JSON output format")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envValue string
		check    func(cfg *Config) string
		want     string
	}{
		{
			name:     "LLM_URL override",
			envKey:   "LLM_URL",
			envValue: "http://custom-llm:9999",
			check:    func(cfg *Config) string { return cfg.LLMUrl },
			want:     "http://custom-llm:9999",
		},
		{
			name:     "SD_URL override",
			envKey:   "SD_URL",
			envValue: "http://custom-sd:8888",
			check:    func(cfg *Config) string { return cfg.SDUrl },
			want:     "http://custom-sd:8888",
		},
		{
			name:     "LLM_MODEL override",
			envKey:   "LLM_MODEL",
			envValue: "custom-model-v2",
			check:    func(cfg *Config) string { return cfg.LLMModel },
			want:     "custom-model-v2",
		},
		{
			name:     "SD_PROMPT_MODEL override",
			envKey:   "SD_PROMPT_MODEL",
			envValue: "prompt-model-v1",
			check:    func(cfg *Config) string { return cfg.SDPromptModel },
			want:     "prompt-model-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envKey, tt.envValue)
			cfg := Load()
			got := tt.check(cfg)
			if got != tt.want {
				t.Errorf("config field = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoad_DBPathOverride(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-presets.db")
	t.Setenv("DB_PATH", dbPath)
	cfg := Load()
	if cfg.DBPath != dbPath {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, dbPath)
	}
}

func TestModelMatchesCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		modelName string
		csv       string
		want      bool
	}{
		{name: "exact match", modelName: "flux", csv: "flux", want: true},
		{name: "substring in model name", modelName: "flux1-dev-fp8", csv: "flux,z-image,qwen-image", want: true},
		{name: "model name case insensitive", modelName: "Flux-Dev", csv: "flux", want: true},
		{name: "csv part case insensitive", modelName: "flux-dev", csv: "FLUX,Z-Image", want: true},
		{name: "model name with surrounding spaces", modelName: "  flux-dev  ", csv: "flux", want: true},
		{name: "csv parts with surrounding spaces", modelName: "z-image-turbo", csv: " flux , z-image ", want: true},
		{name: "last csv part matches", modelName: "hunyuan-image-2.1", csv: "flux,z-image,qwen-image,chroma,hunyuan", want: true},
		{name: "empty model name", modelName: "", csv: "flux", want: false},
		{name: "whitespace only model name", modelName: "   ", csv: "flux", want: false},
		{name: "empty csv", modelName: "flux-dev", csv: "", want: false},
		{name: "csv with only empty parts", modelName: "flux-dev", csv: " , , ,", want: false},
		{name: "empty part between valid parts skipped", modelName: "flux-dev", csv: ",,flux,,", want: true},
		{name: "no match", modelName: "epicrealismXL_pureFix", csv: "flux,z-image,qwen-image,chroma,hunyuan", want: false},
		{name: "partial letters do not match", modelName: "fl", csv: "flux", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, ModelMatchesCSV(tt.modelName, tt.csv))
		})
	}
}

func TestDefaultSDPromptInstructionProseInvariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		token     string
		contained bool
	}{
		{name: "appearance variation section present", token: "APPEARANCE VARIATION", contained: true},
		{name: "json prompt key present", token: `"prompt"`, contained: true},
		{name: "json negative prompt key present", token: `"negative_prompt"`, contained: true},
		{name: "no open parenthesis", token: "(", contained: false},
		{name: "no close parenthesis", token: ")", contained: false},
		{name: "no example leak freckles", token: "freckles", contained: false},
		{name: "no example leak auburn", token: "auburn", contained: false},
		{name: "no example leak blonde", token: "blonde", contained: false},
		{name: "no example leak curly", token: "curly", contained: false},
		{name: "no example leak green eyes", token: "green eyes", contained: false},
		{name: "no weight above one", token: ":1.", contained: false},
		{name: "no weight below one", token: ":0.", contained: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.contained {
				require.Contains(t, DefaultSDPromptInstructionProse, tt.token)
				return
			}
			require.NotContains(t, DefaultSDPromptInstructionProse, tt.token)
		})
	}
}

func TestDefaultSDPromptInstructionAppearance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		token     string
		contained bool
	}{
		{name: "appearance variation section present", token: "APPEARANCE VARIATION", contained: true},
		{name: "slot age range", token: "(age range:1.2)", contained: true},
		{name: "slot ethnicity or heritage", token: "(ethnicity or heritage:1.2)", contained: true},
		{name: "slot face shape", token: "(face shape:1.2)", contained: true},
		{name: "slot hair color and texture", token: "(hair color and texture:1.2)", contained: true},
		{name: "slot body type", token: "(body type:1.1)", contained: true},
		{name: "slot distinctive facial feature", token: "(distinctive facial feature:1.2)", contained: true},
		{name: "slot second distinctive feature", token: "(second distinctive feature:1.1)", contained: true},
		{name: "rule 4 appearance carve-out", token: "EXCEPT the mandatory appearance slots", contained: true},
		{name: "no example leak freckles", token: "freckles", contained: false},
		{name: "no example leak auburn", token: "auburn", contained: false},
		{name: "no example leak curly", token: "curly", contained: false},
		{name: "no example leak green eyes", token: "green eyes", contained: false},
		{name: "no example leak blonde", token: "blonde", contained: false},
		{name: "no example leak bella", token: "bella", contained: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.contained {
				require.Contains(t, DefaultSDPromptInstruction, tt.token)
				return
			}
			require.NotContains(t, DefaultSDPromptInstruction, tt.token)
		})
	}
}
