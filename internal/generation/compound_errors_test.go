package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

func compoundTestLLM() *mockLLM {
	return &mockLLM{
		genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
			return `{"prompt":"llm-scene-tags","negative_prompt":"llm-neg"}`, nil
		},
	}
}

func makeCompound(t *testing.T, db *preset.DB, steps int) *preset.CompoundPreset {
	t.Helper()
	p := makeTestPreset(t, db, nil)
	list := make([]preset.CompoundPresetStep, 0, steps)
	for i := 0; i < steps; i++ {
		list = append(list, preset.CompoundPresetStep{PresetID: p.ID, DenoisingStrength: 0.6})
	}
	cp := &preset.CompoundPreset{Name: "cp", Description: "cp", Steps: list}
	require.NoError(t, db.CreateCompoundPreset(cp))
	return cp
}

func sessionItemCount(svc *Service) int {
	m := svc.sessions.(*mockSessions)
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

func TestGenerateCompoundImage_StepErrors(t *testing.T) {
	t.Parallel()
	okImg := &sd.Txt2ImgResponse{Images: []string{"img"}, Info: json.RawMessage(`{}`)}
	empty := &sd.Txt2ImgResponse{}
	sdErr := fmt.Errorf("sd exploded")

	tests := []struct {
		name      string
		txt2img   func(svc *Service) (*sd.Txt2ImgResponse, error)
		img2img   func(svc *Service) (*sd.Txt2ImgResponse, error)
		wantErr   string
		wantI2I   int
		wantIsErr error
	}{
		{
			name:      "first step txt2img error",
			txt2img:   func(*Service) (*sd.Txt2ImgResponse, error) { return nil, sdErr },
			wantErr:   "step 1 (txt2img)",
			wantIsErr: sdErr,
		},
		{
			name:    "first step no image",
			txt2img: func(*Service) (*sd.Txt2ImgResponse, error) { return empty, nil },
			wantErr: "step 1: no image returned",
		},
		{
			name: "first step interrupted",
			txt2img: func(svc *Service) (*sd.Txt2ImgResponse, error) {
				_ = svc.InterruptGeneration()
				return nil, sdErr
			},
			wantErr: "interrupted",
		},
		{
			name:      "chain step img2img error",
			txt2img:   func(*Service) (*sd.Txt2ImgResponse, error) { return okImg, nil },
			img2img:   func(*Service) (*sd.Txt2ImgResponse, error) { return nil, sdErr },
			wantErr:   "step 2 (img2img)",
			wantI2I:   1,
			wantIsErr: sdErr,
		},
		{
			name:    "chain step no image",
			txt2img: func(*Service) (*sd.Txt2ImgResponse, error) { return okImg, nil },
			img2img: func(*Service) (*sd.Txt2ImgResponse, error) { return empty, nil },
			wantErr: "step 2: no image returned",
			wantI2I: 1,
		},
		{
			name:    "chain step interrupted with empty response",
			txt2img: func(*Service) (*sd.Txt2ImgResponse, error) { return okImg, nil },
			img2img: func(svc *Service) (*sd.Txt2ImgResponse, error) {
				_ = svc.InterruptGeneration()
				return empty, nil
			},
			wantErr: "interrupted",
			wantI2I: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			cp := makeCompound(t, db, 3)

			var svc *Service
			i2iCalls := 0
			sdSvc := &mockSD{
				txt2img: func(sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) { return tt.txt2img(svc) },
				img2img: func(sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
					i2iCalls++
					if tt.img2img == nil {
						return nil, fmt.Errorf("unexpected img2img")
					}
					return tt.img2img(svc)
				},
			}
			svc = newTestService(t, db, compoundTestLLM(), sdSvc)
			svc.ctx = context.Background()

			result, err := svc.GenerateCompoundImage(GenerateCompoundImageParams{CompoundPresetID: cp.ID})
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.wantErr)
			if tt.wantIsErr != nil {
				assert.ErrorIs(t, err, tt.wantIsErr)
			}
			assert.Equal(t, tt.wantI2I, i2iCalls, "chain must stop at the failed step")
			assert.Equal(t, 0, sessionItemCount(svc))
		})
	}
}

func TestGenerateFromImage_CompoundFirstStep(t *testing.T) {
	t.Parallel()
	sdErr := fmt.Errorf("sd exploded")

	tests := []struct {
		name          string
		mode          string
		denoising     float64
		t2iResp       *sd.Txt2ImgResponse
		t2iErr        error
		i2iResp       *sd.Txt2ImgResponse
		i2iErr        error
		wantErr       string
		wantT2I       int
		wantI2I       int
		wantDenoising float64
	}{
		{
			name:          "img2img uses source image and default denoising",
			mode:          "img2img",
			i2iResp:       &sd.Txt2ImgResponse{Images: []string{"step1-img"}},
			wantI2I:       1,
			wantDenoising: 0.5,
		},
		{
			name:          "img2img keeps given denoising",
			mode:          "img2img",
			denoising:     0.3,
			i2iResp:       &sd.Txt2ImgResponse{Images: []string{"step1-img"}},
			wantI2I:       1,
			wantDenoising: 0.3,
		},
		{
			name:    "img2img error",
			mode:    "img2img",
			i2iErr:  sdErr,
			wantErr: "step 1 (img2img)",
			wantI2I: 1,
		},
		{
			name:    "img2img no image",
			mode:    "img2img",
			i2iResp: &sd.Txt2ImgResponse{},
			wantErr: "step 1: no image returned",
			wantI2I: 1,
		},
		{
			name:    "txt2img generates at source size",
			mode:    "txt2img",
			t2iResp: &sd.Txt2ImgResponse{Images: []string{"step1-img"}},
			wantT2I: 1,
		},
		{
			name:    "txt2img error",
			mode:    "txt2img",
			t2iErr:  sdErr,
			wantErr: "step 1 (txt2img)",
			wantT2I: 1,
		},
		{
			name:    "txt2img no image",
			mode:    "txt2img",
			t2iResp: &sd.Txt2ImgResponse{},
			wantErr: "step 1: no image returned",
			wantT2I: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			cp := makeCompound(t, db, 1)
			src := makePNGBase64(t, 640, 384)

			var t2i []sd.Txt2ImgRequest
			var i2i []sd.Img2ImgRequest
			sdSvc := &mockSD{
				txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
					t2i = append(t2i, req)
					return tt.t2iResp, tt.t2iErr
				},
				img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
					i2i = append(i2i, req)
					return tt.i2iResp, tt.i2iErr
				},
			}
			svc := newTestService(t, db, compoundTestLLM(), sdSvc)
			svc.ctx = context.Background()

			result, err := svc.GenerateFromImage(GenerateFromImageParams{
				ImageBase64:       src,
				Mode:              tt.mode,
				GenMode:           "compound",
				CompoundPresetID:  cp.ID,
				Tags:              "custom prompt",
				DenoisingStrength: tt.denoising,
			})
			require.Len(t, t2i, tt.wantT2I)
			require.Len(t, i2i, tt.wantI2I)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Equal(t, 0, sessionItemCount(svc))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "step1-img", result.Image)
			assert.Equal(t, 1, sessionItemCount(svc))
			if tt.wantI2I == 1 {
				req := i2i[0]
				assert.Equal(t, []string{src}, req.InitImages)
				require.NotNil(t, req.DenoisingStrength)
				assert.Equal(t, tt.wantDenoising, *req.DenoisingStrength)
				assert.Equal(t, 640, req.Width)
				assert.Equal(t, 384, req.Height)
				assert.Contains(t, req.Prompt, "llm-scene-tags")
			}
			if tt.wantT2I == 1 {
				req := t2i[0]
				assert.Equal(t, 640, req.Width)
				assert.Equal(t, 384, req.Height)
				assert.Contains(t, req.Prompt, "llm-scene-tags")
			}
		})
	}
}

func TestGenerateFromImage_CompoundChainStepError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		resp    *sd.Txt2ImgResponse
		err     error
		wantErr string
	}{
		{"img2img error", nil, fmt.Errorf("sd exploded"), "step 2 (img2img)"},
		{"no image", &sd.Txt2ImgResponse{}, nil, "step 2: no image returned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			cp := makeCompound(t, db, 3)
			src := makePNGBase64(t, 512, 512)

			calls := 0
			sdSvc := &mockSD{
				img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
					calls++
					if req.InitImages[0] == src {
						return &sd.Txt2ImgResponse{Images: []string{"step1-img"}}, nil
					}
					assert.Equal(t, "step1-img", req.InitImages[0])
					require.NotNil(t, req.DenoisingStrength)
					assert.Equal(t, 0.6, *req.DenoisingStrength)
					return tt.resp, tt.err
				},
			}
			svc := newTestService(t, db, compoundTestLLM(), sdSvc)
			svc.ctx = context.Background()

			_, err := svc.GenerateFromImage(GenerateFromImageParams{
				ImageBase64:      src,
				Mode:             "img2img",
				GenMode:          "compound",
				CompoundPresetID: cp.ID,
				Tags:             "custom prompt",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, 2, calls, "third step must not run")
			assert.Equal(t, 0, sessionItemCount(svc))
		})
	}
}
