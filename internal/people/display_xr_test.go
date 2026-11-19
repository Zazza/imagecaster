package people

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func caseMixedPeople() []serverclient.Person {
	return []serverclient.Person{
		{Name: "natasha_curated", DisplayName: "Наташа", Base: "Z-Image", Identity: false},
		{Name: "natasha_curated_moss", DisplayName: "наташа", Base: "Moss-Image", Identity: true},
		{Name: "olga_sdxl", DisplayName: "Ольга", Base: "SDXL"},
	}
}

func TestLorasByBase_CaseInsensitiveXR(t *testing.T) {
	t.Parallel()
	want := map[string]string{"Z-Image": "natasha_curated", "Moss-Image": "natasha_curated_moss"}
	for _, key := range []string{"Наташа", "наташа", "НАТАША"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, LorasByBase(caseMixedPeople(), key))
		})
	}
}

func TestFaceSwapName_CaseInsensitiveXR(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "natasha_curated_moss", FaceSwapName(caseMixedPeople(), "Наташа", []string{"Z-Image", "Moss-Image"}))
	assert.Equal(t, "natasha_curated_moss", FaceSwapName(caseMixedPeople(), "наташа", []string{"Moss-Image"}))
	assert.Equal(t, "", FaceSwapName(caseMixedPeople(), "Наташа", []string{"Z-Image"}))
}

func findPeople() []serverclient.Person {
	return []serverclient.Person{
		{Name: "anna", DisplayName: "Анна", Base: "Z-Image"},
		{Name: "alina", DisplayName: "anna", Base: "Z-Image"},
	}
}

func TestFind_ExactLoraWinsXR(t *testing.T) {
	t.Parallel()
	got, ok := Find(findPeople(), "anna")
	require.True(t, ok)
	assert.Equal(t, "Анна", got)
	assert.Equal(t, map[string]string{"Z-Image": "anna"}, LorasByBase(findPeople(), got))
}

func TestFind_DisplayCaseInsensitiveWhenNoExactLoraXR(t *testing.T) {
	t.Parallel()
	got, ok := Find(findPeople(), "ANNA")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"Z-Image": "alina"}, LorasByBase(findPeople(), got))
}

func TestFind_NotFoundXR(t *testing.T) {
	t.Parallel()
	_, ok := Find(findPeople(), "Мария")
	assert.False(t, ok)
	_, ok = Find(nil, "anna")
	assert.False(t, ok)
}

func TestTrainReplace_DisplayReplaceXR(t *testing.T) {
	tests := []struct {
		name    string
		replace bool
	}{
		{"Train", false},
		{"TrainReplace", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			dir := t.TempDir()
			makePhotos(t, dir, 12)
			api := &fakeAPI{submitJob: &serverclient.TrainJob{ID: "j1"}}
			s := New(api, func() string { return "http://192.168.1.184:8080" }, func() string { return "inst-1" })

			var err error
			if tt.replace {
				_, err = s.TrainReplace(context.Background(), dir, "natasha", "Наташа", "woman", "Z-Image", false)
			} else {
				_, err = s.Train(context.Background(), dir, "natasha", "Наташа", "woman", "Z-Image", false)
			}

			require.NoError(t, err)
			require.Len(t, api.submits, 1)
			assert.Equal(t, tt.replace, api.submits[0].DisplayReplace)
			assert.Equal(t, "Наташа", api.submits[0].DisplayName)
			assert.Equal(t, "natasha", api.submits[0].Name)
		})
	}
}
