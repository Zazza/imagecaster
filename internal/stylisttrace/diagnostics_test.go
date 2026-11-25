package stylisttrace_test

import (
	"github.com/Zazza/imagecaster/internal/stylisttrace"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnosticsClearRequiresConfirmationAndIdleRecorder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "diagnostics")
	r, err := stylisttrace.NewWithOptions(root, stylisttrace.Options{KeepOriginals: true, Retention: 1})
	require.NoError(t, err)
	require.NoError(t, r.Save(stylisttrace.Base, testImage(t)))
	require.Error(t, stylisttrace.Clear(root, false))
	require.Error(t, stylisttrace.Clear(root, true))
	stats, err := stylisttrace.Storage(root)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Active)
	require.NoError(t, r.Finish(1))
	require.NoError(t, r.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "history-sentinel"), []byte("keep"), 0600))
	require.NoError(t, stylisttrace.Clear(root, true))
	data, err := os.ReadFile(filepath.Join(root, "history-sentinel"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
	stats, err = stylisttrace.Storage(root)
	require.NoError(t, err)
	require.Zero(t, stats.Frames)
}

func TestDiagnosticsClearRejectsSymlinkWithoutTouchingOutside(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "diagnostics")
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.MkdirAll(root, 0700))
	require.NoError(t, os.MkdirAll(outside, 0700))
	sentinel := filepath.Join(outside, "photo.png")
	require.NoError(t, os.WriteFile(sentinel, []byte("private photo"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "frame-unsafe")))
	require.Error(t, stylisttrace.Clear(root, true))
	data, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "private photo", string(data))
}

func TestDiagnosticsRetentionRespectsConfiguredLimit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "diagnostics")
	for id := int64(1); id <= 3; id++ {
		r, err := stylisttrace.NewWithOptions(root, stylisttrace.Options{DebugImages: true, KeepOriginals: true, Retention: 1})
		require.NoError(t, err)
		require.NoError(t, r.Save(stylisttrace.Base, testImage(t)))
		require.NoError(t, r.Finish(id))
		require.NoError(t, r.Close())
	}
	stats, err := stylisttrace.Storage(root)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Frames)
	require.Positive(t, stats.Bytes)
}

func TestDiagnosticsOriginalsRemainRestorableWithoutDebugImages(t *testing.T) {
	root := filepath.Join(t.TempDir(), "diagnostics")
	r, err := stylisttrace.NewWithOptions(root, stylisttrace.Options{KeepOriginals: true, Retention: 10})
	require.NoError(t, err)
	image := testImage(t)
	require.NoError(t, r.Save(stylisttrace.Base, image))
	require.NoError(t, r.Save(stylisttrace.Garment, image))
	require.NoError(t, r.Save(stylisttrace.FaceSwap, image))
	require.NoError(t, r.Finish(42))
	require.NoError(t, r.Close())
	got, err := stylisttrace.ReadFrame(root, 42, "base")
	require.NoError(t, err)
	require.Equal(t, image, got)
	got, err = stylisttrace.ReadFrame(root, 42, "before_face")
	require.NoError(t, err)
	require.Equal(t, image, got)
	dirs, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, dirs, 1)
	files, err := os.ReadDir(filepath.Join(root, dirs[0].Name()))
	require.NoError(t, err)
	for _, file := range files {
		require.NotEqual(t, "face-swap.png", file.Name())
	}
}

func TestDiagnosticsStorageDoesNotCreateMissingFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	stats, err := stylisttrace.Storage(root)
	require.NoError(t, err)
	require.False(t, stats.Exists)
	require.Equal(t, root, stats.Path)
	_, err = os.Stat(root)
	require.True(t, os.IsNotExist(err))
}

func TestDiagnosticsDisabledCreatesNoFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "disabled")
	recorder, err := stylisttrace.NewWithOptions(root, stylisttrace.Options{})
	require.NoError(t, err)
	require.Nil(t, recorder)
	_, err = os.Stat(root)
	require.True(t, os.IsNotExist(err))
}
