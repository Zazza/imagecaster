package filebrowser

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cacheEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	require.NoError(t, err)
	return entries
}

func TestThumbCache(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		make  func(t *testing.T, path string)
		ext   string
		check func(t *testing.T, c *ThumbCache, src, cacheDir string, first string)
	}{
		{
			name: "hit returns cached bytes without re-reading source",
			file: "big.png",
			make: func(t *testing.T, p string) { createPNGFile(t, p, 600, 400) },
			ext:  ".png",
			check: func(t *testing.T, c *ThumbCache, src, cacheDir, first string) {
				entries := cacheEntries(t, cacheDir)
				require.Len(t, entries, 1)
				require.NoError(t, os.WriteFile(entries[0], []byte("cached-marker"), 0o644))
				got, err := c.Read(src)
				require.NoError(t, err)
				assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("cached-marker")), got)
			},
		},
		{
			name: "mtime change invalidates and replaces the entry",
			file: "big.jpg",
			make: func(t *testing.T, p string) { createJPEGFile(t, p, 500, 500) },
			ext:  ".jpg",
			check: func(t *testing.T, c *ThumbCache, src, cacheDir, first string) {
				old := cacheEntries(t, cacheDir)
				require.Len(t, old, 1)
				require.NoError(t, os.WriteFile(old[0], []byte("stale"), 0o644))
				later := time.Now().Add(time.Hour)
				require.NoError(t, os.Chtimes(src, later, later))
				got, err := c.Read(src)
				require.NoError(t, err)
				assert.Equal(t, first, got)
				entries := cacheEntries(t, cacheDir)
				require.Len(t, entries, 1)
				assert.NotEqual(t, old[0], entries[0])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cacheDir := filepath.Join(t.TempDir(), "cache")
			src := filepath.Join(dir, tt.file)
			tt.make(t, src)
			c := NewThumbCache(cacheDir)
			first, err := c.Read(src)
			require.NoError(t, err)
			uncached, err := ReadThumbnail(src)
			require.NoError(t, err)
			assert.Equal(t, uncached, first)
			entries := cacheEntries(t, cacheDir)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.ext, filepath.Ext(entries[0]))
			tt.check(t, c, src, cacheDir, first)
		})
	}
}

func TestThumbCache_SmallImageReturnedAsIsNotCached(t *testing.T) {
	src := filepath.Join(t.TempDir(), "small.png")
	createPNGFile(t, src, 40, 30)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	got, err := NewThumbCache(cacheDir).Read(src)
	require.NoError(t, err)
	raw, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Equal(t, base64.StdEncoding.EncodeToString(raw), got)
	assert.Empty(t, cacheEntries(t, cacheDir))
}

func TestThumbCache_DisabledWritesNothing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "big.png")
	createPNGFile(t, src, 600, 400)
	_, err := ReadThumbnail(src)
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}
