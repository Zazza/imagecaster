package preset

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func queryPlan(t *testing.T, db *DB, q string, args ...any) string {
	t.Helper()
	rows, err := db.DB().Query(`EXPLAIN QUERY PLAN `+q, args...)
	require.NoError(t, err)
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		parts = append(parts, detail)
	}
	require.NoError(t, rows.Err())
	return strings.Join(parts, "; ")
}

func TestSchemaV34_QueryIndexes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path func(t *testing.T) string
	}{
		{"fresh", freshDB},
		{"upgraded from v33 with data", func(t *testing.T) string {
			path := legacyDB(t, len(legacyMigrations))
			raw := openRaw(t, path)
			require.NoError(t, setUserVersion(raw, legacySchemaVersion))
			require.NoError(t, runSchemaStep(raw, schemaSteps[0]))
			v, err := userVersion(raw)
			require.NoError(t, err)
			require.Equal(t, 33, v)
			_, err = raw.Exec(`INSERT INTO sessions (name) VALUES ('s')`)
			require.NoError(t, err)
			_, err = raw.Exec(`INSERT INTO session_items (session_id, file_name) VALUES (1, '1.png'), (1, '2.png')`)
			require.NoError(t, err)
			_, err = raw.Exec(`INSERT INTO job_queue (type, status) VALUES ('txt2img', 'pending')`)
			require.NoError(t, err)
			require.NoError(t, raw.Close())
			return path
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openFileDB(t, tt.path(t))
			defer db.Close()
			names := queryStrings(t, db.DB(), `SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name IN ('session_items', 'job_queue') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
			assert.Equal(t, []string{"idx_job_queue_next_retry", "idx_job_queue_status_created", "idx_session_items_session_created"}, names)
			assert.Contains(t, queryPlan(t, db, `SELECT id FROM session_items WHERE session_id = ? ORDER BY created_at ASC`, 1), "idx_session_items_session_created")
			assert.Contains(t, queryPlan(t, db, `SELECT COUNT(*) FROM session_items si WHERE si.session_id = ?`, 1), "idx_session_items_session_created")
			assert.Contains(t, queryPlan(t, db, `SELECT id FROM job_queue WHERE status = 'pending' ORDER BY created_at ASC`), "idx_job_queue_status_created")
			v, err := userVersion(db.DB())
			require.NoError(t, err)
			assert.Equal(t, schemaVersion, v)
		})
	}
}

func TestSchemaV34_KeepsRows(t *testing.T) {
	t.Parallel()
	path := legacyDB(t, len(legacyMigrations))
	raw := openRaw(t, path)
	require.NoError(t, setUserVersion(raw, legacySchemaVersion))
	require.NoError(t, runSchemaStep(raw, schemaSteps[0]))
	_, err := raw.Exec(`INSERT INTO sessions (name) VALUES ('s')`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO session_items (session_id, file_name) VALUES (1, '1.png'), (1, '2.png')`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO job_queue (type, status) VALUES ('txt2img', 'pending')`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	db := openFileDB(t, path)
	defer db.Close()
	assert.Equal(t, []string{"2"}, queryStrings(t, db.DB(), `SELECT COUNT(*) FROM session_items`))
	assert.Equal(t, []string{"1"}, queryStrings(t, db.DB(), `SELECT COUNT(*) FROM job_queue`))
}
