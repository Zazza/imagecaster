package preset

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func presetColumnNames(t *testing.T, raw *sql.DB) []string {
	t.Helper()
	rows, err := raw.Query(`SELECT name FROM pragma_table_info('presets')`)
	require.NoError(t, err)
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		cols = append(cols, c)
	}
	require.NoError(t, rows.Err())
	return cols
}

func TestSchemaV36_DropsFastModeLeftByV35(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		withColumn bool
	}{
		{"v35 database with fast_mode", true},
		{"v34 database without fast_mode", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "p.db")
			db, err := Open(path)
			require.NoError(t, err)
			require.NoError(t, db.Create(&Preset{Name: "keep", PresetType: "test", Prompt: "p", Sampler: "euler", Steps: 20, CfgScale: 7}))
			require.NoError(t, db.Close())

			raw, err := sql.Open("sqlite", path)
			require.NoError(t, err)
			version := 34
			if tt.withColumn {
				_, err = raw.Exec(`ALTER TABLE presets ADD COLUMN fast_mode TEXT NOT NULL DEFAULT ''`)
				require.NoError(t, err)
				_, err = raw.Exec(`UPDATE presets SET fast_mode = 'on'`)
				require.NoError(t, err)
				version = 35
			}
			_, err = raw.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version))
			require.NoError(t, err)
			require.NoError(t, raw.Close())

			db, err = Open(path)
			require.NoError(t, err)
			list, err := db.List()
			require.NoError(t, err)
			require.NoError(t, db.Close())

			raw, err = sql.Open("sqlite", path)
			require.NoError(t, err)
			defer raw.Close()
			assert.NotContains(t, presetColumnNames(t, raw), "fast_mode")
			v, err := userVersion(raw)
			require.NoError(t, err)
			assert.Equal(t, schemaVersion, v)
			names := []string{}
			for _, p := range list {
				names = append(names, p.Name)
			}
			assert.Contains(t, names, "keep")
		})
	}
}
