package preset

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var spaceRe = regexp.MustCompile(`\s+`)

func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { raw.Close() })
	return raw
}

func queryStrings(t *testing.T, db *sql.DB, q string) []string {
	t.Helper()
	rows, err := db.Query(q)
	require.NoError(t, err)
	defer rows.Close()
	cols, err := rows.Columns()
	require.NoError(t, err)
	var out []string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.NoError(t, rows.Scan(ptrs...))
		parts := make([]string, len(vals))
		for i, v := range vals {
			if v.Valid {
				parts[i] = v.String
			} else {
				parts[i] = "NULL"
			}
		}
		out = append(out, strings.Join(parts, "|"))
	}
	require.NoError(t, rows.Err())
	return out
}

func userTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	return queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
}

func schemaOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, tb := range userTables(t, db) {
		for _, c := range queryStrings(t, db, fmt.Sprintf(`SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info('%s')`, tb)) {
			out = append(out, "column "+tb+" "+c)
		}
		for _, fk := range queryStrings(t, db, fmt.Sprintf(`SELECT "table", "from", "to", on_update, on_delete FROM pragma_foreign_key_list('%s') ORDER BY "from"`, tb)) {
			out = append(out, "fk "+tb+" "+fk)
		}
	}
	for _, s := range queryStrings(t, db, `SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master WHERE type IN ('index', 'trigger', 'view') ORDER BY type, name`) {
		out = append(out, strings.TrimSpace(spaceRe.ReplaceAllString(s, " ")))
	}
	return out
}

func timestampColumns(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, c := range queryStrings(t, db, fmt.Sprintf(`SELECT name FROM pragma_table_info('%s') WHERE name IN ('created_at', 'updated_at')`, table)) {
		out[c] = true
	}
	return out
}

func dataOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, tb := range userTables(t, db) {
		cols := queryStrings(t, db, fmt.Sprintf(`SELECT name FROM pragma_table_info('%s') ORDER BY cid`, tb))
		skip := timestampColumns(t, db, tb)
		var keep []string
		for _, c := range cols {
			if !skip[c] {
				keep = append(keep, `"`+c+`"`)
			}
		}
		rows := queryStrings(t, db, fmt.Sprintf(`SELECT %s FROM "%s" ORDER BY 1`, strings.Join(keep, ", "), tb))
		for _, r := range rows {
			out = append(out, tb+" "+r)
		}
	}
	out = append(out, queryStrings(t, db, `SELECT 'seq ' || name || ' ' || seq FROM sqlite_sequence ORDER BY name`)...)
	sort.Strings(out)
	return out
}

func dataVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	require.NoError(t, db.QueryRow(`PRAGMA data_version`).Scan(&v))
	return v
}

func legacyDB(t *testing.T, steps int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	defer raw.Close()
	for _, m := range legacyMigrations[:steps] {
		require.NoError(t, m.apply(raw), m.name)
	}
	return path
}

func freshDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fresh.db")
	require.NoError(t, openFileDB(t, path).Close())
	return path
}

func TestSchema_FreshEqualsFullLegacyChain(t *testing.T) {
	t.Parallel()
	fresh := openRaw(t, freshDB(t))
	legacy := openRaw(t, legacyDB(t, len(legacyMigrations)))
	require.NoError(t, setUserVersion(legacy, legacySchemaVersion))
	require.NoError(t, migrateSchema(legacy))

	assert.Equal(t, schemaOf(t, legacy), schemaOf(t, fresh))
	assert.Equal(t, dataOf(t, legacy), dataOf(t, fresh))

	v, err := userVersion(fresh)
	require.NoError(t, err)
	assert.Equal(t, schemaVersion, v)
}

func TestSchema_OldDBUpgradesToLatest(t *testing.T) {
	t.Parallel()
	want := openRaw(t, freshDB(t))
	wantSchema := schemaOf(t, want)
	wantData := dataOf(t, want)

	tests := []struct {
		name  string
		steps int
	}{
		{"v1 base tables", 1},
		{"before export presets", 5},
		{"before hires profiles", 11},
		{"before v20 cleanup", 17},
		{"before job queue", 22},
		{"before v32 names", len(legacyMigrations) - 1},
		{"full legacy chain without version", len(legacyMigrations)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := legacyDB(t, tt.steps)
			require.NoError(t, openFileDB(t, path).Close())
			got := openRaw(t, path)
			assert.Equal(t, wantSchema, schemaOf(t, got))
			assert.Equal(t, wantData, dataOf(t, got))
			v, err := userVersion(got)
			require.NoError(t, err)
			assert.Equal(t, schemaVersion, v)
		})
	}
}

func TestSchema_ReopenWritesNothing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path func(t *testing.T) string
	}{
		{"fresh", freshDB},
		{"upgraded from v1", func(t *testing.T) string {
			path := legacyDB(t, 1)
			require.NoError(t, openFileDB(t, path).Close())
			return path
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.path(t)
			watcher := openRaw(t, path)
			before := dataVersion(t, watcher)
			require.NoError(t, openFileDB(t, path).Close())
			require.NoError(t, openFileDB(t, path).Close())
			assert.Equal(t, before, dataVersion(t, watcher))
		})
	}
}

func TestSchema_NewerVersionIsLeftAlone(t *testing.T) {
	t.Parallel()
	path := freshDB(t)
	raw := openRaw(t, path)
	_, err := raw.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion+5))
	require.NoError(t, err)
	before := dataVersion(t, raw)
	require.NoError(t, openFileDB(t, path).Close())
	assert.Equal(t, before, dataVersion(t, raw))
	v, err := userVersion(raw)
	require.NoError(t, err)
	assert.Equal(t, schemaVersion+5, v)
}

func TestSchemaV33_PresetsSamplerDefaultKeepsData(t *testing.T) {
	t.Parallel()
	path := legacyDB(t, len(legacyMigrations))
	raw := openRaw(t, path)
	_, err := raw.Exec(`INSERT INTO preset_types (name) VALUES ('t')`)
	require.NoError(t, err)
	for i := 1; i <= 5; i++ {
		_, err = raw.Exec(`INSERT INTO presets (name, sampler, schedule_type, type_id, loras) VALUES (?, 'dpm++2m', 'karras', 1, 'l')`, fmt.Sprintf("p%d", i))
		require.NoError(t, err)
	}
	_, err = raw.Exec(`DELETE FROM presets WHERE name = 'p5'`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO compound_presets (name) VALUES ('c')`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO compound_preset_steps (compound_preset_id, step_order, preset_id) VALUES (1, 1, 2)`)
	require.NoError(t, err)
	_, err = raw.Exec(`CREATE INDEX idx_presets_user ON presets(name)`)
	require.NoError(t, err)
	require.NoError(t, setUserVersion(raw, legacySchemaVersion))
	before := queryStrings(t, raw, `SELECT id, name, sampler, schedule_type, type_id, loras, created_at FROM presets ORDER BY id`)
	require.NoError(t, raw.Close())

	db := openFileDB(t, path)
	defer db.Close()
	after := queryStrings(t, db.DB(), `SELECT id, name, sampler, schedule_type, type_id, loras, created_at FROM presets ORDER BY id`)
	assert.Equal(t, before, after)
	assert.Equal(t, []string{"'euler_a'"}, queryStrings(t, db.DB(), `SELECT dflt_value FROM pragma_table_info('presets') WHERE name = 'sampler'`))
	assert.Equal(t, []string{"5"}, queryStrings(t, db.DB(), `SELECT seq FROM sqlite_sequence WHERE name = 'presets'`))
	assert.Equal(t, []string{"idx_presets_user"}, queryStrings(t, db.DB(), `SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'presets'`))
	assert.Equal(t, []string{"2"}, queryStrings(t, db.DB(), `SELECT preset_id FROM compound_preset_steps`))
	assert.Empty(t, queryStrings(t, db.DB(), `SELECT name FROM sqlite_master WHERE name = 'presets_v33'`))

	_, err = db.DB().Exec(`INSERT INTO presets (name) VALUES ('default sampler')`)
	require.NoError(t, err)
	assert.Equal(t, []string{"6|euler_a"}, queryStrings(t, db.DB(), `SELECT id, sampler FROM presets WHERE name = 'default sampler'`))
	v, err := userVersion(db.DB())
	require.NoError(t, err)
	assert.Equal(t, 33, v)
}
