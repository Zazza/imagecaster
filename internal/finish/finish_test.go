package finish

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fnPx(img image.Image, x, y int) color.NRGBA {
	b := img.Bounds()
	return color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
}

func fnSize(img image.Image) (int, int) {
	return img.Bounds().Dx(), img.Bounds().Dy()
}

func fnSolid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func fnColumns(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(x / 256 * 60), B: 90, A: 255})
		}
	}
	return img
}

func fnRows(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 90, G: uint8(y % 256), B: uint8(y / 256 * 60), A: 255})
		}
	}
	return img
}

var (
	fnRed    = color.NRGBA{R: 220, G: 30, B: 30, A: 255}
	fnGreen  = color.NRGBA{R: 30, G: 200, B: 40, A: 255}
	fnBlue   = color.NRGBA{R: 30, G: 40, B: 210, A: 255}
	fnYellow = color.NRGBA{R: 230, G: 210, B: 20, A: 255}
	fnWhite  = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	fnBlack  = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	fnGray   = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
)

func fnQuadrants(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := fnRed
			switch {
			case x >= w/2 && y < h/2:
				c = fnGreen
			case x < w/2 && y >= h/2:
				c = fnBlue
			case x >= w/2 && y >= h/2:
				c = fnYellow
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func fnNear(a, b color.NRGBA, tol int) bool {
	d := func(x, y uint8) int { return int(math.Abs(float64(int(x) - int(y)))) }
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol && d(a.A, b.A) <= tol
}

func fnRowChanged(a, b image.Image, y int) bool {
	w, _ := fnSize(a)
	for x := 0; x < w; x++ {
		if fnPx(a, x, y) != fnPx(b, x, y) {
			return true
		}
	}
	return false
}

func fnChangedRows(a, b image.Image, y0, y1 int) int {
	n := 0
	for y := y0; y < y1; y++ {
		if fnRowChanged(a, b, y) {
			n++
		}
	}
	return n
}

func fnChangedExtent(a, b image.Image) (int, int) {
	_, h := fnSize(a)
	first, last := -1, -1
	for y := 0; y < h; y++ {
		if fnRowChanged(a, b, y) {
			if first < 0 {
				first = y
			}
			last = y
		}
	}
	return first, last
}

func fnApply(t *testing.T, img image.Image, p Params) image.Image {
	t.Helper()
	out, err := Apply(img, p)
	require.NoError(t, err)
	require.NotNil(t, out)
	return out
}

func fnOff(v float64) *float64 { return &v }

func TestRatios_MatchCard(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, []string{"1:1", "4:5", "3:4", "9:16", "16:9", "3:2"}, Ratios)
}

func TestApply_Crop4x5Offset(t *testing.T) {
	t.Parallel()
	src := fnColumns(1000, 1000)
	tests := []struct {
		name      string
		offset    *float64
		firstSrcX int
	}{
		{name: "offset 0 takes the left edge", offset: fnOff(0), firstSrcX: 0},
		{name: "offset 0.5 takes the center", offset: fnOff(0.5), firstSrcX: 100},
		{name: "offset 1 takes the right edge", offset: fnOff(1), firstSrcX: 200},
		{name: "no offset defaults to center", offset: nil, firstSrcX: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := fnApply(t, src, Params{Ratio: "4:5", Fit: "crop", Offset: tt.offset})
			w, h := fnSize(out)
			require.Equal(t, 800, w)
			require.Equal(t, 1000, h)
			for _, dx := range []int{0, 1, 400, 799} {
				assert.True(t, fnNear(fnPx(src, tt.firstSrcX+dx, 500), fnPx(out, dx, 500), 2),
					"out x=%d must be source x=%d: got %v want %v", dx, tt.firstSrcX+dx, fnPx(out, dx, 500), fnPx(src, tt.firstSrcX+dx, 500))
			}
		})
	}
}

func TestApply_CropVerticalOffset(t *testing.T) {
	t.Parallel()
	src := fnRows(1000, 1000)
	tests := []struct {
		name   string
		offset float64
	}{
		{name: "offset 0 takes the top", offset: 0},
		{name: "offset 1 takes the bottom", offset: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := fnApply(t, src, Params{Ratio: "16:9", Fit: "crop", Offset: fnOff(tt.offset)})
			w, h := fnSize(out)
			require.Equal(t, 1000, w)
			require.InDelta(t, 562.5, float64(h), 1)
			first := 0
			if tt.offset == 1 {
				first = 1000 - h
			}
			assert.True(t, fnNear(fnPx(src, 500, first), fnPx(out, 500, 0), 2))
			assert.True(t, fnNear(fnPx(src, 500, first+h-1), fnPx(out, 500, h-1), 2))
		})
	}
}

func TestApply_EveryRatioCrop(t *testing.T) {
	t.Parallel()
	for _, r := range Ratios {
		t.Run(r, func(t *testing.T) {
			t.Parallel()
			parts := strings.Split(r, ":")
			require.Len(t, parts, 2)
			a, err := strconv.ParseFloat(parts[0], 64)
			require.NoError(t, err)
			b, err := strconv.ParseFloat(parts[1], 64)
			require.NoError(t, err)
			out := fnApply(t, fnColumns(900, 700), Params{Ratio: r, Fit: "crop"})
			w, h := fnSize(out)
			assert.InDelta(t, a/b, float64(w)/float64(h), 0.01, "%dx%d", w, h)
			assert.True(t, w <= 900 && h <= 700, "crop must not enlarge: %dx%d", w, h)
			assert.True(t, w == 900 || h == 700, "crop must keep one full side: %dx%d", w, h)
		})
	}
}

func TestApply_Fit9x16WithMargins(t *testing.T) {
	t.Parallel()
	src := fnQuadrants(1000, 1000)
	tests := []struct {
		fit    string
		margin func(t *testing.T, c color.NRGBA)
	}{
		{fit: "white", margin: func(t *testing.T, c color.NRGBA) { assert.True(t, fnNear(c, fnWhite, 3), "white margin: %v", c) }},
		{fit: "black", margin: func(t *testing.T, c color.NRGBA) { assert.True(t, fnNear(c, fnBlack, 3), "black margin: %v", c) }},
		{fit: "blur", margin: func(t *testing.T, c color.NRGBA) {
			assert.Equal(t, uint8(255), c.A, "blur margin must not be transparent: %v", c)
			assert.False(t, fnNear(c, fnWhite, 10), "blur margin must not be empty white: %v", c)
			assert.False(t, fnNear(c, fnBlack, 10), "blur margin must not be empty black: %v", c)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.fit, func(t *testing.T) {
			t.Parallel()
			out := fnApply(t, src, Params{Ratio: "9:16", Fit: tt.fit})
			w, h := fnSize(out)
			require.InDelta(t, 9.0/16.0, float64(w)/float64(h), 0.01, "%dx%d", w, h)
			band := w
			top := (h - band) / 2
			pad := band / 20
			assert.True(t, fnNear(fnPx(out, pad, top+pad), fnRed, 12), "source top-left in place: %v", fnPx(out, pad, top+pad))
			assert.True(t, fnNear(fnPx(out, w-1-pad, top+pad), fnGreen, 12), "source top-right in place: %v", fnPx(out, w-1-pad, top+pad))
			assert.True(t, fnNear(fnPx(out, pad, top+band-1-pad), fnBlue, 12), "source bottom-left in place: %v", fnPx(out, pad, top+band-1-pad))
			assert.True(t, fnNear(fnPx(out, w-1-pad, top+band-1-pad), fnYellow, 12), "source bottom-right in place: %v", fnPx(out, w-1-pad, top+band-1-pad))
			for _, y := range []int{2, top / 2, top - pad, top + band + pad, h - 3} {
				for _, x := range []int{2, w / 4, w / 2, w - 3} {
					tt.margin(t, fnPx(out, x, y))
				}
			}
		})
	}

	t.Run("blur margins come from the photo", func(t *testing.T) {
		t.Parallel()
		out := fnApply(t, src, Params{Ratio: "9:16", Fit: "blur"})
		w, _ := fnSize(out)
		left, right := fnPx(out, w/8, 3), fnPx(out, w-1-w/8, 3)
		assert.NotEqual(t, left, right, "blurred margin must vary like the photo, not be one flat color")
	})
}

func TestApply_Fit16x9Horizontal(t *testing.T) {
	t.Parallel()
	src := fnQuadrants(1000, 1000)
	out := fnApply(t, src, Params{Ratio: "16:9", Fit: "black"})
	w, h := fnSize(out)
	require.InDelta(t, 16.0/9.0, float64(w)/float64(h), 0.01, "%dx%d", w, h)
	left := (w - h) / 2
	pad := h / 20
	assert.True(t, fnNear(fnPx(out, left+pad, pad), fnRed, 12), "%v", fnPx(out, left+pad, pad))
	assert.True(t, fnNear(fnPx(out, left+h-1-pad, h-1-pad), fnYellow, 12), "%v", fnPx(out, left+h-1-pad, h-1-pad))
	assert.True(t, fnNear(fnPx(out, 2, h/2), fnBlack, 3))
	assert.True(t, fnNear(fnPx(out, w-3, h/2), fnBlack, 3))
}

func fnUniformRows(img image.Image, fromTop bool, ok func(color.NRGBA) bool) int {
	w, h := fnSize(img)
	n := 0
	for i := 0; i < h; i++ {
		y := i
		if !fromTop {
			y = h - 1 - i
		}
		for x := 0; x < w; x++ {
			if !ok(fnPx(img, x, y)) {
				return n
			}
		}
		n++
	}
	return n
}

func fnUniformCols(img image.Image, fromLeft bool, ok func(color.NRGBA) bool) int {
	w, h := fnSize(img)
	n := 0
	for i := 0; i < w; i++ {
		x := i
		if !fromLeft {
			x = w - 1 - i
		}
		for y := 0; y < h; y++ {
			if !ok(fnPx(img, x, y)) {
				return n
			}
		}
		n++
	}
	return n
}

func TestApply_Frames(t *testing.T) {
	t.Parallel()
	dark := color.NRGBA{R: 60, G: 60, B: 60, A: 255}
	light := color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	isLight := func(c color.NRGBA) bool { return c.R >= 235 && c.G >= 235 && c.B >= 235 }
	isDark := func(c color.NRGBA) bool { return c.R <= 20 && c.G <= 20 && c.B <= 20 }
	tests := []struct {
		frame string
		src   color.NRGBA
		edge  func(color.NRGBA) bool
	}{
		{frame: "white", src: dark, edge: isLight},
		{frame: "black", src: light, edge: isDark},
		{frame: "polaroid", src: dark, edge: isLight},
	}
	for _, tt := range tests {
		t.Run(tt.frame, func(t *testing.T) {
			t.Parallel()
			out := fnApply(t, fnSolid(600, 400, tt.src), Params{Frame: tt.frame})
			w, h := fnSize(out)
			assert.Greater(t, w, 600)
			assert.Greater(t, h, 400)
			top := fnUniformRows(out, true, tt.edge)
			bottom := fnUniformRows(out, false, tt.edge)
			left := fnUniformCols(out, true, tt.edge)
			right := fnUniformCols(out, false, tt.edge)
			assert.Greater(t, top, 0, "top edge must be frame color")
			assert.Greater(t, bottom, 0, "bottom edge must be frame color")
			assert.Greater(t, left, 0, "left edge must be frame color")
			assert.Greater(t, right, 0, "right edge must be frame color")
			assert.True(t, fnNear(fnPx(out, w/2, top+(h-top-bottom)/2), tt.src, 3), "photo stays inside the frame")
			if tt.frame == "polaroid" {
				assert.Greater(t, bottom, top, "polaroid bottom must be wider than top")
			}
		})
	}
}

func TestApply_CaptionPositions(t *testing.T) {
	t.Parallel()
	src := fnSolid(600, 400, fnGray)
	tests := []struct {
		pos         string
		changedFrom int
		changedTo   int
		clean       [][2]int
	}{
		{pos: "bottom", changedFrom: 200, changedTo: 400, clean: [][2]int{{0, 200}}},
		{pos: "top", changedFrom: 0, changedTo: 200, clean: [][2]int{{200, 400}}},
		{pos: "center", changedFrom: 140, changedTo: 260, clean: [][2]int{{0, 60}, {340, 400}}},
	}
	for _, tt := range tests {
		t.Run(tt.pos, func(t *testing.T) {
			t.Parallel()
			out := fnApply(t, src, Params{Caption: "Hello", CaptionPos: tt.pos, CaptionSize: "s"})
			w, h := fnSize(out)
			require.Equal(t, 600, w)
			require.Equal(t, 400, h)
			assert.Greater(t, fnChangedRows(src, out, tt.changedFrom, tt.changedTo), 0, "caption must change its zone")
			for _, r := range tt.clean {
				assert.Zero(t, fnChangedRows(src, out, r[0], r[1]), "rows %d..%d must stay untouched", r[0], r[1])
			}
		})
	}
}

func TestApply_CaptionReadableOnAnyBackground(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]color.NRGBA{"white": fnWhite, "black": fnBlack} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := fnSolid(600, 400, c)
			out := fnApply(t, src, Params{Caption: "Hello", CaptionPos: "bottom", CaptionSize: "m"})
			assert.Greater(t, fnChangedRows(src, out, 0, 400), 0, "caption must be visible on %s", name)
		})
	}
}

func TestApply_CaptionCyrillicHasGlyphs(t *testing.T) {
	t.Parallel()
	src := fnSolid(600, 400, fnGray)
	render := func(text string) image.Image {
		return fnApply(t, src, Params{Caption: text, CaptionPos: "center", CaptionSize: "l"})
	}
	missing := render("")
	zhe := render("ЖЖЖ")
	be := render("БББ")
	assert.Greater(t, fnChangedRows(src, zhe, 0, 400), 0)
	assert.NotEqual(t, imgPix(missing), imgPix(zhe), "Ж must not render as the missing-glyph box")
	assert.NotEqual(t, imgPix(missing), imgPix(be), "Б must not render as the missing-glyph box")
	assert.NotEqual(t, imgPix(zhe), imgPix(be), "different Cyrillic letters must render differently")
}

func imgPix(img image.Image) []color.NRGBA {
	w, h := fnSize(img)
	out := make([]color.NRGBA, 0, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out = append(out, fnPx(img, x, y))
		}
	}
	return out
}

func TestApply_EmptyCaptionChangesNothing(t *testing.T) {
	t.Parallel()
	src := fnQuadrants(300, 200)
	out := fnApply(t, src, Params{Caption: "", CaptionPos: "bottom", CaptionSize: "l"})
	assert.Equal(t, imgPix(src), imgPix(out))
}

func TestApply_LongCaptionWraps(t *testing.T) {
	t.Parallel()
	src := fnSolid(300, 600, fnGray)
	short := fnApply(t, src, Params{Caption: "Word", CaptionPos: "center", CaptionSize: "m"})
	long := strings.TrimSpace(strings.Repeat("слово word ", 15))
	require.LessOrEqual(t, len([]rune(long)), CaptionMaxLen)
	wrapped := fnApply(t, src, Params{Caption: long, CaptionPos: "center", CaptionSize: "m"})
	w, h := fnSize(wrapped)
	assert.Equal(t, 300, w, "long text must not widen the image")
	assert.Equal(t, 600, h)
	s0, s1 := fnChangedExtent(src, short)
	l0, l1 := fnChangedExtent(src, wrapped)
	require.GreaterOrEqual(t, s0, 0)
	require.GreaterOrEqual(t, l0, 0)
	assert.Greater(t, l1-l0, 2*(s1-s0), "long text must wrap onto several lines")
}

func TestApply_PolaroidCaptionDarkOnBottomMargin(t *testing.T) {
	t.Parallel()
	src := fnSolid(600, 400, color.NRGBA{R: 60, G: 60, B: 60, A: 255})
	plain := fnApply(t, src, Params{Frame: "polaroid"})
	captioned := fnApply(t, src, Params{Frame: "polaroid", Caption: "Лето 2026", CaptionSize: "m"})
	require.Equal(t, plain.Bounds().Size(), captioned.Bounds().Size())
	isLight := func(c color.NRGBA) bool { return c.R >= 235 && c.G >= 235 && c.B >= 235 }
	_, h := fnSize(plain)
	bottom := fnUniformRows(plain, false, isLight)
	photoEnd := h - bottom
	assert.Zero(t, fnChangedRows(plain, captioned, 0, photoEnd), "caption must not touch the photo or top margin")
	assert.Greater(t, fnChangedRows(plain, captioned, photoEnd, h), 0, "caption must be on the bottom margin")
	w, _ := fnSize(plain)
	darker := false
	for y := photoEnd; y < h && !darker; y++ {
		for x := 0; x < w; x++ {
			a, b := fnPx(plain, x, y), fnPx(captioned, x, y)
			if int(b.R)+int(b.G)+int(b.B) < int(a.R)+int(a.G)+int(a.B)-150 {
				darker = true
				break
			}
		}
	}
	assert.True(t, darker, "polaroid caption must be dark text on the light margin")
}

func TestApply_NeutralIsUnchanged(t *testing.T) {
	t.Parallel()
	src := fnQuadrants(320, 240)
	p := Params{}
	assert.True(t, p.IsNeutral())
	require.NoError(t, p.Validate())
	out := fnApply(t, src, p)
	require.Equal(t, src.Bounds().Size(), out.Bounds().Size())
	if n, ok := out.(*image.NRGBA); ok {
		assert.Equal(t, src.Pix, n.Pix)
	}
	assert.Equal(t, imgPix(src), imgPix(out))
}

func TestParams_IsNeutral(t *testing.T) {
	t.Parallel()
	assert.False(t, Params{Ratio: "1:1", Fit: "crop"}.IsNeutral())
	assert.False(t, Params{Frame: "white"}.IsNeutral())
	assert.False(t, Params{Caption: "hi"}.IsNeutral())
}

func TestParams_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		p       Params
		wantErr bool
	}{
		{name: "empty", p: Params{}},
		{name: "crop 4:5", p: Params{Ratio: "4:5", Fit: "crop", Offset: fnOff(0.3)}},
		{name: "offset 0", p: Params{Ratio: "4:5", Fit: "crop", Offset: fnOff(0)}},
		{name: "offset 1", p: Params{Ratio: "4:5", Fit: "crop", Offset: fnOff(1)}},
		{name: "fit blur", p: Params{Ratio: "9:16", Fit: "blur"}},
		{name: "frame polaroid", p: Params{Frame: "polaroid"}},
		{name: "caption full", p: Params{Caption: "Привет", CaptionPos: "top", CaptionSize: "l"}},
		{name: "caption 200 cyrillic chars", p: Params{Caption: strings.Repeat("ж", CaptionMaxLen)}},
		{name: "bad ratio", p: Params{Ratio: "2:1", Fit: "crop"}, wantErr: true},
		{name: "bad fit", p: Params{Ratio: "1:1", Fit: "zoom"}, wantErr: true},
		{name: "bad frame", p: Params{Frame: "gold"}, wantErr: true},
		{name: "offset below 0", p: Params{Ratio: "4:5", Fit: "crop", Offset: fnOff(-0.1)}, wantErr: true},
		{name: "offset above 1", p: Params{Ratio: "4:5", Fit: "crop", Offset: fnOff(1.1)}, wantErr: true},
		{name: "caption too long", p: Params{Caption: strings.Repeat("a", CaptionMaxLen+1)}, wantErr: true},
		{name: "bad caption pos", p: Params{Caption: "hi", CaptionPos: "left"}, wantErr: true},
		{name: "bad caption size", p: Params{Caption: "hi", CaptionSize: "xl"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.p.Validate()
			_, applyErr := Apply(fnSolid(100, 100, fnGray), tt.p)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Error(t, applyErr, "Apply must refuse invalid params too")
				return
			}
			assert.NoError(t, err)
			assert.NoError(t, applyErr)
		})
	}
}

func fnNonWhiteRows(img image.Image, x int) int {
	_, h := fnSize(img)
	n := 0
	for y := 0; y < h; y++ {
		if !fnNear(fnPx(img, x, y), fnWhite, 3) {
			n++
		}
	}
	return n
}

func fnNonWhiteCols(img image.Image, y int) int {
	w, _ := fnSize(img)
	n := 0
	for x := 0; x < w; x++ {
		if !fnNear(fnPx(img, x, y), fnWhite, 3) {
			n++
		}
	}
	return n
}

func TestApply_FitHugeSourceCanvasCapped(t *testing.T) {
	t.Parallel()

	t.Run("9:16 from 16000x1000: sides capped, ratio kept, source whole and centered", func(t *testing.T) {
		t.Parallel()
		out := fnApply(t, fnQuadrants(16000, 1000), Params{Ratio: "9:16", Fit: "white"})
		w, h := fnSize(out)
		require.LessOrEqual(t, w, MaxSide, "%dx%d", w, h)
		require.LessOrEqual(t, h, MaxSide, "%dx%d", w, h)
		assert.InDelta(t, float64(h)*9/16, float64(w), 1, "ratio 9:16 within 1px: %dx%d", w, h)
		assert.Greater(t, w, 1000, "canvas must not collapse: %dx%d", w, h)

		band := float64(w) * 1000 / 16000
		assert.InDelta(t, band, float64(fnNonWhiteRows(out, w/4)), 3, "whole source height (scaled) must be visible, left half")
		assert.InDelta(t, band, float64(fnNonWhiteRows(out, w*3/4)), 3, "whole source height (scaled) must be visible, right half")

		top := (float64(h) - band) / 2
		yTop := int(top + band*0.25)
		yBot := int(top + band*0.75)
		padX := w / 20
		assert.True(t, fnNear(fnPx(out, padX, yTop), fnRed, 12), "top-left of source: %v", fnPx(out, padX, yTop))
		assert.True(t, fnNear(fnPx(out, w-1-padX, yTop), fnGreen, 12), "top-right of source: %v", fnPx(out, w-1-padX, yTop))
		assert.True(t, fnNear(fnPx(out, padX, yBot), fnBlue, 12), "bottom-left of source: %v", fnPx(out, padX, yBot))
		assert.True(t, fnNear(fnPx(out, w-1-padX, yBot), fnYellow, 12), "bottom-right of source: %v", fnPx(out, w-1-padX, yBot))
		for _, y := range []int{2, int(top) / 2, h - 1 - int(top)/2, h - 3} {
			assert.True(t, fnNear(fnPx(out, w/2, y), fnWhite, 3), "margin at y=%d must be white: %v", y, fnPx(out, w/2, y))
		}
	})

	t.Run("16:9 from 1000x16000: sides capped, ratio kept, source whole and centered", func(t *testing.T) {
		t.Parallel()
		out := fnApply(t, fnQuadrants(1000, 16000), Params{Ratio: "16:9", Fit: "white"})
		w, h := fnSize(out)
		require.LessOrEqual(t, w, MaxSide, "%dx%d", w, h)
		require.LessOrEqual(t, h, MaxSide, "%dx%d", w, h)
		assert.InDelta(t, float64(w)*9/16, float64(h), 1, "ratio 16:9 within 1px: %dx%d", w, h)
		assert.Greater(t, h, 1000, "canvas must not collapse: %dx%d", w, h)

		band := float64(h) * 1000 / 16000
		assert.InDelta(t, band, float64(fnNonWhiteCols(out, h/4)), 3, "whole source width (scaled) must be visible, top half")
		assert.InDelta(t, band, float64(fnNonWhiteCols(out, h*3/4)), 3, "whole source width (scaled) must be visible, bottom half")

		left := (float64(w) - band) / 2
		xL := int(left + band*0.25)
		xR := int(left + band*0.75)
		padY := h / 20
		assert.True(t, fnNear(fnPx(out, xL, padY), fnRed, 12), "%v", fnPx(out, xL, padY))
		assert.True(t, fnNear(fnPx(out, xR, padY), fnGreen, 12), "%v", fnPx(out, xR, padY))
		assert.True(t, fnNear(fnPx(out, xL, h-1-padY), fnBlue, 12), "%v", fnPx(out, xL, h-1-padY))
		assert.True(t, fnNear(fnPx(out, xR, h-1-padY), fnYellow, 12), "%v", fnPx(out, xR, h-1-padY))
		assert.True(t, fnNear(fnPx(out, 2, h/2), fnWhite, 3))
		assert.True(t, fnNear(fnPx(out, w-3, h/2), fnWhite, 3))
	})

	for _, fit := range []string{"black", "blur"} {
		t.Run("9:16 from 16000x1000 fit "+fit+": sides capped", func(t *testing.T) {
			t.Parallel()
			out := fnApply(t, fnQuadrants(16000, 1000), Params{Ratio: "9:16", Fit: fit})
			w, h := fnSize(out)
			assert.LessOrEqual(t, w, MaxSide, "%dx%d", w, h)
			assert.LessOrEqual(t, h, MaxSide, "%dx%d", w, h)
			assert.InDelta(t, float64(h)*9/16, float64(w), 1, "%dx%d", w, h)
		})
	}

	t.Run("fit that already fits is not shrunk", func(t *testing.T) {
		t.Parallel()
		out := fnApply(t, fnQuadrants(4000, 1000), Params{Ratio: "9:16", Fit: "white"})
		w, h := fnSize(out)
		assert.Equal(t, 4000, w, "source width kept when canvas fits under the cap: %dx%d", w, h)
		assert.InDelta(t, 4000.0*16/9, float64(h), 1, "%dx%d", w, h)
	})
}
