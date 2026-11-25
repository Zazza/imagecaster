package generation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type performanceSD struct {
	guardSD
	txt          sd.Txt2ImgRequest
	img          sd.Img2ImgRequest
	upscaleInput string
	upscaleName  string
	upscaleScale float64
	calls        int
	response     *sd.Txt2ImgResponse
	callErr      error
}

func (s *performanceSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	s.txt = req
	s.calls++
	return s.response, s.callErr
}
func (s *performanceSD) Img2Img(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
	s.img = req
	s.calls++
	return s.response, s.callErr
}
func (s *performanceSD) UpscaleImage(img, name string, scale float64) (string, error) {
	s.upscaleInput = img
	s.upscaleName = name
	s.upscaleScale = scale
	s.calls++
	return "returned-image", s.callErr
}

func TestGenerationPerformance_ForwardingAndPrivacy(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"txt2img", "img2img", "upscale"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			backend := &performanceSD{response: &sd.Txt2ImgResponse{Images: []string{"returned-image"}}}
			log := logger.New(nil)
			db := openTestDB(t)
			require.NoError(t, db.SetSetting("diagnostics_verbose_logs", "true"))
			svc := &Service{db: db, sd: backend, log: log}
			seed := int64(42)
			switch stage {
			case "txt2img":
				req := sd.Txt2ImgRequest{Model: sd.ModelSpec{Checkpoint: "/PRIVATE_DIRECTORY/checkpoint.safetensors", VAE: "PRIVATE_VAE", Modules: []string{"PRIVATE_LORA"}}, Prompt: "PRIVATE_PROMPT <lora:PRIVATE_PERSON:1>", NegativePrompt: "PRIVATE_NEGATIVE", Width: 768, Height: 1024, Steps: 23, Seed: &seed, SamplerName: "euler", Scheduler: "normal"}
				got, err := svc.measuredTxt2Img(req)
				require.NoError(t, err)
				assert.Same(t, backend.response, got)
				assert.Equal(t, req, backend.txt)
			case "img2img":
				req := sd.Img2ImgRequest{Model: sd.ModelSpec{Checkpoint: "/PRIVATE_DIRECTORY/checkpoint.safetensors", VAE: "PRIVATE_VAE", Modules: []string{"PRIVATE_LORA"}}, Prompt: "PRIVATE_PROMPT <lora:PRIVATE_PERSON:1>", InitImages: []string{"PRIVATE_BASE64"}, Mask: "PRIVATE_MASK", RefImage: "PRIVATE_REFERENCE", Width: 768, Height: 1024, Steps: 23, Seed: &seed, SamplerName: "euler"}
				got, err := svc.measuredImg2Img(req)
				require.NoError(t, err)
				assert.Same(t, backend.response, got)
				assert.Equal(t, req, backend.img)
			case "upscale":
				got, err := svc.measuredUpscale("PRIVATE_BASE64", "PRIVATE_UPSCALER", 2)
				require.NoError(t, err)
				assert.Equal(t, "returned-image", got)
				assert.Equal(t, "PRIVATE_BASE64", backend.upscaleInput)
				assert.Equal(t, "PRIVATE_UPSCALER", backend.upscaleName)
				assert.Equal(t, 2.0, backend.upscaleScale)
			}
			assert.Equal(t, 1, backend.calls)
			assert.Zero(t, backend.memoryInfoCalls.Load())
			entries := log.Entries()
			require.Len(t, entries, 2)
			assert.Contains(t, entries[0].Message, "generation_sd stage="+stage)
			assert.Contains(t, entries[0].Message, "status=start")
			assert.Contains(t, entries[1].Message, "status=end")
			assert.Contains(t, entries[1].Message, "outcome=ok")
			assert.Regexp(t, `elapsed_ms=\d+`, entries[1].Message)
			for _, e := range entries {
				assert.NotContains(t, e.Message, "PRIVATE_")
			}
		})
	}
}

func TestGenerationPerformance_ErrorAndEmptyResponse(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("PRIVATE_ERROR PRIVATE_BASE64")
	for _, test := range []struct {
		name     string
		response *sd.Txt2ImgResponse
		err      error
	}{
		{name: "backend error", err: sentinel},
		{name: "nil response"},
		{name: "empty images", response: &sd.Txt2ImgResponse{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			backend := &performanceSD{response: test.response, callErr: test.err}
			log := logger.New(nil)
			db := openTestDB(t)
			require.NoError(t, db.SetSetting("diagnostics_verbose_logs", "true"))
			svc := &Service{db: db, sd: backend, log: log}
			got, err := svc.measuredTxt2Img(sd.Txt2ImgRequest{})
			assert.Same(t, test.response, got)
			assert.Equal(t, test.err, err)
			entries := log.Entries()
			require.Len(t, entries, 2)
			assert.Contains(t, entries[1].Message, "outcome=error")
			assert.NotContains(t, entries[1].Message, "PRIVATE_")
		})
	}
}

func TestGenerationPerformance_ModelPathAndNilLogger(t *testing.T) {
	t.Parallel()
	log := logger.New(nil)
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("diagnostics_verbose_logs", "true"))
	svc := &Service{db: db, log: log}
	finish := svc.startSDStage("img2img", `C:\PRIVATE_DIRECTORY\checkpoint.safetensors`, 768, 1024, 23)
	finish(nil)
	for _, e := range log.Entries() {
		assert.Contains(t, e.Message, `model="checkpoint.safetensors"`)
		assert.Contains(t, e.Message, "width=768 height=1024 steps=23")
		assert.False(t, strings.Contains(e.Message, "PRIVATE_"))
	}
	require.NotPanics(t, func() { (&Service{}).startSDStage("txt2img", "model", 0, 0, 0)(nil) })
}

func TestGenerationPerformance_FaceSwapLegitimateSkip(t *testing.T) {
	t.Parallel()
	backend := &mockSD{faceSwap: func(context.Context, string, string, float64) (*sd.FaceSwapResult, error) {
		return &sd.FaceSwapResult{Status: sd.FaceSwapSkipped, Skip: "no_face"}, nil
	}}
	svc := newTestService(t, openTestDB(t), &mockLLM{}, backend)
	require.NoError(t, svc.db.SetSetting("diagnostics_verbose_logs", "true"))
	svc.cfg.SDUrl = "http://127.0.0.1:7860"
	img, status := svc.FaceSwap("PRIVATE_BASE64", "PRIVATE_PERSON")
	assert.Equal(t, "PRIVATE_BASE64", img)
	assert.Equal(t, FaceSwapSkipped("no_face"), status)
	var entries []logger.Entry
	for _, entry := range svc.log.Entries() {
		if strings.HasPrefix(entry.Message, "generation_sd ") {
			entries = append(entries, entry)
		}
	}
	require.Len(t, entries, 2)
	assert.Contains(t, entries[0].Message, "stage=faceswap status=start")
	assert.Contains(t, entries[1].Message, "status=end")
	assert.Contains(t, entries[1].Message, "outcome=skipped")
	for _, entry := range entries {
		assert.NotContains(t, entry.Message, "PRIVATE_")
	}
}
