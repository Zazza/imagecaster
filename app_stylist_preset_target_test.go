package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/stylist"
)

type ptPipes struct {
	snPipes
	presets map[int64][2]string
}

func (p ptPipes) PresetModel(id int64) (string, string, error) {
	v, ok := p.presets[id]
	if !ok {
		return "", "", errors.New("пресет не найден")
	}
	return v[0], v[1], nil
}

func ptPipesAll() ptPipes {
	return ptPipes{snPipes: snPipesAll(), presets: map[int64][2]string{
		3: {"Плёнка", stylist.Models["Moss-Image"].Model},
		4: {"Журнал", stylist.Models["SDXL"].Model},
		5: {"Кино", stylist.Models["Flux.1"].Model},
	}}
}

func TestStylistShootPreset_Target(t *testing.T) {
	tests := []struct {
		name      string
		preset    int64
		faceSwap  bool
		wantErr   string
		wantLora  string
		wantBase  string
		wantSwap  bool
		wantTitle string
	}{
		{"style on Moss picks Moss LoRA", 3, false, "", "natasha_curated_moss", "Moss-Image", false, "Плёнка"},
		{"style on Flux.1 with identity enables face swap", 5, true, "", "natasha_curated_flux1", "Flux.1", true, "Кино"},
		{"style on a model without the person's LoRA", 4, false, "«Журнал»", "", "", false, ""},
		{"unknown style", 99, false, "пресет", "", "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			req := snReq("Наташа", "", 0, tt.faceSwap)
			req.Preset = tt.preset
			_, err := stylistShoot(context.Background(), snNatasha(false, true), ptPipesAll(), q, false, req)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, q.jobs)
				return
			}
			require.NoError(t, err)
			require.Len(t, q.jobs, 1)
			p := q.jobs[0].params
			assert.Equal(t, tt.preset, p.Preset)
			assert.Equal(t, tt.wantTitle, p.PresetName)
			assert.Equal(t, tt.wantLora, p.Person)
			assert.Equal(t, tt.wantLora, p.PersonLora)
			assert.Equal(t, tt.wantBase, p.Base)
			assert.NotContains(t, p.Prompt, "<lora:", "LoRA goes through the preset, not the prompt")
			assert.Equal(t, tt.wantSwap, p.FaceSwap)
			assert.Zero(t, p.Pipeline)
			assert.True(t, p.ToFolder)
		})
	}
}

func TestStylistShootPreset_NoLookup(t *testing.T) {
	req := snReq("Наташа", "", 0, false)
	req.Preset = 3
	_, err := stylistShoot(context.Background(), snNatasha(false, false), snPipesAll(), &spQueue{}, false, req)
	require.Error(t, err)
}

func TestStylistShootOverrides(t *testing.T) {
	steps := 12
	ov := &generation.PresetOverrides{Steps: &steps}
	tests := []struct {
		name     string
		base     string
		pipeline int64
		preset   int64
		wantOv   bool
	}{
		{"model with overrides goes the preset way", "Moss-Image", 0, 0, true},
		{"style with overrides", "", 0, 3, true},
		{"workflow drops overrides", "", 7, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			req := snReq("Наташа", tt.base, tt.pipeline, false)
			req.Preset = tt.preset
			req.Overrides = ov
			_, err := stylistShoot(context.Background(), snNatasha(false, false), ptPipesAll(), q, false, req)
			require.NoError(t, err)
			require.Len(t, q.jobs, 1)
			p := q.jobs[0].params
			if tt.wantOv {
				require.NotNil(t, p.Overrides)
				assert.Equal(t, 12, *p.Overrides.Steps)
				assert.NotEmpty(t, p.PersonLora)
				assert.NotContains(t, p.Prompt, "<lora:")
			} else {
				assert.Nil(t, p.Overrides)
				assert.Empty(t, p.PersonLora)
			}
		})
	}

	t.Run("model without overrides keeps LoRA in the prompt", func(t *testing.T) {
		q := &spQueue{}
		_, err := stylistShoot(context.Background(), snNatasha(false, false), ptPipesAll(), q, false, snReq("Наташа", "Moss-Image", 0, false))
		require.NoError(t, err)
		p := q.jobs[0].params
		assert.Nil(t, p.Overrides)
		assert.Empty(t, p.PersonLora)
		assert.Contains(t, p.Prompt, "<lora:natasha_curated_moss:1>")
	})
}

func TestStylistShoot_StyleDecidesTheLook(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		pipeline int64
		preset   int64
		photo    bool
	}{
		{"model keeps the photo wording", "Moss-Image", 0, 0, true},
		{"style: no photo wording", "", 0, 3, false},
		{"style on SDXL: no photo wording", "", 0, 4, false},
		{"workflow: no photo wording", "", 7, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			req := snReq("Наташа", tt.base, tt.pipeline, false)
			req.Preset = tt.preset
			people := snNatasha(false, false)
			if tt.preset == 4 {
				people.list = append(people.list, snPerson("natasha_sdxl", "SDXL", "Наташа", false))
			}
			_, err := stylistShoot(context.Background(), people, ptPipesAll(), q, false, req)
			require.NoError(t, err)
			require.Len(t, q.jobs, 1)
			p := q.jobs[0].params.Prompt
			for _, w := range []string{"photograph", "realistic skin", "no stylization"} {
				if tt.photo {
					assert.Contains(t, p, w)
				} else {
					assert.NotContains(t, p, w)
				}
			}
			assert.Contains(t, p, "facing camera")
		})
	}
}
