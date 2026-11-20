package mcp

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
)

type presetShooter struct {
	fakeShooter
	calls []generation.PresetShotParams
	err   error
}

func (p *presetShooter) GeneratePresetShot(params generation.PresetShotParams) (*generation.PresetShotResult, error) {
	p.calls = append(p.calls, params)
	if p.err != nil {
		return nil, p.err
	}
	return &generation.PresetShotResult{Image: base64.StdEncoding.EncodeToString(styPNG(3)), Seed: 9, Model: "Плёнка"}, nil
}

func TestRunShootFrame_PresetPath(t *testing.T) {
	t.Parallel()
	steps := 12
	tests := []struct {
		name      string
		preset    int64
		overrides *generation.PresetOverrides
		wantModel string
	}{
		{"style", 3, nil, "Плёнка"},
		{"model with overrides", 0, &generation.PresetOverrides{Steps: &steps}, "z-image-turbo-fp8-aio"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sh := &presetShooter{}
			sess := &fakeSessions{}
			p := sqFrameParams()
			p.Prompt = "A woman in a red dress."
			p.Preset, p.Overrides, p.PersonLora = tt.preset, tt.overrides, "anna_lora_v1"
			if tt.preset > 0 {
				p.PresetName = "Плёнка"
			}
			seed := int64(4)
			p.Seed = &seed
			p.Width, p.Height = 832, 1216

			_, err := runShootFrame(sh, sess, &stubFaceEng{}, nil, p, nil)
			require.NoError(t, err)
			assert.Empty(t, sh.genCalls(), "plain txt2img is not used")
			require.Len(t, sh.calls, 1)
			c := sh.calls[0]
			assert.Equal(t, tt.preset, c.PresetID)
			assert.Equal(t, "z-image-turbo-fp8-aio", c.Model)
			assert.Equal(t, "A woman in a red dress.", c.ScenePrompt)
			assert.Equal(t, shootNegative, c.ExtraNegativePrompt)
			assert.Equal(t, "anna_lora_v1", c.PersonLora)
			assert.Equal(t, tt.overrides, c.Overrides)
			assert.Equal(t, 832, c.Width)
			assert.Equal(t, 1216, c.Height)
			require.NotNil(t, c.Seed)
			assert.Equal(t, int64(4), *c.Seed)
			calls := sess.namedCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, tt.wantModel, calls[0].meta.Model)
		})
	}
}

func TestRunShootFrame_PresetError(t *testing.T) {
	t.Parallel()
	sess := &fakeSessions{}
	p := sqFrameParams()
	p.Preset = 3
	_, err := runShootFrame(&presetShooter{err: errors.New("пресет не найден")}, sess, &stubFaceEng{}, nil, p, nil)
	require.Error(t, err)
	assert.Empty(t, sess.namedCalls())
}

func TestRunShootFrame_NoPresetSupport(t *testing.T) {
	t.Parallel()
	sess := &fakeSessions{}
	p := sqFrameParams()
	p.Preset = 3
	_, err := runShootFrame(&fakeShooter{}, sess, &stubFaceEng{}, nil, p, nil)
	require.Error(t, err)
	assert.Empty(t, sess.namedCalls())
}
