package main

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp(t *testing.T) (*App, *preset.DB) {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	cfg := &config.Config{LLMModel: "test-llm-model", SDPromptModel: "test-sd-model"}
	app := NewApp(db, llm.New("http://localhost:1234"), sd.New("http://localhost:7860"), serverclient.NewClient(), cfg)
	return app, db
}

func TestApp_ResolutionCRUD(t *testing.T) {
	t.Parallel()
	app, db := newTestApp(t)

	r := preset.Resolution{Name: "Test Res", Width: 512, Height: 512}
	created, err := app.CreateResolution(r)
	require.NoError(t, err)
	assert.Greater(t, created.ID, int64(0))
	assert.Equal(t, "Test Res", created.Name)

	got, err := db.GetResolution(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test Res", got.Name)

	created.Name = "Updated Res"
	created.Width = 768
	created.Height = 768
	updated, err := app.UpdateResolution(*created)
	require.NoError(t, err)
	assert.Equal(t, "Updated Res", updated.Name)
	assert.Equal(t, 768, updated.Width)

	err = app.DeleteResolution(created.ID)
	require.NoError(t, err)

	_, err = db.GetResolution(created.ID)
	assert.Error(t, err)
}

func TestApp_ResolutionValidation(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)

	tests := []struct {
		name    string
		r       preset.Resolution
		wantErr string
	}{
		{"empty name", preset.Resolution{Name: "", Width: 512, Height: 512}, "name is required"},
		{"whitespace name", preset.Resolution{Name: "   ", Width: 512, Height: 512}, "name is required"},
		{"width too small", preset.Resolution{Name: "Res", Width: 63, Height: 512}, "width and height must be between 64 and 4096"},
		{"width too large", preset.Resolution{Name: "Res", Width: 4097, Height: 512}, "width and height must be between 64 and 4096"},
		{"height too small", preset.Resolution{Name: "Res", Width: 512, Height: 32}, "width and height must be between 64 and 4096"},
		{"height too large", preset.Resolution{Name: "Res", Width: 512, Height: 5000}, "width and height must be between 64 and 4096"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.CreateResolution(tt.r)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestApp_ResolutionUpdateValidation(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)

	tests := []struct {
		name    string
		r       preset.Resolution
		wantErr string
	}{
		{"zero id", preset.Resolution{ID: 0, Name: "Res", Width: 512, Height: 512}, "id is required"},
		{"negative id", preset.Resolution{ID: -1, Name: "Res", Width: 512, Height: 512}, "id is required"},
		{"empty name", preset.Resolution{ID: 1, Name: "", Width: 512, Height: 512}, "name is required"},
		{"width out of range", preset.Resolution{ID: 1, Name: "Res", Width: 10, Height: 512}, "width and height must be between 64 and 4096"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.UpdateResolution(tt.r)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestApp_ListResolutionsEmptyNotNil(t *testing.T) {
	t.Parallel()
	app, db := newTestApp(t)

	items, err := db.ListResolutions()
	require.NoError(t, err)
	assert.NotEmpty(t, items)

	result, err := app.ListResolutions()
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestApp_HiresProfileCRUD(t *testing.T) {
	t.Parallel()
	app, db := newTestApp(t)

	h := preset.HiresProfile{Name: "Test Profile", Upscale: 2.0, DenoisingStrength: 0.5, Upscaler: "R-ESRGAN 4x+"}
	created, err := app.CreateHiresProfile(h)
	require.NoError(t, err)
	assert.Greater(t, created.ID, int64(0))
	assert.Equal(t, "Test Profile", created.Name)

	got, err := db.GetHiresProfile(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test Profile", got.Name)

	created.Name = "Updated Profile"
	created.Upscale = 3.0
	created.DenoisingStrength = 0.6
	updated, err := app.UpdateHiresProfile(*created)
	require.NoError(t, err)
	assert.Equal(t, "Updated Profile", updated.Name)
	assert.Equal(t, 3.0, updated.Upscale)

	err = app.DeleteHiresProfile(created.ID)
	require.NoError(t, err)

	_, err = db.GetHiresProfile(created.ID)
	assert.Error(t, err)
}

func TestApp_HiresProfileValidation(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)

	tests := []struct {
		name    string
		h       preset.HiresProfile
		wantErr string
	}{
		{"empty name", preset.HiresProfile{Name: "", Upscale: 2.0, DenoisingStrength: 0.5}, "name is required"},
		{"whitespace name", preset.HiresProfile{Name: "   ", Upscale: 2.0, DenoisingStrength: 0.5}, "name is required"},
		{"upscale too low", preset.HiresProfile{Name: "P", Upscale: 0.5, DenoisingStrength: 0.5}, "upscale must be between 1.0 and 4.0"},
		{"upscale too high", preset.HiresProfile{Name: "P", Upscale: 5.0, DenoisingStrength: 0.5}, "upscale must be between 1.0 and 4.0"},
		{"denoising too low", preset.HiresProfile{Name: "P", Upscale: 2.0, DenoisingStrength: -0.1}, "denoising_strength must be between 0.0 and 1.0"},
		{"denoising too high", preset.HiresProfile{Name: "P", Upscale: 2.0, DenoisingStrength: 1.1}, "denoising_strength must be between 0.0 and 1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.CreateHiresProfile(tt.h)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestApp_HiresProfileUpdateValidation(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)

	tests := []struct {
		name    string
		h       preset.HiresProfile
		wantErr string
	}{
		{"zero id", preset.HiresProfile{ID: 0, Name: "P", Upscale: 2.0, DenoisingStrength: 0.5}, "id is required"},
		{"negative id", preset.HiresProfile{ID: -1, Name: "P", Upscale: 2.0, DenoisingStrength: 0.5}, "id is required"},
		{"empty name", preset.HiresProfile{ID: 1, Name: "", Upscale: 2.0, DenoisingStrength: 0.5}, "name is required"},
		{"upscale out of range", preset.HiresProfile{ID: 1, Name: "P", Upscale: 0.0, DenoisingStrength: 0.5}, "upscale must be between 1.0 and 4.0"},
		{"denoising out of range", preset.HiresProfile{ID: 1, Name: "P", Upscale: 2.0, DenoisingStrength: 2.0}, "denoising_strength must be between 0.0 and 1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.UpdateHiresProfile(tt.h)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestApp_ListHiresProfilesEmptyNotNil(t *testing.T) {
	t.Parallel()
	app, db := newTestApp(t)

	items, err := db.ListHiresProfiles()
	require.NoError(t, err)
	assert.NotEmpty(t, items)

	result, err := app.ListHiresProfiles()
	require.NoError(t, err)
	assert.NotNil(t, result)
}
