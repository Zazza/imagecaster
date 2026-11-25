package main

import (
	"errors"

	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/stylist"
)

func (a *App) StylistShootOptions(person string) ([]stylist.ShootOption, error) {
	if a.presets == nil || a.serverClient == nil || a.sd == nil {
		return nil, errors.New("сервисы автоподбора недоступны")
	}
	all, err := a.ListPeople()
	if err != nil {
		return nil, err
	}
	models, err := a.sd.GetModels()
	if err != nil {
		return nil, err
	}
	loras, err := a.sd.GetLoRAs()
	if err != nil {
		return nil, err
	}
	presets, err := a.presets.List()
	if err != nil {
		return nil, err
	}
	compounds, err := a.presets.ListCompoundPresetsFull()
	if err != nil {
		return nil, err
	}
	return stylist.BuildShootOptions(all, person, presets, compounds, models, loras)
}

func (a *App) StylistSuggest(req stylist.SuggestRequest) (*stylist.SuggestResult, error) {
	if llmNotLocal(a.config) {
		return nil, errors.New("автоподбор доступен только с локальным ИИ")
	}
	options, err := a.StylistShootOptions(req.Plan.Person)
	if err != nil {
		return nil, err
	}
	base, err := a.stylistBase()
	if err != nil {
		return nil, err
	}
	if a.gen == nil {
		return nil, errors.New("ИИ для автоподбора недоступен")
	}
	model, err := a.gen.StylistPlannerModel()
	if err != nil {
		return nil, err
	}
	return stylist.Suggest(llm.ForPlanning(a.llm), model, req, base, options)
}
