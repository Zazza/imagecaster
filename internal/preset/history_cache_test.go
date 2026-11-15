package preset

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func historyScans(d *DB) int {
	d.historyMu.Lock()
	defer d.historyMu.Unlock()
	if d.historyCache == nil {
		return 0
	}
	return d.historyCache.scans
}

func TestHistoryStamp_DataVersionSeesOtherConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	a, err := Open(path)
	require.NoError(t, err)
	defer a.Close()
	b, err := Open(path)
	require.NoError(t, err)
	defer b.Close()

	s0, err := a.historyStamp()
	require.NoError(t, err)
	_, err = a.CreateSession("own write")
	require.NoError(t, err)
	s1, err := a.historyStamp()
	require.NoError(t, err)
	assert.Equal(t, s0.dataVersion, s1.dataVersion)
	assert.NotEqual(t, s0.totalChanges, s1.totalChanges)

	_, err = b.CreateSession("other write")
	require.NoError(t, err)
	s2, err := a.historyStamp()
	require.NoError(t, err)
	assert.NotEqual(t, s1.dataVersion, s2.dataVersion)
	assert.Equal(t, s1.totalChanges, s2.totalChanges)

	s3, err := a.historyStamp()
	require.NoError(t, err)
	assert.Equal(t, s2, s3)
}

func TestListHistory_CacheInvalidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	a, err := Open(path)
	require.NoError(t, err)
	defer a.Close()
	other, err := Open(path)
	require.NoError(t, err)
	defer other.Close()
	sid := newSession(t, a, "s")
	addHist(t, a, histRow{session: sid, source: "generate", prompt: "cat", w: 64, h: 64})

	tests := []struct {
		name      string
		filter    HistoryFilter
		write     func(t *testing.T)
		wantTotal int
		wantScan  bool
	}{
		{"first call scans", HistoryFilter{}, nil, 1, true},
		{"next page served from cache", HistoryFilter{Offset: 1}, nil, 1, false},
		{"other search scans", HistoryFilter{Search: "dog"}, nil, 0, true},
		{"back to no search scans", HistoryFilter{}, nil, 1, true},
		{"write from same connection", HistoryFilter{}, func(t *testing.T) {
			addHist(t, a, histRow{session: sid, source: "edit", prompt: "dog", w: 64, h: 64})
		}, 2, true},
		{"write from another connection", HistoryFilter{}, func(t *testing.T) {
			addHist(t, other, histRow{session: sid, source: "edit", prompt: "dog", w: 64, h: 64})
		}, 3, true},
		{"rename session from another connection", HistoryFilter{}, func(t *testing.T) {
			require.NoError(t, other.RenameSession(sid, "renamed"))
		}, 3, true},
		{"unchanged again cached", HistoryFilter{}, nil, 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.write != nil {
				tt.write(t)
			}
			before := historyScans(a)
			p := listHist(t, a, tt.filter)
			assert.Equal(t, tt.wantTotal, p.Total)
			assert.Equal(t, tt.wantScan, historyScans(a) > before)
		})
	}
	p := listHist(t, a, HistoryFilter{})
	f, ok := facet(p.Facets.Sessions, strconv.FormatInt(sid, 10))
	require.True(t, ok)
	assert.Equal(t, "renamed", f.Name)
}
