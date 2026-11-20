package preset

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaV38_FreshDBHasJobSession(t *testing.T) {
	t.Parallel()
	raw := openRaw(t, freshDB(t))
	assert.Contains(t, queryStrings(t, raw, `SELECT name FROM pragma_table_info('job_queue')`), "session_id")
}

func TestSchemaV38_V37DatabaseKeepsJobsAndGetsColumn(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.db")
	db, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	raw := openRaw(t, path)
	_, err = raw.Exec(`ALTER TABLE job_queue DROP COLUMN session_id`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO job_queue (type, params) VALUES ('txt2img', '{}')`)
	require.NoError(t, err)
	require.NoError(t, setUserVersion(raw, 37))
	require.NoError(t, raw.Close())

	db, err = Open(path)
	require.NoError(t, err)
	defer db.Close()
	v, err := userVersion(db.DB())
	require.NoError(t, err)
	assert.Equal(t, schemaVersion, v)
	var n int
	require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM job_queue WHERE type = 'txt2img' AND session_id IS NULL`).Scan(&n))
	assert.Equal(t, 1, n, "old jobs stay, without a folder")
}
