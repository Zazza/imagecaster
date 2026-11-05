package generation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
)

type faceDetectingSD struct {
	mockSD
	pts    []sd.Point
	err    error
	images []string
}

func (f *faceDetectingSD) FaceLandmarks(imageBase64 string) ([]sd.Point, error) {
	f.images = append(f.images, imageBase64)
	if f.err != nil {
		return nil, f.err
	}
	return f.pts, nil
}

func newFaceTestService(t *testing.T, sdSvc sd.Service) *Service {
	t.Helper()
	db := openTestDB(t)
	return New(db, &mockLLM{}, sdSvc, &config.Config{SDPromptModel: "test-model", VisionModel: "test-vision"},
		t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, &mockSettings{}, nil, logger.New(nil))
}

func TestFaceLandmarks_ConvertsPointsOneToOne(t *testing.T) {
	t.Parallel()
	pts := make([]sd.Point, 70)
	for i := range pts {
		pts[i] = sd.Point{X: float64(i) / 100, Y: 1 - float64(i)/200}
	}
	det := &faceDetectingSD{pts: pts}
	svc := newFaceTestService(t, det)

	got, err := svc.FaceLandmarks("IMGB64")

	require.NoError(t, err)
	require.Len(t, got, 70)
	for i, p := range got {
		assert.Equal(t, faceguard.Point{X: pts[i].X, Y: pts[i].Y}, p, "point %d", i)
	}
	assert.Equal(t, []string{"IMGB64"}, det.images)
}

func TestFaceLandmarks_ClientWithoutDetector(t *testing.T) {
	t.Parallel()
	svc := newFaceTestService(t, &mockSD{})

	got, err := svc.FaceLandmarks("IMGB64")

	require.Error(t, err)
	assert.Empty(t, got)
}

func TestFaceLandmarks_DetectorErrorPropagates(t *testing.T) {
	t.Parallel()
	detErr := errors.New("detector boom")
	svc := newFaceTestService(t, &faceDetectingSD{err: detErr})

	got, err := svc.FaceLandmarks("IMGB64")

	require.Error(t, err)
	assert.ErrorIs(t, err, detErr)
	assert.Empty(t, got)
}
