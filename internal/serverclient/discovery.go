package serverclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

type DiscoveredServer struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	IPAddress string `json:"ip_address"`
}

type mdnsBrowser interface {
	Browse(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error
}

const discoveryDrainGrace = 2 * time.Second

func DiscoverServers(ctx context.Context, timeout time.Duration) ([]DiscoveredServer, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("mDNS resolver init: %w", err)
	}
	return discoverWith(ctx, resolver, timeout, discoveryDrainGrace)
}

func discoverWith(ctx context.Context, b mdnsBrowser, timeout, grace time.Duration) ([]DiscoveredServer, error) {
	entries := make(chan *zeroconf.ServiceEntry)
	var mu sync.Mutex
	var servers []DiscoveredServer
	drained := make(chan struct{})

	go func() {
		defer close(drained)
		for entry := range entries {
			srv := DiscoveredServer{
				Name: entry.Instance,
				Host: entry.HostName,
				Port: entry.Port,
			}
			if len(entry.AddrIPv4) > 0 {
				srv.IPAddress = entry.AddrIPv4[0].String()
			} else if len(entry.AddrIPv6) > 0 {
				srv.IPAddress = entry.AddrIPv6[0].String()
			}
			mu.Lock()
			servers = append(servers, srv)
			mu.Unlock()
		}
	}()

	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := b.Browse(searchCtx, "_imagecaster._tcp", "local.", entries); err != nil {
		return nil, fmt.Errorf("mDNS browse: %w", err)
	}

	<-searchCtx.Done()
	select {
	case <-drained:
	case <-time.After(grace):
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]DiscoveredServer(nil), servers...), nil
}
