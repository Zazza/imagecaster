package filebrowser

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowseDirectory_ModTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) string
		isDir bool
	}{
		{
			name: "FB1 directory has mod_time",
			setup: func(t *testing.T, dir string) string {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0755))
				return "sub"
			},
			isDir: true,
		},
		{
			name: "file keeps mod_time",
			setup: func(t *testing.T, dir string) string {
				createPNGFile(t, filepath.Join(dir, "a.png"), 4, 4)
				return "a.png"
			},
			isDir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			name := tt.setup(t, dir)
			full := filepath.Join(dir, name)
			stamp := time.Date(2025, 3, 4, 5, 6, 0, 0, time.Local)
			require.NoError(t, os.Chtimes(full, stamp, stamp))

			entries, err := BrowseDirectory(dir)
			require.NoError(t, err)

			var found *FileEntry
			for i := range entries {
				if entries[i].Name == name {
					found = &entries[i]
				}
			}
			require.NotNil(t, found)
			assert.Equal(t, tt.isDir, found.IsDir)
			require.NotEmpty(t, found.ModTime)
			_, err = time.Parse("2006-01-02 15:04", found.ModTime)
			assert.NoError(t, err)
			assert.Equal(t, "2025-03-04 05:06", found.ModTime)
		})
	}
}
