package preset

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func splNames(t *testing.T, db *DB) []string {
	t.Helper()
	list, err := db.ListStylistPlans()
	require.NoError(t, err)
	names := make([]string, 0, len(list))
	for _, p := range list {
		names = append(names, p.Name)
	}
	return names
}

func splCount(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM stylist_plans`).Scan(&n))
	return n
}

func TestStylistPlans_SchemaV41(t *testing.T) {
	t.Parallel()
	assert.GreaterOrEqual(t, schemaVersion, 41)
	raw := openRaw(t, freshDB(t))
	assert.Contains(t, queryStrings(t, raw, `SELECT name FROM sqlite_master WHERE type='table'`), "stylist_plans")
}

func TestStylistPlans_V40DatabaseGetsTable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.db")
	db, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	raw := openRaw(t, path)
	_, err = raw.Exec(`DROP TABLE stylist_plans`)
	require.NoError(t, err)
	require.NoError(t, setUserVersion(raw, 40))
	require.NoError(t, raw.Close())

	db, err = Open(path)
	require.NoError(t, err)
	defer db.Close()
	v, err := userVersion(db.DB())
	require.NoError(t, err)
	assert.Equal(t, schemaVersion, v)

	_, err = db.SaveStylistPlan("Осень", `{"a":1}`, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"Осень"}, splNames(t, db))
}

func TestStylistPlans_EmptyList(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	list, err := db.ListStylistPlans()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStylistPlans_SaveAndList(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	plan := `{"series":[{"shots":[{"pose":"стоя","wear":"пальто"}]}],"x":"  пробелы  "}`
	saved, err := db.SaveStylistPlan("Осень в парке", plan, false)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Greater(t, saved.ID, int64(0))
	assert.Equal(t, "Осень в парке", saved.Name)
	assert.Equal(t, plan, saved.Plan)

	list, err := db.ListStylistPlans()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, saved.ID, list[0].ID)
	assert.Equal(t, "Осень в парке", list[0].Name)
	assert.Equal(t, plan, list[0].Plan)

	got, err := db.GetStylistPlan(saved.ID)
	require.NoError(t, err)
	assert.Equal(t, plan, got.Plan)
	assert.Equal(t, "Осень в парке", got.Name)
}

func TestStylistPlans_GetUnknown(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	_, err := db.GetStylistPlan(999)
	assert.True(t, errors.Is(err, ErrStylistPlanNotFound), "got %v", err)
}

func TestStylistPlans_ListNewestFirst(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	_, err := db.SaveStylistPlan("Первый", `{}`, false)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)
	_, err = db.SaveStylistPlan("Второй", `{}`, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"Второй", "Первый"}, splNames(t, db))
}

func TestStylistPlans_DuplicateNameWithoutReplace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		first string
		dup   string
	}{
		{"same", "Осень", "Осень"},
		{"cyrillic lower", "Осень", "осень"},
		{"cyrillic upper", "осень", "ОСЕНЬ"},
		{"latin case", "Autumn", "aUTUMN"},
		{"extra spaces", "Осень в парке", "Осень   в парке"},
		{"stored with extra spaces", "Осень   в парке", "Осень в парке"},
		{"leading trailing spaces", "Осень", "  Осень  "},
		{"case and spaces", "Осень в парке", "  осень  В   ПАРКЕ "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openMemDB(t)
			_, err := db.SaveStylistPlan(tt.first, `{"v":"old"}`, false)
			require.NoError(t, err)

			_, err = db.SaveStylistPlan(tt.dup, `{"v":"new"}`, false)
			assert.True(t, errors.Is(err, ErrStylistPlanExists), "got %v", err)

			list, err := db.ListStylistPlans()
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, `{"v":"old"}`, list[0].Plan)
		})
	}
}

func TestStylistPlans_DifferentNamesCoexist(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	_, err := db.SaveStylistPlan("Осень", `{}`, false)
	require.NoError(t, err)
	_, err = db.SaveStylistPlan("Осень в парке", `{}`, false)
	require.NoError(t, err)
	assert.Equal(t, 2, splCount(t, db))
}

func TestStylistPlans_ReplaceExisting(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	first, err := db.SaveStylistPlan("Осень в парке", `{"v":"old"}`, false)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)
	_, err = db.SaveStylistPlan("Зима", `{"v":"winter"}`, false)
	require.NoError(t, err)
	require.Equal(t, []string{"Зима", "Осень в парке"}, splNames(t, db))
	time.Sleep(1100 * time.Millisecond)

	replaced, err := db.SaveStylistPlan("  осень   В ПАРКЕ", `{"v":"new"}`, true)
	require.NoError(t, err)
	require.NotNil(t, replaced)
	assert.Equal(t, first.ID, replaced.ID)
	assert.Equal(t, `{"v":"new"}`, replaced.Plan)

	assert.Equal(t, 2, splCount(t, db))
	list, err := db.ListStylistPlans()
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, first.ID, list[0].ID)
	assert.Equal(t, `{"v":"new"}`, list[0].Plan)
	assert.Equal(t, "Зима", list[1].Name)
	assert.Equal(t, `{"v":"winter"}`, list[1].Plan)
}

func TestStylistPlans_ReplaceWhenAbsentCreates(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	saved, err := db.SaveStylistPlan("Новый", `{"a":1}`, true)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, 1, splCount(t, db))
	assert.Equal(t, []string{"Новый"}, splNames(t, db))
}

func TestStylistPlans_Rename(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	p, err := db.SaveStylistPlan("Осень", `{"a":1}`, false)
	require.NoError(t, err)

	renamed, err := db.RenameStylistPlan(p.ID, "Зима в горах")
	require.NoError(t, err)
	require.NotNil(t, renamed)
	assert.Equal(t, "Зима в горах", renamed.Name)
	assert.Equal(t, `{"a":1}`, renamed.Plan)

	assert.Equal(t, []string{"Зима в горах"}, splNames(t, db))
	got, err := db.GetStylistPlan(p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Зима в горах", got.Name)
	assert.Equal(t, `{"a":1}`, got.Plan)

	_, err = db.SaveStylistPlan("Осень", `{}`, false)
	assert.NoError(t, err, "old name must be free after rename")
}

func TestStylistPlans_RenameToOtherPlanName(t *testing.T) {
	t.Parallel()
	tests := []string{"Зима", "зима", "ЗИМА", "  зима ", "зИмА"}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			db := openMemDB(t)
			a, err := db.SaveStylistPlan("Осень", `{}`, false)
			require.NoError(t, err)
			_, err = db.SaveStylistPlan("Зима", `{}`, false)
			require.NoError(t, err)

			_, err = db.RenameStylistPlan(a.ID, target)
			assert.True(t, errors.Is(err, ErrStylistPlanExists), "got %v", err)

			got, err := db.GetStylistPlan(a.ID)
			require.NoError(t, err)
			assert.Equal(t, "Осень", got.Name)
		})
	}
}

func TestStylistPlans_RenameOtherPlanNameWithSpaces(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	a, err := db.SaveStylistPlan("Осень", `{}`, false)
	require.NoError(t, err)
	_, err = db.SaveStylistPlan("Зима в горах", `{}`, false)
	require.NoError(t, err)

	_, err = db.RenameStylistPlan(a.ID, "зима   в  горах")
	assert.True(t, errors.Is(err, ErrStylistPlanExists), "got %v", err)
}

func TestStylistPlans_RenameSameNameDifferentCase(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	p, err := db.SaveStylistPlan("осень", `{}`, false)
	require.NoError(t, err)

	renamed, err := db.RenameStylistPlan(p.ID, "Осень")
	require.NoError(t, err)
	assert.Equal(t, "Осень", renamed.Name)
	assert.Equal(t, []string{"Осень"}, splNames(t, db))
	assert.Equal(t, 1, splCount(t, db))
}

func TestStylistPlans_RenameUnknown(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	_, err := db.RenameStylistPlan(999, "Осень")
	assert.True(t, errors.Is(err, ErrStylistPlanNotFound), "got %v", err)
	assert.Equal(t, 0, splCount(t, db))
}

func TestStylistPlans_Delete(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	a, err := db.SaveStylistPlan("Осень", `{}`, false)
	require.NoError(t, err)
	_, err = db.SaveStylistPlan("Зима", `{}`, false)
	require.NoError(t, err)

	require.NoError(t, db.DeleteStylistPlan(a.ID))
	assert.Equal(t, []string{"Зима"}, splNames(t, db))
	_, err = db.GetStylistPlan(a.ID)
	assert.True(t, errors.Is(err, ErrStylistPlanNotFound), "got %v", err)

	_, err = db.SaveStylistPlan("осень", `{}`, false)
	assert.NoError(t, err, "name must be free after delete")
}

func TestStylistPlans_DeleteUnknown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   int64
	}{
		{"never existed", 999},
		{"zero", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openMemDB(t)
			_, err := db.SaveStylistPlan("Осень", `{}`, false)
			require.NoError(t, err)
			err = db.DeleteStylistPlan(tt.id)
			assert.True(t, errors.Is(err, ErrStylistPlanNotFound), "got %v", err)
			assert.Equal(t, 1, splCount(t, db))
		})
	}
}

func TestStylistPlans_DeleteTwice(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	p, err := db.SaveStylistPlan("Осень", `{}`, false)
	require.NoError(t, err)
	require.NoError(t, db.DeleteStylistPlan(p.ID))
	err = db.DeleteStylistPlan(p.ID)
	assert.True(t, errors.Is(err, ErrStylistPlanNotFound), "got %v", err)
}
