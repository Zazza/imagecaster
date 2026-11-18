package sd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImg2Img_RefImageParam(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ref  string
	}{
		{"with garment reference", "garment-b64"},
		{"without reference", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts, body := captureSubmit(t)
			c := New(ts.URL)
			c.PollInterval = time.Millisecond
			_, err := c.Img2Img(Img2ImgRequest{
				Model:      ModelSpec{Checkpoint: "flux-2-klein-4b", KleinReference: true},
				InitImages: []string{"init"},
				Prompt:     "p",
				RefImage:   tt.ref,
			})
			require.NoError(t, err)
			p, ok := (*body)["params"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "init", p["init_image"])
			if tt.ref == "" {
				v, present := p["ref_image"]
				if present {
					assert.Empty(t, v)
				}
				return
			}
			assert.Equal(t, tt.ref, p["ref_image"])
		})
	}
}
