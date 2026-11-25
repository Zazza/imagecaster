package generation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/stylisttrace"
	"github.com/stretchr/testify/require"
)

func TestNewStylistTraceLocalOnly(t *testing.T) {
	tests := []struct {
		name, url string
		wantTrace bool
	}{
		{name: "loopback", url: "http://127.0.0.1:7861", wantTrace: true},
		{name: "remote", url: "https://8.8.8.8", wantTrace: false},
		{name: "unset", url: "", wantTrace: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			svc := &Service{cfg: &config.Config{SDUrl: tt.url}, dataDir: root}
			trace := svc.NewStylistTrace()
			if tt.wantTrace {
				require.NotNil(t, trace)
				require.NoError(t, trace.Close())
			} else {
				require.Nil(t, trace)
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Empty(t, entries)
			}
		})
	}
}
func TestNewStylistTraceStorageFailureIsOptional(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, stylisttrace.Directory)
	require.NoError(t, os.WriteFile(blocked, []byte("existing file"), 0600))
	svc := &Service{cfg: &config.Config{SDUrl: "http://127.0.0.1:7861"}, dataDir: root}
	require.Nil(t, svc.NewStylistTrace())
	data, err := os.ReadFile(blocked)
	require.NoError(t, err)
	require.Equal(t, "existing file", string(data))
}

func TestNewStylistTraceRequiresDataDirectory(t *testing.T) {
	svc := &Service{cfg: &config.Config{SDUrl: "http://127.0.0.1:7861"}}
	require.Nil(t, svc.NewStylistTrace())
}
