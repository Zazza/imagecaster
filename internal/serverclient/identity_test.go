package serverclient

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadIdentity_Multipart(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"person":{"name":"anna","base":"Z-Image","identity":true}}`)
	paths, contents := trPhotos(t, 6)

	p, err := c.UploadIdentity(context.Background(), "anna", paths)

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/api/train/people/anna/identity", req.path)
	mp := trParseMultipart(t, req)
	assert.Empty(t, mp.other)
	assert.Empty(t, mp.fields)
	require.Len(t, mp.order, 6)
	for name, data := range contents {
		assert.Equal(t, data, mp.files[name], name)
	}
	require.NotNil(t, p)
	assert.Equal(t, "anna", p.Name)
	assert.True(t, p.Identity)
}

func TestUploadIdentity_APIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "too_few_faces"},
		{http.StatusServiceUnavailable, "face_models_missing"},
		{http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, tt.status, `{"error":{"code":"`+tt.code+`","message":"x"}}`)
			paths, _ := trPhotos(t, 5)
			_, err := c.UploadIdentity(context.Background(), "anna", paths)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "got %v", err)
			assert.Equal(t, tt.code, apiErr.Code)
			assert.Equal(t, tt.status, apiErr.Status)
		})
	}
}

func TestPeople_IdentityFlag(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, `{"people":[{"name":"anna","identity":true},{"name":"bob","identity":false},{"name":"old"}]}`)
	list, err := c.People(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.True(t, list[0].Identity)
	assert.False(t, list[1].Identity)
	assert.False(t, list[2].Identity)
}

func TestCatalog_FaceSwapSection(t *testing.T) {
	t.Parallel()
	cat, err := LoadCatalog()
	require.NoError(t, err)
	fs := cat.FaceSwap
	assert.Contains(t, strings.ToLower(fs.Note), "non-commercial")
	assert.Contains(t, fs.NoteRu, "некоммерческ")
	want := []string{"det_10g.onnx", "1k3d68.onnx", "w600k_r50.onnx", "inswapper_128.onnx", "bisenet_resnet_34.onnx", "gfpgan_1.4.onnx"}
	require.Len(t, fs.Files, len(want))
	hexRe := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for i, f := range fs.Files {
		assert.Equal(t, want[i], f.Filename)
		assert.Equal(t, "face", f.Type, f.Filename)
		assert.True(t, strings.HasPrefix(f.URL, "https://huggingface.co/"), f.URL)
		assert.True(t, strings.HasSuffix(f.URL, "/"+f.Filename), f.URL)
		assert.Regexp(t, hexRe, f.SHA256, f.Filename)
		assert.Greater(t, f.SizeGB, 0.0, f.Filename)
	}
}

func TestCatalog_FaceSwapIsIndependentCopy(t *testing.T) {
	t.Parallel()
	first, err := LoadCatalog()
	require.NoError(t, err)
	first.FaceSwap.Files[0].Filename = "changed"
	second, err := LoadCatalog()
	require.NoError(t, err)
	assert.Equal(t, "det_10g.onnx", second.FaceSwap.Files[0].Filename)
}
