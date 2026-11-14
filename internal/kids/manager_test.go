package kids

import (
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/preset"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager(t *testing.T) (*Manager, *preset.DB) {
	t.Helper()
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewManager(db), db
}

func TestStorePin_Valid(t *testing.T) {
	m, db := newTestManager(t)
	err := m.storePin("1234")
	assert.NoError(t, err)
	hash, _ := db.GetSetting("kids_pin_hash")
	assert.NotEmpty(t, hash)
	salt, _ := db.GetSetting("kids_pin_salt")
	assert.NotEmpty(t, salt)
}

func TestStorePin_InvalidLength(t *testing.T) {
	m, _ := newTestManager(t)
	err := m.storePin("12")
	assert.Error(t, err)
}

func TestVerifyPin_Correct(t *testing.T) {
	m, _ := newTestManager(t)
	require.NoError(t, m.storePin("1234"))
	ok, err := m.verifyPin("1234")
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestVerifyPin_Incorrect(t *testing.T) {
	m, _ := newTestManager(t)
	require.NoError(t, m.storePin("1234"))
	ok, err := m.verifyPin("0000")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestVerifyPin_NotSet(t *testing.T) {
	m, _ := newTestManager(t)
	_, err := m.verifyPin("1234")
	assert.Error(t, err)
}

func TestHashPin_UsesSalt(t *testing.T) {
	h1 := hashPin("1234", "salt1")
	h2 := hashPin("1234", "salt2")
	assert.NotEqual(t, h1, h2, "same PIN with different salts must produce different hashes")
}

func TestSetKidsMode_EnableWithPin(t *testing.T) {
	m, db := newTestManager(t)
	err := m.SetKidsMode(true, "1234")
	assert.NoError(t, err)
	v, _ := db.GetSetting("kids_mode")
	assert.Equal(t, "true", v)
}

func TestSetKidsMode_DisableWithCorrectPin(t *testing.T) {
	m, db := newTestManager(t)
	require.NoError(t, m.SetKidsMode(true, "1234"))
	err := m.SetKidsMode(false, "1234")
	assert.NoError(t, err)
	v, _ := db.GetSetting("kids_mode")
	assert.Equal(t, "false", v)
}

func TestSetKidsMode_DisableWithWrongPin(t *testing.T) {
	m, _ := newTestManager(t)
	require.NoError(t, m.SetKidsMode(true, "1234"))
	err := m.SetKidsMode(false, "0000")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "incorrect PIN")
}

func TestSetKidsMode_DisableWithoutPin(t *testing.T) {
	m, _ := newTestManager(t)
	require.NoError(t, m.SetKidsMode(true, "1234"))
	err := m.SetKidsMode(false, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PIN required")
}

func TestSetKidsMode_EnableBadPin(t *testing.T) {
	m, _ := newTestManager(t)
	err := m.SetKidsMode(true, "12")
	assert.Error(t, err)
}

func TestSetKidsCategory_RejectsWrongPin(t *testing.T) {
	m, _ := newTestManager(t)
	require.NoError(t, m.SetKidsMode(true, "1234"))
	err := m.SetCategory("violence", false, "0000")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "incorrect PIN")
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newClockedManager(t *testing.T) (*Manager, *preset.DB, *fakeClock) {
	t.Helper()
	m, db := newTestManager(t)
	clock := &fakeClock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	m.now = clock.now
	return m, db, clock
}

func TestSetKidsMode_PinLockout(t *testing.T) {
	m, db, clock := newClockedManager(t)
	require.NoError(t, m.SetKidsMode(true, "1234"))

	for i := 0; i < maxPinAttempts; i++ {
		err := m.SetKidsMode(false, "0000")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "incorrect PIN")
	}

	err := m.SetKidsMode(false, "1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PIN locked")
	assert.True(t, m.IsActive())

	other := NewManager(db)
	other.now = clock.now
	err = other.SetKidsMode(false, "1234")
	require.Error(t, err, "lockout must survive manager restart")
	assert.Contains(t, err.Error(), "PIN locked")

	clock.t = clock.t.Add(pinLockoutMins*time.Minute - time.Second)
	err = m.SetKidsMode(false, "1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PIN locked")

	clock.t = clock.t.Add(2 * time.Second)
	require.NoError(t, m.SetKidsMode(false, "1234"))
	assert.False(t, m.IsActive())
}

func TestSetKidsMode_CorrectPinResetsFailedAttempts(t *testing.T) {
	m, _, _ := newClockedManager(t)
	require.NoError(t, m.SetKidsMode(true, "1234"))

	for i := 0; i < maxPinAttempts-1; i++ {
		require.Error(t, m.SetKidsMode(false, "0000"))
	}
	require.NoError(t, m.SetKidsMode(false, "1234"))
	require.NoError(t, m.SetKidsMode(true, ""))

	for i := 0; i < maxPinAttempts-1; i++ {
		require.Error(t, m.SetKidsMode(false, "0000"))
	}
	require.NoError(t, m.SetKidsMode(false, "1234"))
}

func TestSetCategory_PinLockout(t *testing.T) {
	m, _, clock := newClockedManager(t)
	require.NoError(t, m.SetKidsMode(true, "1234"))

	for i := 0; i < maxPinAttempts; i++ {
		require.Error(t, m.SetCategory("violence", false, "0000"))
	}
	err := m.SetCategory("violence", false, "1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PIN locked")
	assert.Empty(t, m.getDisabledCategories())

	err = m.SetKidsMode(false, "1234")
	require.Error(t, err, "lockout is shared between category and mode changes")
	assert.Contains(t, err.Error(), "PIN locked")

	clock.t = clock.t.Add(pinLockoutMins*time.Minute + time.Second)
	require.NoError(t, m.SetCategory("violence", false, "1234"))
	assert.Equal(t, map[string]bool{"violence": true}, m.getDisabledCategories())
}

func TestManager_InactivePassesThrough(t *testing.T) {
	m, _ := newTestManager(t)

	assert.Equal(t, "lowres", m.ApplyNegative("lowres"))
	assert.Equal(t, "", m.ApplyNegative(""))

	out, err := m.FilterInput("nude painting with blood")
	require.NoError(t, err)
	assert.Equal(t, "nude painting with blood", out)

	assert.Equal(t, "nude, blood, red dress", m.FilterOutput("nude, blood, red dress"))
}

func TestManager_ApplyNegative(t *testing.T) {
	m, _ := newTestManager(t)
	require.NoError(t, m.SetKidsMode(true, ""))

	all := NegativePrompt(nil)
	assert.Equal(t, all, m.ApplyNegative(""))
	assert.Equal(t, "lowres, "+all, m.ApplyNegative("lowres"))

	require.NoError(t, m.SetCategory("violence", false, ""))
	got := m.ApplyNegative("lowres")
	assert.Equal(t, "lowres, "+NegativePrompt(map[string]bool{"violence": true}), got)
	assert.NotContains(t, got, "gore")
	assert.Contains(t, got, "nsfw")
}

func TestManager_FilterInputAndOutputFollowCategories(t *testing.T) {
	m, _ := newTestManager(t)
	require.NoError(t, m.SetKidsMode(true, ""))

	_, err := m.FilterInput("a knight with a gun")
	require.Error(t, err)
	assert.Equal(t, "beautiful eyes, long hair, red dress", m.FilterOutput("beautiful eyes, gun, long hair, red dress"))

	require.NoError(t, m.SetCategory("weapons", false, ""))

	out, err := m.FilterInput("a knight with a gun")
	require.NoError(t, err)
	assert.Equal(t, "a knight with a gun", out)
	assert.Equal(t, "beautiful eyes, gun, long hair, red dress", m.FilterOutput("beautiful eyes, gun, long hair, red dress"))

	_, err = m.FilterInput("nude knight")
	require.Error(t, err)
}

func TestManager_GetDisabledCategoriesIgnoresAlwaysOn(t *testing.T) {
	m, db := newTestManager(t)
	require.NoError(t, m.SetKidsMode(true, ""))

	require.NoError(t, db.SetSetting("kids_cat_nsfw", "false"))
	require.NoError(t, db.SetSetting("kids_cat_horror", "false"))
	require.NoError(t, db.SetSetting("kids_cat_weapons", "true"))

	assert.Equal(t, map[string]bool{"horror": true}, m.getDisabledCategories())

	_, err := m.FilterInput("nude painting")
	require.Error(t, err)
	_, err = m.FilterInput("scary zombie")
	require.NoError(t, err)
}
