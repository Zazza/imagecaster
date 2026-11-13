package generation

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKleinLookModelIsStem(t *testing.T) {
	assert.Empty(t, filepath.Ext(kleinLookModel))
	assert.Equal(t, kleinLookModel, kleinRefSpec().Checkpoint)
}
