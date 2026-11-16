package promptutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDedupeSentences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no repeats unchanged", "Кот спит на окне. За окном идёт дождь.", "Кот спит на окне. За окном идёт дождь."},
		{"same sentence looped", "Кот спит на окне. За окном дождь. За окном дождь. За окном дождь.", "Кот спит на окне. За окном дождь."},
		{"repeat differs in case and spaces", "A red dress. a  RED dress. Black shoes.", "A red dress. Black shoes."},
		{"loop cut by token limit mid-sentence", "Кот спит. Лампа светит тёплым светом. Лампа светит тёплым светом. Лампа светит", "Кот спит. Лампа светит тёплым светом."},
		{"unfinished new sentence is kept", "Кот спит. Лампа светит", "Кот спит. Лампа светит"},
		{"repeated clause loop in one sentence", "a cream silk blouse, with a soft bow at the neck, with a soft bow at the neck, with a soft bow at the neck, elegant style", "a cream silk blouse, with a soft bow at the neck, elegant style"},
		{"short repeated words kept", "red, red, red roses", "red, red, red roses"},
		{"repeat across lines", "Первая строка.\nВторая строка.\nВторая строка.", "Первая строка.\nВторая строка."},
		{"empty", "", ""},
		{"tag weights with dots are not split", "(red dress:1.2), blue sky, (smile:1.2), (smile:1.3)", "(red dress:1.2), blue sky, (smile:1.2), (smile:1.3)"},
		{"duplicated long tag dropped", "(red evening dress:1.2), blue sky, (red evening dress:1.2), sunset", "(red evening dress:1.2), blue sky, sunset"},
		{"decimal numbers kept", "Version 2.1 is shown. Price 3.5 is shown.", "Version 2.1 is shown. Price 3.5 is shown."},
		{"looped sentence with ellipsis and exclamation", "Опаздываю! Опаздываю! Бегу…", "Опаздываю! Бегу…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DedupeSentences(tt.in))
		})
	}
}

func TestTruncateRepetitive_DropsLoopedSentences(t *testing.T) {
	t.Parallel()
	in := "A lighthouse on a cliff at dusk. Waves crash below. Waves crash below. Waves crash below."
	assert.Equal(t, "A lighthouse on a cliff at dusk. Waves crash below", TruncateRepetitive(in, 1000))
}
