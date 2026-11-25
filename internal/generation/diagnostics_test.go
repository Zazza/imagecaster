package generation

import (
	"encoding/json"
	"errors"
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/stylisttrace"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnosticsStageSettingTakesEffectWithoutRestart(t *testing.T) {
	db := openTestDB(t)
	backend := &performanceSD{response: &sd.Txt2ImgResponse{Images: []string{"image"}}}
	log := logger.New(nil)
	svc := &Service{db: db, sd: backend, log: log}
	_, err := svc.measuredTxt2Img(sd.Txt2ImgRequest{})
	require.NoError(t, err)
	require.Empty(t, log.Entries())
	require.NoError(t, db.SetSetting("diagnostics_verbose_logs", "true"))
	_, err = svc.measuredTxt2Img(sd.Txt2ImgRequest{})
	require.NoError(t, err)
	require.Len(t, log.Entries(), 2)
	require.NoError(t, db.SetSetting("diagnostics_verbose_logs", "false"))
	_, err = svc.measuredTxt2Img(sd.Txt2ImgRequest{})
	require.NoError(t, err)
	require.Len(t, log.Entries(), 2)
	require.Equal(t, 3, backend.calls)
}

func TestDiagnosticsDisabledPreservesMemoryGuardUnloadingAndWarnings(t *testing.T) {
	db := openTestDB(t)
	loaded := &guardLLM{listLoadedFn: func(call int) ([]llm.LoadedModel, error) {
		if call == 1 {
			return []llm.LoadedModel{{Name: "model", Size: gibI(6), SizeVRAM: gibI(6)}}, nil
		}
		return nil, nil
	}}
	log := logger.New(nil)
	guard := NewMemoryGuard(loaded, &guardSD{stats: cudaStats(gib(2), gib(20))}, db, log)
	require.True(t, guard.EnsureHeadroom("flux", "test"))
	require.EqualValues(t, 1, loaded.unloadAllCalls.Load())
	require.Empty(t, log.Entries())
	broken := NewMemoryGuard(&guardLLM{listLoadedFn: func(int) ([]llm.LoadedModel, error) { return nil, errors.New("unavailable") }}, &guardSD{}, db, log)
	require.False(t, broken.EnsureHeadroom("flux", "test"))
	entries := log.Entries()
	require.NotEmpty(t, entries)
	require.Contains(t, entries[len(entries)-1].Message, "unavailable")
}

func TestDiagnosticsTraceSnapshotsRetentionAtCreation(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("diagnostics_retention", "1"))
	svc := &Service{db: db, cfg: &config.Config{SDUrl: "http://127.0.0.1:7860"}, dataDir: t.TempDir()}
	original := svc.NewStylistTrace()
	require.NotNil(t, original)
	require.NoError(t, original.Save(stylisttrace.Base, tinyPNGBase64(t)))
	defer original.Close()
	require.NoError(t, db.SetSetting("diagnostics_retention", "50"))
	for _, id := range []int64{2, 3} {
		next := svc.NewStylistTrace()
		require.NotNil(t, next)
		require.NoError(t, next.Save(stylisttrace.Base, tinyPNGBase64(t)))
		require.NoError(t, next.Finish(id))
		require.NoError(t, next.Close())
	}
	root := filepath.Join(svc.dataDir, stylisttrace.Directory)
	before, err := stylisttrace.Storage(root)
	require.NoError(t, err)
	require.Equal(t, 3, before.Frames)
	require.Equal(t, 1, before.Active)
	require.NoError(t, original.Finish(1))
	require.NoError(t, original.Close())
	after, err := stylisttrace.Storage(root)
	require.NoError(t, err)
	require.Equal(t, 1, after.Frames)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	raw, err := os.ReadFile(filepath.Join(root, entries[0].Name(), "metadata.json"))
	require.NoError(t, err)
	var meta struct {
		FrameID int64 `json:"frame_id"`
	}
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.EqualValues(t, 1, meta.FrameID)
}
