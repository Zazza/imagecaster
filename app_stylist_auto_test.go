package main

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/require"
)

func TestStylistSuggestDoesNotSendPlanningContextToRemoteLLM(t *testing.T) {
	app := &App{config: &config.Config{LLMUrl: "https://example.com"}}
	result, err := app.StylistSuggest(stylist.SuggestRequest{Plan: stylist.PlanRequest{Person: "private person", Wish: "private wish", Frames: 1}})
	require.ErrorContains(t, err, "локальным ИИ")
	require.Nil(t, result)
}

func TestStylistAutoServicesUnavailableReturnsError(t *testing.T) {
	app := &App{}
	options, err := app.StylistShootOptions("person")
	require.Error(t, err)
	require.Nil(t, options)
	suggestion, err := app.StylistSuggest(stylist.SuggestRequest{Plan: stylist.PlanRequest{Person: "person", Frames: 1}})
	require.Error(t, err)
	require.Nil(t, suggestion)
}
