package preset

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaV37_FreshDBHasStylistPresets(t *testing.T) {
	t.Parallel()
	raw := openRaw(t, freshDB(t))
	assert.Contains(t, userTables(t, raw), "stylist_presets")
	v, err := userVersion(raw)
	require.NoError(t, err)
	assert.Equal(t, schemaVersion, v)
}

func TestSchemaV37_V36DatabaseGetsTable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.db")
	db, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, db.Create(&Preset{Name: "keep", PresetType: "test", Prompt: "p", Sampler: "euler", Steps: 20, CfgScale: 7}))
	require.NoError(t, db.SetSetting("stylist_appearance", `{"anna":"165 cm"}`))
	require.NoError(t, db.Close())

	raw := openRaw(t, path)
	_, err = raw.Exec(`DROP TABLE IF EXISTS stylist_presets`)
	require.NoError(t, err)
	require.NoError(t, setUserVersion(raw, 36))
	assert.NotContains(t, userTables(t, raw), "stylist_presets")
	require.NoError(t, raw.Close())

	db, err = Open(path)
	require.NoError(t, err)
	defer db.Close()

	v, err := userVersion(db.DB())
	require.NoError(t, err)
	assert.Equal(t, schemaVersion, v)
	assert.Contains(t, userTables(t, db.DB()), "stylist_presets")

	list, err := db.List()
	require.NoError(t, err)
	names := []string{}
	for _, p := range list {
		names = append(names, p.Name)
	}
	assert.Contains(t, names, "keep")
	val, err := db.GetSetting("stylist_appearance")
	require.NoError(t, err)
	assert.Equal(t, `{"anna":"165 cm"}`, val)

	p := StylistPreset{Aspect: "hair", Name: "Коса", Prompt: "braid"}
	require.NoError(t, db.CreateStylistPreset(&p))
	got, err := db.ListStylistPresets()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "braid", got[0].Prompt)
}
