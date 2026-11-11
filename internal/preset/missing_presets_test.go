package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingPresets(t *testing.T) {
	tests := []struct {
		name    string
		raw     func(t *testing.T) []byte
		want    int
		wantErr bool
	}{
		{"all new", func(t *testing.T) []byte {
			return installFile(t,
				installPreset("Miss A", "miss_type", "m1"),
				installPreset("Miss B", "miss_type", "m1"),
				installPreset("Miss C", "miss_type", "m1"))
		}, 3, false},
		{"duplicate names count once", func(t *testing.T) []byte {
			return installFile(t,
				installPreset("Miss Dup", "miss_type", "m1"),
				installPreset("Miss Dup", "miss_type", "m1"),
				installPreset("Miss Other", "miss_type", "m1"))
		}, 2, false},
		{"empty name ignored", func(t *testing.T) []byte {
			return installFile(t,
				installPreset("", "miss_type", "m1"),
				installPreset("Miss Named", "miss_type", "m1"))
		}, 1, false},
		{"empty list", func(t *testing.T) []byte { return []byte(`{"version":2,"presets":[]}`) }, 0, false},
		{"invalid json", func(t *testing.T) []byte { return []byte(`{not json`) }, 0, true},
		{"version 1", func(t *testing.T) []byte {
			return []byte(`{"version":1,"presets":[{"name":"Miss V1"}]}`)
		}, 0, true},
		{"version missing", func(t *testing.T) []byte {
			return []byte(`{"presets":[{"name":"Miss V0"}]}`)
		}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testDB(t)
			n, err := db.MissingPresets(tt.raw(t))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, n)
		})
	}
}

func TestMissingPresets_DoesNotInsert(t *testing.T) {
	db := testDB(t)
	before, err := db.List()
	require.NoError(t, err)
	raw := installFile(t, installPreset("NoIns A", "noins_type", "m1"), installPreset("NoIns B", "noins_type", "m1"))

	n, err := db.MissingPresets(raw)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	after, err := db.List()
	require.NoError(t, err)
	assert.Len(t, after, len(before))
	n, err = db.MissingPresets(raw)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestMissingPresets_AfterInstallAndDelete(t *testing.T) {
	db := testDB(t)
	raw := installFile(t,
		installPreset("Cycle A", "cycle_type", "m1"),
		installPreset("Cycle B", "cycle_type", "m1"),
		installPreset("Cycle C", "cycle_type", "m1"))

	_, err := db.InstallPresets(raw)
	require.NoError(t, err)
	n, err := db.MissingPresets(raw)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	got := presetsByName(t, db)
	require.Len(t, got["Cycle B"], 1)
	require.NoError(t, db.Delete(got["Cycle B"][0].ID))

	n, err = db.MissingPresets(raw)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
