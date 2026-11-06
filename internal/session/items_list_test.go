package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionItems_OnlyOwnSessionOldestFirst(t *testing.T) {
	t.Parallel()
	svc, _, _ := testService(t)
	first, err := svc.CreateSession("Первая")
	require.NoError(t, err)
	second, err := svc.CreateSession("Вторая")
	require.NoError(t, err)

	older := svc.AddToSessionID(first.ID, makeImageJPEG(t, 64, 64), nil, "sd", false, nil)
	require.Greater(t, older, int64(0))
	other := svc.AddToSessionID(second.ID, makeImageJPEG(t, 64, 64), nil, "sd", false, nil)
	require.Greater(t, other, int64(0))
	newer := svc.AddToSessionID(first.ID, makeImageJPEG(t, 64, 64), nil, "sd", false, nil)
	require.Greater(t, newer, int64(0))

	items, err := svc.SessionItems(first.ID)

	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, older, items[0].ID)
	assert.Equal(t, newer, items[1].ID)
	for _, it := range items {
		assert.Equal(t, first.ID, it.SessionID)
	}
}

func TestSessionItems_EmptyAndMissing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   func(t *testing.T, svc *Service) int64
	}{
		{name: "empty session", id: func(t *testing.T, svc *Service) int64 {
			si, err := svc.CreateSession("Пустая")
			require.NoError(t, err)
			return si.ID
		}},
		{name: "missing session", id: func(t *testing.T, svc *Service) int64 { return 987654 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := testService(t)
			items, err := svc.SessionItems(tt.id(t, svc))
			require.NoError(t, err)
			assert.Empty(t, items)
		})
	}
}
