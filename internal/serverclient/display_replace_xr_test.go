package serverclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmitTraining_DisplayReplaceXR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		display     string
		replace     bool
		wantDisplay []string
		wantReplace []string
	}{
		{"name with replace", "Наташа", true, []string{"Наташа"}, []string{"1"}},
		{"name without replace", "Наташа", false, []string{"Наташа"}, nil},
		{"replace without name", "", true, nil, nil},
		{"neither", "", false, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, http.StatusAccepted, trHoldJobJSON)
			paths, _ := trPhotos(t, 10)

			_, err := c.SubmitTraining(context.Background(), TrainRequest{
				Name: "anna", Gender: "woman", Base: "zimage", Client: "inst-1", ClientRef: "inst-1:train:anna",
				Photos: paths, DisplayName: tt.display, DisplayReplace: tt.replace,
			})

			require.NoError(t, err)
			mp := trParseMultipart(t, rec.last(t))
			gotReplace, ok := mp.fields["display_replace"]
			if tt.wantReplace == nil {
				assert.False(t, ok, "display_replace must be absent, got %v", gotReplace)
			} else {
				assert.Equal(t, tt.wantReplace, gotReplace)
			}
			gotDisplay, ok := mp.fields["display_name"]
			if tt.wantDisplay == nil {
				assert.False(t, ok && len(gotDisplay) > 0 && gotDisplay[0] != "", "display_name must be absent or empty, got %v", gotDisplay)
			} else {
				assert.Equal(t, tt.wantDisplay, gotDisplay)
			}
		})
	}
}
