package mcp

import (
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStylistAutoToolsReadOnly(t *testing.T) {
	env := newLocalTLEnv(t)
	env.srv.deps.People = &fakePeople{people: shootPeopleAll()}
	name := shootPeopleAll()[0].Name
	env.sd.models = []sd.SDModel{{Name: stylist.Models[shootPeopleAll()[0].Base].Model}}
	options, ok := env.srv.tools["stylist_shoot_options"]
	require.True(t, ok)
	_, err := options.Handler(env.srv, map[string]any{"person": name})
	require.NoError(t, err)
	suggest, ok := env.srv.tools["stylist_suggest"]
	require.True(t, ok)
	_, err = suggest.Handler(env.srv, map[string]any{"plan": map[string]any{"person": name, "frames": 2, "wish": "noir"}})
	require.Error(t, err, "invalid fake LLM response must not become a random successful plan")
	require.Empty(t, env.sd.txt)
	require.Empty(t, env.sd.img)
	require.Zero(t, env.sd.upscales)
}

func TestStylistAutoToolsRejectUnavailableAndRemote(t *testing.T) {
	s := NewServer(Deps{})
	RegisterTools(s)
	for _, name := range []string{"stylist_suggest", "stylist_shoot_options"} {
		tool, ok := s.tools[name]
		require.True(t, ok)
		_, err := tool.Handler(s, map[string]any{"person": "missing"})
		require.Error(t, err)
	}
	env := newLocalTLEnv(t)
	env.srv.deps.Cfg = &config.Config{LLMUrl: "https://example.com", SDUrl: "http://localhost:7860"}
	for _, name := range []string{"stylist_suggest", "stylist_shoot_options"} {
		_, err := env.srv.tools[name].Handler(env.srv, map[string]any{"person": "missing"})
		require.ErrorContains(t, err, "локальной")
	}
}
