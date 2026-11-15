package preset

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func v34DBWithPreset(t *testing.T) string {
	t.Helper()
	path := legacyDB(t, len(legacyMigrations))
	raw := openRaw(t, path)
	require.NoError(t, setUserVersion(raw, legacySchemaVersion))
	for _, step := range schemaSteps {
		if step.version > 34 {
			break
		}
		require.NoError(t, runSchemaStep(raw, step))
	}
	v, err := userVersion(raw)
	require.NoError(t, err)
	require.Equal(t, 34, v)
	if len(queryStrings(t, raw, `SELECT name FROM pragma_table_info('presets') WHERE name = 'fast_mode'`)) > 0 {
		_, err = raw.Exec(`ALTER TABLE presets DROP COLUMN fast_mode`)
		require.NoError(t, err)
	}
	require.Empty(t, queryStrings(t, raw, `SELECT name FROM pragma_table_info('presets') WHERE name = 'fast_mode'`))
	_, err = raw.Exec(`INSERT INTO presets (name, preset_type) VALUES ('old', 'txt2img')`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	return path
}

func TestFastMode_Migration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantOld bool
		reopenN int
	}{
		{"v34 with preset upgraded", v34DBWithPreset, true, 2},
		{"fresh database", func(t *testing.T) string { return filepath.Join(t.TempDir(), "new.db") }, false, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.path(t)
			for i := 0; i < tt.reopenN; i++ {
				db := openFileDB(t, path)
				cols := queryStrings(t, db.DB(), `SELECT name FROM pragma_table_info('presets') WHERE name = 'fast_mode'`)
				assert.Equal(t, []string{"fast_mode"}, cols, "open %d", i)
				v, err := userVersion(db.DB())
				require.NoError(t, err)
				assert.Equal(t, schemaVersion, v)
				assert.GreaterOrEqual(t, v, 35)
				if tt.wantOld {
					var mode string
					require.NoError(t, db.DB().QueryRow(`SELECT fast_mode FROM presets WHERE name = 'old'`).Scan(&mode))
					assert.Equal(t, "", mode)
					list, err := db.List()
					require.NoError(t, err)
					found := false
					for _, p := range list {
						if p.Name == "old" {
							found = true
							assert.Equal(t, "", p.FastMode)
						}
					}
					assert.True(t, found)
				}
				require.NoError(t, db.Close())
			}
		})
	}
}

func TestFastMode_FreshSchemaMatchesUpgraded(t *testing.T) {
	t.Parallel()
	fresh := openFileDB(t, freshDB(t))
	defer fresh.Close()
	upgraded := openFileDB(t, v34DBWithPreset(t))
	defer upgraded.Close()
	q := `SELECT name || ' ' || type || ' ' || "notnull" || ' ' || coalesce(dflt_value, 'NULL') FROM pragma_table_info('presets') WHERE name = 'fast_mode'`
	f := queryStrings(t, fresh.DB(), q)
	require.Len(t, f, 1)
	assert.Equal(t, f, queryStrings(t, upgraded.DB(), q))
}

func TestFastMode_CRUD(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		create     string
		wantCreate string
		update     string
		wantUpdate string
	}{
		{"on then off", "on", "on", "off", "off"},
		{"off then on", "off", "off", "on", "on"},
		{"empty then on", "", "", "on", "on"},
		{"on then empty", "on", "on", "", ""},
		{"invalid on create", "maybe", "", "off", "off"},
		{"invalid on update", "on", "on", "maybe", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testDB(t)
			p := &Preset{Name: "fm", PresetType: "txt2img", Sampler: "euler", Steps: 20, CfgScale: 7, FastMode: tt.create}
			require.NoError(t, db.Create(p))
			got, err := db.Get(p.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreate, got.FastMode)

			got.FastMode = tt.update
			require.NoError(t, db.Update(got))
			again, err := db.Get(p.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantUpdate, again.FastMode)
		})
	}
}

func TestFastMode_CreateBatchNormalizes(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	items := []Preset{
		{Name: "a", PresetType: "txt2img", Steps: 20, CfgScale: 7, FastMode: "on"},
		{Name: "b", PresetType: "txt2img", Steps: 20, CfgScale: 7, FastMode: "off"},
		{Name: "c", PresetType: "txt2img", Steps: 20, CfgScale: 7, FastMode: "maybe"},
		{Name: "d", PresetType: "txt2img", Steps: 20, CfgScale: 7},
	}
	created, err := db.CreateBatch(items)
	require.NoError(t, err)
	require.Len(t, created, 4)
	want := map[string]string{"a": "on", "b": "off", "c": "", "d": ""}
	for _, c := range created {
		got, err := db.Get(c.ID)
		require.NoError(t, err)
		assert.Equal(t, want[got.Name], got.FastMode, got.Name)
	}
}

func TestNormalizeFastMode(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"", ""},
		{"on", "on"},
		{"off", "off"},
		{"maybe", ""},
		{"true", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, NormalizeFastMode(tt.in))
		})
	}
}

func TestFastGenerationSetting_DefaultOff(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	v, err := db.GetSetting("fast_generation")
	if err == nil {
		assert.NotEqual(t, "true", v)
	}
	require.NoError(t, db.SetSetting("fast_generation", "true"))
	v, err = db.GetSetting("fast_generation")
	require.NoError(t, err)
	assert.Equal(t, "true", v)
}
