package generation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zazza/imagecaster/internal/sd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateImage_SessionItemIDFromAddToSession(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	makeTestPreset(t, db, nil)

	sdSvc := &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			return &sd.Txt2ImgResponse{
				Images:     []string{"base64imagedata"},
				Info:       json.RawMessage(`{"seed": 42}`),
				Parameters: json.RawMessage(`{}`),
			}, nil
		},
	}

	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	sessions := &mockSessions{items: make([]sessionItem, 6)}
	svc.sessions = sessions
	svc.ctx = context.Background()

	result, err := svc.GenerateImage(GenerateImageParams{PresetID: 1})
	require.NoError(t, err)
	require.Len(t, sessions.items, 7)
	assert.Equal(t, int64(7), result.SessionItemID)

	raw, err := json.Marshal(result)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.EqualValues(t, 7, decoded["session_item_id"])
}
