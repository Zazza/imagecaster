package people

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

func TestAddIdentity(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		photos    int
		apiErr    error
		wantErr   error
		wantCalls bool
	}{
		{name: "remote server refused", serverURL: "http://8.8.8.8:8080", photos: 6, wantErr: ErrNotLocal},
		{name: "too few photos", serverURL: "http://192.168.1.184:8080", photos: 4, wantErr: ErrIdentityCount},
		{name: "too many photos", serverURL: "http://192.168.1.184:8080", photos: MaxPhotos + 1, wantErr: ErrIdentityCount},
		{name: "minimum uploads", serverURL: "http://192.168.1.184:8080", photos: 5, wantCalls: true},
		{name: "maximum uploads", serverURL: "http://192.168.1.184:8080", photos: 50, wantCalls: true},
		{
			name: "too few faces", serverURL: "http://192.168.1.184:8080", photos: 8, wantCalls: true,
			apiErr: &serverclient.TrainAPIError{Status: http.StatusBadRequest, Code: "too_few_faces"}, wantErr: ErrTooFewFaces,
		},
		{
			name: "no face models", serverURL: "http://192.168.1.184:8080", photos: 8, wantCalls: true,
			apiErr: &serverclient.TrainAPIError{Status: http.StatusServiceUnavailable, Code: "face_models_missing"}, wantErr: ErrNoFaceModels,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			dir := t.TempDir()
			want := makePhotos(t, dir, tt.photos)
			api := &fakeAPI{identityErr: tt.apiErr}
			s := New(api, func() string { return tt.serverURL }, func() string { return "inst-1" })

			p, err := s.AddIdentity(context.Background(), dir, "anna")

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr), "want %v, got %v", tt.wantErr, err)
				assert.Nil(t, p)
			} else {
				require.NoError(t, err)
				require.NotNil(t, p)
				assert.True(t, p.Identity)
			}
			if tt.wantCalls {
				assert.Equal(t, "anna", api.identityName)
				assert.ElementsMatch(t, want, api.identityPhotos)
			} else {
				assert.Empty(t, api.identityPhotos)
			}
		})
	}
}
