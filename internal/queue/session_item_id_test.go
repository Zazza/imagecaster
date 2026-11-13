package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
)

func TestProcessGeneration_SessionItemIDInResultJSON(t *testing.T) {
	cases := []struct {
		name    string
		itemID  int64
		want    string
		wantKey bool
	}{
		{name: "saved to session", itemID: 42, want: `"session_item_id":42`, wantKey: true},
		{name: "not saved", itemID: 0, wantKey: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &processor{emit: &mockEmitter{}}
			res, err := p.processGeneration(&Job{}, func() (*generation.GenerateImageResult, error) {
				return &generation.GenerateImageResult{Image: "img", SessionItemID: tc.itemID}, nil
			})
			if err != nil {
				t.Fatalf("processGeneration returned error: %v", err)
			}
			resultJSON, err := json.Marshal(res)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			payloadJSON, err := json.Marshal(EventPayload{JobID: 1, Result: res})
			if err != nil {
				t.Fatalf("marshal EventPayload: %v", err)
			}
			for label, encoded := range map[string]string{"result": string(resultJSON), "payload": string(payloadJSON)} {
				if !strings.Contains(encoded, `"image_base64":"img"`) {
					t.Errorf("%s JSON %s lacks image_base64", label, encoded)
				}
				has := strings.Contains(encoded, `"session_item_id"`)
				if has != tc.wantKey {
					t.Errorf("%s JSON %s: session_item_id present = %v, want %v", label, encoded, has, tc.wantKey)
				}
				if tc.wantKey && !strings.Contains(encoded, tc.want) {
					t.Errorf("%s JSON %s does not contain %s", label, encoded, tc.want)
				}
			}
		})
	}
}
