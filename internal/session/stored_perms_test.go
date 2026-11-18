package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTemp_SharedGroupPermissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		existing bool
	}{
		{name: "new session dir", existing: false},
		{name: "existing session dir", existing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			require.NoError(t, os.Chmod(root, os.ModeSetgid|0o775))
			dir := filepath.Join(root, "sessions", "7")
			if tt.existing {
				require.NoError(t, os.MkdirAll(dir, 0o700))
			}

			path, err := writeTemp(dir, []byte("png"))
			require.NoError(t, err)

			fi, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o640), fi.Mode().Perm(), "stored image must be group-readable for imagecaster-mcp under another user")

			di, err := os.Stat(dir)
			require.NoError(t, err)
			if tt.existing {
				assert.Equal(t, os.FileMode(0o700), di.Mode().Perm(), "existing dir mode is left alone")
			} else {
				for _, d := range []string{filepath.Dir(dir), dir} {
					st, err := os.Stat(d)
					require.NoError(t, err)
					assert.Equal(t, os.FileMode(0o770), st.Mode().Perm(), "%s must be group-writable regardless of umask", d)
					assert.NotZero(t, st.Mode()&os.ModeSetgid, "%s must keep setgid so files inherit the shared group", d)
				}
			}
		})
	}
}
