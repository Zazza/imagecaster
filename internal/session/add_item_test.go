package session

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serviceWithDB(t *testing.T) (*Service, *mockEmitter, string, *preset.DB) {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	dataDir := t.TempDir()
	emitter := &mockEmitter{}
	return New(db, dataDir, emitter), emitter, dataDir, db
}

func pngBase64(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7)
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func gifBase64(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func dirFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return nil
	}
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestAddItem_WritesOriginalBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		image     func(t *testing.T) string
		wantExt   string
		thumbExt  string
		sameBytes bool
		thumbSame bool
		w, h      int
	}{
		{"small png kept as is", func(t *testing.T) string { return pngBase64(t, 100, 80) }, ".png", ".png", true, true, 100, 80},
		{"big png kept as is, jpeg thumb", func(t *testing.T) string { return pngBase64(t, 300, 200) }, ".png", ".jpg", true, false, 300, 200},
		{"jpeg kept as is with jpg extension", func(t *testing.T) string { return makeImageJPEG(t, 320, 240) }, ".jpg", ".jpg", true, false, 320, 240},
		{"small jpeg thumb is the original", func(t *testing.T) string { return makeImageJPEG(t, 64, 64) }, ".jpg", ".jpg", true, true, 64, 64},
		{"gif converted to png", func(t *testing.T) string { return gifBase64(t, 40, 30) }, ".png", ".png", false, true, 40, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, dataDir, _ := serviceWithDB(t)
			b64 := tt.image(t)
			id := svc.AddToSession(b64, nil, "sd", nil)
			require.Greater(t, id, int64(0))
			item, err := svc.SessionItem(id)
			require.NoError(t, err)
			idStr := strconv.FormatInt(id, 10)
			assert.Equal(t, idStr+tt.wantExt, item.FileName)
			assert.Equal(t, idStr+tt.thumbExt, item.ThumbName)
			assert.Equal(t, tt.w, item.Width)
			assert.Equal(t, tt.h, item.Height)

			sid := strconv.FormatInt(item.SessionID, 10)
			written, err := os.ReadFile(filepath.Join(dataDir, "sessions", sid, item.FileName))
			require.NoError(t, err)
			orig, err := base64.StdEncoding.DecodeString(b64)
			require.NoError(t, err)
			if tt.sameBytes {
				assert.Equal(t, orig, written)
			} else {
				_, format, err := image.DecodeConfig(bytes.NewReader(written))
				require.NoError(t, err)
				assert.Equal(t, "png", format)
			}
			thumb, err := os.ReadFile(filepath.Join(dataDir, "thumbs", sid, item.ThumbName))
			require.NoError(t, err)
			if tt.thumbSame {
				assert.Equal(t, written, thumb)
			} else {
				cfg, format, err := image.DecodeConfig(bytes.NewReader(thumb))
				require.NoError(t, err)
				assert.Equal(t, "jpeg", format)
				assert.LessOrEqual(t, max(cfg.Width, cfg.Height), thumbSize)
			}
			assert.Len(t, dirFiles(t, filepath.Join(dataDir, "sessions", sid)), 1)
			assert.Len(t, dirFiles(t, filepath.Join(dataDir, "thumbs", sid)), 1)
		})
	}
}

func TestAddItem_FailureLeavesNoRowAndNoFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(t *testing.T, db *preset.DB, dataDir string, sessionID int64)
		image func(t *testing.T) string
	}{
		{"broken base64", nil, func(*testing.T) string { return "!!!" }},
		{"not an image", nil, func(*testing.T) string { return base64.StdEncoding.EncodeToString([]byte("text")) }},
		{"session dir cannot be created", func(t *testing.T, _ *preset.DB, dataDir string, _ int64) {
			require.NoError(t, os.WriteFile(filepath.Join(dataDir, "sessions"), []byte("x"), 0o644))
		}, func(t *testing.T) string { return pngBase64(t, 300, 200) }},
		{"thumb dir cannot be created", func(t *testing.T, _ *preset.DB, dataDir string, _ int64) {
			require.NoError(t, os.WriteFile(filepath.Join(dataDir, "thumbs"), []byte("x"), 0o644))
		}, func(t *testing.T) string { return pngBase64(t, 300, 200) }},
		{"insert fails", func(t *testing.T, db *preset.DB, _ string, _ int64) {
			_, err := db.DB().Exec(`CREATE TRIGGER no_insert BEFORE INSERT ON session_items BEGIN SELECT RAISE(ABORT, 'insert refused'); END`)
			require.NoError(t, err)
		}, func(t *testing.T) string { return pngBase64(t, 300, 200) }},
		{"saving names fails after files are placed", func(t *testing.T, db *preset.DB, _ string, _ int64) {
			_, err := db.DB().Exec(`CREATE TRIGGER no_names BEFORE UPDATE OF file_name ON session_items BEGIN SELECT RAISE(ABORT, 'update refused'); END`)
			require.NoError(t, err)
		}, func(t *testing.T) string { return makeImageJPEG(t, 300, 200) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, emitter, dataDir, db := serviceWithDB(t)
			sessionID, err := db.GetActiveSessionID()
			require.NoError(t, err)
			require.NotZero(t, sessionID)
			if tt.setup != nil {
				tt.setup(t, db, dataDir, sessionID)
			}
			id := svc.AddToSession(tt.image(t), nil, "sd", nil)
			assert.Zero(t, id)
			items, err := db.GetSessionItems(sessionID)
			require.NoError(t, err)
			assert.Empty(t, items)
			sid := strconv.FormatInt(sessionID, 10)
			assert.Empty(t, dirFiles(t, filepath.Join(dataDir, "sessions", sid)))
			assert.Empty(t, dirFiles(t, filepath.Join(dataDir, "thumbs", sid)))
			assert.False(t, emitter.hasEvent("session:added"))
		})
	}
}

func TestDeleteSession_ActiveSwitchFailureKeepsSession(t *testing.T) {
	t.Parallel()
	svc, emitter, _, db := serviceWithDB(t)
	extra, err := svc.CreateSession("extra")
	require.NoError(t, err)
	id := svc.AddToSession(pngBase64(t, 32, 32), nil, "sd", nil)
	require.Greater(t, id, int64(0))
	_, err = db.DB().Exec(`CREATE TRIGGER no_switch BEFORE UPDATE ON session_active BEGIN SELECT RAISE(ABORT, 'switch refused'); END`)
	require.NoError(t, err)

	err = svc.DeleteSession(extra.ID)
	require.Error(t, err)
	assert.False(t, emitter.hasEvent("session:deleted"))
	active, err := db.GetActiveSessionID()
	require.NoError(t, err)
	assert.Equal(t, extra.ID, active)
	item, err := svc.SessionItem(id)
	require.NoError(t, err)
	assert.NotNil(t, item)
}

func TestDeleteSession_RemovesFilesAndSwitchesActive(t *testing.T) {
	t.Parallel()
	svc, emitter, dataDir, db := serviceWithDB(t)
	extra, err := svc.CreateSession("extra")
	require.NoError(t, err)
	require.Greater(t, svc.AddToSession(pngBase64(t, 300, 200), nil, "sd", nil), int64(0))

	require.NoError(t, svc.DeleteSession(extra.ID))
	assert.True(t, emitter.hasEvent("session:deleted"))
	active, err := db.GetActiveSessionID()
	require.NoError(t, err)
	assert.NotEqual(t, extra.ID, active)
	assert.NotZero(t, active)
	sid := strconv.FormatInt(extra.ID, 10)
	assert.NoDirExists(t, filepath.Join(dataDir, "sessions", sid))
	assert.NoDirExists(t, filepath.Join(dataDir, "thumbs", sid))
	assert.Greater(t, svc.AddToSession(pngBase64(t, 32, 32), nil, "sd", nil), int64(0))
}
