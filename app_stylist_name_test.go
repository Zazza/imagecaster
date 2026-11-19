package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
)

type snPipes map[int64]struct {
	name   string
	models []string
}

func (p snPipes) PipelineModels(id int64) (string, []string, error) {
	v, ok := p[id]
	if !ok {
		return "", nil, errors.New("пайплайн не найден")
	}
	return v.name, v.models, nil
}

func snPerson(lora, base, display string, identity bool) serverclient.Person {
	return serverclient.Person{Name: lora, Lora: lora, Base: base, DisplayName: display, Gender: "woman", Identity: identity}
}

func snNatasha(zID, fluxID bool) spPeople {
	return spPeople{list: []serverclient.Person{
		snPerson("natasha_curated", "Z-Image", "Наташа", zID),
		snPerson("natasha_curated_moss", "Moss-Image", "Наташа", false),
		snPerson("natasha_curated_flux1", "Flux.1", "Наташа", fluxID),
		snPerson("ivan_x", "SDXL", "", false),
	}}
}

func snPipesAll() snPipes {
	return snPipes{
		7: {name: "Z→SDXL→Flux", models: []string{stylist.Models["Z-Image"].Model, stylist.Models["SDXL"].Model, stylist.Models["Flux.1"].Model}},
		8: {name: "Только SDXL", models: []string{stylist.Models["SDXL"].Model}},
	}
}

func snReq(person, base string, pipeline int64, faceSwap bool) StylistShootRequest {
	return StylistShootRequest{
		Person: person, Base: base, Pipeline: pipeline, FaceSwap: faceSwap, Appearance: "a woman",
		Series: []stylist.Series{spSeries(stylist.Shot{Pose: "Анфас", Prompt: "facing camera"})},
	}
}

func TestStylistShootName_ByBase(t *testing.T) {
	tests := []struct {
		name      string
		people    spPeople
		req       StylistShootRequest
		wantErr   []string
		wantLora  string
		wantBase  string
		wantModel string
	}{
		{"name with several bases requires base", snNatasha(false, false), snReq("Наташа", "", 0, false), []string{"Z-Image", "Moss-Image", "Flux.1"}, "", "", ""},
		{"name with base", snNatasha(false, false), snReq("Наташа", "Flux.1", 0, false), nil, "natasha_curated_flux1", "Flux.1", stylist.Models["Flux.1"].Model},
		{"name with other base", snNatasha(false, false), snReq("Наташа", "Moss-Image", 0, false), nil, "natasha_curated_moss", "Moss-Image", stylist.Models["Moss-Image"].Model},
		{"name with base without lora", snNatasha(false, false), snReq("Наташа", "SDXL", 0, false), []string{"SDXL"}, "", "", ""},
		{"single lora needs no base", spPeople{list: []serverclient.Person{snPerson("olga_z", "Z-Image", "Ольга", false)}}, snReq("Ольга", "", 0, false), nil, "olga_z", "Z-Image", stylist.Models["Z-Image"].Model},
		{"lora without name by lora name", snNatasha(false, false), snReq("ivan_x", "", 0, false), nil, "ivan_x", "SDXL", stylist.Models["SDXL"].Model},
		{"unknown name", snNatasha(false, false), snReq("Мария", "", 0, false), []string{"Мария"}, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			_, err := stylistShoot(context.Background(), tt.people, snPipesAll(), q, false, tt.req)
			if tt.wantErr != nil {
				require.Error(t, err)
				for _, s := range tt.wantErr {
					assert.Contains(t, err.Error(), s)
				}
				assert.Empty(t, q.jobs)
				return
			}
			require.NoError(t, err)
			require.Len(t, q.jobs, 1)
			p := q.jobs[0].params
			assert.Equal(t, tt.wantLora, p.Person)
			assert.Equal(t, tt.wantBase, p.Base)
			assert.Equal(t, tt.wantModel, p.Model)
			assert.Contains(t, p.Prompt, "<lora:"+tt.wantLora+":1>")
			assert.Zero(t, p.Pipeline)
			assert.Empty(t, p.PersonLoras)
		})
	}
}

func TestStylistShootName_Pipeline(t *testing.T) {
	tests := []struct {
		name     string
		people   spPeople
		pipeline int64
		faceSwap bool
		wantErr  bool
		wantSwap bool
		swapName string
	}{
		{"loras on z and flux, identity on flux", snNatasha(false, true), 7, true, false, true, "natasha_curated_flux1"},
		{"identity on both, first by step order", snNatasha(true, true), 7, true, false, true, "natasha_curated"},
		{"no identity, no face swap", snNatasha(false, false), 7, true, false, false, ""},
		{"checkbox off, no face swap", snNatasha(true, true), 7, false, false, false, ""},
		{"no step with lora", snNatasha(true, true), 8, true, true, false, ""},
		{"unknown pipeline", snNatasha(true, true), 99, true, true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &spQueue{}
			_, err := stylistShoot(context.Background(), tt.people, snPipesAll(), q, false, snReq("Наташа", "", tt.pipeline, tt.faceSwap))
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, q.jobs)
				return
			}
			require.NoError(t, err)
			require.Len(t, q.jobs, 1)
			p := q.jobs[0].params
			assert.Equal(t, tt.pipeline, p.Pipeline)
			assert.Equal(t, "Z→SDXL→Flux", p.PipelineName)
			assert.Equal(t, map[string]string{"Z-Image": "natasha_curated", "Flux.1": "natasha_curated_flux1"}, p.PersonLoras)
			assert.NotContains(t, p.Prompt, "<lora:")
			assert.Equal(t, tt.wantSwap, p.FaceSwap)
			if tt.wantSwap {
				assert.Equal(t, tt.swapName, p.FaceSwapName)
			}
		})
	}
}

func TestStylistShootName_PipelineWithoutLookup(t *testing.T) {
	q := &spQueue{}
	_, err := stylistShoot(context.Background(), snNatasha(true, true), nil, q, false, snReq("Наташа", "", 7, true))
	require.Error(t, err)
	assert.Empty(t, q.jobs)
}
