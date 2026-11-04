package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/importexport"
)

func TestSaveProcessedImageOutputDir(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		subdir       string
		wantFilename string
	}{
		{
			name:         "plain filename saves without dialog",
			filename:     "export_20260101_000000.png",
			wantFilename: "export_20260101_000000.png",
		},
		{
			name:         "creates missing nested output dir",
			filename:     "out.png",
			subdir:       "batch/nested",
			wantFilename: "out.png",
		},
		{
			name:         "traversal filename flattened to base",
			filename:     "../../evil.png",
			wantFilename: "evil.png",
		},
		{
			name:         "absolute filename flattened to base",
			filename:     "/etc/passwd.png",
			wantFilename: "passwd.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{}
			outputDir := filepath.Join(t.TempDir(), tt.subdir)
			processed := &importexport.ProcessedImage{
				Data:     []byte("image-bytes"),
				Filename: tt.filename,
				Format:   "png",
			}

			path, err := app.saveProcessedImage(processed, outputDir)
			require.NoError(t, err)

			wantPath := filepath.Join(outputDir, tt.wantFilename)
			assert.Equal(t, wantPath, path)

			data, err := os.ReadFile(wantPath)
			require.NoError(t, err)
			assert.Equal(t, []byte("image-bytes"), data)
		})
	}
}

func hugePNGHeaderApp(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8
	ihdr[9] = 6

	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	lenb := make([]byte, 4)
	binary.BigEndian.PutUint32(lenb, uint32(len(ihdr)))
	out.Write(lenb)
	out.WriteString("IHDR")
	out.Write(ihdr)
	crcb := make([]byte, 4)
	binary.BigEndian.PutUint32(crcb, crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...)))
	out.Write(crcb)
	return out.Bytes()
}

func TestPreviewAdjustRejectsDecompressionBomb(t *testing.T) {
	app := &App{}
	b64 := base64.StdEncoding.EncodeToString(hugePNGHeaderApp(60000, 60000))

	_, err := app.PreviewAdjust(b64, AdjustParams{}, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")
}

func TestPreviewAdjustRejectsUnreadableHeader(t *testing.T) {
	app := &App{}
	b64 := base64.StdEncoding.EncodeToString([]byte("not an image at all"))

	_, err := app.PreviewAdjust(b64, AdjustParams{}, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")
}
