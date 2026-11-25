package mcp

import (
	"encoding/json"
	"errors"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

const anatomyAttemptLimit = 3

type anatomyEngine struct {
	sgEngine
	verdicts []string
	checkErr error
	checks   []string
	seeds    []int64
	onCheck  func()
}

func (e *anatomyEngine) CheckStylistAnatomy(image, prompt string) (generation.StylistAnatomyResult, error) {
	e.checks = append(e.checks, image)
	if e.onCheck != nil {
		e.onCheck()
	}
	verdict := "ok"
	if len(e.verdicts) >= len(e.checks) {
		verdict = e.verdicts[len(e.checks)-1]
	}
	return generation.StylistAnatomyResult{Verdict: verdict}, e.checkErr
}
func (e *anatomyEngine) KleinTryOn(image, garment, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	if seed != nil {
		e.seeds = append(e.seeds, *seed)
	}
	return e.sgEngine.KleinTryOn(image, garment, instruction, seed)
}
func anatomyWarning(t *testing.T, res *ShootFrameResult) string {
	t.Helper()
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	warning, _ := fields["anatomy_warning"].(string)
	return warning
}
func TestStylistAnatomyBaseRetries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		verdicts []string
		checkErr error
		count    int
		warning  string
	}{
		{name: "ok", verdicts: []string{"ok"}, count: 1},
		{name: "defect then ok", verdicts: []string{"defect", "ok"}, count: 2},
		{name: "bounded defects", verdicts: []string{"defect", "defect", "defect"}, count: anatomyAttemptLimit, warning: "base_defect"},
		{name: "uncertain", verdicts: []string{"uncertain"}, count: 1, warning: "uncertain"},
		{name: "unavailable", checkErr: errors.New("private http://example.com/photo"), count: 1, warning: "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eng := &anatomyEngine{verdicts: tc.verdicts, checkErr: tc.checkErr}
			sh := &fakeShooter{}
			sess := &fakeSessions{}
			p := sqFrameParams()
			seed := int64(47)
			p.Seed = &seed
			res, err := runShootFrame(sh, sess, eng, nil, p, nil)
			require.NoError(t, err)
			require.Len(t, sh.genCalls(), tc.count)
			require.Len(t, eng.checks, tc.count)
			assert.Equal(t, tc.warning, anatomyWarning(t, res))
			assert.Equal(t, sh.genImages(), eng.checks)
			seen := map[int64]bool{}
			for _, call := range sh.genCalls() {
				require.NotNil(t, call.Seed)
				assert.False(t, seen[*call.Seed])
				seen[*call.Seed] = true
			}
			assert.Equal(t, int64(47), seed)
			require.Len(t, sess.namedCalls(), 1)
			assert.Equal(t, sh.genImages()[tc.count-1], sess.namedCalls()[0].image)
		})
	}
}
func TestStylistAnatomyGarmentRetriesFromBase(t *testing.T) {
	eng := &anatomyEngine{verdicts: []string{"ok", "defect", "ok"}}
	sh := &fakeShooter{}
	sess := &fakeSessions{}
	p := sqFrameParams()
	p.GarmentBase64 = "garment-image"
	res, err := runShootFrame(sh, sess, eng, nil, p, nil)
	require.NoError(t, err)
	require.Len(t, sh.genCalls(), 1)
	require.Len(t, eng.calls(), 2)
	for _, call := range eng.calls() {
		assert.Equal(t, sh.genImages()[0], call.image)
	}
	require.Len(t, eng.seeds, 2)
	assert.NotEqual(t, eng.seeds[0], eng.seeds[1])
	assert.Empty(t, anatomyWarning(t, res))
	assert.Equal(t, eng.tryOnResults()[1], sess.namedCalls()[0].image)
}
func TestStylistAnatomyBadBaseSkipsGarment(t *testing.T) {
	eng := &anatomyEngine{verdicts: []string{"defect", "defect", "defect"}}
	p := sqFrameParams()
	p.GarmentBase64 = "garment-image"
	res, err := runShootFrame(&fakeShooter{}, &fakeSessions{}, eng, nil, p, nil)
	require.NoError(t, err)
	assert.Empty(t, eng.calls())
	assert.Equal(t, "base_defect", anatomyWarning(t, res))
}
func TestStylistAnatomyCancellationStopsNextStage(t *testing.T) {
	stopped := false
	eng := &anatomyEngine{verdicts: []string{"defect"}, onCheck: func() { stopped = true }}
	sh := &fakeShooter{}
	sess := &fakeSessions{}
	p := sqFrameParams()
	p.GarmentBase64 = "garment-image"
	_, err := runShootFrame(sh, sess, eng, nil, p, func() bool { return stopped })
	require.Error(t, err)
	assert.True(t, generation.IsInterrupted(err))
	assert.Len(t, sh.genCalls(), 1)
	assert.Empty(t, eng.calls())
	assert.Empty(t, sess.namedCalls())
}

type anatomyPipelineShooter struct {
	fakeShooter
	params []generation.GenerateCompoundImageParams
}

func (s *anatomyPipelineShooter) GenerateCompoundImage(p generation.GenerateCompoundImageParams) (*generation.GenerateImageResult, error) {
	cp := p
	if p.Seed != nil {
		v := *p.Seed
		cp.Seed = &v
	}
	s.params = append(s.params, cp)
	return &generation.GenerateImageResult{Image: "pipeline-frame", Info: json.RawMessage(`{"seed":90}`)}, nil
}
func TestStylistAnatomyPresetRetryPreservesSettings(t *testing.T) {
	sh := &presetShooter{}
	eng := &anatomyEngine{verdicts: []string{"defect", "ok"}}
	p := sqFrameParams()
	const presetID int64 = 23
	p.Preset = presetID
	p.PersonLora = "person-lora"
	p.Width = 832
	p.Height = 1216
	steps := 12
	p.Overrides = &generation.PresetOverrides{Steps: &steps}
	seed := int64(47)
	p.Seed = &seed
	_, err := runShootFrame(sh, &fakeSessions{}, eng, nil, p, nil)
	require.NoError(t, err)
	require.Len(t, sh.calls, 2)
	for _, c := range sh.calls {
		assert.Equal(t, presetID, c.PresetID)
		assert.Equal(t, p.Prompt, c.ScenePrompt)
		assert.Equal(t, p.PersonLora, c.PersonLora)
		assert.Equal(t, p.Overrides, c.Overrides)
		assert.Equal(t, p.Width, c.Width)
		assert.Equal(t, p.Height, c.Height)
	}
	require.NotNil(t, sh.calls[1].Seed)
	assert.NotEqual(t, seed, *sh.calls[1].Seed)
	assert.NotEqual(t, int64(9), *sh.calls[1].Seed)
	assert.Equal(t, int64(47), seed)
}
func TestStylistAnatomyPipelineRetryPreservesSettings(t *testing.T) {
	sh := &anatomyPipelineShooter{}
	eng := &anatomyEngine{verdicts: []string{"defect", "ok"}}
	p := sqFrameParams()
	const pipelineID int64 = 19
	p.Pipeline = pipelineID
	p.PersonLoras = map[string]string{"Z-Image": "person-lora"}
	p.Width = 832
	p.Height = 1216
	seed := int64(47)
	p.Seed = &seed
	_, err := runShootFrame(sh, &fakeSessions{}, eng, nil, p, nil)
	require.NoError(t, err)
	require.Len(t, sh.params, 2)
	for _, c := range sh.params {
		assert.Equal(t, pipelineID, c.CompoundPresetID)
		assert.Equal(t, p.Prompt, c.ScenePrompt)
		assert.Equal(t, p.PersonLoras, c.Person.Loras)
		assert.Equal(t, p.Width, c.Width)
		assert.Equal(t, p.Height, c.Height)
	}
	require.NotNil(t, sh.params[1].Seed)
	assert.NotEqual(t, seed, *sh.params[1].Seed)
	assert.NotEqual(t, int64(90), *sh.params[1].Seed)
	assert.Equal(t, int64(47), seed)
}
func TestStylistAnatomyGarmentDefectsAreBounded(t *testing.T) {
	eng := &anatomyEngine{verdicts: []string{"ok", "defect", "defect", "defect"}}
	sh := &fakeShooter{}
	sess := &fakeSessions{}
	p := sqFrameParams()
	p.GarmentBase64 = "garment-image"
	res, err := runShootFrame(sh, sess, eng, nil, p, nil)
	require.NoError(t, err)
	require.Len(t, eng.calls(), anatomyAttemptLimit)
	assert.Len(t, sh.genCalls(), 1)
	assert.Equal(t, "garment_defect", anatomyWarning(t, res))
	assert.Equal(t, eng.tryOnResults()[anatomyAttemptLimit-1], sess.namedCalls()[0].image)
}

func TestStylistAnatomyGarmentUncertainOrUnavailableDoesNotRetry(t *testing.T) {
	for _, failure := range []string{"uncertain", "error"} {
		t.Run(failure, func(t *testing.T) {
			eng := &anatomyEngine{verdicts: []string{"ok", "uncertain"}}
			if failure == "error" {
				eng.onCheck = func() {
					if len(eng.checks) == 2 {
						eng.checkErr = errors.New("private failed photo")
					}
				}
			}
			sh := &fakeShooter{}
			sess := &fakeSessions{}
			p := sqFrameParams()
			p.GarmentBase64 = "garment-image"
			res, err := runShootFrame(sh, sess, eng, nil, p, nil)
			require.NoError(t, err)
			assert.Len(t, sh.genCalls(), 1)
			assert.Len(t, eng.calls(), 1)
			warning := "uncertain"
			if failure == "error" {
				warning = "unavailable"
			}
			assert.Equal(t, warning, anatomyWarning(t, res))
			assert.Equal(t, eng.tryOnResults()[0], sess.namedCalls()[0].image)
		})
	}
}
