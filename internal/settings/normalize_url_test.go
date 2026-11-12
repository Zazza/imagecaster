package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeServerURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"srv:8080", "http://srv:8080"},
		{"192.168.1.10:8080/", "http://192.168.1.10:8080"},
		{"  http://srv:8080//  ", "http://srv:8080"},
		{"https://srv", "https://srv"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, NormalizeServerURL(tc.in), tc.in)
	}
}
