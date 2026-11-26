package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"

	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcePersonChoosesPersonalBaseIndependentlyOfPoseEditor(t *testing.T) {
	for _, tc := range []struct {
		name, base, wantBase, wantLora string
		swap                           bool
	}{
		{name: "automatic", wantBase: "Z-Image", wantLora: "natasha_curated"},
		{name: "manual different base", base: "Flux.1", wantBase: "Flux.1", wantLora: "natasha_curated_flux1", swap: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := sourceTestRequest(t)
			req.Person, req.Base, req.FaceSwap = "Наташа", tc.base, tc.swap
			var err error
			req.target, err = resolveSourcePerson(snNatasha(true, true).list, req)
			require.NoError(t, err)
			q := &spQueue{}
			_, err = enqueueSourceVariants(newLsStore(), q, sourceTestSession(), false, req)
			require.NoError(t, err)
			require.Len(t, q.jobs, len(req.Instructions))
			for _, job := range q.jobs {
				p := job.params
				assert.Equal(t, tc.wantLora, p.Person)
				assert.Equal(t, tc.wantBase, p.Base)
				assert.Contains(t, p.Prompt, "<lora:"+tc.wantLora+":1>")
				assert.True(t, p.FacePass)
				assert.Equal(t, tc.swap, p.FaceSwap)
				if tc.swap {
					assert.Equal(t, tc.wantLora, p.FaceSwapName)
				} else {
					assert.Empty(t, p.FaceSwapName)
				}
				require.NotNil(t, p.Editor)
				assert.Equal(t, stylistEditorModel, p.Editor.Model)
				assert.Equal(t, stylistEditorModel, p.Model)
				assert.NotEqual(t, stylist.Models[tc.wantBase].Model, p.Editor.Model)
			}
		})
	}
}

func TestSourcePersonRejectsUnusableSelectionsBeforeImport(t *testing.T) {
	for _, tc := range []struct {
		name, person, base string
		all                []serverclient.Person
	}{
		{name: "incompatible manual base", person: "Наташа", base: "SDXL", all: snNatasha(true, true).list},
		{name: "unknown person", person: "Missing", base: "Z-Image", all: snNatasha(true, true).list},
		{name: "no personal LoRA", person: "Наташа", base: "Z-Image", all: []serverclient.Person{{Name: "pending", DisplayName: "Наташа", Base: "Z-Image"}}},
		{name: "unsupported automatic base", person: "Наташа", all: []serverclient.Person{snPerson("old", "SD1.5", "Наташа", false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := sourceTestRequest(t)
			req.Person, req.Base = tc.person, tc.base
			_, err := resolveSourcePerson(tc.all, req)
			require.Error(t, err)
			q, sessions := &spQueue{}, sourceTestSession()
			_, err = enqueueSourceVariants(newLsStore(), q, sessions, false, req)
			require.Error(t, err)
			assert.Zero(t, sessions.imports)
			assert.Empty(t, q.jobs)
		})
	}
}

func TestSourcePersonSnapshotSurvivesReshootAndLaterEdit(t *testing.T) {
	req := sourceTestRequest(t)
	req.Person, req.Base, req.FaceSwap = "Наташа", "Flux.1", true
	all := snNatasha(true, true).list
	var err error
	req.target, err = resolveSourcePerson(all, req)
	require.NoError(t, err)
	st, q := newLsStore(editSourceItem), &editTestQueue{}
	result, err := enqueueSourceVariants(st, q, sourceTestSession(), false, req)
	require.NoError(t, err)
	original := q.jobs[0].params
	for i := range all {
		all[i].Name = "renamed"
		all[i].Lora = "retrained"
		all[i].Identity = false
	}
	req.Person, req.Base, req.FaceSwap = "someone else", "SDXL", false
	req.Editor.Loras[0].Name = "new editor style"
	replayID, err := reshootFrame(st, q, false, result.JobIDs[0], "")
	require.NoError(t, err)
	replay := q.jobs[len(q.jobs)-1].params
	assert.Equal(t, original.Person, replay.Person)
	assert.Equal(t, original.Base, replay.Base)
	assert.Equal(t, original.Prompt, replay.Prompt)
	assert.Equal(t, original.FacePass, replay.FacePass)
	assert.Equal(t, original.FaceSwapName, replay.FaceSwapName)
	assert.Equal(t, "Chosen", replay.Editor.Loras[0].Name)
	frame, err := json.Marshal(mcp.ShootFrameResult{ID: editSourceItem})
	require.NoError(t, err)
	require.NoError(t, recordShootFrame(st, replayID, frame))
	_, err = enqueueStylistEdit(st, q, &editTestItems{item: sourceTestSession().item}, false, replayID, "turn slightly right")
	require.NoError(t, err)
	edited := q.jobs[len(q.jobs)-1].params
	assert.Equal(t, original.Person, edited.Person)
	assert.Equal(t, original.Base, edited.Base)
	assert.Equal(t, original.Prompt, edited.Prompt)
	assert.True(t, edited.FacePass)
	assert.True(t, edited.FaceSwap)
	assert.Equal(t, original.FaceSwapName, edited.FaceSwapName)
}

type sourceIdentityEngine struct {
	selectedStylistEngine
	events    []string
	refine    generation.FaceRefineParams
	refineErr error
	frame     string
}

func (e *sourceIdentityEngine) PrepareSourcePose(string, string) (string, error) {
	return "Turn slightly right.", nil
}
func (e *sourceIdentityEngine) StylistReferenceEdit(image, prompt string, seed *int64, editor *generation.PostEditorParams) (*generation.GenerateImageResult, error) {
	e.events = append(e.events, "pose")
	return e.selectedStylistEngine.StylistReferenceEdit(image, prompt, seed, editor)
}
func (e *sourceIdentityEngine) FaceLandmarks(input string) ([]faceguard.Point, error) {
	const landmarkCount = 68
	const faceRadius = 0.125
	pts := make([]faceguard.Point, landmarkCount)
	for i := range pts {
		angle := 2 * math.Pi * float64(i) / landmarkCount
		pts[i] = faceguard.Point{X: 0.5 + faceRadius*math.Cos(angle), Y: 0.5 + faceRadius*math.Sin(angle)}
		if input != e.frame {
			pts[i].X = pts[i].X*2 - 0.5
			pts[i].Y = pts[i].Y*2 - 0.5
		}
	}
	return pts, nil
}
func (e *sourceIdentityEngine) FaceRefine(p generation.FaceRefineParams) (string, error) {
	e.events = append(e.events, "face")
	e.refine = p
	return p.ImageBase64, e.refineErr
}
func (e *sourceIdentityEngine) FaceSwap(image, name string) (string, string) {
	e.events = append(e.events, "swap")
	return e.editTestEngine.FaceSwap(image, name)
}

func sourceIdentityRun(t *testing.T) (*sourceIdentityEngine, *editTestSessions, mcp.ShootFrameParams) {
	t.Helper()
	legacy, sessions, p := editRunFixture()
	const frameSize = 256
	frame := image.NewRGBA(image.Rect(0, 0, frameSize, frameSize))
	frame.SetRGBA(0, 0, color.RGBA{R: 190, A: 255})
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, frame))
	b64 := base64.StdEncoding.EncodeToString(encoded.Bytes())
	legacy.result.Image = b64
	eng := &sourceIdentityEngine{selectedStylistEngine: selectedStylistEngine{editTestEngine: *legacy}, frame: b64}
	p.SourcePose, p.FacePass = true, true
	p.Base, p.Prompt = "Z-Image", "<lora:original-person:1> identity"
	p.Editor = sourceTestRequest(t).Editor
	return eng, sessions, p
}

func TestSourcePersonWorkerUsesExistingFacePassBeforeSwap(t *testing.T) {
	eng, sessions, p := sourceIdentityRun(t)
	result, err := runStylistEdit(eng, sessions, p, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"pose", "face", "swap"}, eng.events)
	assert.Equal(t, "done", result.FacePass)
	assert.Equal(t, "done", result.FaceSwap)
	assert.Equal(t, stylist.Models[p.Base].Model, eng.refine.Model)
	assert.Equal(t, p.Prompt, eng.refine.Prompt)
	assert.NotEqual(t, eng.frame, eng.refine.ImageBase64, "face pass must receive a crop")
	assert.Equal(t, "swapped-image", sessions.savedImage)
	assert.Equal(t, "original-face", eng.faceName)
}

func TestSourcePersonWorkerReportsSkipAndStopsBeforeSavingOnCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "refinement failure", true: "interrupted refinement"}[cancel], func(t *testing.T) {
			eng, sessions, p := sourceIdentityRun(t)
			eng.refineErr = errors.New("face service unavailable")
			if cancel {
				eng.refineErr = generation.ErrInterrupted
			}
			result, err := runStylistEdit(eng, sessions, p, nil)
			if cancel {
				require.ErrorIs(t, err, generation.ErrInterrupted)
				assert.Nil(t, result)
				assert.Equal(t, []string{"pose", "face"}, eng.events)
				assert.Empty(t, sessions.savedImage)
			} else {
				require.NoError(t, err)
				assert.Contains(t, result.FacePass, "skipped:")
				assert.Equal(t, []string{"pose", "face", "swap"}, eng.events)
				assert.Equal(t, eng.frame, eng.faceImage, "failed face pass must preserve edited photo")
			}
		})
	}
}
