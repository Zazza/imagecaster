package mcp

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

type folderRec struct {
	mu      sync.Mutex
	sources []string
}

func (f *folderRec) AddToSession(_ string, _ json.RawMessage, source string, _ *preset.GenerationMeta) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sources = append(f.sources, source)
	return 500
}

func TestShootFrameHandler_PageFrameGoesToJobFolder(t *testing.T) {
	for _, tt := range []struct {
		name      string
		toFolder  bool
		wantID    int64
		inFolder  int
		inStylist int
	}{
		{name: "app page: folder of the job", toFolder: true, wantID: 500, inFolder: 1},
		{name: "mcp: «Стилист» folder", toFolder: false, wantID: 101, inStylist: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, err := preset.Open(":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { db.Close() })
			rec := &folderRec{}
			fsd := &pprSD{out: base64.StdEncoding.EncodeToString(styPNG(7))}
			cfg := &config.Config{SDUrl: "http://192.168.1.10:7860"}
			gen := generation.New(db, &tlLLM{}, fsd, cfg, t.TempDir(), NoopEmitter{}, kids.NewManager(db), rec, logger.New(nil))
			named := &fakeSessions{}

			p := sqFrameParams()
			p.ToFolder = tt.toFolder
			raw, err := json.Marshal(p)
			require.NoError(t, err)
			res, err := NewShootFrameHandler(named)(gen, &queue.Job{ID: 1, Params: string(raw)})
			require.NoError(t, err)

			var frame ShootFrameResult
			require.NoError(t, json.Unmarshal(res.Frame, &frame))
			assert.Equal(t, tt.wantID, frame.ID)
			assert.Len(t, rec.sources, tt.inFolder)
			assert.Len(t, named.calls, tt.inStylist)
			if tt.inStylist > 0 {
				assert.Equal(t, StylistSessionName, named.calls[0].name)
			}
			if tt.inFolder > 0 {
				assert.Equal(t, []string{"stylist"}, rec.sources)
			}
		})
	}
}
