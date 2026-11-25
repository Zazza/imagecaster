package mcp

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type detailRestoreEngine struct {
	swapEng
	blends []float64
}

func (e *detailRestoreEngine) FaceSwapWithRestore(img, person string, blend float64) (string, string) {
	e.blends = append(e.blends, blend)
	return e.swapEng.FaceSwap(img, person)
}
func TestRunShootFrameDetailRestoreStrength(t *testing.T) {
	for _, blend := range []float64{0, 0.3, 0.6, 1} {
		eng := &detailRestoreEngine{swapEng: swapEng{img: "restored", status: "done"}}
		p := sqFrameParams()
		p.FaceSwap = true
		p.FacePass = false
		p.FaceRestoreBlend = &blend
		sess := &fakeSessions{}
		result, err := RunShootFrame(&fakeShooter{}, sess, eng, nil, p)
		require.NoError(t, err)
		assert.Equal(t, "done", result.FaceSwap)
		assert.Equal(t, []float64{blend}, eng.blends)
		require.Len(t, sess.namedCalls(), 1)
		assert.Equal(t, "restored", sess.namedCalls()[0].image)
	}
}
