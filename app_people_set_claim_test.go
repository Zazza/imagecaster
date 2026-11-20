package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

type fakeDisplaySvc struct {
	people []serverclient.Person
	set    [][2]string
}

func (f *fakeDisplaySvc) List(context.Context) ([]serverclient.Person, error) { return f.people, nil }

func (f *fakeDisplaySvc) SetDisplay(_ context.Context, name, display string, _ bool) (*serverclient.Person, error) {
	f.set = append(f.set, [2]string{name, display})
	return &serverclient.Person{Name: name, DisplayName: display}, nil
}

func TestClaimTechKey(t *testing.T) {
	tests := []struct {
		name   string
		people []serverclient.Person
		key    string
		want   [][2]string
	}{
		{"tech name without Имя gets it", []serverclient.Person{{Name: "anna"}}, "Anna", [][2]string{{"anna", "Anna"}}},
		{"LoRA already has Имя", []serverclient.Person{{Name: "anna", DisplayName: "Аня"}}, "anna", nil},
		{"key is an Имя", []serverclient.Person{{Name: "anna_z", DisplayName: "Аня"}}, "Аня", nil},
		{"no such LoRA", nil, "Оля", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeDisplaySvc{people: tt.people}
			require.NoError(t, claimTechKey(context.Background(), f, tt.key))
			assert.Equal(t, tt.want, f.set)
		})
	}
}
