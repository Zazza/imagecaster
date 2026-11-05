package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

func namedSessions(t *testing.T, svc *Service, name string) []preset.SessionInfo {
	t.Helper()
	all, err := svc.ListSessions()
	require.NoError(t, err)
	var out []preset.SessionInfo
	for _, si := range all {
		if si.Name == name {
			out = append(out, si)
		}
	}
	return out
}

func activeSession(t *testing.T, svc *Service) preset.SessionInfo {
	t.Helper()
	all, err := svc.ListSessions()
	require.NoError(t, err)
	for _, si := range all {
		if si.IsActive {
			return si
		}
	}
	require.FailNow(t, "no active session")
	return preset.SessionInfo{}
}

func TestAddToNamedSession_CreatesMissingSession(t *testing.T) {
	t.Parallel()
	svc, _, _ := testService(t)
	before := activeSession(t, svc)
	require.Empty(t, namedSessions(t, svc, "Стилист"))

	id := svc.AddToNamedSession("Стилист", makeImageJPEG(t, 64, 64), nil, "stylist", false, &preset.GenerationMeta{Label: "bob"})

	assert.Greater(t, id, int64(0))
	named := namedSessions(t, svc, "Стилист")
	require.Len(t, named, 1)
	assert.Equal(t, 1, named[0].ItemCount)
	after := activeSession(t, svc)
	assert.Equal(t, before.ID, after.ID, "active session must not change")
	assert.Equal(t, before.ItemCount, after.ItemCount)
}

func TestAddToNamedSession_ReusesSession(t *testing.T) {
	t.Parallel()

	t.Run("second call goes to the same session", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := testService(t)
		before := activeSession(t, svc)

		id1 := svc.AddToNamedSession("ИИ-агент", makeImageJPEG(t, 64, 64), nil, "sd", false, nil)
		id2 := svc.AddToNamedSession("ИИ-агент", makeImageJPEG(t, 64, 64), nil, "sd", false, nil)

		assert.Greater(t, id1, int64(0))
		assert.Greater(t, id2, int64(0))
		assert.NotEqual(t, id1, id2)
		named := namedSessions(t, svc, "ИИ-агент")
		require.Len(t, named, 1)
		assert.Equal(t, 2, named[0].ItemCount)
		assert.Equal(t, before.ID, activeSession(t, svc).ID)
	})

	t.Run("newest of same-named sessions is used", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := testService(t)
		original := activeSession(t, svc)
		older, err := svc.CreateSession("Стилист")
		require.NoError(t, err)
		newer, err := svc.CreateSession("Стилист")
		require.NoError(t, err)
		require.NoError(t, svc.SwitchSession(original.ID))

		id := svc.AddToNamedSession("Стилист", makeImageJPEG(t, 64, 64), nil, "stylist", false, nil)

		assert.Greater(t, id, int64(0))
		counts := map[int64]int{}
		for _, si := range namedSessions(t, svc, "Стилист") {
			counts[si.ID] = si.ItemCount
		}
		assert.Len(t, counts, 2, "no extra session must be created")
		assert.Equal(t, 1, counts[newer.ID])
		assert.Equal(t, 0, counts[older.ID])
		assert.Equal(t, original.ID, activeSession(t, svc).ID)
	})
}

func TestAddToSession_StillWritesToActiveSession(t *testing.T) {
	t.Parallel()
	svc, _, _ := testService(t)
	active := activeSession(t, svc)
	require.Greater(t, svc.AddToNamedSession("ИИ-агент", makeImageJPEG(t, 64, 64), nil, "sd", false, nil), int64(0))

	id := svc.AddToSession(makeImageJPEG(t, 64, 64), nil, "sd", false, nil)

	assert.Greater(t, id, int64(0))
	after := activeSession(t, svc)
	assert.Equal(t, active.ID, after.ID)
	assert.Equal(t, active.ItemCount+1, after.ItemCount)
	named := namedSessions(t, svc, "ИИ-агент")
	require.Len(t, named, 1)
	assert.Equal(t, 1, named[0].ItemCount)
}
