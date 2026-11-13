package preset

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationV32_ReopenKeepsSdcppNamesAndWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presets.db")
	db := openFileDB(t, path)
	_, err := db.DB().Exec(`INSERT INTO presets (name, preset_type, sampler, schedule_type) VALUES ('forge', 'txt2img', 'DPM++ 2M', 'Karras')`)
	require.NoError(t, err)
	_, err = db.DB().Exec(`INSERT INTO settings (key, value) VALUES ('test_sampler', 'Euler a'), ('test_schedule_type', 'Exponential')`)
	require.NoError(t, err)
	_, err = db.DB().Exec(`PRAGMA user_version = 0`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	for i := 0; i < 2; i++ {
		db = openFileDB(t, path)
		var sampler, sched string
		require.NoError(t, db.DB().QueryRow(`SELECT sampler, schedule_type FROM presets WHERE name = 'forge'`).Scan(&sampler, &sched))
		assert.Equal(t, "dpm++2m", sampler)
		assert.Equal(t, "karras", sched)
		require.NoError(t, db.Close())
	}

	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	require.NoError(t, migrateV32(raw))
	var changes int
	require.NoError(t, raw.QueryRow(`SELECT total_changes()`).Scan(&changes))
	assert.Zero(t, changes)
}

func TestOpen_ReopenDoesNotRewriteSdcppNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presets.db")
	db := openFileDB(t, path)
	_, err := db.DB().Exec(`INSERT INTO presets (name, preset_type, sampler, schedule_type) VALUES ('forge', 'txt2img', 'DPM++ 2M', 'Karras')`)
	require.NoError(t, err)
	_, err = db.DB().Exec(`INSERT INTO settings (key, value) VALUES ('test_sampler', 'Euler a'), ('test_schedule_type', 'Exponential')`)
	require.NoError(t, err)
	_, err = db.DB().Exec(`PRAGMA user_version = 0`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.NoError(t, openFileDB(t, path).Close())

	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer raw.Close()
	_, err = raw.Exec(`
		CREATE TABLE name_rewrites (what TEXT);
		CREATE TRIGGER presets_names_rw AFTER UPDATE OF sampler, schedule_type ON presets
			WHEN old.sampler IS NOT new.sampler OR old.schedule_type IS NOT new.schedule_type
			BEGIN INSERT INTO name_rewrites VALUES ('preset ' || new.sampler || ' ' || new.schedule_type); END;
		CREATE TRIGGER hires_names_rw AFTER UPDATE OF upscaler ON hires_profiles
			WHEN old.upscaler IS NOT new.upscaler
			BEGIN INSERT INTO name_rewrites VALUES ('hires ' || new.upscaler); END;
		CREATE TRIGGER settings_names_rw AFTER UPDATE OF value ON settings
			WHEN new.key IN ('test_sampler', 'test_schedule_type') AND old.value IS NOT new.value
			BEGIN INSERT INTO name_rewrites VALUES ('setting ' || new.key || ' ' || new.value); END;
	`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	require.NoError(t, openFileDB(t, path).Close())

	raw, err = sql.Open("sqlite", path)
	require.NoError(t, err)
	defer raw.Close()
	rows, err := raw.Query(`SELECT what FROM name_rewrites`)
	require.NoError(t, err)
	defer rows.Close()
	var rewrites []string
	for rows.Next() {
		var w string
		require.NoError(t, rows.Scan(&w))
		rewrites = append(rewrites, w)
	}
	require.NoError(t, rows.Err())
	assert.Empty(t, rewrites)
}
