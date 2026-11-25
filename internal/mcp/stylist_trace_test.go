package mcp

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/stylisttrace"
	"github.com/stretchr/testify/require"
)

type traceEngine struct {
	stubFaceEng
	root       string
	swapStatus string
}

func (e *traceEngine) NewStylistTrace() *stylisttrace.Recorder {
	r, _ := stylisttrace.New(e.root)
	return r
}
func (e *traceEngine) FaceSwap(img, _ string) (string, string) {
	return img, e.swapStatus
}
func readTrace(t *testing.T, root string) (string, int64, []string) {
	t.Helper()
	dirs, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, dirs, 1)
	dir := filepath.Join(root, dirs[0].Name())
	raw, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	require.NoError(t, err)
	var meta struct {
		FrameID int64 `json:"frame_id"`
		Stages  []struct {
			Stage string `json:"stage"`
		} `json:"stages"`
	}
	require.NoError(t, json.Unmarshal(raw, &meta))
	var stages []string
	for _, s := range meta.Stages {
		stages = append(stages, s.Stage)
	}
	return dir, meta.FrameID, stages
}
func TestQueuedShootTraceRecordsOnlySuccessfulStages(t *testing.T) {
	root := t.TempDir()
	eng := &traceEngine{root: root, swapStatus: "done"}
	sessions := &fakeSessions{}
	p := sqFrameParams()
	p.FaceSwap = true
	result, err := RunShootFrame(&fakeShooter{}, sessions, eng, nil, p)
	require.NoError(t, err)
	dir, id, stages := readTrace(t, root)
	require.Equal(t, result.ID, id)
	require.Equal(t, []string{"base", "face-swap"}, stages)
	saved, err := os.ReadFile(filepath.Join(dir, "face-swap.png"))
	require.NoError(t, err)
	expected, err := base64.StdEncoding.DecodeString(sessions.namedCalls()[0].image)
	require.NoError(t, err)
	require.Equal(t, expected, saved)
}
func TestInterruptedShootTraceIsRemoved(t *testing.T) {
	root := t.TempDir()
	eng := &traceEngine{root: root, swapStatus: generation.FaceSwapInterrupted}
	p := sqFrameParams()
	p.FaceSwap = true
	_, err := RunShootFrame(&fakeShooter{}, &fakeSessions{}, eng, nil, p)
	require.ErrorIs(t, err, generation.ErrInterrupted)
	dirs, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, dirs)
}
func TestSynchronousShootTraceRecordsFinalSessionID(t *testing.T) {
	root := t.TempDir()
	eng := &traceEngine{root: root}
	sessions := &fakeSessions{}
	args := shootArgs("anna", "front full body")
	args["face_pass"] = false
	_, err := stylistShoot(&fakePeople{people: shootPeopleAll()}, &fakeShooter{}, sessions, eng, nil, args)
	require.NoError(t, err)
	_, id, stages := readTrace(t, root)
	require.Equal(t, sessions.namedCalls()[0].returned, id)
	require.Equal(t, []string{"base"}, stages)
}

func (e *traceEngine) KleinTryOn(img, _, _ string, _ *int64) (*generation.GenerateImageResult, error) {
	return &generation.GenerateImageResult{Image: img}, nil
}
func TestQueuedShootTraceIncludesGarmentAndFacePass(t *testing.T) {
	root := t.TempDir()
	eng := &traceEngine{root: root}
	p := sqFrameParams()
	p.GarmentBase64 = "synthetic garment"
	p.FacePass = true
	result, err := RunShootFrame(&fakeShooter{}, &fakeSessions{}, eng, &fakeRefiner{size: 256}, p)
	require.NoError(t, err)
	require.Equal(t, "done", result.Garment)
	require.Equal(t, "done", result.FacePass)
	_, _, stages := readTrace(t, root)
	require.Equal(t, []string{"base", "garment", "face-pass"}, stages)
}
func TestSkippedSwapIsNotRecordedAsSuccessfulStage(t *testing.T) {
	root := t.TempDir()
	eng := &traceEngine{root: root, swapStatus: "skipped:no face"}
	p := sqFrameParams()
	p.FaceSwap = true
	_, err := RunShootFrame(&fakeShooter{}, &fakeSessions{}, eng, nil, p)
	require.NoError(t, err)
	_, _, stages := readTrace(t, root)
	require.Equal(t, []string{"base"}, stages)
}
