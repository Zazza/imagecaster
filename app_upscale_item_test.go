package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
)

type uiItems map[int64]*preset.SessionItem

func (u uiItems) GetSessionItem(id int64) (*preset.SessionItem, error) { return u[id], nil }

type uiImages struct{ err error }

func (u uiImages) GetSessionImage(id int64) (string, error) { return "IMG", u.err }

type uiUp struct {
	got []generation.UpscaleImageParams
}

func (u *uiUp) UpscaleImage(p generation.UpscaleImageParams) (*generation.GenerateImageResult, error) {
	u.got = append(u.got, p)
	return &generation.GenerateImageResult{Image: "BIG", SessionItemID: 77}, nil
}

func TestUpscaleSessionItem_BuildsInfoFromHistory(t *testing.T) {
	seed := int64(42)
	items := uiItems{5: {ID: 5, Prompt: "a woman", NegativePrompt: "blurry", Sampler: "euler", Steps: 8, CfgScale: 1, Width: 832, Height: 1216, Seed: &seed, Meta: &preset.GenerationMeta{Model: "z-image-turbo-fp8-aio"}}}
	up := &uiUp{}
	res, err := upscaleSessionItem(items, uiImages{}, up, 5, 3)
	require.NoError(t, err)
	assert.Equal(t, int64(77), res.SessionItemID)
	require.Len(t, up.got, 1)
	assert.Equal(t, "IMG", up.got[0].ImageBase64)
	assert.Equal(t, int64(3), up.got[0].PresetID)
	var info map[string]any
	require.NoError(t, json.Unmarshal([]byte(up.got[0].GenInfo), &info))
	assert.Equal(t, float64(832), info["width"])
	assert.Equal(t, float64(1216), info["height"])
	assert.Equal(t, "a woman", info["prompt"])
	assert.Equal(t, float64(42), info["seed"])
	assert.Equal(t, "z-image-turbo-fp8-aio", info["checkpoint"])
}

func TestUpscaleSessionItem_Refusals(t *testing.T) {
	up := &uiUp{}
	_, err := upscaleSessionItem(uiItems{}, uiImages{}, up, 5, 0)
	require.Error(t, err)
	_, err = upscaleSessionItem(uiItems{5: {ID: 5, Width: 10, Height: 10}}, uiImages{err: errors.New("no file")}, up, 5, 0)
	require.Error(t, err)
	assert.Empty(t, up.got)
}
