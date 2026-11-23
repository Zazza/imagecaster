package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/objectmask"
)

func TestObjectMaskRouteRestrictsPathsAndMethods(t *testing.T) {
	const fixtureMode = 0600
	const fixtureDirectoryMode = 0700
	dataDir := t.TempDir()
	directory := filepath.Join(dataDir, objectmask.Directory)
	if err := os.MkdirAll(directory, fixtureDirectoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, objectmask.EncoderName), []byte("local weights"), fixtureMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "private.txt"), []byte("secret"), fixtureMode); err != nil {
		t.Fatal(err)
	}
	handler := &imageFileHandler{dataDir: dataDir}
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"encoder", http.MethodGet, objectmask.EncoderURL, http.StatusOK},
		{"missing decoder", http.MethodGet, objectmask.DecoderURL, http.StatusNotFound},
		{"unknown file", http.MethodGet, "/api/object-mask/private.txt", http.StatusNotFound},
		{"parent path", http.MethodGet, "/api/object-mask/../private.txt", http.StatusNotFound},
		{"nested path", http.MethodGet, "/api/object-mask/nested/encoder.onnx", http.StatusNotFound},
		{"post", http.MethodPost, objectmask.EncoderURL, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
			if recorder.Code != tc.status {
				t.Fatalf("HTTP %d, want %d: %s", recorder.Code, tc.status, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "secret") {
				t.Fatal("private file exposed")
			}
			if tc.status == http.StatusOK && recorder.Body.String() != "local weights" {
				t.Fatalf("body: %q", recorder.Body.String())
			}
		})
	}
	if err := os.Symlink(filepath.Join(dataDir, "private.txt"), filepath.Join(directory, objectmask.DecoderName)); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, objectmask.DecoderURL, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("symlink HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
}
