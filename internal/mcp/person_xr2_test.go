package mcp

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestStylistShootPipeline_NameVsLoraXR2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		person string
		want   map[string]string
	}{
		{"lora name of alina picks alina", "alina", map[string]string{"Z-Image": "alina"}},
		{"display name Анна picks anna", "Анна", map[string]string{"Z-Image": "anna"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			people := []serverclient.Person{
				pnPerson("anna", "Z-Image", "Анна", false),
				pnPerson("alina", "Z-Image", "anna", false),
			}
			env := pnNewEnv(t, people, true)
			id := env.pipeline(t, "Цепочка", "Z-Image", "Z-Image")

			out, err := styExec(env.s, "stylist_shoot", pnArgs(tt.person, map[string]any{"pipeline": float64(id)}))

			require.NoError(t, err, out)
			_, p := sqJob(t, env.store, pnJobIDs(t, out)[0])
			assert.Equal(t, id, p.Pipeline)
			assert.Equal(t, tt.want, p.PersonLoras)
		})
	}
}

type xr2SwapSD struct {
	pprSD
	status string
	skip   string
}

func (f *xr2SwapSD) FaceSwap(_ context.Context, img, person string, _ float64) (*sd.FaceSwapResult, error) {
	f.swapMu.Lock()
	defer f.swapMu.Unlock()
	f.persons = append(f.persons, person)
	if f.status == sd.FaceSwapDone {
		return &sd.FaceSwapResult{Status: sd.FaceSwapDone, Image: f.out}, nil
	}
	return &sd.FaceSwapResult{Status: f.status, Skip: f.skip}, nil
}

func xr2NewEnv(t *testing.T, people []serverclient.Person, fsd *xr2SwapSD) *pprEnv {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	fllm := &tlLLM{}
	cfg := &config.Config{LLMUrl: "http://192.168.1.10:11434", SDUrl: "http://192.168.1.10:7860", SDPromptModel: "prompt-model", VisionModel: "vision-model"}
	gen := generation.New(db, fllm, fsd, cfg, t.TempDir(), NoopEmitter{}, kids.NewManager(db), tlSessions{}, logger.New(nil))
	srv := NewServer(Deps{DB: db, Gen: gen, SD: fsd, LLM: fllm, Kids: kids.NewManager(db), Cfg: cfg, OutDir: t.TempDir(), People: &fakePeople{people: people}})
	RegisterTools(srv)
	return &pprEnv{srv: srv, db: db, sd: &fsd.pprSD, llm: fllm}
}

func TestTools_CompoundGenerateFaceSwapStatusXR2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fluxID   bool
		person   any
		faceSwap any
		sdStatus string
		sdSkip   string
		want     any
	}{
		{"swap done", true, "Наташа", true, sd.FaceSwapDone, "", generation.FaceSwapDone},
		{"swap skipped by server", true, "Наташа", true, sd.FaceSwapSkipped, "no_face", generation.FaceSwapSkipped("no_face")},
		{"person without a face", false, "Наташа", true, sd.FaceSwapDone, "", "skipped:no_identity"},
		{"face swap off", true, "Наташа", false, sd.FaceSwapDone, "", nil},
		{"face swap absent", true, "Наташа", nil, sd.FaceSwapDone, "", nil},
		{"no person", true, nil, true, sd.FaceSwapDone, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fsd := &xr2SwapSD{pprSD: pprSD{out: base64.StdEncoding.EncodeToString(styPNG(7))}, status: tt.sdStatus, skip: tt.sdSkip}
			e := xr2NewEnv(t, pnNatasha(false, tt.fluxID), fsd)
			z := e.preset(t, "z", "Z-Image", "")
			f := e.preset(t, "f", "Flux.1", "")
			cp := &preset.CompoundPreset{Name: "Цепочка", Steps: []preset.CompoundPresetStep{
				{StepOrder: 1, PresetID: z.ID},
				{StepOrder: 2, PresetID: f.ID, DenoisingStrength: 0.4},
			}}
			require.NoError(t, e.db.CreateCompoundPreset(cp))
			args := map[string]any{"compound_preset_id": float64(cp.ID), "description": "a woman on a pier", "seed": float64(2)}
			if tt.person != nil {
				args["person"] = tt.person
			}
			if tt.faceSwap != nil {
				args["face_swap"] = tt.faceSwap
			}

			m, out := styCall(t, e.srv, "compound_generate", args)

			got, ok := m["face_swap"]
			if tt.want == nil {
				assert.False(t, ok, out)
				return
			}
			require.True(t, ok, out)
			assert.Equal(t, tt.want, got, out)
		})
	}
}

func TestStylistPeople_CaseInsensitiveGroupXR2(t *testing.T) {
	t.Parallel()
	list := []serverclient.Person{
		{Name: "nat_z", Lora: "nat_z", Base: "Z-Image", DisplayName: "Наташа", Gender: "woman"},
		{Name: "nat_m", Lora: "nat_m", Base: "Moss-Image", DisplayName: "наташа", Gender: "woman"},
	}
	env := shootNewEnv(t, &fakePeople{people: list}, &fakeShooter{}, nil)
	m, out := styCall(t, env.s, "stylist_people", map[string]any{})
	got := shootPeopleList(t, m)
	require.Len(t, got, 1, out)
	loras, ok := got[0]["loras"].([]any)
	require.True(t, ok, out)
	assert.Len(t, loras, 2)
}
