package mcp

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunShootFrame_PipelineSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		width, height int
		wantW, wantH  int
	}{
		{"landscape passed through", 1536, 1024, 1536, 1024},
		{"portrait passed through", 1024, 1536, 1024, 1536},
		{"zero falls back to shoot default", 0, 0, 1024, 1536},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shooter := &pprPipelineShooter{image: base64.StdEncoding.EncodeToString(styPNG(4))}
			eng := &pprSwapEng{out: base64.StdEncoding.EncodeToString(styPNG(8))}
			p := pprFrameParams()
			p.Width, p.Height = tt.width, tt.height

			_, err := RunShootFrame(shooter, &fakeSessions{}, eng, &fakeRefiner{size: 256}, p)

			require.NoError(t, err)
			calls := shooter.compoundCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, tt.wantW, calls[0].Width)
			assert.Equal(t, tt.wantH, calls[0].Height)
		})
	}
}
