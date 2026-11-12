package preset

import (
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const migrationV20Key = "migration_v20_done"

func openFileDB(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(path)
	require.NoError(t, err)
	return db
}

func insertBundledPreset(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	res, err := db.DB().Exec(`INSERT INTO presets (name, preset_type, is_bundled) VALUES (?, 'post', 1)`, name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func presetExists(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	var n int
	require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM presets WHERE id = ?`, id).Scan(&n))
	return n > 0
}

func presetTypeExists(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	types, err := db.ListPresetTypes()
	require.NoError(t, err)
	for _, pt := range types {
		if pt.ID == id {
			return true
		}
	}
	return false
}

func bundledIDs(t *testing.T, db *DB) []int64 {
	t.Helper()
	all, err := db.List()
	require.NoError(t, err)
	var ids []int64
	for _, p := range all {
		if p.IsBundled {
			ids = append(ids, p.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func TestMigrationV20_ReopenKeepsBundledAndEmptyCategory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")

	db := openFileDB(t, path)
	done, err := db.GetSetting(migrationV20Key)
	require.NoError(t, err)
	assert.NotEmpty(t, done, "key must be written after first Open")

	pt := &PresetType{Name: "Empty category V20"}
	require.NoError(t, db.CreatePresetType(pt))
	require.Greater(t, pt.ID, int64(0))
	bundledID := insertBundledPreset(t, db, "Bundled V20")
	require.NoError(t, db.Close())

	reopened := openFileDB(t, path)
	t.Cleanup(func() { reopened.Close() })

	assert.True(t, presetExists(t, reopened, bundledID), "bundled preset must survive reopen")
	assert.True(t, presetTypeExists(t, reopened, pt.ID), "empty category must survive reopen")

	done, err = reopened.GetSetting(migrationV20Key)
	require.NoError(t, err)
	assert.NotEmpty(t, done)
}

func TestMigrationV20_ReopenThenSeedKeepsBundledIDs(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	fsys := fstest.MapFS{
		"data/presets/x.json": seedFile(t,
			seedPreset("V20 Seed One", "post", nil),
			seedPreset("V20 Seed Two", "post", nil),
		),
	}

	db := openFileDB(t, path)
	require.NoError(t, seedAllForTest(db, fsys))
	before := bundledIDs(t, db)
	require.Len(t, before, 2)
	require.NoError(t, db.Close())

	reopened := openFileDB(t, path)
	t.Cleanup(func() { reopened.Close() })
	assert.Equal(t, before, bundledIDs(t, reopened), "reopen must not delete bundled presets")

	require.NoError(t, seedAllForTest(reopened, fsys))
	assert.Equal(t, before, bundledIDs(t, reopened), "seed after reopen must not recreate bundled presets")
}

func TestMigrationV20_OldInstallWithoutKeyRunsOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")

	db := openFileDB(t, path)
	pt := &PresetType{Name: "Empty category old install"}
	require.NoError(t, db.CreatePresetType(pt))
	bundledID := insertBundledPreset(t, db, "Bundled old install")
	userPreset := &Preset{Name: "User preset old install"}
	require.NoError(t, db.Create(userPreset))
	_, err := db.DB().Exec(`DELETE FROM settings WHERE key = ?`, migrationV20Key)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened := openFileDB(t, path)

	assert.False(t, presetExists(t, reopened, bundledID), "bundled preset must be deleted on old install")
	assert.False(t, presetTypeExists(t, reopened, pt.ID), "empty category must be deleted on old install")
	assert.True(t, presetExists(t, reopened, userPreset.ID), "user preset must survive")

	done, err := reopened.GetSetting(migrationV20Key)
	require.NoError(t, err)
	assert.NotEmpty(t, done, "key must be written again")

	pt2 := &PresetType{Name: "Empty category after rerun"}
	require.NoError(t, reopened.CreatePresetType(pt2))
	bundled2 := insertBundledPreset(t, reopened, "Bundled after rerun")
	require.NoError(t, reopened.Close())

	third := openFileDB(t, path)
	t.Cleanup(func() { third.Close() })
	assert.True(t, presetExists(t, third, bundled2), "migration must not run a second time")
	assert.True(t, presetTypeExists(t, third, pt2.ID), "migration must not run a second time")
}
