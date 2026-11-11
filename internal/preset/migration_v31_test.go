package preset

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func v31SettingExists(t *testing.T, db *DB, key string) bool {
	t.Helper()
	var v string
	err := db.DB().QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return false
	}
	require.NoError(t, err)
	return true
}

func TestMigrationV31_RemovesModelListSettings(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedOldDB(t, path, map[string]string{
		"heavy_models":    "flux,moss,megatron",
		"prose_models":    "flux,z-image",
		"llm_auto_unload": "false",
		"analyze_prompt":  "keep me",
	})

	db := openFileDB(t, path)
	t.Cleanup(func() { db.Close() })

	assert.False(t, v31SettingExists(t, db, "heavy_models"))
	assert.False(t, v31SettingExists(t, db, "prose_models"))

	v, err := db.GetSetting("llm_auto_unload")
	require.NoError(t, err)
	assert.Equal(t, "false", v)
	v, err = db.GetSetting("analyze_prompt")
	require.NoError(t, err)
	assert.Equal(t, "keep me", v)
}

func TestMigrationV31_EmptyValuesRemovedToo(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedOldDB(t, path, map[string]string{"heavy_models": "", "prose_models": ""})
	db := openFileDB(t, path)
	t.Cleanup(func() { db.Close() })
	assert.False(t, v31SettingExists(t, db, "heavy_models"))
	assert.False(t, v31SettingExists(t, db, "prose_models"))
}

func TestMigrationV31_OnlyOneKeyPresent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedOldDB(t, path, map[string]string{"prose_models": "flux", "sd_prompt_instruction": "mine"})
	db := openFileDB(t, path)
	t.Cleanup(func() { db.Close() })
	assert.False(t, v31SettingExists(t, db, "prose_models"))
	v, err := db.GetSetting("sd_prompt_instruction")
	require.NoError(t, err)
	assert.Equal(t, "mine", v)
}

func TestMigrationV31_FreshDBIsFine(t *testing.T) {
	t.Parallel()
	db := openFileDB(t, filepath.Join(t.TempDir(), "presets.db"))
	t.Cleanup(func() { db.Close() })
	assert.False(t, v31SettingExists(t, db, "heavy_models"))
	assert.False(t, v31SettingExists(t, db, "prose_models"))
}

func TestMigrationV31_ReopenIsSafe(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedOldDB(t, path, map[string]string{
		"heavy_models":   "flux",
		"prose_models":   "flux",
		"analyze_prompt": "keep me",
	})
	db := openFileDB(t, path)
	require.NoError(t, db.Close())

	for i := 0; i < 2; i++ {
		reopened, err := Open(path)
		require.NoError(t, err, "reopen #%d", i+1)
		assert.False(t, v31SettingExists(t, reopened, "heavy_models"))
		assert.False(t, v31SettingExists(t, reopened, "prose_models"))
		v, err := reopened.GetSetting("analyze_prompt")
		require.NoError(t, err)
		assert.Equal(t, "keep me", v)
		require.NoError(t, reopened.Close())
	}
}
