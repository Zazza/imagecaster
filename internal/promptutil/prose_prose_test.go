package promptutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProseBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		base string
		want bool
	}{
		{"Z-Image", true},
		{"Moss-Image", true},
		{"Flux.1", true},
		{"Flux.2", true},
		{"SDXL", false},
		{"", false},
		{"SD1.5", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ProseBase(tt.base))
		})
	}
}

func TestJoinProse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, scene, style, want string
	}{
		{"scene without punctuation gets a period", "a woman on a pier", "film photo", "a woman on a pier. film photo"},
		{"scene ending with period kept", "a woman on a pier.", "film photo", "a woman on a pier. film photo"},
		{"scene ending with exclamation kept", "a woman on a pier!", "film photo", "a woman on a pier! film photo"},
		{"scene ending with question kept", "a woman on a pier?", "film photo", "a woman on a pier? film photo"},
		{"empty style returns scene only", "a woman on a pier", "", "a woman on a pier"},
		{"empty scene returns style only", "", "film photo", "film photo"},
		{"both empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := JoinProse(tt.scene, tt.style)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "BREAK")
		})
	}
}
