package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/stylist"
)

type stylistPresetStore interface {
	ListStylistPresets() ([]preset.StylistPreset, error)
	CreateStylistPreset(p *preset.StylistPreset) error
	RenameStylistPreset(id int64, name string) error
	DeleteStylistPreset(id int64) error
}

func stylistUserBase(store stylistPresetStore, base *stylist.Base, logf func(error)) (*stylist.Base, error) {
	rows, err := store.ListStylistPresets()
	if err != nil {
		return nil, fmt.Errorf("свои заготовки стилиста: %w", err)
	}
	list := make([]stylist.UserPreset, 0, len(rows))
	for _, r := range rows {
		list = append(list, stylist.UserPreset{
			ID: r.ID, Aspect: stylist.Aspect(r.Aspect), Name: r.Name, Prompt: r.Prompt, PromptRu: r.PromptRu,
			Place: r.Place, Framing: r.Framing, Places: r.Places, Orientation: r.Orientation,
		})
	}
	merged, err := base.WithUser(list)
	if err != nil && logf != nil {
		logf(err)
	}
	return merged, nil
}

func saveStylistPreset(store stylistPresetStore, base *stylist.Base, p stylist.UserPreset) (string, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Prompt = strings.TrimSpace(p.Prompt)
	p.PromptRu = strings.TrimSpace(p.PromptRu)
	if err := base.CheckUser(p); err != nil {
		return "", err
	}
	row := preset.StylistPreset{Aspect: string(p.Aspect), Name: p.Name, Prompt: p.Prompt, PromptRu: p.PromptRu}
	switch p.Aspect {
	case stylist.AspectBackground:
		row.Place = p.Place
	case stylist.AspectPose:
		row.Framing, row.Places, row.Orientation = p.Framing, p.Places, p.Orientation
	}
	if err := store.CreateStylistPreset(&row); err != nil {
		return "", err
	}
	return stylist.UserID(row.ID), nil
}

func parseStylistUserID(id string) (int64, error) {
	n, ok := stylist.ParseUserID(id)
	if !ok {
		return 0, fmt.Errorf("%q — не своя заготовка", id)
	}
	return n, nil
}

func renameStylistPreset(store stylistPresetStore, id, name string) error {
	n, err := parseStylistUserID(id)
	if err != nil {
		return err
	}
	if err := stylist.CheckUserName(name); err != nil {
		return err
	}
	return store.RenameStylistPreset(n, strings.TrimSpace(name))
}

func deleteStylistPreset(store stylistPresetStore, id string) error {
	n, err := parseStylistUserID(id)
	if err != nil {
		return err
	}
	return store.DeleteStylistPreset(n)
}

var errStylistNoDB = errors.New("база приложения недоступна")

func (a *App) stylistBase() (*stylist.Base, error) {
	base, err := stylist.Load()
	if err != nil {
		return nil, err
	}
	if a.presets == nil {
		return base, nil
	}
	return stylistUserBase(a.presets, base, func(err error) {
		if a.log != nil {
			a.log.Warn("stylist page: user presets: %s", err)
		}
	})
}

func (a *App) StylistSavePreset(p stylist.UserPreset) (string, error) {
	if a.presets == nil {
		return "", errStylistNoDB
	}
	base, err := stylist.Load()
	if err != nil {
		return "", err
	}
	return saveStylistPreset(a.presets, base, p)
}

func (a *App) StylistRenamePreset(id, name string) error {
	if a.presets == nil {
		return errStylistNoDB
	}
	return renameStylistPreset(a.presets, id, name)
}

func (a *App) StylistDeletePreset(id string) error {
	if a.presets == nil {
		return errStylistNoDB
	}
	return deleteStylistPreset(a.presets, id)
}
