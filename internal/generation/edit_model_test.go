package generation

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

type editModelRec struct {
	mu        sync.Mutex
	ops       []string
	models    []string
	reqs      []sd.Img2ImgRequest
	upscales  int
	img2err   error
	upscalers []sd.Upscaler
}

func (r *editModelRec) mock(t *testing.T) *mockSD {
	return &mockSD{
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			r.mu.Lock()
			if req.Model.Checkpoint != "" {
				r.ops = append(r.ops, "model:"+req.Model.Checkpoint)
				r.models = append(r.models, req.Model.Checkpoint)
			}
			if req.Model.KleinReference {
				r.ops = append(r.ops, kleinRefOn)
			}
			r.ops = append(r.ops, "img2img")
			if req.Model.KleinReference {
				r.ops = append(r.ops, kleinRefOff)
			}
			r.reqs = append(r.reqs, req)
			err := r.img2err
			r.mu.Unlock()
			if err != nil {
				return nil, err
			}
			return &sd.Txt2ImgResponse{Images: []string{solidPNGBase64(t, req.Width, req.Height, workGray)}}, nil
		},
		getUpscalers: func() ([]sd.Upscaler, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.upscalers, nil
		},
		upscale: func(in, upscaler string, scale float64) (string, error) {
			r.mu.Lock()
			r.upscales++
			r.ops = append(r.ops, "upscale")
			r.mu.Unlock()
			return "", fmt.Errorf("neural upscale unavailable in test")
		},
	}
}

func (r *editModelRec) filtered(keys ...string) []string {
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

func (r *editModelRec) snapshot() ([]string, []string, []sd.Img2ImgRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ops...), append([]string(nil), r.models...), append([]sd.Img2ImgRequest(nil), r.reqs...)
}

func editModelSetup(t *testing.T) (*Service, *editModelRec, *preset.DB, *mockSessions) {
	t.Helper()
	db := openTestDB(t)
	rec := &editModelRec{}
	svc := newTestService(t, db, &mockLLM{}, rec.mock(t))
	sessions := &mockSessions{}
	svc.sessions = sessions
	return svc, rec, db, sessions
}

func indexOf(ops []string, want string) int {
	for i, op := range ops {
		if op == want {
			return i
		}
	}
	return -1
}

func lastIndexOf(ops []string, want string) int {
	for i := len(ops) - 1; i >= 0; i-- {
		if ops[i] == want {
			return i
		}
	}
	return -1
}

func lastWithPrefix(ops []string, prefix string) string {
	for i := len(ops) - 1; i >= 0; i-- {
		if strings.HasPrefix(ops[i], prefix) {
			return ops[i]
		}
	}
	return ""
}

const (
	kleinRefOn    = "option:" + kleinReferenceOption + "=true"
	kleinRefOff   = "option:" + kleinReferenceOption + "=false"
	mossRefPrefix = "option:moss_do_reference="
	mossModelName = "moss_image_sft_bf16"
)

func TestEditImageParams_ModelJSON(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(EditImageParams{Model: "klein"})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"model":"klein"`)

	b, err = json.Marshal(EditImageParams{})
	require.NoError(t, err)
	assert.NotContains(t, string(b), `"model"`)

	var p EditImageParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"klein"}`), &p))
	assert.Equal(t, "klein", p.Model)
}

func TestEditModel_T1_DefaultIsKlein(t *testing.T) {
	t.Parallel()
	svc, rec, _, _ := editModelSetup(t)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: makePNGBase64(t, 512, 512),
		Instruction: "make hair red",
		Loras:       []preset.LoRAEntry{{Name: "Klein_Detail", Weight: 0.8}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	ops, models, reqs := rec.snapshot()
	assert.Contains(t, models, kleinLookModel)
	assert.NotContains(t, models, mossModelName)
	assert.Contains(t, ops, kleinRefOn)
	assert.Equal(t, kleinRefOff, lastWithPrefix(ops, "option:"+kleinReferenceOption+"="))
	assert.Empty(t, lastWithPrefix(ops, mossRefPrefix))
	require.Len(t, reqs, 1)
	assert.NotContains(t, reqs[0].Prompt, "Moss_Edit_LoRA")
	assert.Contains(t, reqs[0].Prompt, "<lora:Klein_Detail:0.8>")
	assert.Equal(t, 8, reqs[0].Steps)
	assert.InDelta(t, 1.0, reqs[0].CfgScale, 1e-9)
}

func TestEditModel_T2_KleinRequest(t *testing.T) {
	t.Parallel()
	svc, rec, _, _ := editModelSetup(t)

	result, err := svc.EditImage(EditImageParams{
		ImageBase64: makePNGBase64(t, 1024, 1024),
		Instruction: "make hair red",
		Loras:       []preset.LoRAEntry{{Name: "Klein_Detail", Weight: 0.7}},
		Model:       "klein",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	ops, models, reqs := rec.snapshot()
	assert.Contains(t, models, kleinLookModel)
	assert.NotContains(t, models, mossModelName)

	kOps := rec.filtered(kleinReferenceOption)
	iOn := indexOf(kOps, kleinRefOn)
	iImg := indexOf(kOps, "img2img")
	require.GreaterOrEqual(t, iOn, 0, "klein reference must be enabled: %v", kOps)
	require.GreaterOrEqual(t, iImg, 0)
	assert.Less(t, iOn, iImg, "klein reference enabled before img2img: %v", kOps)
	assert.Equal(t, kleinRefOff, lastWithPrefix(ops, "option:"+kleinReferenceOption+"="))
	assert.Greater(t, lastIndexOf(kOps, kleinRefOff), lastIndexOf(kOps, "img2img"))
	assert.Empty(t, lastWithPrefix(ops, mossRefPrefix))

	require.Len(t, reqs, 1)
	req := reqs[0]
	assert.NotContains(t, req.Prompt, "Moss_Edit_LoRA")
	assert.Contains(t, req.Prompt, "<lora:Klein_Detail:0.7>")
	assert.Contains(t, req.Prompt, "make hair red")
	assert.Equal(t, 8, req.Steps)
	assert.InDelta(t, 1.0, req.CfgScale, 1e-9)
	assert.Equal(t, "euler", req.SamplerName)
	assert.Equal(t, "simple", req.Scheduler)
	require.NotNil(t, req.DenoisingStrength)
	assert.InDelta(t, 1.0, *req.DenoisingStrength, 1e-9)
	assert.NotContains(t, result.EffectivePrompt, "Moss_Edit_LoRA")
}

func TestEditModel_T3_KleinImg2ImgErrorDisablesReference(t *testing.T) {
	t.Parallel()
	svc, rec, _, _ := editModelSetup(t)
	rec.img2err = fmt.Errorf("forge failed")

	_, err := svc.EditImage(EditImageParams{
		ImageBase64: makePNGBase64(t, 512, 512),
		Instruction: "make hair red",
		Model:       "klein",
	})
	require.Error(t, err)

	ops, _, _ := rec.snapshot()
	assert.Contains(t, ops, kleinRefOn)
	assert.Equal(t, kleinRefOff, lastWithPrefix(ops, "option:"+kleinReferenceOption+"="))
	assert.Empty(t, lastWithPrefix(ops, mossRefPrefix))
}

func TestEditModel_T4_StylePresetModelMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		editModel  string
		styleModel string
	}{
		{name: "klein edit with moss preset", editModel: "klein", styleModel: "moss_image_sft_bf16"},
		{name: "default edit with moss preset", editModel: "", styleModel: "moss_image_sft_bf16"},
		{name: "klein edit with z-image preset", editModel: "klein", styleModel: "z-image-turbo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, db, _ := editModelSetup(t)
			p := makeTestPreset(t, db, &preset.Preset{Name: "style " + tt.name, ModelName: tt.styleModel, Prompt: "tags"})

			_, err := svc.EditImage(EditImageParams{
				ImageBase64:   makePNGBase64(t, 512, 512),
				Instruction:   "make hair red",
				StylePresetID: p.ID,
				Model:         tt.editModel,
			})
			require.Error(t, err)
			_, _, reqs := rec.snapshot()
			assert.Empty(t, reqs, "img2img must not be called")
		})
	}
}

func TestEditModel_T4_KleinStylePresetApplied(t *testing.T) {
	t.Parallel()
	svc, rec, db, sessions := editModelSetup(t)
	style := makeTestPreset(t, db, &preset.Preset{
		Name:      "Klein Film",
		ModelName: "flux-2-klein-4b",
		Prompt:    "klein_film_style, soft grain",
		Loras:     `[{"name": "Klein_Film_Lora", "weight": 0.6}]`,
	})

	result, err := svc.EditImage(EditImageParams{
		ImageBase64:   makePNGBase64(t, 512, 512),
		Instruction:   "make hair red",
		StylePresetID: style.ID,
		StyleTags:     true,
		Loras:         []preset.LoRAEntry{{Name: "Own_Klein", Weight: 0.5}},
		Model:         "klein",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	_, _, reqs := rec.snapshot()
	require.Len(t, reqs, 1)
	prompt := reqs[0].Prompt
	assert.Contains(t, prompt, "<lora:Klein_Film_Lora:0.6>")
	assert.Contains(t, prompt, "<lora:Own_Klein:0.5>")
	assert.Contains(t, prompt, "klein_film_style, soft grain")
	assert.NotContains(t, prompt, "Moss_Edit_LoRA")

	require.Len(t, sessions.items, 1)
	meta := sessions.items[0].meta
	require.NotNil(t, meta)
	require.NotNil(t, meta.PresetID)
	assert.Equal(t, style.ID, *meta.PresetID)
}

func TestEditModel_T5_UnknownModelRejectedBeforeForge(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"bogus", "KLEIN-XL", "z-image", "moss"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			svc, rec, _, sessions := editModelSetup(t)

			_, err := svc.EditImage(EditImageParams{
				ImageBase64: makePNGBase64(t, 512, 512),
				Instruction: "make hair red",
				Model:       model,
			})
			require.Error(t, err)

			ops, models, reqs := rec.snapshot()
			assert.Empty(t, ops, "no Forge calls expected")
			assert.Empty(t, models)
			assert.Empty(t, reqs)
			assert.Empty(t, sessions.items)
		})
	}
}

func TestEditModel_T6_KleinQuality(t *testing.T) {
	t.Parallel()
	svc, rec, db, sessions := editModelSetup(t)
	res := &preset.Resolution{Name: "q-2048", Width: 2048, Height: 2048}
	require.NoError(t, db.CreateResolution(res))

	resID := res.ID
	result, err := svc.EditImage(EditImageParams{
		ImageBase64:  makePNGBase64(t, 512, 512),
		Instruction:  "make hair red",
		Loras:        []preset.LoRAEntry{{Name: "Klein_Detail", Weight: 0.7}},
		OutputMode:   "quality",
		ResolutionID: &resID,
		Model:        "klein",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	ops, models, reqs := rec.snapshot()
	assert.NotContains(t, models, mossModelName)
	assert.Empty(t, lastWithPrefix(ops, mossRefPrefix))
	assert.Equal(t, kleinRefOff, lastWithPrefix(ops, "option:"+kleinReferenceOption+"="))

	require.GreaterOrEqual(t, len(reqs), 2, "base edit + redenoise expected")
	for i, req := range reqs {
		assert.NotContains(t, req.Prompt, "Moss_Edit_LoRA", "req %d", i)
		assert.Equal(t, 8, req.Steps, "req %d", i)
		assert.InDelta(t, 1.0, req.CfgScale, 1e-9, "req %d", i)
		assert.Equal(t, "euler", req.SamplerName, "req %d", i)
		assert.Equal(t, "simple", req.Scheduler, "req %d", i)
	}
	last := reqs[len(reqs)-1]
	assert.Equal(t, 2048, last.Width)
	assert.Equal(t, 2048, last.Height)

	img := decodePNGBase64(t, result.Image)
	assert.Equal(t, 2048, img.Bounds().Dx())
	assert.Equal(t, 2048, img.Bounds().Dy())
	assert.False(t, result.QualityUpscaleSkipped)
	assert.False(t, result.QualityRedenoiseSkipped)

	require.Len(t, sessions.items, 1)
	meta := sessions.items[0].meta
	require.NotNil(t, meta)
	require.Len(t, meta.Chain, 3)
	assert.Equal(t, kleinLookModel, meta.Chain[0].Model)
}

func TestEditModel_T7_KleinLargeSourceKeepsSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		w, h int
	}{
		{name: "portrait 1360x2048", w: 1360, h: 2048},
		{name: "landscape 2048x1536", w: 2048, h: 1536},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _, _ := editModelSetup(t)

			result, err := svc.EditImage(EditImageParams{
				ImageBase64: makePNGBase64(t, tt.w, tt.h),
				Instruction: "the pants to a skirt",
				Model:       "klein",
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			_, _, reqs := rec.snapshot()
			require.Len(t, reqs, 1)
			assertKleinWorkSize(t, tt.w, tt.h, reqs[0].Width, reqs[0].Height)

			img := decodePNGBase64(t, result.Image)
			assert.Equal(t, tt.w, img.Bounds().Dx())
			assert.Equal(t, tt.h, img.Bounds().Dy())
		})
	}
}
