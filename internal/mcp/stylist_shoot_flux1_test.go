package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func flux1People() []serverclient.Person {
	return append(shootPeopleAll(), shootPerson("fiona", "Flux.1"))
}

func TestStylistPeople_ListsFlux1(t *testing.T) {
	t.Parallel()
	env := shootNewEnv(t, &fakePeople{people: flux1People()}, &fakeShooter{}, nil)

	m, out := styCall(t, env.s, "stylist_people", map[string]any{})

	byName := map[string]map[string]any{}
	for _, p := range shootPeopleList(t, m) {
		name, _ := p["name"].(string)
		byName[name] = p
	}
	p, ok := byName["fiona"]
	require.True(t, ok, "Flux.1 person missing: %s", out)
	assert.Equal(t, "Flux.1", p["base"])
	assert.Contains(t, byName, "gleb")
	assert.NotContains(t, byName, "dina")
}

func TestStylistPeople_OnlyFlux1(t *testing.T) {
	t.Parallel()
	env := shootNewEnv(t, &fakePeople{people: []serverclient.Person{shootPerson("fiona", "Flux.1")}}, &fakeShooter{}, nil)

	m, out := styCall(t, env.s, "stylist_people", map[string]any{})

	list := shootPeopleList(t, m)
	require.Len(t, list, 1, out)
	assert.Equal(t, "fiona", list[0]["name"])
}

func TestStylistShoot_Flux1Params(t *testing.T) {
	t.Parallel()
	env := shootNewEnv(t, &fakePeople{people: flux1People()}, &fakeShooter{}, nil)

	m, _ := styCall(t, env.s, "stylist_shoot", shootArgs("fiona", "front full body", "side view"))

	calls := env.shooter.genCalls()
	require.Len(t, calls, 2)
	for i, c := range calls {
		assert.Equal(t, []string{"flux1-dev-fp8"}, c.SelectedModels, "call %d", i)
		assert.Equal(t, "euler", c.Sampler, "call %d", i)
		assert.Equal(t, 20, c.Steps, "call %d", i)
		assert.InDelta(t, 1.0, c.CfgScale, 1e-9, "call %d", i)
		assert.Contains(t, c.Prompt, "<lora:fiona_lora_v1:1>", "call %d", i)
	}
	assert.Equal(t, "fiona", m["person"])
	assert.Equal(t, "Flux.1", m["base"])
	assert.Equal(t, "flux1-dev-fp8", m["model"])
}
