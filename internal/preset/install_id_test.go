package preset

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallID_GeneratedOnceAndKeptAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presets.db")
	db := openFileDB(t, path)
	id, err := db.InstallID()
	require.NoError(t, err)
	assert.Regexp(t, `^[0-9a-f]{16}$`, id)
	again, err := db.InstallID()
	require.NoError(t, err)
	assert.Equal(t, id, again)
	require.NoError(t, db.Close())

	db = openFileDB(t, path)
	defer db.Close()
	reopened, err := db.InstallID()
	require.NoError(t, err)
	assert.Equal(t, id, reopened)

	other := openFileDB(t, filepath.Join(t.TempDir(), "other.db"))
	defer other.Close()
	otherID, err := other.InstallID()
	require.NoError(t, err)
	assert.NotEqual(t, id, otherID)
}
