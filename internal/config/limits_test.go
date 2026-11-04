package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxImageBytesIs128MB(t *testing.T) {
	assert.Equal(t, 128*1024*1024, MaxImageBytes)
}

func TestMaxImageBase64LenCoversMaxImageBytes(t *testing.T) {
	assert.GreaterOrEqual(t, MaxImageBase64Len, base64.StdEncoding.EncodedLen(MaxImageBytes))
}

func TestMaxImageBase64LenAboveObservedPostprocessImage(t *testing.T) {
	assert.Greater(t, MaxImageBase64Len, 27*1024*1024)
}

func TestImageTooLargeMessage(t *testing.T) {
	tests := []struct {
		what string
		want string
	}{
		{what: "image", want: "image too large"},
		{what: "reference image", want: "reference image too large"},
		{what: "mask", want: "mask too large"},
	}
	for _, tt := range tests {
		t.Run(tt.what, func(t *testing.T) {
			err := ImageTooLarge(tt.what)
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), tt.want), err.Error())
			assert.Contains(t, err.Error(), "max 128 MB")
			assert.NotContains(t, err.Error(), "16 MB")
		})
	}
}
