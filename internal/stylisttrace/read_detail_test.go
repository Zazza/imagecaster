package stylisttrace_test

import (
	"bytes"
	"encoding/base64"
	"github.com/Zazza/imagecaster/internal/stylisttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"image"
	"image/png"
	"testing"
)

func TestReadFrameDetailStageAllowlistAndAssociation(t *testing.T) {
	root := t.TempDir()
	rec, err := stylisttrace.New(root)
	require.NoError(t, err)
	defer rec.Close()
	original := testImage(t)
	require.NoError(t, rec.Save(stylisttrace.Base, original))
	require.NoError(t, rec.Finish(42))
	got, err := stylisttrace.ReadFrame(root, 42, "base")
	require.NoError(t, err)
	assert.Equal(t, original, got)
	got, err = stylisttrace.ReadFrame(root, 42, "before_face")
	require.NoError(t, err)
	assert.Equal(t, original, got)
	for _, stage := range []string{"../base", "../../etc/passwd", "face-swap", "metadata.json", ""} {
		_, err = stylisttrace.ReadFrame(root, 42, stage)
		require.Error(t, err, stage)
	}
	_, err = stylisttrace.ReadFrame(root, 43, "base")
	require.Error(t, err)
}

func TestReadFrameDetailBeforeFaceStagePriority(t *testing.T) {
	for _, withPass := range []bool{false, true} {
		root := t.TempDir()
		rec, err := stylisttrace.New(root)
		require.NoError(t, err)
		defer rec.Close()
		require.NoError(t, rec.Save(stylisttrace.Base, testImage(t)))
		stageImage := func(width int) string {
			var buf bytes.Buffer
			require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, 2))))
			return base64.StdEncoding.EncodeToString(buf.Bytes())
		}
		garment := stageImage(4)
		require.NoError(t, rec.Save(stylisttrace.Garment, garment))
		want := garment
		if withPass {
			want = stageImage(5)
			require.NoError(t, rec.Save(stylisttrace.FacePass, want))
		}
		require.NoError(t, rec.Save(stylisttrace.FaceSwap, stageImage(6)))
		require.NoError(t, rec.Finish(42))
		got, err := stylisttrace.ReadFrame(root, 42, "before_face")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}
