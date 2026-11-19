package postpreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/bodyshape"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/finish"
)

func TestPostRetouch(t *testing.T) {
	t.Parallel()
	src := testPNG(t, 300, 200, 40)
	adj := adjust.Params{Brightness: 20, WhitePoint: 255, Gamma: 1}
	tests := []struct {
		name     string
		r        faceguard.Retouch
		pts      []faceguard.Point
		samePost bool
	}{
		{name: "zero retouch equals plain post", r: faceguard.Retouch{}, samePost: true},
		{name: "local retouch without points equals plain post", r: faceguard.Retouch{UnderEyes: true, Teeth: true}, samePost: true},
		{name: "light changes the preview", r: faceguard.Retouch{Light: true}, samePost: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := New(DefaultCacheSize)
			plain, err := s.Post(src, adj, finish.Params{}, 120)
			require.NoError(t, err)
			got, err := s.PostRetouch(src, adj, tc.r, tc.pts, bodyshape.Params{}, nil, finish.Params{}, 120)
			require.NoError(t, err)
			assert.Equal(t, tc.samePost, plain == got)
			assert.Equal(t, 120, decodedSize(t, got).X)
		})
	}
}
