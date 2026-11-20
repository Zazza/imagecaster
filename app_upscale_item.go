package main

import (
	"encoding/json"
	"errors"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
)

type upscaleItemSource interface {
	GetSessionItem(id int64) (*preset.SessionItem, error)
}

type upscaleImageReader interface {
	GetSessionImage(id int64) (string, error)
}

type upscaler interface {
	UpscaleImage(params generation.UpscaleImageParams) (*generation.GenerateImageResult, error)
}

func sessionItemGenInfo(item *preset.SessionItem) (string, error) {
	info := map[string]any{
		"prompt":          item.Prompt,
		"negative_prompt": item.NegativePrompt,
		"sampler":         item.Sampler,
		"steps":           item.Steps,
		"cfg_scale":       item.CfgScale,
		"width":           item.Width,
		"height":          item.Height,
	}
	if item.Seed != nil {
		info["seed"] = *item.Seed
	}
	if item.Meta != nil && item.Meta.Model != "" {
		info["checkpoint"] = item.Meta.Model
	}
	raw, err := json.Marshal(info)
	return string(raw), err
}

func upscaleSessionItem(items upscaleItemSource, images upscaleImageReader, up upscaler, itemID, presetID int64) (*generation.GenerateImageResult, error) {
	item, err := items.GetSessionItem(itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, errors.New("кадра нет в истории")
	}
	img, err := images.GetSessionImage(itemID)
	if err != nil {
		return nil, err
	}
	info, err := sessionItemGenInfo(item)
	if err != nil {
		return nil, err
	}
	return up.UpscaleImage(generation.UpscaleImageParams{ImageBase64: img, GenInfo: info, PresetID: presetID})
}

func (a *App) UpscaleSessionItem(itemID, presetID int64) (*GenerateImageResult, error) {
	if a.presets == nil || a.sessions == nil || a.gen == nil {
		return nil, errors.New("история недоступна")
	}
	return upscaleSessionItem(a.presets, a.sessions, a.gen, itemID, presetID)
}
