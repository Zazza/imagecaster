package preset

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func personPhotoFixture(key, file string, dhash uint64, emb []float32) *PersonPhoto {
	return &PersonPhoto{
		PersonKey:  key,
		FileName:   file,
		SourceName: "src-" + file,
		Width:      1024,
		Height:     768,
		FaceHeight: 320,
		Yaw:        -27.5,
		Sharpness:  412.25,
		Embedding:  emb,
		DHash:      dhash,
	}
}

func TestPersonPhotos_SchemaV39(t *testing.T) {
	t.Parallel()
	raw := openRaw(t, freshDB(t))
	assert.Contains(t, queryStrings(t, raw, `SELECT name FROM sqlite_master WHERE type='table'`), "person_photos")
}

func TestPersonPhotos_V38DatabaseGetsTable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.db")
	db, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	raw := openRaw(t, path)
	_, err = raw.Exec(`DROP TABLE person_photos`)
	require.NoError(t, err)
	require.NoError(t, setUserVersion(raw, 38))
	require.NoError(t, raw.Close())

	db, err = Open(path)
	require.NoError(t, err)
	defer db.Close()
	v, err := userVersion(db.DB())
	require.NoError(t, err)
	assert.Equal(t, schemaVersion, v)

	p := personPhotoFixture("A", "a.jpg", 1, []float32{1})
	require.NoError(t, db.AddPersonPhoto(p))
	list, err := db.ListPersonPhotos("A")
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestPersonPhotos_RoundTripExact(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	emb := []float32{0.1, -0.2, 3.4028235e38, -1e-38, float32(math.Pi), 0, 1e-7}
	tests := []struct {
		name  string
		dhash uint64
		emb   []float32
	}{
		{name: "regular", dhash: 0x0123456789abcdef, emb: emb},
		{name: "high bit dhash", dhash: math.MaxUint64, emb: []float32{-0.5}},
		{name: "zero dhash", dhash: 0, emb: []float32{0.25, 0.75}},
	}
	for i, tt := range tests {
		p := personPhotoFixture("Наташа", tt.name+".jpg", tt.dhash, tt.emb)
		require.NoError(t, db.AddPersonPhoto(p), tt.name)
		require.Positive(t, p.ID, "id assigned")

		got, err := db.GetPersonPhoto(p.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, tt.dhash, got.DHash, tt.name)
		assert.Equal(t, tt.emb, got.Embedding, tt.name)
		assert.Equal(t, "Наташа", got.PersonKey)
		assert.Equal(t, tt.name+".jpg", got.FileName)
		assert.Equal(t, "src-"+tt.name+".jpg", got.SourceName)
		assert.Equal(t, 1024, got.Width)
		assert.Equal(t, 768, got.Height)
		assert.Equal(t, 320, got.FaceHeight)
		assert.Equal(t, -27.5, got.Yaw)
		assert.Equal(t, 412.25, got.Sharpness)
		assert.NotEmpty(t, got.AddedAt)

		list, err := db.ListPersonPhotos("Наташа")
		require.NoError(t, err)
		require.Len(t, list, i+1)
		var found bool
		for _, l := range list {
			if l.ID == p.ID {
				found = true
				assert.Equal(t, tt.dhash, l.DHash)
				assert.Equal(t, tt.emb, l.Embedding)
			}
		}
		assert.True(t, found)
	}
}

func TestPersonPhotos_ListByKeyAndAll(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	list, err := db.ListPersonPhotos("")
	require.NoError(t, err)
	assert.Empty(t, list)

	require.NoError(t, db.AddPersonPhoto(personPhotoFixture("A", "a1.jpg", 1, []float32{1})))
	require.NoError(t, db.AddPersonPhoto(personPhotoFixture("A", "a2.jpg", 2, []float32{2})))
	require.NoError(t, db.AddPersonPhoto(personPhotoFixture("B", "b1.jpg", 3, []float32{3})))

	tests := []struct {
		key  string
		want int
	}{
		{"A", 2}, {"B", 1}, {"", 3}, {"nobody", 0},
	}
	for _, tt := range tests {
		list, err := db.ListPersonPhotos(tt.key)
		require.NoError(t, err)
		assert.Len(t, list, tt.want, "key %q", tt.key)
		for _, p := range list {
			if tt.key != "" {
				assert.Equal(t, tt.key, p.PersonKey)
			}
		}
	}
}

func TestPersonPhotos_GetMissing(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	got, err := db.GetPersonPhoto(12345)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestPersonPhotos_Delete(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	a := personPhotoFixture("A", "a.jpg", 1, []float32{1})
	b := personPhotoFixture("A", "b.jpg", 2, []float32{2})
	require.NoError(t, db.AddPersonPhoto(a))
	require.NoError(t, db.AddPersonPhoto(b))

	require.NoError(t, db.DeletePersonPhoto(a.ID))
	got, err := db.GetPersonPhoto(a.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	list, err := db.ListPersonPhotos("A")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, b.ID, list[0].ID)
}

func TestPersonPhotos_Keys(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	keys, err := db.PersonPhotoKeys()
	require.NoError(t, err)
	assert.Empty(t, keys)
	for _, k := range []string{"Аня", "Аня", "Оля"} {
		require.NoError(t, db.AddPersonPhoto(&PersonPhoto{PersonKey: k, FileName: "x.jpg"}))
	}
	keys, err = db.PersonPhotoKeys()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Аня", "Оля"}, keys)
}

func TestPersonPhotos_Rename(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	require.NoError(t, db.AddPersonPhoto(personPhotoFixture("old", "1.jpg", 1, []float32{1})))
	require.NoError(t, db.AddPersonPhoto(personPhotoFixture("old", "2.jpg", 2, []float32{2})))
	require.NoError(t, db.AddPersonPhoto(personPhotoFixture("other", "3.jpg", 3, []float32{3})))

	require.NoError(t, db.RenamePersonPhotos("old", "new"))

	tests := []struct {
		key  string
		want int
	}{
		{"old", 0}, {"new", 2}, {"other", 1},
	}
	for _, tt := range tests {
		list, err := db.ListPersonPhotos(tt.key)
		require.NoError(t, err)
		assert.Len(t, list, tt.want, "key %q", tt.key)
	}

	require.NoError(t, db.RenamePersonPhotos("missing", "x"))
	list, err := db.ListPersonPhotos("x")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestPersonPhotos_RenameIgnoresCase(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	require.NoError(t, db.AddPersonPhoto(&PersonPhoto{PersonKey: "Anna", FileName: "a.jpg"}))
	require.NoError(t, db.AddPersonPhoto(&PersonPhoto{PersonKey: "Оля", FileName: "b.jpg"}))
	require.NoError(t, db.RenamePersonPhotos("anna", "Аня"))
	keys, err := db.PersonPhotoKeys()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Аня", "Оля"}, keys)
}
