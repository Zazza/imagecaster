package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/queue"
)

func TestQueueHandlersRefuseNonLocalSD(t *testing.T) {
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
	tests := []struct {
		name    string
		handler func(cfg *config.Config) queue.Handler
		notLoc  error
	}{
		{name: "stylist frame", handler: func(cfg *config.Config) queue.Handler { return stylistFrameHandler(nil, cfg) }, notLoc: errStylistFrameNotLocal},
		{name: "retouch", handler: func(cfg *config.Config) queue.Handler { return retouchHandler(nil, cfg) }, notLoc: errRetouchNotLocal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &queue.Job{Params: `{}`}

			_, err := tt.handler(&config.Config{SDUrl: "http://8.8.8.8:7860"})(nil, job)
			require.ErrorIs(t, err, tt.notLoc)

			_, err = tt.handler(&config.Config{SDUrl: "http://192.168.1.184:8080"})(nil, job)
			require.Error(t, err)
			assert.NotErrorIs(t, err, tt.notLoc)
		})
	}
}
