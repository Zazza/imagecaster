package sdnames

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpscalerXReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"latent", "Latent", "latent"},
		{"latent antialiased", "Latent (antialiased)", "latent"},
		{"none", "None", ""},
		{"empty", "", ""},
		{"esrgan forge", "R-ESRGAN 4x+", "RealESRGAN_x4plus"},
		{"sdcpp esrgan stays", "RealESRGAN_x4plus", "RealESRGAN_x4plus"},
		{"sdcpp latent stays", "latent", "latent"},
		{"sdcpp lanczos stays", "lanczos", "lanczos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Upscaler(tt.in))
		})
	}
}
