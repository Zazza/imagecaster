package queue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_EnqueueLabeled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params any
		label  string
	}{
		{name: "post save", params: map[string]any{"format": "png"}, label: "До 4K: a.png"},
		{name: "empty label", params: map[string]any{}, label: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := NewStore(openTestDB(t))
			emitter := &mockEmitter{}
			svc := NewService(store, &mockProcessor{}, emitter)

			id, err := svc.EnqueueLabeled(JobPostSave, tt.params, "post", tt.label)

			require.NoError(t, err)
			job := sfGet(t, store, id)
			assert.Equal(t, JobPostSave, job.Type)
			assert.Equal(t, StatusPending, job.Status)
			assert.Equal(t, "post", job.Source)
			want, err := json.Marshal(tt.params)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), job.Params)
			var detail map[string]any
			require.NoError(t, json.Unmarshal([]byte(job.ProgressDetail), &detail))
			assert.Equal(t, tt.label, detail["label"])
			assert.True(t, emitter.hasEvent(EventQueueChanged))
		})
	}
}
