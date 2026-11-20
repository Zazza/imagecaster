package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

type fakeSetRenamer struct {
	calls [][2]string
	err   error
}

func (f *fakeSetRenamer) RenamePersonPhotos(oldKey, newKey string) error {
	f.calls = append(f.calls, [2]string{oldKey, newKey})
	return f.err
}

func TestFollowSetKey(t *testing.T) {
	t.Parallel()
	lora := func(name, display string) serverclient.Person {
		return serverclient.Person{Name: name, DisplayName: display}
	}
	tests := []struct {
		name      string
		before    []serverclient.Person
		after     []serverclient.Person
		lora      string
		newKey    string
		wantCalls [][2]string
	}{
		{
			name:      "display name changed, old key gone",
			before:    []serverclient.Person{lora("nat_z", "Наташа"), lora("ivan", "Иван")},
			after:     []serverclient.Person{lora("nat_z", "Наталья"), lora("ivan", "Иван")},
			lora:      "nat_z",
			newKey:    "Наталья",
			wantCalls: [][2]string{{"Наташа", "Наталья"}},
		},
		{
			name:      "no display name before: key is the lora name",
			before:    []serverclient.Person{lora("nat_z", "")},
			after:     []serverclient.Person{lora("nat_z", "Наташа")},
			lora:      "nat_z",
			newKey:    "Наташа",
			wantCalls: [][2]string{{"nat_z", "Наташа"}},
		},
		{
			name:      "display name cleared: new key is the lora name",
			before:    []serverclient.Person{lora("nat_z", "Наташа")},
			after:     []serverclient.Person{lora("nat_z", "")},
			lora:      "nat_z",
			newKey:    "nat_z",
			wantCalls: [][2]string{{"Наташа", "nat_z"}},
		},
		{
			name:   "another lora keeps the old key",
			before: []serverclient.Person{lora("nat_z", "Наташа"), lora("nat_sdxl", "Наташа")},
			after:  []serverclient.Person{lora("nat_z", "Наталья"), lora("nat_sdxl", "Наташа")},
			lora:   "nat_z",
			newKey: "Наталья",
		},
		{
			name:   "another lora keeps the old key through its name",
			before: []serverclient.Person{lora("nat_z", "nat"), lora("nat", "")},
			after:  []serverclient.Person{lora("nat_z", "Наташа"), lora("nat", "")},
			lora:   "nat_z",
			newKey: "Наташа",
		},
		{
			name:   "key unchanged",
			before: []serverclient.Person{lora("nat_z", "Наташа")},
			after:  []serverclient.Person{lora("nat_z", "Наташа")},
			lora:   "nat_z",
			newKey: "Наташа",
		},
		{
			name:   "lora not found before",
			before: []serverclient.Person{lora("ivan", "Иван")},
			after:  []serverclient.Person{lora("ivan", "Иван"), lora("nat_z", "Наташа")},
			lora:   "nat_z",
			newKey: "Наташа",
		},
		{
			name:   "empty lists",
			lora:   "nat_z",
			newKey: "Наташа",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &fakeSetRenamer{}
			require.NoError(t, followSetKey(st, tt.before, tt.after, tt.lora, tt.newKey))
			assert.Equal(t, tt.wantCalls, st.calls)
		})
	}
}

func TestFollowSetKey_RenameErrorReturned(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	st := &fakeSetRenamer{err: boom}
	err := followSetKey(st,
		[]serverclient.Person{{Name: "nat_z", DisplayName: "Наташа"}},
		[]serverclient.Person{{Name: "nat_z", DisplayName: "Наталья"}},
		"nat_z", "Наталья")
	assert.ErrorIs(t, err, boom)
}
