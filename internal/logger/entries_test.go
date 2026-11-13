package logger

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntries_Ring(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		n       int
		wantLen int
	}{
		{name: "empty", n: 0, wantLen: 0},
		{name: "one", n: 1, wantLen: 1},
		{name: "below cap", n: 499, wantLen: 499},
		{name: "exactly cap", n: 500, wantLen: 500},
		{name: "cap plus one", n: 501, wantLen: 500},
		{name: "far above cap", n: 1234, wantLen: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := New(nil)
			for i := 1; i <= tt.n; i++ {
				l.Info("msg-%d", i)
			}
			got := l.Entries()
			require.Len(t, got, tt.wantLen)
			first := tt.n - tt.wantLen + 1
			for i, e := range got {
				num := first + i
				assert.Equal(t, fmt.Sprintf("msg-%d", num), e.Message)
				assert.Equal(t, INFO, e.Level)
				assert.Equal(t, int64(num), e.ID)
			}
		})
	}
}

func TestEntries_LevelsAndIncreasingIDs(t *testing.T) {
	t.Parallel()
	l := New(nil)
	l.Error("e")
	l.Warn("w")
	l.Info("i")
	l.Debug("d")
	got := l.Entries()
	require.Len(t, got, 4)
	wantLevels := []Level{ERROR, WARN, INFO, DEBUG}
	wantMsgs := []string{"e", "w", "i", "d"}
	for i, e := range got {
		assert.Equal(t, wantLevels[i], e.Level)
		assert.Equal(t, wantMsgs[i], e.Message)
		if i > 0 {
			assert.Greater(t, e.ID, got[i-1].ID)
		}
	}
	assert.Equal(t, int64(1), got[0].ID)
}

func TestEntries_ReturnsCopy(t *testing.T) {
	t.Parallel()
	l := New(nil)
	l.Info("original")
	got := l.Entries()
	require.Len(t, got, 1)
	got[0].Message = "changed"
	l.Info("second")
	again := l.Entries()
	require.Len(t, again, 2)
	assert.Equal(t, "original", again[0].Message)
	assert.Equal(t, "second", again[1].Message)
}

func TestEntries_ConcurrentUniqueIDs(t *testing.T) {
	t.Parallel()
	l := New(nil)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				l.Info("x")
			}
		}()
	}
	wg.Wait()
	got := l.Entries()
	require.Len(t, got, 400)
	for i := 1; i < len(got); i++ {
		assert.Greater(t, got[i].ID, got[i-1].ID)
	}
}
