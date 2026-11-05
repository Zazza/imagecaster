package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

func TestWatchExternal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "presets.db")
	db1, err := preset.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db1.Close() })
	emitter := &mockEmitter{}
	svc := New(db1, dir, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		svc.WatchExternal(ctx, 20*time.Millisecond)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 0, emitter.eventCount("session:created"), "no changes — no events")

	db2, err := preset.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db2.Close() })
	_, err = db2.CreateSession("external")
	require.NoError(t, err)

	assert.Eventually(t, func() bool { return emitter.hasEvent("session:created") }, 2*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchExternal did not return after ctx cancel")
	}
}
