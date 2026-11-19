package main

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/bodyshape"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/finish"
)

type phAdd struct {
	got []string
	id  int64
}

func (a *phAdd) add(img string) int64 {
	a.got = append(a.got, img)
	return a.id
}

func phFull() ExportImageParams {
	return ExportImageParams{
		ImageBase64: "c3Jj",
		Format:      "jpeg",
		Width:       800,
		Height:      600,
		Quality:     70,
		OutputDir:   "/some/dir",
		Adjust:      &adjust.Params{Brightness: 12, Saturation: -5},
		Finish:      &finish.Params{Ratio: "4:5", Frame: "white", Caption: "hi"},
		Retouch:     &faceguard.Retouch{UnderEyes: true, EyeColor: "blue"},
		FacePoints:  []faceguard.Point{{X: 0.1, Y: 0.2}, {X: 0.3, Y: 0.4}},
		Body:        &bodyshape.Params{Width: -0.1, Height: 0.05},
		BodyPoints:  []bodyshape.Point{{X: 0.5, Y: 0.6}},
	}
}

func TestSavePostToHistory_ForcesFullSizePNGNoDir(t *testing.T) {
	cases := []struct {
		name string
		p    ExportImageParams
	}{
		{"caller sent jpeg resize and dir", phFull()},
		{"caller sent webp only width", ExportImageParams{ImageBase64: "c3Jj", Format: "webp", Width: 1024, OutputDir: "x"}},
		{"caller sent defaults", ExportImageParams{ImageBase64: "c3Jj"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := &prExporter{}
			a := &phAdd{id: 7}
			_, err := savePostToHistory(ex, a.add, tc.p)
			require.NoError(t, err)
			require.Len(t, ex.got, 1)
			got := ex.got[0]
			assert.Equal(t, "png", got.Format)
			assert.Equal(t, 0, got.Width)
			assert.Equal(t, 0, got.Height)
			assert.Empty(t, got.OutputDir)
			assert.Equal(t, tc.p.ImageBase64, got.ImageBase64)
		})
	}
}

func TestSavePostToHistory_PassesEditsThrough(t *testing.T) {
	p := phFull()
	ex := &prExporter{}
	a := &phAdd{id: 3}
	_, err := savePostToHistory(ex, a.add, p)
	require.NoError(t, err)
	require.Len(t, ex.got, 1)
	got := ex.got[0]
	assert.Equal(t, p.Adjust, got.Adjust)
	assert.Equal(t, p.Finish, got.Finish)
	assert.Equal(t, p.Retouch, got.Retouch)
	assert.Equal(t, p.FacePoints, got.FacePoints)
	assert.Equal(t, p.Body, got.Body)
	assert.Equal(t, p.BodyPoints, got.BodyPoints)
}

func TestSavePostToHistory_NoEditsSavesCopy(t *testing.T) {
	ex := &prExporter{}
	a := &phAdd{id: 1}
	id, err := savePostToHistory(ex, a.add, ExportImageParams{ImageBase64: "c3Jj"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), id)
	require.Len(t, ex.got, 1)
	assert.Nil(t, ex.got[0].Adjust)
	assert.Nil(t, ex.got[0].Retouch)
	assert.Nil(t, ex.got[0].Body)
	assert.Nil(t, ex.got[0].Finish)
}

func TestSavePostToHistory_AddsEncodedResultAndReturnsID(t *testing.T) {
	ex := &prExporter{}
	a := &phAdd{id: 42}
	id, err := savePostToHistory(ex, a.add, phFull())
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)
	require.Len(t, a.got, 1)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("img")), a.got[0])
}

func TestSavePostToHistory_EachCallIsNewFrame(t *testing.T) {
	ex := &prExporter{}
	a := &phAdd{id: 5}
	p := phFull()
	_, err := savePostToHistory(ex, a.add, p)
	require.NoError(t, err)
	_, err = savePostToHistory(ex, a.add, p)
	require.NoError(t, err)
	assert.Len(t, ex.got, 2)
	assert.Len(t, a.got, 2)
}

func TestSavePostToHistory_NoActiveSession(t *testing.T) {
	ex := &prExporter{}
	a := &phAdd{id: 0}
	id, err := savePostToHistory(ex, a.add, phFull())
	require.Error(t, err)
	assert.ErrorIs(t, err, errPostNoSession)
	assert.Equal(t, int64(0), id)
}

func TestSavePostToHistory_ExporterError(t *testing.T) {
	boom := errors.New("decode failed")
	ex := &prExporter{err: boom}
	a := &phAdd{id: 9}
	id, err := savePostToHistory(ex, a.add, phFull())
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, int64(0), id)
	assert.Empty(t, a.got)
}
