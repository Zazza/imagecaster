package stylisttrace_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/stylisttrace"
)

func testImage(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}
func entries(t *testing.T, root string) []os.DirEntry {
	t.Helper()
	e, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestRecorderStagesMetadataAndPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "diagnostics")
	r, err := stylisttrace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, s := range []stylisttrace.Stage{stylisttrace.Base, stylisttrace.Garment, stylisttrace.FacePass, stylisttrace.FaceSwap} {
		if err := r.Save(s, testImage(t)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Finish(42); err != nil {
		t.Fatal(err)
	}
	dirs := entries(t, root)
	if len(dirs) != 1 {
		t.Fatalf("directories=%d", len(dirs))
	}
	dir := filepath.Join(root, dirs[0].Name())
	for _, path := range []string{root, dir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
			t.Fatalf("directory mode %o", info.Mode().Perm())
		}
	}
	for _, f := range entries(t, dir) {
		info, err := f.Info()
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("file %s mode %o", f.Name(), info.Mode().Perm())
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		FrameID int64 `json:"frame_id"`
		Stages  []struct {
			Stage  string `json:"stage"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"stages"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.FrameID != 42 || len(meta.Stages) != 4 {
		t.Fatalf("metadata: %s", raw)
	}
	for _, s := range meta.Stages {
		if s.Width != 3 || s.Height != 2 {
			t.Fatalf("dimensions: %+v", s)
		}
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("unexpected metadata fields: %s", raw)
	}
	for _, name := range []string{"base.png", "garment.png", "face-pass.png", "face-swap.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestCloseRemovesIncompleteAndPreservesCompleted(t *testing.T) {
	root := t.TempDir()
	r, err := stylisttrace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Save(stylisttrace.Base, testImage(t)); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if len(entries(t, root)) != 0 {
		t.Fatal("unfinished trace remains")
	}
	r, err = stylisttrace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Save(stylisttrace.Base, testImage(t)); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(7); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if len(entries(t, root)) != 1 {
		t.Fatal("completed trace removed")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	var absent *stylisttrace.Recorder
	if err := absent.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestRetentionProtectsActiveTrace(t *testing.T) {
	root := t.TempDir()
	active, err := stylisttrace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	if err := active.Save(stylisttrace.Base, testImage(t)); err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 12; i++ {
		r, err := stylisttrace.New(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Save(stylisttrace.Base, testImage(t)); err != nil {
			t.Fatal(err)
		}
		if err := r.Finish(i); err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if len(entries(t, root)) != 11 {
		t.Fatalf("want ten completed and active, got %d", len(entries(t, root)))
	}
	if err := active.Finish(99); err != nil {
		t.Fatalf("active pruned: %v", err)
	}
	if len(entries(t, root)) != 10 {
		t.Fatalf("retention after finish=%d", len(entries(t, root)))
	}
}
func TestActiveTraceLimitReleasesOnClose(t *testing.T) {
	root := t.TempDir()
	var rs []*stylisttrace.Recorder
	for i := 0; i < 4; i++ {
		r, err := stylisttrace.New(root)
		if err != nil {
			t.Fatal(err)
		}
		rs = append(rs, r)
		defer r.Close()
	}
	extra, err := stylisttrace.New(root)
	if err == nil {
		extra.Close()
		t.Fatal("accepted fifth active trace")
	}
	if err := rs[0].Close(); err != nil {
		t.Fatal(err)
	}
	extra, err = stylisttrace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
}
func TestRejectsInvalidInputWithoutEchoingIt(t *testing.T) {
	r, err := stylisttrace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	secret := "private-image-content!"
	if err := r.Save(stylisttrace.Base, secret); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe error: %v", err)
	}
	if err := r.Save(stylisttrace.Stage("../outside"), testImage(t)); err == nil {
		t.Fatal("accepted arbitrary stage")
	}
	if err := r.Finish(0); err == nil {
		t.Fatal("accepted unsaved session item")
	}
}

func TestRejectsOversizedInput(t *testing.T) {
	const decodedLimit = 16 * 1024 * 1024
	r, err := stylisttrace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	oversized := base64.StdEncoding.EncodeToString(make([]byte, decodedLimit+1))
	if err := r.Save(stylisttrace.Base, oversized); err == nil {
		t.Fatal("accepted oversized image")
	}
}

func TestRejectsSymlinkRootWithoutChangingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges depend on host configuration")
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "diagnostics")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	r, err := stylisttrace.New(link)
	if err == nil {
		r.Close()
		t.Fatal("accepted symlink diagnostics root")
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatal("changed symlink target permissions")
	}
	if len(entries(t, target)) != 0 {
		t.Fatal("wrote to symlink target")
	}
}
