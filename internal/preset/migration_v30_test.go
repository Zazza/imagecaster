package preset

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const legacyDescribeRuText = `Опиши это изображение связным литературным текстом на русском языке, 4–6 предложений.
Расскажи, что на нём происходит: главные объекты и их детали, обстановка, свет и цвета, атмосфера, заметные мелочи.
Пиши живым текстом для человека, а не списком тегов. Не выдумывай того, чего нет на изображении.`

func seedOldDB(t *testing.T, path string, settings map[string]string) {
	t.Helper()
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer raw.Close()
	_, err = raw.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`)
	require.NoError(t, err)
	for k, v := range settings {
		_, err = raw.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, k, v)
		require.NoError(t, err)
	}
}

func describeSetting(t *testing.T, db *DB) (string, bool) {
	t.Helper()
	var v string
	err := db.DB().QueryRow(`SELECT value FROM settings WHERE key = 'analyze_describe_prompt'`).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false
	}
	require.NoError(t, err)
	return v, true
}

func TestMigrationV30_LegacyConstantMatchesCard(t *testing.T) {
	t.Parallel()
	assert.Equal(t, legacyDescribeRuText, legacyAnalyzeDescribePromptRu)
}

func TestMigrationV30_RemovesLegacyRussianDefault(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedOldDB(t, path, map[string]string{
		"analyze_describe_prompt": legacyDescribeRuText,
		"analyze_prompt":          "keep me",
	})

	db := openFileDB(t, path)
	t.Cleanup(func() { db.Close() })

	_, ok := describeSetting(t, db)
	assert.False(t, ok, "legacy default must be deleted")
	other, err := db.GetSetting("analyze_prompt")
	require.NoError(t, err)
	assert.Equal(t, "keep me", other, "other settings untouched")
}

func TestMigrationV30_KeepsCustomPrompts(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"custom text":        "Мой собственный промпт описания",
		"legacy plus suffix": legacyDescribeRuText + "\nДобавь про погоду.",
		"legacy first line":  "Опиши это изображение связным литературным текстом на русском языке, 4–6 предложений.",
		"legacy with space":  legacyDescribeRuText + " ",
		"empty value":        "",
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "presets.db")
			seedOldDB(t, path, map[string]string{"analyze_describe_prompt": val})

			db := openFileDB(t, path)
			t.Cleanup(func() { db.Close() })

			got, ok := describeSetting(t, db)
			require.True(t, ok, "non-legacy value must stay")
			assert.Equal(t, val, got)
		})
	}
}

func TestMigrationV30_NoSettingIsFine(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	db := openFileDB(t, path)
	t.Cleanup(func() { db.Close() })
	_, ok := describeSetting(t, db)
	assert.False(t, ok)
}

func TestMigrationV30_ReopenIsSafe(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.db")
	seedOldDB(t, path, map[string]string{"analyze_describe_prompt": legacyDescribeRuText})

	db := openFileDB(t, path)
	_, ok := describeSetting(t, db)
	assert.False(t, ok)
	require.NoError(t, db.SetSetting("analyze_describe_prompt", "user text after upgrade"))
	require.NoError(t, db.Close())

	for i := 0; i < 2; i++ {
		reopened, err := Open(path)
		require.NoError(t, err, "reopen #%d", i+1)
		got, ok := describeSetting(t, reopened)
		assert.True(t, ok)
		assert.Equal(t, "user text after upgrade", got)
		require.NoError(t, reopened.Close())
	}
}
