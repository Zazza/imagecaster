package generation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const emoPresetPrompt = "vintage film grain look"

type emoRec struct {
	mu      sync.Mutex
	ops     []string
	reqs    []sd.Img2ImgRequest
	img2err error
}

func (r *emoRec) mock(t *testing.T) *mockSD {
	return &mockSD{
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			r.mu.Lock()
			r.ops = append(r.ops, "img2img")
			r.reqs = append(r.reqs, req)
			err := r.img2err
			r.mu.Unlock()
			if err != nil {
				return nil, err
			}
			return &sd.Txt2ImgResponse{Images: []string{solidPNGBase64(t, req.Width, req.Height, emGray)}}, nil
		},
		setOption: func(key string, value any) error {
			r.mu.Lock()
			r.ops = append(r.ops, fmt.Sprintf("option:%s=%v", key, value))
			r.mu.Unlock()
			return nil
		},
	}
}

func (r *emoRec) filtered(keys ...string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, op := range r.ops {
		if op == "img2img" {
			out = append(out, op)
			continue
		}
		for _, k := range keys {
			if strings.HasPrefix(op, "option:"+k+"=") {
				out = append(out, op)
				break
			}
		}
	}
	return out
}

func (r *emoRec) requests() []sd.Img2ImgRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sd.Img2ImgRequest(nil), r.reqs...)
}

func emoSetup(t *testing.T) (*Service, *emoRec, int64) {
	t.Helper()
	db := openTestDB(t)
	p := makeTestPreset(t, db, &preset.Preset{
		Name:         "effect-post-film",
		PresetType:   "post",
		Prompt:       emoPresetPrompt,
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        8,
		CfgScale:     1.0,
		ModelName:    "flux-2-klein-4b",
	})
	rec := &emoRec{}
	svc := newTestService(t, db, emLLM(), rec.mock(t))
	svc.ctx = context.Background()
	svc.sessions = &mockSessions{}
	return svc, rec, p.ID
}

func emoParams(t *testing.T, presetID int64, model, loras string) GenerateFromImageParams {
	p := GenerateFromImageParams{
		ImageBase64:       makePNGBase64(t, 256, 256),
		Mode:              "img2img",
		GenMode:           "preset",
		PresetID:          presetID,
		PreserveIdentity:  true,
		DenoisingStrength: 0.6,
	}
	if model != "" || loras != "" {
		ov := &PresetOverrides{}
		if model != "" {
			ov.ModelName = &model
		}
		if loras != "" {
			ov.Loras = &loras
		}
		p.Overrides = ov
	}
	return p
}

func TestEffectModel_MossTogglesMossReference(t *testing.T) {
	t.Parallel()
	svc, rec, id := emoSetup(t)

	_, err := svc.GenerateFromImage(emoParams(t, id, mossEditModel, `[{"name":"X","weight":0.7}]`))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"option:" + mossReferenceOption + "=" + mossReferenceOn,
		"img2img",
		"option:" + mossReferenceOption + "=" + mossReferenceOff,
	}, rec.filtered(mossReferenceOption))
	assert.NotContains(t, rec.filtered(kleinReferenceOption), "option:"+kleinReferenceOption+"=true")
}

func TestEffectModel_MossImg2ImgRequest(t *testing.T) {
	t.Parallel()
	svc, rec, id := emoSetup(t)

	_, err := svc.GenerateFromImage(emoParams(t, id, mossEditModel, `[{"name":"X","weight":0.7}]`))
	require.NoError(t, err)

	reqs := rec.requests()
	require.Len(t, reqs, 1)
	req := reqs[0]
	assert.True(t, strings.HasPrefix(req.Prompt, editLoRATag), "prompt %q must start with %q", req.Prompt, editLoRATag)
	assert.Contains(t, req.Prompt, "<lora:X:0.7>")
	assert.Contains(t, req.Prompt, emoPresetPrompt)
	assert.Equal(t, 30, req.Steps)
	assert.Equal(t, 4.0, req.CfgScale)
	assert.Equal(t, "Simple", req.Scheduler)
	assert.Contains(t, req.SamplerName, "Euler")
	require.NotNil(t, req.DenoisingStrength)
	assert.InDelta(t, 0.6, *req.DenoisingStrength, 1e-9)
}

func TestEffectModel_MossImg2ImgErrorStillDisablesReference(t *testing.T) {
	t.Parallel()
	svc, rec, id := emoSetup(t)
	rec.img2err = fmt.Errorf("forge failed")

	_, err := svc.GenerateFromImage(emoParams(t, id, mossEditModel, ""))
	require.Error(t, err)

	ops := rec.filtered(mossReferenceOption)
	require.NotEmpty(t, ops)
	assert.Equal(t, "option:"+mossReferenceOption+"="+mossReferenceOff, ops[len(ops)-1])
	assert.Contains(t, ops, "option:"+mossReferenceOption+"="+mossReferenceOn)
}

func TestEffectModel_KleinWithoutOverrides(t *testing.T) {
	t.Parallel()
	svc, rec, id := emoSetup(t)

	_, err := svc.GenerateFromImage(emoParams(t, id, "", ""))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"option:" + kleinReferenceOption + "=true",
		"img2img",
		"option:" + kleinReferenceOption + "=false",
	}, rec.filtered(kleinReferenceOption))
	assert.Equal(t, []string{"img2img"}, rec.filtered(mossReferenceOption))

	reqs := rec.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, 8, reqs[0].Steps)
	assert.Equal(t, 1.0, reqs[0].CfgScale)
	assert.NotContains(t, reqs[0].Prompt, "Moss_Edit_LoRA")
}

func TestEffectModel_PreserveIdentityRejectsOtherModels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		model string
	}{
		{"sdxl", "sdxl_base.safetensors"},
		{"sd15", "epicrealism_naturalSin.safetensors"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, id := emoSetup(t)

			_, err := svc.GenerateFromImage(emoParams(t, id, tt.model, ""))
			require.Error(t, err)
			assert.Empty(t, rec.requests(), "img2img must not be called")
		})
	}
}
