package maskcomp

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func colorDist(a, b color.RGBA) int {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) + d(a.G, b.G) + d(a.B, b.B)
}

func TestKeepColor_LargeImageFastAndKeepsColor(t *testing.T) {
	const w, h = 3000, 4000
	blue := color.RGBA{30, 40, 220, 255}
	red := color.RGBA{220, 30, 40, 255}
	orig := solidRGBA(w, h, blue)
	fillRect(orig, image.Rect(w/2, 0, w, h), red)

	yellowA := color.RGBA{240, 220, 20, 255}
	yellowB := color.RGBA{160, 140, 0, 255}
	edited := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if ((x/2)+(y/2))%2 == 0 {
				edited.SetRGBA(x, y, yellowA)
			} else {
				edited.SetRGBA(x, y, yellowB)
			}
		}
	}

	start := time.Now()
	out := KeepColor(orig, edited, 0.02)
	elapsed := time.Since(start)

	require.NotNil(t, out)
	assert.Equal(t, orig.Bounds().Size(), out.Bounds().Size())
	assert.Less(t, elapsed, 3*time.Second)

	for _, c := range []struct {
		x, y int
		want color.RGBA
		not  color.RGBA
	}{
		{w / 4, h / 2, blue, red},
		{3 * w / 4, h / 2, red, blue},
	} {
		var sr, sg, sb, n int
		for y := c.y - 8; y < c.y+8; y++ {
			for x := c.x - 8; x < c.x+8; x++ {
				p := out.RGBAAt(x, y)
				sr += int(p.R)
				sg += int(p.G)
				sb += int(p.B)
				n++
			}
		}
		avg := color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}
		assert.Less(t, colorDist(avg, c.want), 90, "avg %v want %v", avg, c.want)
		assert.Less(t, colorDist(avg, c.want), colorDist(avg, c.not))
		assert.Less(t, colorDist(avg, c.want), colorDist(avg, yellowA))

		maxNeighbour := 0
		for y := c.y - 8; y < c.y+8; y++ {
			for x := c.x - 8; x < c.x+8; x++ {
				if d := colorDist(out.RGBAAt(x, y), out.RGBAAt(x+2, y)); d > maxNeighbour {
					maxNeighbour = d
				}
			}
		}
		assert.Greater(t, maxNeighbour, 30, "checkerboard not visible at %d,%d", c.x, c.y)
	}
}
