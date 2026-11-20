package mcp

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wearPlainTryOn struct{ instruction string }

func (w *wearPlainTryOn) KleinTryOn(_, _, instruction string, _ *int64) (*generation.GenerateImageResult, error) {
	w.instruction = instruction
	return &generation.GenerateImageResult{Image: "plain"}, nil
}

type wearWornTryOn struct {
	wearPlainTryOn
	worn, wear string
}

func (w *wearWornTryOn) KleinTryOnWorn(_, _, instruction, wear string, _ *int64) (*generation.GenerateImageResult, error) {
	w.worn, w.wear = instruction, wear
	return &generation.GenerateImageResult{Image: "worn"}, nil
}

func TestTryOnGarment(t *testing.T) {
	tests := []struct {
		name      string
		worn      bool
		wear      string
		wantImage string
		wantPlain string
		wantWear  string
	}{
		{"no wear uses plain try-on", true, " ", "plain", "a blouse", ""},
		{"wear uses worn try-on", true, " torn sleeve ", "worn", "", "torn sleeve"},
		{"engine without worn appends wear", false, "torn sleeve", "plain", "a blouse. torn sleeve", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worn := &wearWornTryOn{}
			var ge StylistGarmentEngine = &worn.wearPlainTryOn
			if tt.worn {
				ge = worn
			}
			res, err := tryOnGarment(ge, "img", "garment", "a blouse", tt.wear, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.wantImage, res.Image)
			assert.Equal(t, tt.wantPlain, worn.instruction)
			assert.Equal(t, tt.wantWear, worn.wear)
			if tt.wantWear != "" {
				assert.Equal(t, "a blouse", worn.worn)
			}
		})
	}
}
