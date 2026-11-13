package generation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	qwenTestModel    = "qwen_image_2.1-Q8_0"
	kleinTestModel   = "flux-2-klein-4b"
	hidreamTestModel = "hidream_o1_image_dev_fp8_scaled"
	qwenTestVision   = "mmproj-Qwen3VL-8B-Instruct-F16.gguf"
	oneMegapixel     = 1024 * 1024
	oneMegapixelTol  = 0.10
)

func hasMMProj(mods []string) bool {
	for _, m := range mods {
		if strings.Contains(strings.ToLower(m), "mmproj") {
			return true
		}
	}
	return false
}

func TestQwenModules_PlainGenerationWithoutVision(t *testing.T) {
	t.Parallel()
	for _, model := range []string{qwenTestModel, qwenTestModel + ".gguf"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			mods := modulesForModel(model)
			require.NotEmpty(t, mods)
			assert.False(t, hasMMProj(mods), "%v", mods)
			assert.False(t, hasMMProj(modelSpec(model).Modules), "%v", modelSpec(model).Modules)
		})
	}
}

func TestQwenModules_ReferenceSpecCarriesVision(t *testing.T) {
	t.Parallel()
	plain := modelSpec(qwenTestModel)
	ref := withReference(plain)
	assert.True(t, ref.KleinReference)
	assert.Contains(t, ref.Modules, qwenTestVision)
	for _, m := range plain.Modules {
		assert.Contains(t, ref.Modules, m)
	}
	assert.Len(t, ref.Modules, len(plain.Modules)+1)
	assert.False(t, hasMMProj(plain.Modules), "withReference must not mutate the plain spec")
}

func TestKleinModules_ReferenceSpecUnchanged(t *testing.T) {
	t.Parallel()
	plain := modelSpec(kleinTestModel)
	ref := withReference(plain)
	assert.True(t, ref.KleinReference)
	assert.Equal(t, plain.Modules, ref.Modules)
	assert.Equal(t, plain.Checkpoint, ref.Checkpoint)
	assert.False(t, hasMMProj(ref.Modules))
}

func TestHiDreamModules_SingleFile(t *testing.T) {
	t.Parallel()
	assert.Empty(t, modulesForModel(hidreamTestModel))
	assert.Empty(t, withReference(modelSpec(hidreamTestModel)).Modules)
}

func TestHiDream_IsNewGenProseFamily(t *testing.T) {
	t.Parallel()
	assert.True(t, config.IsNewGenModel(hidreamTestModel))
}

func TestResolveEditModel(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "klein"} {
		t.Run("klein/"+in, func(t *testing.T) {
			t.Parallel()
			p, err := resolveEditModel(in)
			require.NoError(t, err)
			assert.Equal(t, "klein", p.family)
			assert.True(t, config.IsFlux2Klein(p.model), p.model)
		})
	}

	t.Run("qwen", func(t *testing.T) {
		t.Parallel()
		p, err := resolveEditModel("qwen")
		require.NoError(t, err)
		assert.True(t, config.IsQwenImage(p.model), p.model)
		assert.NotEqual(t, "klein", p.family)
		assert.Equal(t, 20, p.steps)
		assert.InDelta(t, 6.0, p.cfg, 1e-9)
	})

	for _, in := range []string{"moss", "z-image", "unknown", "Qwen ", "flux-2-klein-4b"} {
		t.Run("rejected/"+in, func(t *testing.T) {
			t.Parallel()
			_, err := resolveEditModel(in)
			assert.Error(t, err)
		})
	}
}

func assertQwenWorkSize(t *testing.T, srcW, srcH, gotW, gotH int) {
	t.Helper()
	assert.Zero(t, gotW%32, "src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	assert.Zero(t, gotH%32, "src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	area := float64(gotW * gotH)
	assert.LessOrEqual(t, area, oneMegapixel*(1+oneMegapixelTol), "src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
	srcRatio := float64(srcW) / float64(srcH)
	gotRatio := float64(gotW) / float64(gotH)
	tol := max(0.02, 32.0/float64(min(gotW, gotH)))
	assert.InDelta(t, srcRatio, gotRatio, srcRatio*tol, "aspect src %dx%d -> %dx%d", srcW, srcH, gotW, gotH)
}

func TestRefEditWorkSize_Qwen(t *testing.T) {
	t.Parallel()
	cases := [][2]int{{3000, 2000}, {2000, 3000}, {4096, 4096}, {1500, 1000}, {5000, 1200}}
	for _, c := range cases {
		w, h := refEditWorkSize(qwenTestModel, c[0], c[1])
		assertQwenWorkSize(t, c[0], c[1], w, h)
		assert.InDelta(t, oneMegapixel, float64(w*h), oneMegapixel*oneMegapixelTol, "src %dx%d -> %dx%d", c[0], c[1], w, h)
	}
}

func TestRefEditWorkSize_Qwen3000x2000(t *testing.T) {
	t.Parallel()
	w, h := refEditWorkSize(qwenTestModel, 3000, 2000)
	assert.Zero(t, w%32)
	assert.Zero(t, h%32)
	assert.InDelta(t, oneMegapixel, float64(w*h), oneMegapixel*oneMegapixelTol)
	assert.Greater(t, w, h)
}

func TestRefEditWorkSize_KleinStays1MP(t *testing.T) {
	t.Parallel()
	w, h := refEditWorkSize(kleinTestModel, 3000, 2000)
	assertKleinWorkSize(t, 3000, 2000, w, h)
}

func runQwenEffect(t *testing.T, srcW, srcH int) sd.Img2ImgRequest {
	t.Helper()
	db := openTestDB(t)
	p := makeTestPreset(t, db, &preset.Preset{
		Name:         "Golden Hour (Qwen-Image)",
		PresetType:   "post_light",
		Prompt:       "golden hour light",
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        20,
		CfgScale:     4.0,
		ModelName:    qwenTestModel,
	})
	rec := &emoRec{}
	svc := newTestService(t, db, emLLM(), rec.mock(t))
	svc.ctx = context.Background()
	svc.sessions = &mockSessions{}
	params := emoParams(t, p.ID, "", "")
	params.ImageBase64 = makePNGBase64(t, srcW, srcH)
	_, err := svc.GenerateFromImage(params)
	require.NoError(t, err)
	reqs := rec.requests()
	require.NotEmpty(t, reqs)
	return reqs[0]
}

func TestQwenEffect_SizeAlignedAndVisionModule(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]int{{3000, 2000}, {600, 800}} {
		req := runQwenEffect(t, c[0], c[1])
		assertQwenWorkSize(t, c[0], c[1], req.Width, req.Height)
		assert.True(t, req.Model.KleinReference, "effect on Qwen must be a reference request")
		assert.Contains(t, req.Model.Modules, qwenTestVision)
	}
}

func TestQwenRemix_VisionOnlyWithReference(t *testing.T) {
	t.Parallel()
	req := runFromImageSize(t, qwenTestModel, 600, 800)
	assert.Equal(t, req.Model.KleinReference, hasMMProj(req.Model.Modules),
		"ref=%v modules=%v", req.Model.KleinReference, req.Model.Modules)
}

func TestLoadEditStyle_QwenFamily(t *testing.T) {
	t.Parallel()
	qwenFamily := func(t *testing.T) string {
		p, err := resolveEditModel("qwen")
		require.NoError(t, err)
		return p.family
	}

	t.Run("klein preset rejected for qwen", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		p := makeTestPreset(t, db, &preset.Preset{Name: "klein-style", ModelName: kleinTestModel, Prompt: "film look"})
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		_, err := svc.loadEditStyle(p.ID, qwenFamily(t))
		assert.Error(t, err)
	})

	t.Run("qwen preset rejected for klein", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		p := makeTestPreset(t, db, &preset.Preset{Name: "qwen-style", ModelName: qwenTestModel, Prompt: "film look"})
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		_, err := svc.loadEditStyle(p.ID, "klein")
		assert.Error(t, err)
	})

	t.Run("qwen preset loads for qwen", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		p := makeTestPreset(t, db, &preset.Preset{Name: "qwen-style-ok", ModelName: qwenTestModel, Prompt: "warm film look"})
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		st, err := svc.loadEditStyle(p.ID, qwenFamily(t))
		require.NoError(t, err)
		require.NotNil(t, st)
		assert.Equal(t, p.ID, st.presetID)
		assert.Equal(t, "warm film look", st.tags)
	})

	t.Run("zero id is no style for qwen", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		st, err := svc.loadEditStyle(0, qwenFamily(t))
		require.NoError(t, err)
		assert.Nil(t, st)
	})

	t.Run("missing preset for qwen", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		svc := newTestService(t, db, &mockLLM{}, &mockSD{})
		_, err := svc.loadEditStyle(999, qwenFamily(t))
		assert.Error(t, err)
	})
}
