package generation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKleinTryOnWorn_Prompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wear      string
		wantWear  string
		wantExact bool
	}{
		{name: "wear replaces exact-details rule", wear: " blouse hanging open. ", wantWear: "worn differently than in the second image: blouse hanging open.", wantExact: false},
		{name: "empty wear falls back to plain try-on", wear: "  ", wantExact: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, nil)
			seed := int64(5)
			garment := makePNGBase64(t, 64, 96)

			_, err := svc.KleinTryOnWorn(makePNGBase64(t, objTestSize, objTestSize), garment, "a blue blouse", tt.wear, &seed)
			require.NoError(t, err)

			rec.mu.Lock()
			defer rec.mu.Unlock()
			require.Len(t, rec.reqs, 1)
			req := rec.reqs[0]
			assert.Equal(t, garment, req.RefImage)
			assert.Contains(t, req.Prompt, "a blue blouse")
			assert.Equal(t, tt.wantExact, strings.Contains(req.Prompt, "exact color, pattern, material, length, cut and details"), req.Prompt)
			if tt.wantWear != "" {
				assert.Contains(t, req.Prompt, tt.wantWear)
			}
		})
	}
}

func TestKleinTryOnWorn_NeedsGarment(t *testing.T) {
	t.Parallel()
	svc, _, _ := objTestSetup(t, nil, nil)
	_, err := svc.KleinTryOnWorn(makePNGBase64(t, objTestSize, objTestSize), "", "a blouse", "torn sleeve", nil)
	require.Error(t, err)
}
