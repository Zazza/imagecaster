package preset

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openMemDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestImageDescriptions_SchemaV40(t *testing.T) {
	t.Parallel()
	assert.GreaterOrEqual(t, schemaVersion, 40)
	raw := openRaw(t, freshDB(t))
	assert.Contains(t, queryStrings(t, raw, `SELECT name FROM sqlite_master WHERE type='table'`), "image_descriptions")
}

func TestImageDescriptions_SaveAndGet(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	require.NoError(t, db.SaveImageDescription("h1", "quick", "tag1, tag2"))

	text, found, err := db.GetImageDescription("h1", "quick")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "tag1, tag2", text)
}

func TestImageDescriptions_NotFound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		hash string
		mode string
	}{
		{"empty db", "nope", "quick"},
		{"other hash", "h2", "quick"},
		{"other mode", "h1", "describe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openMemDB(t)
			if tt.name != "empty db" {
				require.NoError(t, db.SaveImageDescription("h1", "quick", "x"))
			}
			text, found, err := db.GetImageDescription(tt.hash, tt.mode)
			require.NoError(t, err)
			assert.False(t, found)
			assert.Empty(t, text)
		})
	}
}

func TestImageDescriptions_SameKeyReplaces(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	require.NoError(t, db.SaveImageDescription("h1", "describe", "old"))
	require.NoError(t, db.SaveImageDescription("h1", "describe", "new"))

	text, found, err := db.GetImageDescription("h1", "describe")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "new", text)

	var n int
	require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM image_descriptions`).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestImageDescriptions_DifferentModeSeparate(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	require.NoError(t, db.SaveImageDescription("h1", "quick", "tags"))
	require.NoError(t, db.SaveImageDescription("h1", "describe", "text"))

	q, found, err := db.GetImageDescription("h1", "quick")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "tags", q)

	d, found, err := db.GetImageDescription("h1", "describe")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "text", d)
}

func TestImageDescriptions_V39DatabaseGetsTable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.db")
	db, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	raw := openRaw(t, path)
	_, err = raw.Exec(`DROP TABLE image_descriptions`)
	require.NoError(t, err)
	require.NoError(t, setUserVersion(raw, 39))
	require.NoError(t, raw.Close())

	db, err = Open(path)
	require.NoError(t, err)
	defer db.Close()
	v, err := userVersion(db.DB())
	require.NoError(t, err)
	assert.Equal(t, schemaVersion, v)

	require.NoError(t, db.SaveImageDescription("h", "quick", "t"))
	text, found, err := db.GetImageDescription("h", "quick")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "t", text)
}
