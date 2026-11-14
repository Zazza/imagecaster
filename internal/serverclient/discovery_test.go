package serverclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBrowser struct {
	before   []*zeroconf.ServiceEntry
	atCancel []*zeroconf.ServiceEntry
	noClose  bool
	err      error
}

func (f *fakeBrowser) Browse(ctx context.Context, _, _ string, entries chan<- *zeroconf.ServiceEntry) error {
	if f.err != nil {
		return f.err
	}
	go func() {
		for _, e := range f.before {
			select {
			case entries <- e:
			case <-ctx.Done():
				return
			}
		}
		<-ctx.Done()
		for _, e := range f.atCancel {
			time.Sleep(5 * time.Millisecond)
			entries <- e
		}
		if !f.noClose {
			close(entries)
		}
	}()
	return nil
}

func discEntry(i int, v6 bool) *zeroconf.ServiceEntry {
	e := zeroconf.NewServiceEntry(fmt.Sprintf("srv-%d", i), "_imagecaster._tcp", "local.")
	e.HostName = fmt.Sprintf("host-%d.local.", i)
	e.Port = 8000 + i
	if v6 {
		e.AddrIPv6 = []net.IP{net.ParseIP("fe80::1")}
	} else {
		e.AddrIPv4 = []net.IP{net.IPv4(192, 168, 1, byte(i))}
	}
	return e
}

func TestDiscoverWith(t *testing.T) {
	t.Parallel()
	many := make([]*zeroconf.ServiceEntry, 0, 50)
	for i := 1; i <= 50; i++ {
		many = append(many, discEntry(i, false))
	}
	tests := []struct {
		name      string
		browser   *fakeBrowser
		wantNames []string
		wantErr   bool
		maxWait   time.Duration
	}{
		{
			name:      "entries sent while the browse closes are all returned",
			browser:   &fakeBrowser{before: []*zeroconf.ServiceEntry{discEntry(1, false)}, atCancel: []*zeroconf.ServiceEntry{discEntry(2, true), discEntry(3, false)}},
			wantNames: []string{"srv-1", "srv-2", "srv-3"},
			maxWait:   time.Second,
		},
		{
			name:    "many entries",
			browser: &fakeBrowser{before: many},
			maxWait: time.Second,
		},
		{
			name:      "browser never closes the channel: returns after grace",
			browser:   &fakeBrowser{before: []*zeroconf.ServiceEntry{discEntry(4, false)}, noClose: true},
			wantNames: []string{"srv-4"},
			maxWait:   time.Second,
		},
		{
			name:    "browse error",
			browser: &fakeBrowser{err: errors.New("no multicast")},
			wantErr: true,
			maxWait: time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			got, err := discoverWith(context.Background(), tt.browser, 50*time.Millisecond, 200*time.Millisecond)
			assert.Less(t, time.Since(start), tt.maxWait)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no multicast")
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			if tt.wantNames == nil {
				assert.Len(t, got, len(tt.browser.before))
				return
			}
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.Name)
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

func TestDiscoverWith_AddressFields(t *testing.T) {
	t.Parallel()
	b := &fakeBrowser{before: []*zeroconf.ServiceEntry{discEntry(7, false), discEntry(8, true)}}
	got, err := discoverWith(context.Background(), b, 30*time.Millisecond, 200*time.Millisecond)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, DiscoveredServer{Name: "srv-7", Host: "host-7.local.", Port: 8007, IPAddress: "192.168.1.7"}, got[0])
	assert.Equal(t, DiscoveredServer{Name: "srv-8", Host: "host-8.local.", Port: 8008, IPAddress: "fe80::1"}, got[1])
}

func TestDiscoverWith_ParentCancelStopsEarly(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	got, err := discoverWith(ctx, &fakeBrowser{}, time.Hour, time.Second)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
}
