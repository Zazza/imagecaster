package generation

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpscaleDetail_TileScheduler(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		model   string
		sampler string
		want    string
	}{
		{name: "sdxl without scheduler gets karras", model: "juggernautXL_ragnarokBy", sampler: "dpm++2m_sde", want: "karras"},
		{name: "embedded scheduler is kept", model: "juggernautXL_ragnarokBy", sampler: "DPM++ 2M SDE Exponential", want: "exponential"},
		{name: "klein keeps the server default", model: "flux-2-klein-4b", sampler: "euler", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &udRec{upscalers: udQuality()}
			svc, _ := udSetup(t, rec)
			p := udDetailParams(t, 512, 512)
			p.TargetLong = 0
			p.Model, p.Sampler = tt.model, tt.sampler

			_, err := svc.UpscaleDetail(p)

			require.NoError(t, err)
			reqs := rec.img2imgReqs()
			require.NotEmpty(t, reqs)
			assert.Equal(t, tt.want, reqs[0].Scheduler)
		})
	}
}

func TestUpscaleDetail_ResizeFallbackMarkedAndTakesJPEG(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 200, 300))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	tests := []struct {
		name    string
		rec     *udRec
		resized bool
	}{
		{name: "no upscaler", rec: &udRec{}, resized: true},
		{name: "upscaler fails", rec: &udRec{upscalers: udQuality(), upscaleErr: errors.New("oom")}, resized: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := udSetup(t, tt.rec)

			res, err := svc.UpscaleDetail(DetailParams{ImageBase64: base64.StdEncoding.EncodeToString(buf.Bytes()), TargetLong: 3840})

			require.NoError(t, err)
			assert.Equal(t, tt.resized, res.Resized)
			assert.True(t, res.Upscaled)
			assert.Equal(t, 2560, res.Width)
			assert.Equal(t, 3840, res.Height)
		})
	}
}

func TestUpscaleDetail_NeuralNotMarkedResized(t *testing.T) {
	t.Parallel()
	svc, _ := udSetup(t, &udRec{upscalers: udQuality()})
	p := udDetailParams(t, 100, 150)
	p.Detail = 0

	res, err := svc.UpscaleDetail(p)

	require.NoError(t, err)
	assert.True(t, res.Upscaled)
	assert.False(t, res.Resized)
}
