package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func referenceCompoundList(t *testing.T, db *DB, ids []int64) []CompoundPreset {
	t.Helper()
	var out []CompoundPreset
	q := `SELECT id FROM compound_presets ORDER BY created_at DESC`
	var all []int64
	rows, err := db.DB().Query(q)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		all = append(all, id)
	}
	require.NoError(t, rows.Err())
	rows.Close()
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for _, id := range all {
		if ids != nil && !want[id] {
			continue
		}
		cp, err := db.GetCompoundPreset(id)
		require.NoError(t, err)
		for i := range cp.Steps {
			p, err := db.Get(cp.Steps[i].PresetID)
			if err == nil {
				cp.Steps[i].Preset = p
			}
		}
		out = append(out, *cp)
	}
	return out
}

func stripPresets(items []CompoundPreset) []CompoundPreset {
	out := make([]CompoundPreset, len(items))
	for i, cp := range items {
		out[i] = cp
		if cp.Steps != nil {
			out[i].Steps = make([]CompoundPresetStep, len(cp.Steps))
			for j, st := range cp.Steps {
				st.Preset = nil
				out[i].Steps[j] = st
			}
		}
	}
	return out
}

func TestCompoundList_SameAsPerItemQueries(t *testing.T) {
	db := testDB(t)
	var presetIDs []int64
	for _, name := range []string{"a", "b", "c"} {
		p := &Preset{Name: name, Prompt: name + " prompt"}
		require.NoError(t, db.Create(p))
		presetIDs = append(presetIDs, p.ID)
	}
	loras := "x:1"
	compounds := []*CompoundPreset{
		{Name: "two steps", Steps: []CompoundPresetStep{{PresetID: presetIDs[2], DenoisingStrength: 0.3}, {PresetID: presetIDs[0], DenoisingStrength: 0.6, Loras: &loras}}},
		{Name: "no steps"},
		{Name: "shared preset", Steps: []CompoundPresetStep{{PresetID: presetIDs[0]}, {PresetID: presetIDs[0]}, {PresetID: presetIDs[1]}}},
	}
	for _, cp := range compounds {
		require.NoError(t, db.CreateCompoundPreset(cp))
	}
	_, err := db.DB().Exec(`INSERT INTO compound_preset_steps (compound_preset_id, step_order, preset_id, denoising_strength) VALUES (?, 9, 99999, 0.5)`, compounds[1].ID)
	require.NoError(t, err)

	tests := []struct {
		name string
		got  func() ([]CompoundPreset, error)
		ids  []int64
		full bool
	}{
		{"list", db.ListCompoundPresets, nil, false},
		{"list full", db.ListCompoundPresetsFull, nil, true},
		{"by ids", func() ([]CompoundPreset, error) {
			return db.GetCompoundPresetsByIDs([]int64{compounds[0].ID, compounds[1].ID})
		}, []int64{compounds[0].ID, compounds[1].ID}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.got()
			require.NoError(t, err)
			want := referenceCompoundList(t, db, tt.ids)
			if !tt.full {
				want = stripPresets(want)
			}
			assert.Equal(t, want, got)
		})
	}

	full, err := db.ListCompoundPresetsFull()
	require.NoError(t, err)
	for _, cp := range full {
		for _, st := range cp.Steps {
			if st.PresetID == 99999 {
				assert.Nil(t, st.Preset)
			} else {
				require.NotNil(t, st.Preset)
				assert.Equal(t, st.PresetID, st.Preset.ID)
			}
		}
	}
	one, err := db.GetCompoundPresetFull(compounds[2].ID)
	require.NoError(t, err)
	require.Len(t, one.Steps, 3)
	assert.Equal(t, "a", one.Steps[0].Preset.Name)
	assert.NotSame(t, one.Steps[0].Preset, one.Steps[1].Preset)
	assert.Equal(t, "b", one.Steps[2].Preset.Name)

	none, err := db.GetCompoundPresetsByIDs([]int64{424242})
	require.NoError(t, err)
	assert.Nil(t, none)
}
