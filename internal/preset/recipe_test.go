package preset

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostRecipe_Migration(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "presets.db")

	db, err := Open(dbPath)
	require.NoError(t, err)

	p := &Preset{Name: "existing preset"}
	require.NoError(t, db.Create(p))
	assert.Greater(t, p.ID, int64(0))

	saved, err := db.SavePostRecipe(PostRecipe{
		Name:   "Cinematic",
		Steps:  `[{"preset_id":42,"strength":0.6}]`,
		Adjust: `{"brightness":1.1}`,
	})
	require.NoError(t, err)
	assert.Greater(t, saved.ID, int64(0))
	require.NoError(t, db.Close())

	reopened, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { reopened.Close() })

	recipes, err := reopened.ListPostRecipes()
	require.NoError(t, err)
	require.Len(t, recipes, 1)
	assert.Equal(t, "Cinematic", recipes[0].Name)
	assert.Equal(t, `[{"preset_id":42,"strength":0.6}]`, recipes[0].Steps)
	assert.Equal(t, `{"brightness":1.1}`, recipes[0].Adjust)
	assert.NotEmpty(t, recipes[0].CreatedAt)
	assert.NotEmpty(t, recipes[0].UpdatedAt)

	presets, err := reopened.List()
	require.NoError(t, err)
	require.Len(t, presets, 1)
	assert.Equal(t, "existing preset", presets[0].Name)
}

func TestPostRecipe_SaveInsertAndUpdate(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	saved, err := db.SavePostRecipe(PostRecipe{
		Name:   "  Soft look  ",
		Steps:  `[{"preset_id":7,"strength":0.5}]`,
		Adjust: `{"saturation":0.9}`,
	})
	require.NoError(t, err)
	assert.Greater(t, saved.ID, int64(0))
	assert.Equal(t, "Soft look", saved.Name)

	recipes, err := db.ListPostRecipes()
	require.NoError(t, err)
	require.Len(t, recipes, 1)
	assert.Equal(t, "Soft look", recipes[0].Name)
	assert.Equal(t, `[{"preset_id":7,"strength":0.5}]`, recipes[0].Steps)
	assert.Equal(t, `{"saturation":0.9}`, recipes[0].Adjust)

	updated, err := db.SavePostRecipe(PostRecipe{
		ID:     saved.ID,
		Name:   "Soft look v2",
		Steps:  `[]`,
		Adjust: `{}`,
	})
	require.NoError(t, err)
	assert.Equal(t, saved.ID, updated.ID)

	recipes, err = db.ListPostRecipes()
	require.NoError(t, err)
	require.Len(t, recipes, 1)
	assert.Equal(t, "Soft look v2", recipes[0].Name)
	assert.Equal(t, "[]", recipes[0].Steps)
	assert.Equal(t, "{}", recipes[0].Adjust)
}

func TestPostRecipe_SaveNonexistentID(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	_, err := db.SavePostRecipe(PostRecipe{ID: 999, Name: "ghost", Steps: "[]", Adjust: "{}"})
	require.Error(t, err)

	recipes, err := db.ListPostRecipes()
	require.NoError(t, err)
	assert.Empty(t, recipes)
}

func TestPostRecipe_ListOrder(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	for _, name := range []string{"first", "second", "third"} {
		_, err := db.SavePostRecipe(PostRecipe{Name: name, Steps: "[]", Adjust: "{}"})
		require.NoError(t, err)
	}

	recipes, err := db.ListPostRecipes()
	require.NoError(t, err)
	require.Len(t, recipes, 3)
	assert.Equal(t, []string{"first", "second", "third"}, []string{recipes[0].Name, recipes[1].Name, recipes[2].Name})
	assert.Less(t, recipes[0].ID, recipes[1].ID)
	assert.Less(t, recipes[1].ID, recipes[2].ID)
}

func TestPostRecipe_Delete(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	keep, err := db.SavePostRecipe(PostRecipe{Name: "keep", Steps: "[]", Adjust: "{}"})
	require.NoError(t, err)
	drop, err := db.SavePostRecipe(PostRecipe{Name: "drop", Steps: "[]", Adjust: "{}"})
	require.NoError(t, err)

	require.NoError(t, db.DeletePostRecipe(drop.ID))

	recipes, err := db.ListPostRecipes()
	require.NoError(t, err)
	require.Len(t, recipes, 1)
	assert.Equal(t, keep.ID, recipes[0].ID)

	assert.NoError(t, db.DeletePostRecipe(drop.ID))

	recipes, err = db.ListPostRecipes()
	require.NoError(t, err)
	assert.Len(t, recipes, 1)
}

func TestPostRecipe_SaveValidation(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	validName := strings.Repeat("n", MaxRecipeNameLen)
	tooLongName := strings.Repeat("n", MaxRecipeNameLen+1)
	tooLongJSON := `"` + strings.Repeat("a", MaxRecipeStepsLen) + `"`

	cases := []struct {
		name   string
		recipe PostRecipe
	}{
		{"empty name", PostRecipe{Name: "", Steps: "[]", Adjust: "{}"}},
		{"whitespace name", PostRecipe{Name: "   ", Steps: "[]", Adjust: "{}"}},
		{"name too long", PostRecipe{Name: tooLongName, Steps: "[]", Adjust: "{}"}},
		{"invalid steps json", PostRecipe{Name: "r", Steps: `[{"preset_id":`, Adjust: "{}"}},
		{"empty steps", PostRecipe{Name: "r", Steps: "", Adjust: "{}"}},
		{"steps too long", PostRecipe{Name: "r", Steps: tooLongJSON, Adjust: "{}"}},
		{"invalid adjust json", PostRecipe{Name: "r", Steps: "[]", Adjust: `{"brightness":`}},
		{"empty adjust", PostRecipe{Name: "r", Steps: "[]", Adjust: ""}},
		{"adjust too long", PostRecipe{Name: "r", Steps: "[]", Adjust: tooLongJSON}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := db.SavePostRecipe(tc.recipe)
			require.Error(t, err)
		})
	}

	saved, err := db.SavePostRecipe(PostRecipe{Name: validName, Steps: `[{"preset_id":1,"strength":0.8}]`, Adjust: `{"contrast":1.05}`})
	require.NoError(t, err)
	assert.Greater(t, saved.ID, int64(0))

	recipes, err := db.ListPostRecipes()
	require.NoError(t, err)
	assert.Len(t, recipes, 1)
}
