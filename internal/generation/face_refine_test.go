package generation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

func TestFaceRefine_Validation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params FaceRefineParams
		errMsg string
	}{
		{name: "empty image", params: FaceRefineParams{}, errMsg: "image is required"},
		{name: "image too large", params: FaceRefineParams{ImageBase64: strings.Repeat("x", MaxImageBase64Len+1)}, errMsg: "image too large"},
		{name: "undecodable image has no size", params: FaceRefineParams{ImageBase64: "dGVzdA=="}, errMsg: "could not determine image size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			svc := newTestService(t, db, &mockLLM{}, &mockSD{})

			_, err := svc.FaceRefine(tt.params)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestFaceRefine_Params(t *testing.T) {
	t.Parallel()
	seed := int64(777)
	tests := []struct {
		name   string
		model  string
		w, h   int
		wantW  int
		wantH  int
		seed   *int64
		wantOn bool
	}{
		{name: "klein aligns to 16", model: "flux-2-klein-4b", w: 1000, h: 640, wantW: 992, wantH: 640},
		{name: "other models align to 8", model: "z-image-turbo-fp8-aio", w: 505, h: 317, wantW: 504, wantH: 312},
		{name: "tiny image clamped to 64", model: "moss_image_sft_bf16", w: 40, h: 30, wantW: 64, wantH: 64},
		{name: "seed passthrough", model: "z-image-turbo-fp8-aio", w: 512, h: 512, wantW: 512, wantH: 512, seed: &seed, wantOn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			img := makePNGBase64(t, tt.w, tt.h)
			var captured sd.Img2ImgRequest
			sdSvc := &mockSD{
				img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
					captured = req
					return &sd.Txt2ImgResponse{Images: []string{"refined-img"}, Info: json.RawMessage(`{"seed":5}`)}, nil
				},
			}
			svc := newTestService(t, db, &mockLLM{}, sdSvc)

			out, err := svc.FaceRefine(FaceRefineParams{
				ImageBase64:       img,
				Prompt:            "portrait <lora:x:1>",
				NegativePrompt:    "deformed",
				Model:             tt.model,
				Sampler:           "euler",
				Scheduler:         "sgm_uniform",
				Steps:             8,
				CfgScale:          1.5,
				DenoisingStrength: 0.55,
				Seed:              tt.seed,
			})

			require.NoError(t, err)
			assert.Equal(t, "refined-img", out)
			assert.Equal(t, tt.model, captured.Model.Checkpoint)
			assert.Equal(t, []string{img}, captured.InitImages)
			assert.Equal(t, "portrait <lora:x:1>", captured.Prompt)
			assert.Equal(t, "deformed", captured.NegativePrompt)
			assert.Equal(t, "euler", captured.SamplerName)
			assert.Equal(t, "sgm_uniform", captured.Scheduler)
			assert.Equal(t, 8, captured.Steps)
			assert.InDelta(t, 1.5, captured.CfgScale, 1e-9)
			assert.Equal(t, tt.wantW, captured.Width)
			assert.Equal(t, tt.wantH, captured.Height)
			require.NotNil(t, captured.DenoisingStrength)
			assert.InDelta(t, 0.55, *captured.DenoisingStrength, 1e-9)
			require.NotNil(t, captured.BatchSize)
			assert.Equal(t, 1, *captured.BatchSize)
			require.NotNil(t, captured.BatchCount)
			assert.Equal(t, 1, *captured.BatchCount)
			if tt.wantOn {
				require.NotNil(t, captured.Seed)
				assert.Equal(t, int64(777), *captured.Seed)
			} else {
				assert.Nil(t, captured.Seed)
			}
		})
	}
}

func TestFaceRefine_SDErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		img2fn func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error)
		errMsg string
	}{
		{
			name:   "sd error propagates",
			img2fn: func(sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) { return nil, fmt.Errorf("sd img2img down") },
			errMsg: "sd img2img down",
		},
		{
			name:   "empty images",
			img2fn: func(sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) { return &sd.Txt2ImgResponse{}, nil },
			errMsg: "no image generated (face refine)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			fn := tt.img2fn
			sdSvc := &mockSD{img2img: fn}
			svc := newTestService(t, db, &mockLLM{}, sdSvc)

			_, err := svc.FaceRefine(FaceRefineParams{
				ImageBase64:       makePNGBase64(t, 128, 128),
				Prompt:            "p",
				Model:             "z-image-turbo-fp8-aio",
				DenoisingStrength: 0.5,
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}
