package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadPlan_RealBundledPresets_NoQwenNoHiDream(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "data", "presets"))
	require.NoError(t, err)

	svc := planService(t, emptyServer())
	svc.SetBundledPresets(os.DirFS(root))
	plan := loadPlan(t, svc)
	require.NotEmpty(t, plan.Items)
	for _, it := range plan.Items {
		low := strings.ToLower(it.Filename + " " + it.Model)
		assert.NotContains(t, low, "qwen_image", it)
		assert.NotContains(t, low, "qwen-image", it)
		assert.NotContains(t, low, "hidream", it)
		assert.NotContains(t, low, "mmproj", it)
	}
}

func TestDownloadPlan_QwenHiDreamNotRecommended(t *testing.T) {
	t.Parallel()
	assert.False(t, catalogModel(t, "qwen_image_2.1-Q8_0").Recommended)
	assert.False(t, catalogModel(t, "hidream_o1_image_dev_fp8_scaled").Recommended)
}
