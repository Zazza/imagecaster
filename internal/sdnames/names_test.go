package sdnames

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPair(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, sampler, sched   string
		wantSampler, wantSched string
		wantUnknown            int
	}{
		{"forge pair", "DPM++ 2M", "Karras", "dpm++2m", "karras", 0},
		{"composite sampler", "Euler a Karras", "", "euler_a", "karras", 0},
		{"explicit scheduler wins over composite", "DPM++ 2M Karras", "Exponential", "dpm++2m", "exponential", 0},
		{"automatic is default", "Euler", "Automatic", "euler", "", 0},
		{"uniform is discrete", "DDIM", "Uniform", "ddim_trailing", "discrete", 0},
		{"already sdcpp", "euler_a", "karras", "euler_a", "karras", 0},
		{"empty", "", "", "", "", 0},
		{"unknown sampler", "Restart", "Simple", "", "simple", 1},
		{"unknown scheduler", "Heun", "Turbo", "heun", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, sc, unknown := Pair(tt.sampler, tt.sched)
			assert.Equal(t, tt.wantSampler, s)
			assert.Equal(t, tt.wantSched, sc)
			assert.Len(t, unknown, tt.wantUnknown)
		})
	}
}

func TestUpscaler(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"R-ESRGAN 4x+":         "RealESRGAN_x4plus",
		"R-ESRGAN 4x+ Anime6B": "RealESRGAN_x4plus_anime_6B",
		"Lanczos":              "lanczos",
		"Nearest":              "nearest",
		"None":                 "",
		"Latent":               "latent",
		"Latent (antialiased)": "latent",
		"":                     "",
		"4x-UltraSharp":        "4x-UltraSharp",
		"lanczos":              "lanczos",
	}
	for in, want := range tests {
		assert.Equal(t, want, Upscaler(in), in)
	}
}
