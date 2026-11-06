package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

func TestAddToSessionID_WritesToGivenSession(t *testing.T) {
	t.Parallel()
	svc, _, _ := testService(t)
	original := activeSession(t, svc)
	other, err := svc.CreateSession("Другая")
	require.NoError(t, err)
	require.NoError(t, svc.SwitchSession(original.ID))

	id := svc.AddToSessionID(other.ID, makeImageJPEG(t, 64, 64), nil, "stylist", false, &preset.GenerationMeta{Label: "bob", SourceItem: 77})

	require.Greater(t, id, int64(0))
	after := activeSession(t, svc)
	assert.Equal(t, original.ID, after.ID, "active session must not change")
	assert.Equal(t, original.ItemCount, after.ItemCount)
	counts := map[int64]int{}
	all, err := svc.ListSessions()
	require.NoError(t, err)
	for _, si := range all {
		counts[si.ID] = si.ItemCount
	}
	assert.Equal(t, 1, counts[other.ID])

	item, err := svc.SessionItem(id)
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, id, item.ID)
	assert.Equal(t, other.ID, item.SessionID)
	assert.Equal(t, "stylist", item.Source)
	require.NotNil(t, item.Meta)
	assert.Equal(t, int64(77), item.Meta.SourceItem)
	assert.Equal(t, "bob", item.Meta.Label)
}

func TestAddToSessionID_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, svc *Service)
	}{
		{name: "session id 0 returns 0", run: func(t *testing.T, svc *Service) {
			before := activeSession(t, svc)
			assert.Equal(t, int64(0), svc.AddToSessionID(0, makeImageJPEG(t, 64, 64), nil, "stylist", false, nil))
			assert.Equal(t, before.ItemCount, activeSession(t, svc).ItemCount)
		}},
		{name: "missing item is nil without error", run: func(t *testing.T, svc *Service) {
			item, err := svc.SessionItem(987654)
			assert.NoError(t, err)
			assert.Nil(t, item)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := testService(t)
			tt.run(t, svc)
		})
	}
}
