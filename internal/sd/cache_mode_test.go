package sd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJob_CacheModeParam(t *testing.T) {
	t.Parallel()
	hr := true
	scale := 1.5
	tests := []struct {
		name string
		mode string
		run  func(c *Client, mode string) error
	}{
		{"txt2img on", "easycache", func(c *Client, mode string) error {
			_, err := c.Txt2Img(Txt2ImgRequest{Model: ModelSpec{Checkpoint: "flux1-dev-fp8"}, Prompt: "p", CacheMode: mode})
			return err
		}},
		{"txt2img off", "", func(c *Client, mode string) error {
			_, err := c.Txt2Img(Txt2ImgRequest{Model: ModelSpec{Checkpoint: "flux1-dev-fp8"}, Prompt: "p", CacheMode: mode})
			return err
		}},
		{"txt2img hires on", "ucache", func(c *Client, mode string) error {
			_, err := c.Txt2Img(Txt2ImgRequest{Model: ModelSpec{Checkpoint: "dreamshaper_8"}, Prompt: "p", HiresFix: &hr, HiresUpscale: &scale, CacheMode: mode})
			return err
		}},
		{"txt2img hires off", "", func(c *Client, mode string) error {
			_, err := c.Txt2Img(Txt2ImgRequest{Model: ModelSpec{Checkpoint: "dreamshaper_8"}, Prompt: "p", HiresFix: &hr, HiresUpscale: &scale, CacheMode: mode})
			return err
		}},
		{"img2img on", "spectrum", func(c *Client, mode string) error {
			_, err := c.Img2Img(Img2ImgRequest{Model: ModelSpec{Checkpoint: "moss_image_sft_bf16"}, InitImages: []string{"init"}, Prompt: "p", CacheMode: mode})
			return err
		}},
		{"img2img off", "", func(c *Client, mode string) error {
			_, err := c.Img2Img(Img2ImgRequest{Model: ModelSpec{Checkpoint: "moss_image_sft_bf16"}, InitImages: []string{"init"}, Prompt: "p", CacheMode: mode})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts, body := captureSubmit(t)
			c := New(ts.URL)
			c.PollInterval = time.Millisecond
			require.NoError(t, tt.run(c, tt.mode))
			p, ok := (*body)["params"].(map[string]any)
			require.True(t, ok)
			if tt.mode == "" {
				assert.NotContains(t, p, "cache_mode")
				return
			}
			assert.Equal(t, tt.mode, p["cache_mode"])
		})
	}
}
