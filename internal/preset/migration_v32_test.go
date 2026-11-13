package preset

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type v32PresetRow struct {
	name          string
	sampler       string
	scheduleType  string
	wantSampler   string
	wantSchedType string
}

type v32HiresRow struct {
	name     string
	upscaler string
	want     string
}

func seedPreV32DB(t *testing.T, path string, presets []v32PresetRow, hires []v32HiresRow) {
	t.Helper()
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer raw.Close()
	_, err = raw.Exec(`
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
		CREATE TABLE presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			preset_type TEXT NOT NULL DEFAULT '',
			prompt TEXT NOT NULL DEFAULT '',
			negative_prompt TEXT NOT NULL DEFAULT '',
			sampler TEXT NOT NULL DEFAULT 'Euler a',
			steps INTEGER NOT NULL DEFAULT 20,
			cfg_scale REAL NOT NULL DEFAULT 7.0,
			width INTEGER NOT NULL DEFAULT 512,
			height INTEGER NOT NULL DEFAULT 512,
			model_name TEXT NOT NULL DEFAULT '',
			seed INTEGER,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			schedule_type TEXT DEFAULT ''
		);
		CREATE TABLE hires_profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			upscale REAL NOT NULL DEFAULT 2.0,
			denoising_strength REAL NOT NULL DEFAULT 0.45,
			upscaler TEXT NOT NULL DEFAULT '',
			is_builtin INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`)
	require.NoError(t, err)
	for _, p := range presets {
		_, err = raw.Exec(`INSERT INTO presets (name, sampler, schedule_type) VALUES (?, ?, ?)`, p.name, p.sampler, p.scheduleType)
		require.NoError(t, err)
	}
	for _, h := range hires {
		_, err = raw.Exec(`INSERT INTO hires_profiles (name, upscaler) VALUES (?, ?)`, h.name, h.upscaler)
		require.NoError(t, err)
	}
}

func v32PresetNames(t *testing.T, db *DB, name string) (string, string) {
	t.Helper()
	var sampler, sched sql.NullString
	require.NoError(t, db.DB().QueryRow(`SELECT sampler, schedule_type FROM presets WHERE name = ?`, name).Scan(&sampler, &sched))
	return sampler.String, sched.String
}

func v32HiresUpscaler(t *testing.T, db *DB, name string) string {
	t.Helper()
	var u string
	require.NoError(t, db.DB().QueryRow(`SELECT upscaler FROM hires_profiles WHERE name = ?`, name).Scan(&u))
	return u
}

var v32Presets = []v32PresetRow{
	{name: "euler a", sampler: "Euler a", scheduleType: "", wantSampler: "euler_a", wantSchedType: ""},
	{name: "dpm karras", sampler: "DPM++ 2M", scheduleType: "Karras", wantSampler: "dpm++2m", wantSchedType: "karras"},
	{name: "ddim uniform", sampler: "DDIM", scheduleType: "Uniform", wantSampler: "ddim_trailing", wantSchedType: "discrete"},
	{name: "automatic", sampler: "Euler", scheduleType: "Automatic", wantSampler: "euler", wantSchedType: ""},
	{name: "auto lower", sampler: "Euler", scheduleType: "auto", wantSampler: "euler", wantSchedType: ""},
	{name: "combined sampler", sampler: "DPM++ 2M Karras", scheduleType: "", wantSampler: "dpm++2m", wantSchedType: "karras"},
	{name: "sgm uniform", sampler: "DPM++ 2M SDE", scheduleType: "SGM Uniform", wantSampler: "dpm++2m_sde", wantSchedType: "sgm_uniform"},
	{name: "already sdcpp", sampler: "euler_a", scheduleType: "karras", wantSampler: "euler_a", wantSchedType: "karras"},
	{name: "already sdcpp default", sampler: "dpm++2m", scheduleType: "", wantSampler: "dpm++2m", wantSchedType: ""},
	{name: "already sdcpp discrete", sampler: "ddim_trailing", scheduleType: "discrete", wantSampler: "ddim_trailing", wantSchedType: "discrete"},
	{name: "unknown sampler", sampler: "Restart", scheduleType: "Simple", wantSampler: "", wantSchedType: "simple"},
	{name: "unknown scheduler", sampler: "Heun", scheduleType: "Turbo", wantSampler: "heun", wantSchedType: ""},
}

var v32Hires = []v32HiresRow{
	{name: "lanczos", upscaler: "Lanczos", want: "lanczos"},
	{name: "nearest", upscaler: "Nearest", want: "nearest"},
	{name: "esrgan forge", upscaler: "R-ESRGAN 4x+", want: "RealESRGAN_x4plus"},
	{name: "esrgan anime forge", upscaler: "R-ESRGAN 4x+ Anime6B", want: "RealESRGAN_x4plus_anime_6B"},
	{name: "already sdcpp builtin", upscaler: "lanczos", want: "lanczos"},
	{name: "already sdcpp file", upscaler: "RealESRGAN_x4plus", want: "RealESRGAN_x4plus"},
}

func TestMigrationV32_PresetSamplerAndSchedulerToSDCpp(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedPreV32DB(t, path, v32Presets, nil)
	db := openFileDB(t, path)
	t.Cleanup(func() { db.Close() })

	for _, tc := range v32Presets {
		t.Run(tc.name, func(t *testing.T) {
			sampler, sched := v32PresetNames(t, db, tc.name)
			assert.Equal(t, tc.wantSampler, sampler, "sampler")
			assert.Equal(t, tc.wantSchedType, sched, "schedule_type")
		})
	}
}

func TestMigrationV32_HiresProfileUpscalerToSDCpp(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedPreV32DB(t, path, nil, v32Hires)
	db := openFileDB(t, path)
	t.Cleanup(func() { db.Close() })

	for _, tc := range v32Hires {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, v32HiresUpscaler(t, db, tc.name))
		})
	}
}

func TestMigrationV32_ReopenKeepsConvertedNames(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedPreV32DB(t, path, v32Presets, v32Hires)
	db := openFileDB(t, path)
	require.NoError(t, db.Close())

	for i := 0; i < 2; i++ {
		reopened, err := Open(path)
		require.NoError(t, err, "reopen #%d", i+1)
		for _, tc := range v32Presets {
			sampler, sched := v32PresetNames(t, reopened, tc.name)
			assert.Equal(t, tc.wantSampler, sampler, "%s sampler after reopen #%d", tc.name, i+1)
			assert.Equal(t, tc.wantSchedType, sched, "%s schedule_type after reopen #%d", tc.name, i+1)
		}
		for _, tc := range v32Hires {
			assert.Equal(t, tc.want, v32HiresUpscaler(t, reopened, tc.name), "%s after reopen #%d", tc.name, i+1)
		}
		require.NoError(t, reopened.Close())
	}
}

func TestMigrationV32_FreshDBHasNoForgeNames(t *testing.T) {
	t.Parallel()
	db := openFileDB(t, filepath.Join(t.TempDir(), "presets.db"))
	t.Cleanup(func() { db.Close() })

	forgeSamplers := []string{"Euler a", "Euler", "DPM++ 2M", "DPM++ 2M SDE", "DDIM", "Heun", "LCM"}
	forgeSchedulers := []string{"Karras", "Uniform", "Automatic", "Simple", "Exponential", "SGM Uniform"}
	forgeUpscalers := []string{"Lanczos", "Nearest", "R-ESRGAN 4x+", "R-ESRGAN 4x+ Anime6B", "Latent"}

	for _, s := range forgeSamplers {
		var n int
		require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM presets WHERE sampler = ?`, s).Scan(&n))
		assert.Zero(t, n, "sampler %q", s)
	}
	for _, s := range forgeSchedulers {
		var n int
		require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM presets WHERE schedule_type = ?`, s).Scan(&n))
		assert.Zero(t, n, "schedule_type %q", s)
	}
	for _, u := range forgeUpscalers {
		var n int
		require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM hires_profiles WHERE upscaler = ?`, u).Scan(&n))
		assert.Zero(t, n, "hires upscaler %q", u)
	}
}
