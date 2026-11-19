package bodyshape

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApply_ShorterWithoutAnklesKeepsBottomFilled(t *testing.T) {
	t.Parallel()
	src := bsImage()
	pts := bsWithout(bsPoints(), 9, 10, 12, 13)
	out, err := Apply(src, pts, Params{Height: -7})
	require.NoError(t, err)
	last := bsH - 1
	same := 0
	for y := last - 1; y >= 0; y-- {
		equal := true
		for x := 0; x < bsW; x++ {
			if !bsSame(out.At(x, y), out.At(x, last)) {
				equal = false
				break
			}
		}
		if !equal {
			break
		}
		same++
	}
	require.Less(t, same, 3, "bottom rows repeat the last row")
}
