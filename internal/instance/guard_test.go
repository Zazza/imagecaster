package instance_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/instance"
	"github.com/stretchr/testify/require"
)

const (
	helperEnvironment       = "IMAGECASTER_INSTANCE_TEST_HELPER"
	helperPathEnvironment   = "IMAGECASTER_INSTANCE_TEST_DB"
	helperTimeout           = 10 * time.Second
	activationTimeout       = 3 * time.Second
	forbiddenActivationWait = 50 * time.Millisecond
	activationTokenBytes    = 32
)

func TestInstanceHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvironment) != "hold" {
		return
	}
	guard, err := instance.Acquire(os.Getenv(helperPathEnvironment))
	if err != nil {
		fmt.Fprintln(os.Stdout, "error:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	if err := guard.Close(); err != nil {
		fmt.Fprintln(os.Stdout, "close:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

type helperProcess struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	once  sync.Once
}

func startHelper(t *testing.T, path string) *helperProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstanceHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnvironment+"=hold", helperPathEnvironment+"="+path)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	process := &helperProcess{cmd: cmd, stdin: stdin}
	t.Cleanup(func() { process.once.Do(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }) })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", line)
	return process
}
func (p *helperProcess) finish(t *testing.T, crash bool) {
	t.Helper()
	p.once.Do(func() {
		if crash {
			require.NoError(t, p.cmd.Process.Kill())
		} else {
			require.NoError(t, p.stdin.Close())
		}
		err := p.cmd.Wait()
		if crash {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	})
}

func TestGuardRejectsAnotherProcessAndRecoversAfterExit(t *testing.T) {
	for _, crash := range []bool{false, true} {
		t.Run(fmt.Sprintf("crash=%v", crash), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "presets.db")
			child := startHelper(t, path)
			duplicate, err := instance.Acquire(path)
			require.ErrorIs(t, err, instance.ErrAlreadyRunning)
			require.Nil(t, duplicate)
			child.finish(t, crash)
			guard, err := instance.Acquire(path)
			require.NoError(t, err)
			require.NoError(t, guard.Close())
			guard, err = instance.Acquire(path)
			require.NoError(t, err)
			require.NoError(t, guard.Close())
		})
	}
}

func TestGuardDistinctDatabasesAreIndependent(t *testing.T) {
	root := t.TempDir()
	first, err := instance.Acquire(filepath.Join(root, "first.db"))
	require.NoError(t, err)
	defer first.Close()
	second, err := instance.Acquire(filepath.Join(root, "second.db"))
	require.NoError(t, err)
	defer second.Close()
}

func TestGuardCanonicalDatabaseAliasesShareLock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "presets.db")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	guard, err := instance.Acquire(path)
	require.NoError(t, err)
	defer guard.Close()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relative, err := filepath.Rel(cwd, path)
	require.NoError(t, err)
	duplicate, err := instance.Acquire(relative)
	require.ErrorIs(t, err, instance.ErrAlreadyRunning)
	require.Nil(t, duplicate)
	link := filepath.Join(root, "alias.db")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	duplicate, err = instance.Acquire(link)
	require.ErrorIs(t, err, instance.ErrAlreadyRunning)
	require.Nil(t, duplicate)
}

func TestGuardCanonicalParentAliasForNewDatabase(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "actual")
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Mkdir(actual, 0o700))
	if err := os.Symlink(actual, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	guard, err := instance.Acquire(filepath.Join(actual, "new.db"))
	require.NoError(t, err)
	defer guard.Close()
	duplicate, err := instance.Acquire(filepath.Join(alias, "new.db"))
	require.ErrorIs(t, err, instance.ErrAlreadyRunning)
	require.Nil(t, duplicate)
}

func TestGuardNotifyActivatesFirstInstanceAndWrongTokenDoesNot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presets.db")
	guard, err := instance.Acquire(path)
	require.NoError(t, err)
	defer guard.Close()
	require.NoError(t, instance.Notify(path))
	select {
	case <-guard.Activations():
	case <-time.After(activationTimeout):
		t.Fatal("activation not received")
	}
	raw, err := os.ReadFile(path + ".instance.lock")
	require.NoError(t, err)
	var metadata struct {
		Address string `json:"address"`
		Token   string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(raw, &metadata))
	require.NotEmpty(t, metadata.Token)
	request, err := http.NewRequest(http.MethodPost, "http://"+metadata.Address+"/activate", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer incorrect-token")
	client := &http.Client{Timeout: activationTimeout, Transport: &http.Transport{Proxy: nil}}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	select {
	case <-guard.Activations():
		t.Fatal("wrong token triggered activation")
	case <-time.After(forbiddenActivationWait):
	}
}

func TestGuardRetainsLockFileAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presets.db")
	guard, err := instance.Acquire(path)
	require.NoError(t, err)
	before, err := os.Stat(path + ".instance.lock")
	require.NoError(t, err)
	require.NoError(t, guard.Close())
	after, err := os.Stat(path + ".instance.lock")
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "lock file inode must remain stable across close")
}

func TestNotifyRejectsMalformedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presets.db")
	require.NoError(t, os.WriteFile(path+".instance.lock", []byte("not-json"), 0o600))
	require.Error(t, instance.Notify(path))
}

func TestNotifyDoesNotFollowRedirect(t *testing.T) {
	var reached atomic.Int64
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	path := filepath.Join(t.TempDir(), "presets.db")
	raw, err := json.Marshal(map[string]string{"address": strings.TrimPrefix(redirect.URL, "http://"), "token": strings.Repeat("ab", activationTokenBytes)})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path+".instance.lock", raw, 0o600))
	require.Error(t, instance.Notify(path))
	require.Positive(t, redirected.Load(), "activation must reach the redirecting endpoint")
	require.Zero(t, reached.Load(), "activation must never follow redirects")
}
