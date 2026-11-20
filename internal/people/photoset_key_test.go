package people

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhotoSet_KeyNormalized(t *testing.T) {
	t.Parallel()
	e := newSetEnv(t)
	e.mustAdd(t, "Анна Мария", 200)
	res := e.mustAdd(t, "  анна   мария ", 201)
	assert.Equal(t, "Анна Мария", res.Photo.PersonKey)
	assert.Equal(t, 2, e.count(t, "Анна Мария"))
}

func TestNormalizeSetKey(t *testing.T) {
	for in, want := range map[string]string{" a  b ": "a b", "": "", "  ": "", "Оля": "Оля"} {
		assert.Equal(t, want, NormalizeSetKey(in), in)
	}
}
