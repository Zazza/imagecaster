package generation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKleinTryOn_Img2ImgRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		instruction string
	}{
		{name: "with look instruction", instruction: "Give the person a short bob haircut."},
		{name: "garment only", instruction: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, nil)
			seed := int64(77)
			garment := makePNGBase64(t, 64, 96)

			result, err := svc.KleinTryOn(makePNGBase64(t, objTestSize, objTestSize), garment, tt.instruction, &seed)
			require.NoError(t, err)
			require.NotNil(t, result)

			rec.mu.Lock()
			defer rec.mu.Unlock()
			require.Len(t, rec.reqs, 1)
			req := rec.reqs[0]
			assert.Equal(t, garment, req.RefImage)
			require.NotEmpty(t, req.InitImages)
			require.NotNil(t, req.Seed)
			assert.Equal(t, int64(77), *req.Seed)
			assert.Equal(t, kleinLookModel, rec.models[len(rec.models)-1])
			lower := strings.ToLower(req.Prompt)
			iTpl := strings.Index(lower, "second image")
			require.GreaterOrEqual(t, iTpl, 0, "try-on template must mention the second image: %q", req.Prompt)
			if tt.instruction != "" {
				iInstr := strings.Index(req.Prompt, tt.instruction)
				require.GreaterOrEqual(t, iInstr, 0, req.Prompt)
				assert.Less(t, iTpl, iInstr, req.Prompt)
			}
		})
	}
}

func TestKleinTryOn_Validation(t *testing.T) {
	t.Parallel()

	img := makePNGBase64(t, objTestSize, objTestSize)
	garment := makePNGBase64(t, 64, 64)
	tests := []struct {
		name    string
		image   string
		garment string
	}{
		{name: "empty image", image: "", garment: garment},
		{name: "empty garment", image: img, garment: ""},
		{name: "garment too large", image: img, garment: strings.Repeat("A", MaxImageBase64Len+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, nil)
			seed := int64(1)

			result, err := svc.KleinTryOn(tt.image, tt.garment, "short bob", &seed)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.Equal(t, 0, rec.img2imgCount())
		})
	}
}
