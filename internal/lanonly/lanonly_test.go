package lanonly

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want bool
	}{
		{raw: "http://localhost:1234", want: true},
		{raw: "http://127.0.0.1:7860", want: true},
		{raw: "http://192.168.1.184:8080/api/llm", want: true},
		{raw: "https://192.168.1.184:8443", want: true},
		{raw: "http://10.0.0.5", want: true},
		{raw: "http://10.255.1.2:9000", want: true},
		{raw: "http://172.16.3.4", want: true},
		{raw: "http://[::1]:11434", want: true},
		{raw: "http://[fd00::1]", want: true},
		{raw: "http://[fe80::1]:8080", want: true},
		{raw: "http://169.254.10.20", want: true},
		{raw: "http://8.8.8.8", want: false},
		{raw: "https://93.184.215.14", want: false},
		{raw: "http://[2001:4860:4860::8888]", want: false},
		{raw: "http://172.32.0.1", want: false},
		{raw: "ftp://192.168.1.1", want: false},
		{raw: "file:///etc/passwd", want: false},
		{raw: "", want: false},
		{raw: "not a url", want: false},
		{raw: "://bad", want: false},
		{raw: "http://no-such-host.invalid", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsLocalURL(tt.raw))
		})
	}
}

func TestIsLocalURL_PublicDNSName(t *testing.T) {
	t.Parallel()
	addrs, err := net.LookupHost("example.com")
	if err != nil || len(addrs) == 0 {
		t.Skip("DNS resolution unavailable")
	}
	assert.False(t, IsLocalURL("https://example.com"))
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy", "REQUEST_METHOD"} {
		t.Setenv(k, "")
	}
}

func TestViaProxy(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		httpProxy  string
		httpsProxy string
		noProxy    string
		want       bool
	}{
		{name: "no proxy configured", raw: "http://192.168.1.184:8080", want: false},
		{name: "http proxy, address not excluded", raw: "http://192.168.1.184:8080", httpProxy: "http://10.0.0.1:3128", want: true},
		{name: "http proxy, address in NO_PROXY", raw: "http://192.168.1.184:8080", httpProxy: "http://10.0.0.1:3128", noProxy: "192.168.1.184", want: false},
		{name: "http proxy, subnet in NO_PROXY", raw: "http://192.168.1.184:8080", httpProxy: "http://10.0.0.1:3128", noProxy: "192.168.0.0/16", want: false},
		{name: "http proxy, other host in NO_PROXY", raw: "http://192.168.1.184:8080", httpProxy: "http://10.0.0.1:3128", noProxy: "192.168.1.200", want: true},
		{name: "https proxy for https address", raw: "https://192.168.1.184:8443", httpsProxy: "http://10.0.0.1:3128", want: true},
		{name: "unparsable url", raw: "://bad", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProxyEnv(t)
			if tt.httpProxy != "" {
				t.Setenv("HTTP_PROXY", tt.httpProxy)
				t.Setenv("http_proxy", tt.httpProxy)
			}
			if tt.httpsProxy != "" {
				t.Setenv("HTTPS_PROXY", tt.httpsProxy)
				t.Setenv("https_proxy", tt.httpsProxy)
			}
			if tt.noProxy != "" {
				t.Setenv("NO_PROXY", tt.noProxy)
				t.Setenv("no_proxy", tt.noProxy)
			}
			assert.Equal(t, tt.want, ViaProxy(tt.raw))
		})
	}
}
