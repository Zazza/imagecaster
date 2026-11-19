package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"regexp"
	"sync"
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
	"github.com/Zazza/imagecaster/internal/stylist"
)

var pprLoraRe = regexp.MustCompile(`<lora:([^:>]+):([^>]+)>`)

func pprLoras(prompt string) map[string]string {
	out := map[string]string{}
	for _, m := range pprLoraRe.FindAllStringSubmatch(prompt, -1) {
		out[m[1]] = m[2]
	}
	return out
}

type pprPipelineShooter struct {
	fakeShooter
	mu     sync.Mutex
	params []generation.GenerateCompoundImageParams
	image  string
}

func (f *pprPipelineShooter) GenerateCompoundImage(p generation.GenerateCompoundImageParams) (*generation.GenerateImageResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.params = append(f.params, p)
	return &generation.GenerateImageResult{Image: f.image, Info: json.RawMessage(`{"seed":5}`), EffectivePrompt: p.ScenePrompt}, nil
}

func (f *pprPipelineShooter) compoundCalls() []generation.GenerateCompoundImageParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]generation.GenerateCompoundImageParams(nil), f.params...)
}

type pprSwapEng struct {
	sgEngine
	swapMu  sync.Mutex
	inputs  []string
	persons []string
	out     string
}

func (e *pprSwapEng) FaceSwap(imgB64, person string) (string, string) {
	e.swapMu.Lock()
	defer e.swapMu.Unlock()
	e.inputs = append(e.inputs, imgB64)
	e.persons = append(e.persons, person)
	return e.out, generation.FaceSwapDone
}

func pprFrameParams() ShootFrameParams {
	return ShootFrameParams{
		Person:        "Наташа",
		Shot:          "front full body",
		Prompt:        "slim woman, front full body, red coat, pier",
		Index:         1,
		Total:         1,
		Outfit:        "a red wool coat",
		GarmentBase64: base64.StdEncoding.EncodeToString(styPNG(5)),
		FacePass:      true,
		FaceSwap:      true,
		Pipeline:      42,
		PipelineName:  "Z→SDXL→Flux",
		PersonLoras:   map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"},
		FaceSwapName:  "natasha_curated_flux1",
	}
}

func TestRunShootFrame_PipelineTryOnThenSwap(t *testing.T) {
	t.Parallel()
	shooter := &pprPipelineShooter{image: base64.StdEncoding.EncodeToString(styPNG(4))}
	eng := &pprSwapEng{out: base64.StdEncoding.EncodeToString(styPNG(8))}
	sess := &fakeSessions{}
	refiner := &fakeRefiner{size: 256}
	p := pprFrameParams()

	res, err := RunShootFrame(shooter, sess, eng, refiner, p)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, shooter.genCalls())
	calls := shooter.compoundCalls()
	require.Len(t, calls, 1)
	c := calls[0]
	assert.Equal(t, int64(42), c.CompoundPresetID)
	assert.Equal(t, p.Prompt, c.ScenePrompt)
	require.NotNil(t, c.Person)
	assert.Equal(t, p.PersonLoras, c.Person.Loras)
	assert.False(t, c.Person.FaceSwap)

	tries := eng.calls()
	require.Len(t, tries, 1)
	assert.Equal(t, shooter.image, tries[0].image)
	assert.Equal(t, styPNG(5), styDecodeB64(t, tries[0].garment))
	assert.Equal(t, "done", res.Garment)

	require.Len(t, eng.inputs, 1)
	assert.Equal(t, eng.tryOnResults()[0], eng.inputs[0])
	assert.Equal(t, []string{"natasha_curated_flux1"}, eng.persons)
	assert.Equal(t, generation.FaceSwapDone, res.FaceSwap)

	assert.Empty(t, refiner.refineCalls())
	assert.NotEqual(t, shootFaceDone, res.FacePass)

	named := sess.namedCalls()
	require.Len(t, named, 1)
	assert.Equal(t, StylistSessionName, named[0].name)
	assert.Equal(t, eng.out, named[0].image)
}

func TestRunShootFrame_PipelineNeedsPipelineShooter(t *testing.T) {
	t.Parallel()
	shooter := &fakeShooter{}
	sess := &fakeSessions{}
	eng := &pprSwapEng{out: base64.StdEncoding.EncodeToString(styPNG(8))}

	_, err := RunShootFrame(shooter, sess, eng, &fakeRefiner{size: 256}, pprFrameParams())

	require.Error(t, err)
	assert.Empty(t, shooter.genCalls())
	assert.Empty(t, sess.namedCalls())
	assert.Empty(t, eng.calls())
	assert.Empty(t, eng.inputs)
}

type pprSD struct {
	tlSD
	swapMu  sync.Mutex
	order   []string
	swapIn  []string
	persons []string
	out     string
}

func (f *pprSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.swapMu.Lock()
	f.order = append(f.order, "step")
	f.swapMu.Unlock()
	return f.tlSD.Txt2Img(req)
}

func (f *pprSD) Img2Img(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.swapMu.Lock()
	f.order = append(f.order, "step")
	f.swapMu.Unlock()
	return f.tlSD.Img2Img(req)
}

func (f *pprSD) FaceSwap(_ context.Context, img, person string, _ float64) (*sd.FaceSwapResult, error) {
	f.swapMu.Lock()
	defer f.swapMu.Unlock()
	f.order = append(f.order, "swap")
	f.swapIn = append(f.swapIn, img)
	f.persons = append(f.persons, person)
	return &sd.FaceSwapResult{Status: sd.FaceSwapDone, Image: f.out}, nil
}

func (f *pprSD) prompts() []string {
	var out []string
	for _, r := range f.txtReqs() {
		out = append(out, r.Prompt)
	}
	for _, r := range f.imgReqs() {
		out = append(out, r.Prompt)
	}
	return out
}

type pprEnv struct {
	srv *Server
	db  *preset.DB
	sd  *pprSD
	llm *tlLLM
}

func pprNewEnv(t *testing.T, people []serverclient.Person) *pprEnv {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	fsd := &pprSD{out: base64.StdEncoding.EncodeToString(styPNG(7))}
	fllm := &tlLLM{}
	cfg := &config.Config{LLMUrl: "http://192.168.1.10:11434", SDUrl: "http://192.168.1.10:7860", SDPromptModel: "prompt-model", VisionModel: "vision-model"}
	gen := generation.New(db, fllm, fsd, cfg, t.TempDir(), NoopEmitter{}, kids.NewManager(db), tlSessions{}, logger.New(nil))
	srv := NewServer(Deps{DB: db, Gen: gen, SD: fsd, LLM: fllm, Kids: kids.NewManager(db), Cfg: cfg, OutDir: t.TempDir(), People: &fakePeople{people: people}})
	RegisterTools(srv)
	return &pprEnv{srv: srv, db: db, sd: fsd, llm: fllm}
}

func (e *pprEnv) preset(t *testing.T, name, base, loras string) *preset.Preset {
	t.Helper()
	m := stylist.Models[base]
	p := &preset.Preset{Name: name, PresetType: "realism", Prompt: "photo", ModelName: m.Model, Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG, Loras: loras}
	require.NoError(t, e.db.Create(p))
	return p
}

func TestTools_GeneratePersonByName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		base     string
		person   string
		wantLora map[string]string
		wantErr  bool
	}{
		{"name on z-image style", "Z-Image", "Наташа", map[string]string{"natasha_curated": "1"}, false},
		{"name on flux.1 style", "Flux.1", "Наташа", map[string]string{"natasha_curated_flux1": "1"}, false},
		{"name case and spaces", "Moss-Image", " наташа ", map[string]string{"natasha_curated_moss": "1"}, false},
		{"old lora name", "Z-Image", "natasha_curated", map[string]string{"natasha_curated": "1"}, false},
		{"no lora for base", "SDXL", "Наташа", nil, true},
		{"unknown name", "Z-Image", "Мария", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := pprNewEnv(t, pnNatasha(false, false))
			p := e.preset(t, "style "+tt.base, tt.base, "")
			args := map[string]any{"preset_id": float64(p.ID), "seed": float64(3), "overrides": map[string]any{"person": map[string]any{"name": tt.person}}}

			out, err := styExec(e.srv, "generate", args)

			if tt.wantErr {
				require.Error(t, err, out)
				assert.Zero(t, e.sd.calls())
				return
			}
			require.NoError(t, err, out)
			reqs := e.sd.txtReqs()
			require.Len(t, reqs, 1)
			assert.Equal(t, tt.wantLora, pprLoras(reqs[0].Prompt), reqs[0].Prompt)
			assert.Empty(t, e.sd.persons)
		})
	}
}

func TestTools_CompoundGeneratePerson(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		fluxID    bool
		faceSwap  bool
		wantSwaps []string
		wantOrder []string
	}{
		{"face swap after last step", true, true, []string{"natasha_curated_flux1"}, []string{"step", "step", "step", "swap"}},
		{"face swap off", true, false, nil, []string{"step", "step", "step"}},
		{"no identity no swap", false, true, nil, []string{"step", "step", "step"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := pprNewEnv(t, pnNatasha(false, tt.fluxID))
			z := e.preset(t, "z", "Z-Image", "")
			x := e.preset(t, "x", "SDXL", `[{"name":"film","weight":0.55}]`)
			f := e.preset(t, "f", "Flux.1", "")
			cp := &preset.CompoundPreset{Name: "Цепочка", Steps: []preset.CompoundPresetStep{
				{StepOrder: 1, PresetID: z.ID},
				{StepOrder: 2, PresetID: x.ID, DenoisingStrength: 0.5},
				{StepOrder: 3, PresetID: f.ID, DenoisingStrength: 0.4},
			}}
			require.NoError(t, e.db.CreateCompoundPreset(cp))

			m, out := styCall(t, e.srv, "compound_generate", map[string]any{
				"compound_preset_id": float64(cp.ID),
				"description":        "a woman on a pier",
				"seed":               float64(2),
				"person":             "Наташа",
				"face_swap":          tt.faceSwap,
			})

			prompts := e.sd.prompts()
			require.Len(t, prompts, 3, out)
			assert.Equal(t, map[string]string{"natasha_curated": "1"}, pprLoras(prompts[0]))
			assert.Equal(t, map[string]string{"film": "0.55"}, pprLoras(prompts[1]))
			assert.Equal(t, map[string]string{"natasha_curated_flux1": "1"}, pprLoras(prompts[2]))
			assert.Equal(t, tt.wantSwaps, e.sd.persons)
			assert.Equal(t, tt.wantOrder, e.sd.order)
			path, _ := m["path"].(string)
			require.NotEmpty(t, path)
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			if tt.wantSwaps != nil {
				assert.Equal(t, styPNG(7), raw)
			} else {
				assert.NotEqual(t, styPNG(7), raw)
			}
		})
	}
}

type pprTrainer struct {
	ptFakeTrainer
	dispMu   sync.Mutex
	displays []string
}

func (f *pprTrainer) Train(ctx context.Context, folder, name, display, gender, base string, hold bool) (*serverclient.TrainJob, error) {
	f.dispMu.Lock()
	f.displays = append(f.displays, display)
	f.dispMu.Unlock()
	return f.ptFakeTrainer.Train(ctx, folder, name, display, gender, base, hold)
}

func TestPeopleTrain_DisplayNamePassedThrough(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		set  any
		want string
	}{
		{"cyrillic name", "Наташа", "Наташа"},
		{"no name", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tr := &pprTrainer{ptFakeTrainer: ptFakeTrainer{bases: ptBases()}}
			s := ptServer(t, tr)
			args := map[string]any{"folder": t.TempDir(), "name": "natasha_z", "gender": "woman", "base": "Z-Image"}
			if tt.set != nil {
				args["display_name"] = tt.set
			}

			styCall(t, s, "people_train", args)

			assert.Equal(t, []string{tt.want}, tr.displays)
			require.Len(t, tr.trainCalls(), 1)
		})
	}
}
