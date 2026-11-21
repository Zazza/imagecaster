package main

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/Zazza/imagecaster/internal/preset"
)

const (
	stylistPlanNameMax = 80
	stylistPlanMaxSize = 512 << 10
)

var (
	errStylistPlanName = errors.New("название плана: от 1 до 80 символов")
	errStylistPlanData = errors.New("план повреждён или слишком большой")
)

func validStylistPlan(name, plan string) (string, error) {
	name = preset.StylistPlanName(name)
	if name == "" || utf8.RuneCountInString(name) > stylistPlanNameMax {
		return "", errStylistPlanName
	}
	var obj map[string]any
	if len(plan) > stylistPlanMaxSize || json.Unmarshal([]byte(plan), &obj) != nil || obj == nil {
		return "", errStylistPlanData
	}
	return name, nil
}

func (a *App) StylistPlans() ([]preset.StylistPlan, error) {
	if a.presets == nil {
		return nil, errStylistNoDB
	}
	return a.presets.ListStylistPlans()
}

func (a *App) StylistSaveNamedPlan(name, plan string, replace bool) (*preset.StylistPlan, error) {
	if a.presets == nil {
		return nil, errStylistNoDB
	}
	name, err := validStylistPlan(name, plan)
	if err != nil {
		return nil, err
	}
	return a.presets.SaveStylistPlan(name, plan, replace)
}

func (a *App) StylistRenamePlan(id int64, name string) (*preset.StylistPlan, error) {
	if a.presets == nil {
		return nil, errStylistNoDB
	}
	name, err := validStylistPlan(name, "{}")
	if err != nil {
		return nil, err
	}
	return a.presets.RenameStylistPlan(id, name)
}

func (a *App) StylistDeletePlan(id int64) error {
	if a.presets == nil {
		return errStylistNoDB
	}
	return a.presets.DeleteStylistPlan(id)
}
