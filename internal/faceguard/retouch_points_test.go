package faceguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetouch_ForPoints(t *testing.T) {
	t.Parallel()
	full := Retouch{UnderEyes: true, Teeth: true, EyeWhites: true, EyeColor: "blue", Light: true, FaceWidth: 5}
	tests := []struct {
		name string
		in   Retouch
		n    int
		want Retouch
	}{
		{name: "zero stays zero", in: Retouch{}, n: 0, want: Retouch{}},
		{name: "light needs no points", in: Retouch{Light: true}, n: 0, want: Retouch{Light: true}},
		{name: "70 points keep everything", in: full, n: 70, want: full},
		{name: "68 points drop eye color only", in: full, n: 68, want: Retouch{UnderEyes: true, Teeth: true, EyeWhites: true, Light: true, FaceWidth: 5}},
		{name: "no face keeps light", in: full, n: 0, want: Retouch{Light: true}},
		{name: "no face without light is a no-op", in: Retouch{Teeth: true}, n: 10, want: Retouch{}},
		{name: "eye color alone on 68 points is a no-op", in: Retouch{EyeColor: "green"}, n: 68, want: Retouch{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.in.ForPoints(tc.n)
			assert.Equal(t, tc.want, got)
		})
	}
}
