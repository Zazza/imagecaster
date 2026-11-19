package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
)

func pnPerson(lora, base, display string, identity bool) serverclient.Person {
	return serverclient.Person{Name: lora, Lora: lora, Base: base, DisplayName: display, Gender: "woman", Identity: identity, Steps: 2000, CreatedAt: shootTime()}
}

func pnNatasha(zID, fluxID bool) []serverclient.Person {
	return []serverclient.Person{
		pnPerson("natasha_curated", "Z-Image", "Наташа", zID),
		pnPerson("natasha_curated_moss", "Moss-Image", "Наташа", false),
		pnPerson("natasha_curated_flux1", "Flux.1", "Наташа", fluxID),
		pnPerson("ivan_x", "SDXL", "", false),
	}
}

func pnModels(bases ...string) []string {
	out := make([]string, 0, len(bases))
	for _, b := range bases {
		out = append(out, stylist.Models[b].Model)
	}
	return out
}

type pnEnv struct {
	s     *Server
	db    *preset.DB
	store *queue.Store
}

func pnNewEnv(t *testing.T, people []serverclient.Person, withQueue bool) *pnEnv {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	env := &pnEnv{db: db}
	deps := Deps{OutDir: t.TempDir(), DB: db, People: &fakePeople{people: people}, Shooter: &fakeShooter{}, Sessions: &fakeSessions{}, Stylist: &sgEngine{}, Refiner: &fakeRefiner{size: 256}}
	if withQueue {
		env.store = queue.NewStore(db.DB())
		deps.Queue = env.store
	}
	env.s = NewServer(deps)
	RegisterTools(env.s)
	return env
}

func (e *pnEnv) pipeline(t *testing.T, name string, bases ...string) int64 {
	t.Helper()
	cp := &preset.CompoundPreset{Name: name}
	for i, m := range pnModels(bases...) {
		p := &preset.Preset{Name: name + " " + bases[i], PresetType: "realism", Prompt: "photo", ModelName: m, Sampler: "euler", Steps: 8, CfgScale: 1}
		require.NoError(t, e.db.Create(p))
		cp.Steps = append(cp.Steps, preset.CompoundPresetStep{PresetID: p.ID, StepOrder: i + 1})
	}
	require.NoError(t, e.db.CreateCompoundPreset(cp))
	return cp.ID
}

func pnArgs(person string, extra map[string]any) map[string]any {
	args := shootArgs(person, "front full body")
	delete(args, "face_pass")
	for k, v := range extra {
		args[k] = v
	}
	return args
}

func TestResolvePersonByName(t *testing.T) {
	tests := []struct {
		name     string
		person   string
		models   []string
		faceSwap bool
		zID      bool
		fluxID   bool
		wantErr  []string
		loras    map[string]string
		swap     bool
		swapName string
	}{
		{"style model z-image", "Наташа", pnModels("Z-Image"), false, false, false, nil, map[string]string{"Z-Image": "natasha_curated"}, false, ""},
		{"style model flux.1", "Наташа", pnModels("Flux.1"), false, false, false, nil, map[string]string{"Flux.1": "natasha_curated_flux1"}, false, ""},
		{"pipeline z sdxl flux", "Наташа", pnModels("Z-Image", "SDXL", "Flux.1"), false, false, false, nil, map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"}, false, ""},
		{"case and spaces of name", "  наташа ", pnModels("Z-Image"), false, false, false, nil, map[string]string{"Z-Image": "natasha_curated"}, false, ""},
		{"lora without name keyed by lora name", "ivan_x", pnModels("SDXL"), false, false, false, nil, map[string]string{"SDXL": "ivan_x"}, false, ""},
		{"no lora for any step", "Наташа", pnModels("SDXL"), false, false, false, []string{}, nil, false, ""},
		{"unknown name", "Мария", pnModels("Z-Image"), false, false, false, []string{"Мария", "Наташа", "ivan_x"}, nil, false, ""},
		{"identity on second lora by step order", "Наташа", pnModels("Z-Image", "SDXL", "Flux.1"), true, false, true, nil, map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"}, true, "natasha_curated_flux1"},
		{"identity on both takes first step", "Наташа", pnModels("Flux.1", "Z-Image"), true, true, true, nil, map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"}, true, "natasha_curated_flux1"},
		{"no identity no face swap", "Наташа", pnModels("Z-Image", "Flux.1"), true, false, false, nil, map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"}, false, ""},
		{"checkbox off", "Наташа", pnModels("Z-Image"), false, true, true, nil, map[string]string{"Z-Image": "natasha_curated"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ov, err := resolvePerson(pnNatasha(tt.zID, tt.fluxID), tt.person, tt.models, tt.faceSwap)
			if tt.wantErr != nil {
				require.Error(t, err)
				for _, s := range tt.wantErr {
					assert.Contains(t, err.Error(), s)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, ov)
			assert.Equal(t, tt.loras, ov.Loras)
			assert.Equal(t, tt.swap, ov.FaceSwap)
			if tt.swap {
				assert.Equal(t, tt.swapName, ov.FaceSwapName)
			}
		})
	}
}

func TestStylistShootName_ByBase(t *testing.T) {
	tests := []struct {
		name      string
		people    []serverclient.Person
		person    string
		extra     map[string]any
		wantErr   []string
		wantLora  string
		wantBase  string
		wantModel string
	}{
		{"several loras without base lists bases", pnNatasha(false, false), "Наташа", nil, []string{"Z-Image", "Moss-Image", "Flux.1"}, "", "", ""},
		{"base given", pnNatasha(false, false), "Наташа", map[string]any{"base": "Moss-Image"}, nil, "natasha_curated_moss", "Moss-Image", stylist.Models["Moss-Image"].Model},
		{"base without lora", pnNatasha(false, false), "Наташа", map[string]any{"base": "SDXL"}, []string{"SDXL"}, "", "", ""},
		{"single lora without base", []serverclient.Person{pnPerson("olga_z", "Z-Image", "Ольга", false)}, "Ольга", nil, nil, "olga_z", "Z-Image", stylist.Models["Z-Image"].Model},
		{"old lora name", pnNatasha(false, false), "natasha_curated_flux1", nil, nil, "natasha_curated_flux1", "Flux.1", stylist.Models["Flux.1"].Model},
		{"lora without name", pnNatasha(false, false), "ivan_x", nil, nil, "ivan_x", "SDXL", stylist.Models["SDXL"].Model},
		{"unknown name lists names", pnNatasha(false, false), "Мария", nil, []string{"Мария", "Наташа", "ivan_x"}, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := pnNewEnv(t, tt.people, true)
			out, err := styExec(env.s, "stylist_shoot", pnArgs(tt.person, tt.extra))
			if tt.wantErr != nil {
				require.Error(t, err, out)
				for _, s := range tt.wantErr {
					assert.Contains(t, err.Error(), s)
				}
				n, err := env.store.ActiveCount()
				require.NoError(t, err)
				assert.Zero(t, n)
				return
			}
			require.NoError(t, err)
			_, p := sqJob(t, env.store, pnJobIDs(t, out)[0])
			assert.Equal(t, tt.wantLora, p.Person)
			assert.Equal(t, tt.wantBase, p.Base)
			assert.Equal(t, tt.wantModel, p.Model)
			assert.Zero(t, p.Pipeline)
			assert.Empty(t, p.PersonLoras)
		})
	}
}

func TestStylistShootName_Pipeline(t *testing.T) {
	tests := []struct {
		name     string
		zID      bool
		fluxID   bool
		bases    []string
		faceSwap bool
		wantErr  bool
		wantSwap bool
		swapName string
	}{
		{"loras on z and flux, identity on flux", false, true, []string{"Z-Image", "SDXL", "Flux.1"}, true, false, true, "natasha_curated_flux1"},
		{"identity on both, first by step order", true, true, []string{"Z-Image", "SDXL", "Flux.1"}, true, false, true, "natasha_curated"},
		{"no identity, no face swap", false, false, []string{"Z-Image", "SDXL", "Flux.1"}, true, false, false, ""},
		{"no step with lora", true, true, []string{"SDXL"}, true, true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := pnNewEnv(t, pnNatasha(tt.zID, tt.fluxID), true)
			id := env.pipeline(t, "Цепочка", tt.bases...)
			out, err := styExec(env.s, "stylist_shoot", pnArgs("Наташа", map[string]any{"pipeline": float64(id), "face_swap": tt.faceSwap}))
			if tt.wantErr {
				require.Error(t, err, out)
				n, err := env.store.ActiveCount()
				require.NoError(t, err)
				assert.Zero(t, n)
				return
			}
			require.NoError(t, err)
			_, p := sqJob(t, env.store, pnJobIDs(t, out)[0])
			assert.Equal(t, id, p.Pipeline)
			assert.Equal(t, "Цепочка", p.PipelineName)
			assert.Equal(t, map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"}, p.PersonLoras)
			assert.NotContains(t, p.Prompt, "<lora:")
			assert.Equal(t, tt.wantSwap, p.FaceSwap)
			if tt.wantSwap {
				assert.Equal(t, tt.swapName, p.FaceSwapName)
			}
		})
	}
}

func TestStylistShootName_PipelineSyncPathRejected(t *testing.T) {
	env := pnNewEnv(t, pnNatasha(true, true), false)
	id := env.pipeline(t, "Цепочка", "Z-Image", "Flux.1")
	_, err := styExec(env.s, "stylist_shoot", pnArgs("Наташа", map[string]any{"pipeline": float64(id)}))
	require.Error(t, err)
}

func TestStylistPeopleName_GroupsLorasAndPipelines(t *testing.T) {
	env := pnNewEnv(t, pnNatasha(false, true), true)
	with := env.pipeline(t, "Z→SDXL→Flux", "Z-Image", "SDXL", "Flux.1")
	env.pipeline(t, "Только SDXL", "SDXL")
	m, _ := styCall(t, env.s, "stylist_people", map[string]any{})
	list := shootPeopleList(t, m)
	byName := map[string]map[string]any{}
	for _, p := range list {
		byName[p["name"].(string)] = p
	}
	require.Len(t, byName, 2)
	require.Contains(t, byName, "Наташа")
	require.Contains(t, byName, "ivan_x")
	nat := byName["Наташа"]
	loras, ok := nat["loras"].([]any)
	require.True(t, ok)
	got := map[string][2]any{}
	for _, l := range loras {
		lm := l.(map[string]any)
		got[lm["lora"].(string)] = [2]any{lm["base"], lm["face"]}
	}
	assert.Equal(t, map[string][2]any{
		"natasha_curated":       {"Z-Image", false},
		"natasha_curated_moss":  {"Moss-Image", false},
		"natasha_curated_flux1": {"Flux.1", true},
	}, got)
	pipes, ok := nat["pipelines"].([]any)
	require.True(t, ok)
	require.Len(t, pipes, 1)
	pm := pipes[0].(map[string]any)
	assert.Equal(t, float64(with), pm["id"])
	assert.Equal(t, "Z→SDXL→Flux", pm["name"])
	assert.Equal(t, float64(3), pm["steps"])
	assert.Equal(t, float64(2), pm["steps_with_lora"])
	ivanPipes, ok := byName["ivan_x"]["pipelines"].([]any)
	require.True(t, ok)
	assert.Len(t, ivanPipes, 2)
}

func pnJobIDs(t *testing.T, out string) []int64 {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m), out)
	ids := sqJobIDs(t, m)
	require.NotEmpty(t, ids)
	return ids
}
