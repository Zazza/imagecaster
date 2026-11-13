package preset

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const schemaVersion = 33

const legacySchemaVersion = 32

type schemaStep struct {
	version int
	apply   func(context.Context, *sql.Conn) error
}

var schemaSteps = []schemaStep{
	{33, migrateV33PresetsSamplerDefault},
}

const presetsTableDDL = `
CREATE TABLE %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	preset_type TEXT NOT NULL DEFAULT '',
	prompt TEXT NOT NULL DEFAULT '',
	negative_prompt TEXT NOT NULL DEFAULT '',
	sampler TEXT NOT NULL DEFAULT 'euler_a',
	steps INTEGER NOT NULL DEFAULT 20,
	cfg_scale REAL NOT NULL DEFAULT 7.0,
	model_name TEXT NOT NULL DEFAULT '',
	seed INTEGER,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	denoising_strength REAL,
	clip_skip INTEGER,
	batch_size INTEGER,
	batch_count INTEGER,
	vae TEXT DEFAULT '',
	schedule_type TEXT DEFAULT '',
	type_id INTEGER,
	tags TEXT DEFAULT '',
	loras TEXT DEFAULT '',
	is_bundled INTEGER NOT NULL DEFAULT 0
)`

var latestSchemaDDL = fmt.Sprintf(presetsTableDDL, "presets") + ";\n" + `
CREATE TABLE settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);
CREATE TABLE saved_descriptions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	text TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	name TEXT DEFAULT '',
	negative_prompt TEXT DEFAULT '',
	type TEXT DEFAULT ''
);
CREATE TABLE saved_prompts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	text TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE preset_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE compound_presets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE resolutions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	width INTEGER NOT NULL,
	height INTEGER NOT NULL,
	is_builtin INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE compound_preset_steps (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	compound_preset_id INTEGER NOT NULL,
	step_order INTEGER NOT NULL,
	preset_id INTEGER NOT NULL,
	denoising_strength REAL NOT NULL DEFAULT 0.5,
	resolution_id INTEGER REFERENCES resolutions(id),
	loras TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (compound_preset_id) REFERENCES compound_presets(id) ON DELETE CASCADE,
	FOREIGN KEY (preset_id) REFERENCES presets(id)
);
CREATE TABLE export_presets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	format TEXT NOT NULL DEFAULT 'png',
	width INTEGER NOT NULL DEFAULT 0,
	height INTEGER NOT NULL DEFAULT 0,
	lock_ratio INTEGER NOT NULL DEFAULT 1,
	quality INTEGER NOT NULL DEFAULT 90,
	interpolation TEXT NOT NULL DEFAULT 'lanczos',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE session_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	file_name TEXT NOT NULL DEFAULT '',
	thumb_name TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '',
	prompt TEXT NOT NULL DEFAULT '',
	negative_prompt TEXT NOT NULL DEFAULT '',
	sampler TEXT NOT NULL DEFAULT '',
	steps INTEGER NOT NULL DEFAULT 0,
	cfg_scale REAL NOT NULL DEFAULT 0,
	seed INTEGER,
	denoising REAL NOT NULL DEFAULT 0,
	width INTEGER NOT NULL DEFAULT 0,
	height INTEGER NOT NULL DEFAULT 0,
	is_preview INTEGER NOT NULL DEFAULT 0,
	preset_id INTEGER,
	is_active INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	meta TEXT
);
CREATE TABLE session_active (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	session_id INTEGER REFERENCES sessions(id)
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
CREATE TABLE job_queue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	type TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	params TEXT NOT NULL DEFAULT '{}',
	progress REAL NOT NULL DEFAULT 0,
	progress_detail TEXT NOT NULL DEFAULT '{}',
	result TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	completed_at DATETIME,
	retry_count INTEGER NOT NULL DEFAULT 0,
	max_retries INTEGER NOT NULL DEFAULT 3,
	next_retry_at DATETIME
);
CREATE INDEX idx_job_queue_status ON job_queue(status);
CREATE INDEX idx_job_queue_next_retry ON job_queue(next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL;
CREATE TABLE post_recipes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	steps TEXT NOT NULL DEFAULT '[]',
	adjust TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`

const latestSeedSQL = `
INSERT INTO export_presets (name, format, width, height, lock_ratio, quality, interpolation) VALUES
	('Quality Photo', 'png', 0, 0, 1, 0, 'lanczos'),
	('Web Optimized', 'jpeg', 1920, 0, 1, 85, 'lanczos'),
	('Social Media', 'jpeg', 1080, 1080, 0, 90, 'lanczos'),
	('Thumbnail', 'jpeg', 256, 256, 0, 75, 'lanczos');
INSERT INTO sessions (name) VALUES ('Default');
INSERT INTO session_active (id, session_id) VALUES (1, last_insert_rowid());
INSERT INTO resolutions (name, width, height, is_builtin) VALUES
	('Square 512x512', 512, 512, 1),
	('Square 768x768', 768, 768, 1),
	('Square 1024x1024', 1024, 1024, 1),
	('Portrait 512x768', 512, 768, 1),
	('Portrait 768x1024', 768, 1024, 1),
	('Portrait 768x1152', 768, 1152, 1),
	('Portrait 1024x1536', 1024, 1536, 1),
	('Landscape 768x512', 768, 512, 1),
	('Landscape 1024x768', 1024, 768, 1),
	('Landscape 1152x768', 1152, 768, 1),
	('Landscape 1536x1024', 1536, 1024, 1),
	('Widescreen 912x512', 912, 512, 1),
	('Widescreen 1344x768', 1344, 768, 1),
	('Widescreen 1024x576', 1024, 576, 1);
INSERT INTO hires_profiles (name, upscale, denoising_strength, upscaler, is_builtin) VALUES
	('Light', 1.5, 0.3, 'RealESRGAN_x4plus', 1),
	('Standard', 2.0, 0.45, 'RealESRGAN_x4plus', 1),
	('Heavy', 2.5, 0.55, 'RealESRGAN_x4plus', 1),
	('Max', 3.0, 0.6, 'RealESRGAN_x4plus', 1);
INSERT INTO settings (key, value) VALUES ('migration_v20_done', '1');
`

func migrateSchema(db *sql.DB) error {
	version, err := userVersion(db)
	if err != nil {
		return err
	}
	if version == 0 {
		created, err := createLatestSchema(db)
		if err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		if !created {
			if err := runLegacyMigrations(db); err != nil {
				return err
			}
			if err := setUserVersion(db, legacySchemaVersion); err != nil {
				return err
			}
		}
		if version, err = userVersion(db); err != nil {
			return err
		}
	}
	for _, step := range schemaSteps {
		if step.version <= version {
			continue
		}
		if err := runSchemaStep(db, step); err != nil {
			return fmt.Errorf("migrate v%d: %w", step.version, err)
		}
		version = step.version
	}
	return nil
}

func userVersion(db *sql.DB) (int, error) {
	var v int
	err := db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}

func setUserVersion(db *sql.DB, v int) error {
	_, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v))
	return err
}

func createLatestSchema(db *sql.DB) (bool, error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		}
	}()

	var version, tables int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return false, err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		return false, err
	}
	if version != 0 {
		return true, nil
	}
	if tables > 0 {
		return false, nil
	}
	for _, q := range []string{latestSchemaDDL, latestSeedSQL, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return false, err
		}
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

func runSchemaStep(db *sql.DB, step schemaStep) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		}
	}()
	var version int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version >= step.version {
		return nil
	}
	if err := step.apply(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, step.version)); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

func migrateV33PresetsSamplerDefault(ctx context.Context, conn *sql.Conn) error {
	var extras []string
	rows, err := conn.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE tbl_name = 'presets' AND type IN ('index', 'trigger') AND sql IS NOT NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			rows.Close()
			return err
		}
		extras = append(extras, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var seq sql.NullInt64
	err = conn.QueryRowContext(ctx, `SELECT seq FROM sqlite_sequence WHERE name = 'presets'`).Scan(&seq)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if _, err := conn.ExecContext(ctx, `DROP TABLE IF EXISTS presets_v33`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(presetsTableDDL, "presets_v33")); err != nil {
		return err
	}
	newCols, err := tableColumns(ctx, conn, "presets_v33")
	if err != nil {
		return err
	}
	oldCols, err := tableColumns(ctx, conn, "presets")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, c := range oldCols {
		have[c] = true
	}
	var cols []string
	for _, c := range newCols {
		if have[c] {
			cols = append(cols, `"`+c+`"`)
		}
	}
	list := strings.Join(cols, ", ")
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO presets_v33 (%s) SELECT %s FROM presets`, list, list),
		`DROP TABLE presets`,
		`ALTER TABLE presets_v33 RENAME TO presets`,
		`DELETE FROM sqlite_sequence WHERE name IN ('presets', 'presets_v33')`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if seq.Valid {
		if _, err := conn.ExecContext(ctx, `INSERT INTO sqlite_sequence (name, seq) VALUES ('presets', ?)`, seq.Int64); err != nil {
			return err
		}
	}
	for _, q := range extras {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func tableColumns(ctx context.Context, conn *sql.Conn, table string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}
