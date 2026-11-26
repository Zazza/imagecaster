// Package instance keeps one desktop worker owner per database.
package instance

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	activationHost          = "127.0.0.1"
	activationPath          = "/activate"
	activationTokenBytes    = 32
	activationTimeout       = 2 * time.Second
	activationRetryInterval = 50 * time.Millisecond
	metadataLimit           = 4 << 10
	activationHeaderLimit   = 8 << 10
	lockSuffix              = ".instance.lock"
)

var ErrAlreadyRunning = errors.New("imagecaster is already running for this database")

type metadata struct {
	Address string `json:"address"`
	Token   string `json:"token"`
}

type Guard struct {
	file        *os.File
	server      *http.Server
	activations chan struct{}
	once        sync.Once
	closeErr    error
}

// lockPath resolves both existing database symlinks and a symlinked data directory.
func lockPath(dbPath string) (string, error) {
	absolute, err := filepath.Abs(dbPath)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved + lockSuffix, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)) + lockSuffix, nil
}

// Acquire must run before opening the database or starting a queue worker.
func Acquire(dbPath string) (*Guard, error) {
	path, err := lockPath(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolve instance lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock: %w", err)
	}
	locked, err := lockFile(file)
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if !locked {
		return nil, errors.Join(ErrAlreadyRunning, file.Close())
	}
	fail := func(err error) (*Guard, error) { return nil, errors.Join(err, file.Close()) }
	if err := file.Chmod(0o600); err != nil {
		return fail(err)
	}
	token := make([]byte, activationTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return fail(err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(activationHost, "0"))
	if err != nil {
		return fail(err)
	}
	info := metadata{Address: listener.Addr().String(), Token: hex.EncodeToString(token)}
	encoded, err := json.Marshal(info)
	if err == nil {
		err = file.Truncate(0)
	}
	if err == nil {
		_, err = file.WriteAt(encoded, 0)
	}
	if err != nil {
		return fail(errors.Join(err, listener.Close()))
	}
	guard := &Guard{file: file, activations: make(chan struct{}, 1)}
	guard.server = &http.Server{
		ReadHeaderTimeout: activationTimeout, ReadTimeout: activationTimeout, WriteTimeout: activationTimeout, IdleTimeout: activationTimeout,
		MaxHeaderBytes: activationHeaderLimit,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != activationPath {
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte("Bearer "+info.Token)) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			select {
			case guard.activations <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() {
		// Serve closes the listener on return; callers retain the OS lock even if activation fails.
		if err := guard.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "Imagecaster activation listener: %v\n", err)
		}
	}()
	return guard, nil
}

func (g *Guard) Activations() <-chan struct{} { return g.activations }

// Close never unlinks the file: replacing its inode would allow two owners.
func (g *Guard) Close() error {
	g.once.Do(func() { g.closeErr = errors.Join(g.server.Close(), g.file.Close()) })
	return g.closeErr
}

func readMetadata(path string) (metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return metadata{}, err
	}
	defer file.Close()
	var info metadata
	if err := json.NewDecoder(io.LimitReader(file, metadataLimit)).Decode(&info); err != nil {
		return metadata{}, err
	}
	address, err := netip.ParseAddrPort(info.Address)
	if err != nil || address.Addr().String() != activationHost || address.Port() == 0 || len(info.Token) != hex.EncodedLen(activationTokenBytes) {
		return metadata{}, errors.New("invalid local activation metadata")
	}
	return info, nil
}

// Notify is bounded and fail-closed: notification failure never grants ownership.
func Notify(dbPath string) error {
	path, err := lockPath(dbPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), activationTimeout)
	defer cancel()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var lastErr error
	for {
		info, err := readMetadata(path)
		if err == nil {
			var request *http.Request
			request, err = http.NewRequestWithContext(ctx, http.MethodPost, "http://"+info.Address+activationPath, nil)
			if err == nil {
				request.Header.Set("Authorization", "Bearer "+info.Token)
				var response *http.Response
				response, err = client.Do(request)
				if err == nil {
					err = response.Body.Close()
					if response.StatusCode != http.StatusNoContent {
						err = fmt.Errorf("activation HTTP status %d", response.StatusCode)
					}
					if err == nil {
						return nil
					}
				}
			}
		}
		lastErr = err
		timer := time.NewTimer(activationRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("activate existing instance: %w", errors.Join(ctx.Err(), lastErr))
		case <-timer.C:
		}
	}
}
