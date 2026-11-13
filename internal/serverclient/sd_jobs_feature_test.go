package serverclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredFeatures_IncludeSDJobs(t *testing.T) {
	t.Parallel()
	assert.Contains(t, RequiredFeatures, "sd_jobs")
}

func TestProbe_SDJobsFeature(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		features    []string
		wantMissing bool
	}{
		{name: "server without sd_jobs", features: []string{"sdcpp", "download_stream", "llamacpp"}, wantMissing: true},
		{name: "server with sd_jobs", features: []string{"sdcpp", "download_stream", "llamacpp", "sd_jobs"}, wantMissing: false},
		{name: "sd_jobs only", features: []string{"sd_jobs"}, wantMissing: false},
		{name: "old server without features", features: nil, wantMissing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts, _ := probeServer(t, 200, infoJSON(t, ServerName, tt.features))
			info, err := Probe(ts.URL)
			require.NoError(t, err)
			missing := info.MissingFeatures()
			if tt.wantMissing {
				assert.Contains(t, missing, "sd_jobs")
			} else {
				assert.NotContains(t, missing, "sd_jobs")
			}
		})
	}
}

func TestServerInfo_FullSetWithSDJobsHasNothingMissing(t *testing.T) {
	t.Parallel()
	info := &ServerInfo{Name: ServerName, Features: []string{"sdcpp", "download_stream", "llamacpp", "sd_jobs"}}
	assert.Empty(t, info.MissingFeatures())
}

func TestServerInfo_OnlySDJobsMissing(t *testing.T) {
	t.Parallel()
	info := &ServerInfo{Name: ServerName, Features: []string{"sdcpp", "download_stream", "llamacpp"}}
	assert.Equal(t, []string{"sd_jobs"}, info.MissingFeatures())
}
