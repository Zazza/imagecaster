package stylist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithSubject(t *testing.T) {
	tests := []struct {
		name, appearance, gender, want string
	}{
		{"empty woman", "", "woman", "a woman"},
		{"empty unknown gender is a woman", " ", "", "a woman"},
		{"empty man", "", "male", "a man"},
		{"person gender", "", "person", "a person"},
		{"measurements only get a noun", "165 cm, average build", "woman", "a woman, 165 cm, average build"},
		{"child gets person", "slim, 150 cm", "person", "a person, slim, 150 cm"},
		{"article kept", "A tall man, beard", "man", "A tall man, beard"},
		{"an kept", "an elderly woman", "woman", "an elderly woman"},
		{"the kept", "the same woman", "woman", "the same woman"},
		{"word starting with a is not an article", "athletic build", "man", "a man, athletic build"},
		{"bare gender word is a noun", "woman, 165 cm", "woman", "woman, 165 cm"},
		{"female is a noun", "female, slim", "woman", "female, slim"},
		{"girl is a noun", "girl 12 years old", "person", "girl 12 years old"},
		{"only commas", ", ,", "man", "a man"},
		{"single comma", ",", "", "a woman"},
		{"womanly is not a noun", "womanly figure", "woman", "a woman, womanly figure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WithSubject(tt.appearance, tt.gender))
		})
	}
}
