package generation

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

var repKleinColor = color.NRGBA{R: 200, G: 30, B: 40, A: 255}

type repRec struct {
	mu      sync.Mutex
	ops     []string
	reqs    []sd.Img2ImgRequest
	models  []string
	events  []map[string]int
	img2img func(n int, req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error)
	option  func(key string, value any) error
}

func (r *repRec) img2imgCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *repRec) referenceOps() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, op := range r.ops {
		if op == "img2img" || strings.HasPrefix(op, "option:"+kleinReferenceOption+"=") {
			out = append(out, op)
		}
	}
	return out
}

type repEmitter struct {
	rec *repRec
}

func (e *repEmitter) Emit(event string, data ...any) {
	if event != "repair:progress" {
		return
	}
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	if len(data) == 0 {
		e.rec.ops = append(e.rec.ops, "progress:nopayload")
		return
	}
	m, ok := data[0].(map[string]int)
	if !ok {
		e.rec.ops = append(e.rec.ops, fmt.Sprintf("progress:badpayload:%T", data[0]))
		return
	}
	e.rec.events = append(e.rec.events, m)
	e.rec.ops = append(e.rec.ops, fmt.Sprintf("progress:%d/%d", m["done"], m["total"]))
}

func repSolidPNG(t *testing.T, w, h int, c color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return objTestPNGBase64(t, img)
}

func repSourcePixel(x, y int) color.NRGBA {
	return color.NRGBA{R: uint8(x % 251), G: uint8(y % 241), B: uint8((x*7 + y*13) % 256), A: 255}
}

func repSourceImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, repSourcePixel(x, y))
		}
	}
	return img
}

func repSource(t *testing.T, w, h int) string {
	t.Helper()
	return objTestPNGBase64(t, repSourceImage(w, h))
}

func repMask(t *testing.T, w, h int, rects ...image.Rectangle) string {
	t.Helper()
	m := image.NewGray(image.Rect(0, 0, w, h))
	for _, r := range rects {
		r = r.Intersect(m.Bounds())
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				m.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return objTestPNGBase64(t, m)
}

func repNRGBA(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func repNear(a, b color.NRGBA, tol int) bool {
	d := func(p, q uint8) bool {
		v := int(p) - int(q)
		if v < 0 {
			v = -v
		}
		return v <= tol
	}
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

func repSetup(t *testing.T) (*Service, *repRec, *mockSessions) {
	t.Helper()
	rec := &repRec{}
	sdSvc := &mockSD{}
	sdSvc.setOption = func(key string, value any) error {
		rec.mu.Lock()
		rec.ops = append(rec.ops, fmt.Sprintf("option:%s=%v", key, value))
		fn := rec.option
		rec.mu.Unlock()
		if fn != nil {
			return fn(key, value)
		}
		return nil
	}
	sdSvc.setModelModules = func(modelName string, modules []string) error {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.models = append(rec.models, modelName)
		return nil
	}
	sdSvc.setModel = func(modelName string) error {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.models = append(rec.models, modelName)
		return nil
	}
	sdSvc.img2img = func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		rec.mu.Lock()
		rec.ops = append(rec.ops, "img2img")
		rec.reqs = append(rec.reqs, req)
		n := len(rec.reqs)
		fn := rec.img2img
		rec.mu.Unlock()
		if fn != nil {
			return fn(n, req)
		}
		info := fmt.Sprintf(`{"seed":42,"width":%d,"height":%d}`, req.Width, req.Height)
		return &sd.Txt2ImgResponse{
			Images: []string{repSolidPNG(t, req.Width, req.Height, repKleinColor)},
			Info:   json.RawMessage(info),
		}, nil
	}
	db := openTestDB(t)
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	sessions := &mockSessions{}
	svc.sessions = sessions
	svc.emitter = &repEmitter{rec: rec}
	return svc, rec, sessions
}

func repWide3(t *testing.T) (string, int, int) {
	t.Helper()
	return repSource(t, 2304, 768), 2304, 768
}

func TestRepairImage_Validation(t *testing.T) {
	t.Parallel()

	img := repSource(t, 256, 256)
	mask := repMask(t, 256, 256, image.Rect(100, 100, 140, 140))
	blackMask := repMask(t, 256, 256)
	transparentMask := objTestPNGBase64(t, image.NewNRGBA(image.Rect(0, 0, 256, 256)))
	tooWide := repSolidPNG(t, 8193, 4, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	tooWideMask := repMask(t, 8193, 4, image.Rect(10, 0, 20, 4))
	tooTall := repSolidPNG(t, 4, 8193, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	tooTallMask := repMask(t, 4, 8193, image.Rect(0, 10, 4, 20))

	tests := []struct {
		name       string
		params     RepairParams
		errContain string
	}{
		{name: "empty image", params: RepairParams{ImageBase64: "", MaskBase64: mask}},
		{name: "empty mask", params: RepairParams{ImageBase64: img, MaskBase64: ""}},
		{name: "image over 128 MB", params: RepairParams{ImageBase64: strings.Repeat("x", MaxImageBase64Len+1), MaskBase64: mask}, errContain: "128 MB"},
		{name: "mask over 128 MB", params: RepairParams{ImageBase64: img, MaskBase64: strings.Repeat("x", MaxImageBase64Len+1)}, errContain: "128 MB"},
		{name: "image not an image", params: RepairParams{ImageBase64: base64.StdEncoding.EncodeToString([]byte("not a png")), MaskBase64: mask}},
		{name: "mask not an image", params: RepairParams{ImageBase64: img, MaskBase64: base64.StdEncoding.EncodeToString([]byte("not a png"))}},
		{name: "mask all black", params: RepairParams{ImageBase64: img, MaskBase64: blackMask}, errContain: "закрасьте"},
		{name: "mask all transparent", params: RepairParams{ImageBase64: img, MaskBase64: transparentMask}, errContain: "закрасьте"},
		{name: "strength below 0.3", params: RepairParams{ImageBase64: img, MaskBase64: mask, Strength: 0.29}},
		{name: "strength above 0.9", params: RepairParams{ImageBase64: img, MaskBase64: mask, Strength: 0.91}},
		{name: "strength 1.0", params: RepairParams{ImageBase64: img, MaskBase64: mask, Strength: 1.0}},
		{name: "strength negative", params: RepairParams{ImageBase64: img, MaskBase64: mask, Strength: -0.5}},
		{name: "width over 8192", params: RepairParams{ImageBase64: tooWide, MaskBase64: tooWideMask}},
		{name: "height over 8192", params: RepairParams{ImageBase64: tooTall, MaskBase64: tooTallMask}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, sessions := repSetup(t)

			result, err := svc.RepairImage(tt.params)

			require.Error(t, err)
			assert.Nil(t, result)
			if tt.errContain != "" {
				assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.errContain))
			}
			assert.Equal(t, 0, rec.img2imgCount())
			sessions.mu.Lock()
			assert.Empty(t, sessions.items)
			sessions.mu.Unlock()
		})
	}
}

func TestRepairImage_StrengthAccepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		strength float64
		want     float64
	}{
		{name: "zero means default 0.8", strength: 0, want: 0.8},
		{name: "lower bound 0.3 inclusive", strength: 0.3, want: 0.3},
		{name: "upper bound 0.9 inclusive", strength: 0.9, want: 0.9},
		{name: "middle 0.5", strength: 0.5, want: 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := repSetup(t)

			_, err := svc.RepairImage(RepairParams{
				ImageBase64: repSource(t, 256, 256),
				MaskBase64:  repMask(t, 256, 256, image.Rect(100, 100, 140, 140)),
				Strength:    tt.strength,
			})

			require.NoError(t, err)
			rec.mu.Lock()
			defer rec.mu.Unlock()
			require.Len(t, rec.reqs, 1)
			require.NotNil(t, rec.reqs[0].DenoisingStrength)
			assert.InDelta(t, tt.want, *rec.reqs[0].DenoisingStrength, 1e-9)
		})
	}
}

func TestRepairImage_MaxSideInclusive(t *testing.T) {
	t.Parallel()
	svc, rec, _ := repSetup(t)

	result, err := svc.RepairImage(RepairParams{
		ImageBase64: repSolidPNG(t, 8192, 32, color.NRGBA{R: 9, G: 9, B: 9, A: 255}),
		MaskBase64:  repMask(t, 8192, 32, image.Rect(100, 8, 120, 24)),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, rec.img2imgCount())
	img := objTestDecodeImage(t, result.Image)
	assert.Equal(t, 8192, img.Bounds().Dx())
	assert.Equal(t, 32, img.Bounds().Dy())
}

func TestRepairImage_TilesOnlyWhereMaskIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rects     []image.Rectangle
		wantTiles int
	}{
		{name: "mask in first core only", rects: []image.Rectangle{image.Rect(300, 300, 400, 400)}, wantTiles: 1},
		{name: "mask in middle core only", rects: []image.Rectangle{image.Rect(1100, 300, 1200, 400)}, wantTiles: 1},
		{name: "mask in first and last cores, middle skipped", rects: []image.Rectangle{image.Rect(300, 300, 400, 400), image.Rect(1900, 300, 2000, 400)}, wantTiles: 2},
		{name: "mask in every core", rects: []image.Rectangle{image.Rect(300, 300, 400, 400), image.Rect(1100, 300, 1200, 400), image.Rect(1900, 300, 2000, 400)}, wantTiles: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := repSetup(t)
			src, w, h := repWide3(t)

			result, err := svc.RepairImage(RepairParams{ImageBase64: src, MaskBase64: repMask(t, w, h, tt.rects...)})

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantTiles, rec.img2imgCount())
			rec.mu.Lock()
			defer rec.mu.Unlock()
			for i, req := range rec.reqs {
				assert.LessOrEqual(t, req.Width, 1024, "tile %d width", i)
				assert.LessOrEqual(t, req.Height, 1024, "tile %d height", i)
				assert.Equal(t, 0, req.Width%16, "tile %d width multiple of 16", i)
				assert.Equal(t, 0, req.Height%16, "tile %d height multiple of 16", i)
			}
		})
	}
}

func TestRepairImage_CornerMaskGivesOneTileShiftedInside(t *testing.T) {
	t.Parallel()

	const w, h = 2000, 1500
	tests := []struct {
		name    string
		rect    image.Rectangle
		originX int
		originY int
	}{
		{name: "top-left corner", rect: image.Rect(0, 0, 20, 20), originX: 0, originY: 0},
		{name: "bottom-right corner", rect: image.Rect(w-20, h-20, w, h), originX: w - 1024, originY: h - 1024},
		{name: "top-right corner", rect: image.Rect(w-20, 0, w, 20), originX: w - 1024, originY: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := repSetup(t)

			result, err := svc.RepairImage(RepairParams{ImageBase64: repSource(t, w, h), MaskBase64: repMask(t, w, h, tt.rect)})

			require.NoError(t, err)
			require.NotNil(t, result)
			rec.mu.Lock()
			defer rec.mu.Unlock()
			require.Len(t, rec.reqs, 1)
			req := rec.reqs[0]
			assert.Equal(t, 1024, req.Width)
			assert.Equal(t, 1024, req.Height)
			require.NotEmpty(t, req.InitImages)
			tile := objTestDecodeImage(t, req.InitImages[0])
			require.Equal(t, 1024, tile.Bounds().Dx())
			require.Equal(t, 1024, tile.Bounds().Dy())
			for _, p := range [][2]int{{0, 0}, {1023, 0}, {0, 1023}, {1023, 1023}, {512, 512}, {100, 900}} {
				got := repNRGBA(tile, tile.Bounds().Min.X+p[0], tile.Bounds().Min.Y+p[1])
				want := repSourcePixel(tt.originX+p[0], tt.originY+p[1])
				assert.Equal(t, want, got, "tile pixel %v must be source pixel at tile origin (%d,%d) + offset", p, tt.originX, tt.originY)
			}
		})
	}
}

func TestRepairImage_SmallImageSingleUpscaledTile(t *testing.T) {
	t.Parallel()
	svc, rec, _ := repSetup(t)

	result, err := svc.RepairImage(RepairParams{
		ImageBase64: repSource(t, 600, 400),
		MaskBase64:  repMask(t, 600, 400, image.Rect(250, 150, 350, 250)),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	rec.mu.Lock()
	require.Len(t, rec.reqs, 1)
	assert.Equal(t, 1024, rec.reqs[0].Width)
	assert.Equal(t, 672, rec.reqs[0].Height)
	require.NotEmpty(t, rec.reqs[0].InitImages)
	rec.mu.Unlock()

	img := objTestDecodeImage(t, result.Image)
	assert.Equal(t, 600, img.Bounds().Dx())
	assert.Equal(t, 400, img.Bounds().Dy())
}

func repAssertMaskComposite(t *testing.T, result *GenerateImageResult, w, h int, rects []image.Rectangle, slack int) {
	t.Helper()
	img := objTestDecodeImage(t, result.Image)
	require.Equal(t, w, img.Bounds().Dx())
	require.Equal(t, h, img.Bounds().Dy())
	ox, oy := img.Bounds().Min.X, img.Bounds().Min.Y

	const feather = 4
	nearMask := func(x, y int) bool {
		for _, r := range rects {
			if r.Inset(-(feather + slack)).Intersect(image.Rect(0, 0, w, h)).Overlaps(image.Rect(x, y, x+1, y+1)) {
				return true
			}
		}
		return false
	}
	deepInside := func(x, y int) bool {
		for _, r := range rects {
			if (image.Point{X: x, Y: y}).In(r.Inset(feather + 1)) {
				return true
			}
		}
		return false
	}

	outsideBad, insideBad, insideChecked := 0, 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			got := repNRGBA(img, ox+x, oy+y)
			if !nearMask(x, y) {
				if got != repSourcePixel(x, y) {
					outsideBad++
				}
				continue
			}
			if deepInside(x, y) {
				insideChecked++
				if !repNear(got, repKleinColor, 3) {
					insideBad++
				}
			}
		}
	}
	assert.Equal(t, 0, outsideBad, "pixels farther than %d px from the mask must equal the source byte for byte", feather)
	assert.Greater(t, insideChecked, 0)
	assert.Equal(t, 0, insideBad, "pixels inside the mask must come from the Klein result")
}

func TestRepairImage_OutsideMaskUntouchedInsideFromKlein(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		w, h  int
		rects []image.Rectangle
	}{
		{name: "small image 600x400 upscaled tile", w: 600, h: 400, rects: []image.Rectangle{image.Rect(250, 150, 350, 250)}},
		{name: "wide image two tiles", w: 2304, h: 768, rects: []image.Rectangle{image.Rect(300, 300, 400, 400), image.Rect(1900, 300, 2000, 400)}},
		{name: "mask crossing a core border", w: 2304, h: 768, rects: []image.Rectangle{image.Rect(700, 200, 840, 260)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := repSetup(t)

			result, err := svc.RepairImage(RepairParams{ImageBase64: repSource(t, tt.w, tt.h), MaskBase64: repMask(t, tt.w, tt.h, tt.rects...)})

			require.NoError(t, err)
			require.NotNil(t, result)
			repAssertMaskComposite(t, result, tt.w, tt.h, tt.rects, 0)
		})
	}
}

func TestRepairImage_MaskOfOtherSizeIsScaledToImage(t *testing.T) {
	t.Parallel()
	svc, _, _ := repSetup(t)

	result, err := svc.RepairImage(RepairParams{
		ImageBase64: repSource(t, 600, 400),
		MaskBase64:  repMask(t, 300, 200, image.Rect(100, 50, 150, 100)),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	repAssertMaskComposite(t, result, 600, 400, []image.Rectangle{image.Rect(200, 100, 300, 200)}, 2)
}

func TestRepairImage_KleinRequest(t *testing.T) {
	t.Parallel()

	seed := int64(777)
	tests := []struct {
		name     string
		seed     *int64
		wantSeed *int64
	}{
		{name: "explicit seed used on every tile", seed: &seed, wantSeed: &seed},
		{name: "no seed: one generated seed shared by every tile", seed: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := repSetup(t)
			src, w, h := repWide3(t)

			_, err := svc.RepairImage(RepairParams{
				ImageBase64: src,
				MaskBase64:  repMask(t, w, h, image.Rect(300, 300, 400, 400), image.Rect(1100, 300, 1200, 400), image.Rect(1900, 300, 2000, 400)),
				Seed:        tt.seed,
			})
			require.NoError(t, err)

			rec.mu.Lock()
			defer rec.mu.Unlock()
			require.Len(t, rec.reqs, 3)
			first := rec.reqs[0]
			require.NotNil(t, first.Seed)
			assert.NotEqual(t, int64(-1), *first.Seed)
			if tt.wantSeed != nil {
				assert.Equal(t, *tt.wantSeed, *first.Seed)
			}
			lower := strings.ToLower(first.Prompt)
			assert.True(t, strings.Contains(lower, "scratch") || strings.Contains(lower, "царапин"), "prompt must ask to remove scratches: %q", first.Prompt)
			for i, req := range rec.reqs {
				assert.Equal(t, 4, req.Steps, "tile %d", i)
				assert.InDelta(t, 1.0, req.CfgScale, 1e-9, "tile %d", i)
				assert.Equal(t, "euler", req.SamplerName, "tile %d", i)
				assert.Equal(t, "simple", req.Scheduler, "tile %d", i)
				require.NotNil(t, req.Seed, "tile %d", i)
				assert.Equal(t, *first.Seed, *req.Seed, "tile %d seed", i)
				assert.Equal(t, first.Prompt, req.Prompt, "tile %d prompt", i)
				require.NotEmpty(t, req.InitImages, "tile %d", i)
				tile := objTestDecodeImage(t, req.InitImages[0])
				assert.Positive(t, tile.Bounds().Dx())
			}
			require.NotEmpty(t, rec.models)
			assert.Equal(t, kleinLookModel, rec.models[len(rec.models)-1])
		})
	}
}

func TestRepairImage_ReferenceToggled(t *testing.T) {
	t.Parallel()

	sdErrOnSecond := func(n int, req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		if n == 2 {
			return nil, fmt.Errorf("sd boom")
		}
		return &sd.Txt2ImgResponse{Images: []string{repSolidPNG(t, req.Width, req.Height, repKleinColor)}}, nil
	}
	tests := []struct {
		name    string
		img2img func(n int, req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error)
		wantErr bool
		wantOps []string
	}{
		{
			name:    "success on three tiles",
			wantOps: []string{"option:" + kleinReferenceOption + "=true", "img2img", "option:" + kleinReferenceOption + "=false", "option:" + kleinReferenceOption + "=true", "img2img", "option:" + kleinReferenceOption + "=false", "option:" + kleinReferenceOption + "=true", "img2img", "option:" + kleinReferenceOption + "=false"},
		},
		{
			name:    "sd fails on second tile of three",
			img2img: sdErrOnSecond,
			wantErr: true,
			wantOps: []string{"option:" + kleinReferenceOption + "=true", "img2img", "option:" + kleinReferenceOption + "=false", "option:" + kleinReferenceOption + "=true", "img2img", "option:" + kleinReferenceOption + "=false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, sessions := repSetup(t)
			rec.img2img = tt.img2img
			src, w, h := repWide3(t)

			result, err := svc.RepairImage(RepairParams{
				ImageBase64: src,
				MaskBase64:  repMask(t, w, h, image.Rect(300, 300, 400, 400), image.Rect(1100, 300, 1200, 400), image.Rect(1900, 300, 2000, 400)),
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result, "no partial result on error")
				sessions.mu.Lock()
				assert.Empty(t, sessions.items)
				sessions.mu.Unlock()
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
			assert.Equal(t, tt.wantOps, rec.referenceOps())
		})
	}
}

func TestRepairImage_InterruptStopsBetweenTiles(t *testing.T) {
	t.Parallel()
	svc, rec, sessions := repSetup(t)
	rec.img2img = func(n int, req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		if n == 1 {
			require.NoError(t, svc.InterruptGeneration())
		}
		return &sd.Txt2ImgResponse{Images: []string{repSolidPNG(t, req.Width, req.Height, repKleinColor)}}, nil
	}
	src, w, h := repWide3(t)

	result, err := svc.RepairImage(RepairParams{
		ImageBase64: src,
		MaskBase64:  repMask(t, w, h, image.Rect(300, 300, 400, 400), image.Rect(1100, 300, 1200, 400), image.Rect(1900, 300, 2000, 400)),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "interrupted")
	assert.Nil(t, result)
	assert.Equal(t, 1, rec.img2imgCount())
	ops := rec.referenceOps()
	require.NotEmpty(t, ops)
	assert.Equal(t, "option:"+kleinReferenceOption+"=false", ops[len(ops)-1])
	sessions.mu.Lock()
	assert.Empty(t, sessions.items)
	sessions.mu.Unlock()
}

func TestRepairImage_ProgressEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		rects []image.Rectangle
		want  []string
	}{
		{name: "two tiles", rects: []image.Rectangle{image.Rect(300, 300, 400, 400), image.Rect(1900, 300, 2000, 400)}, want: []string{"progress:0/2", "img2img", "progress:1/2", "img2img", "progress:2/2"}},
		{name: "one tile", rects: []image.Rectangle{image.Rect(1100, 300, 1200, 400)}, want: []string{"progress:0/1", "img2img", "progress:1/1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := repSetup(t)
			src, w, h := repWide3(t)

			_, err := svc.RepairImage(RepairParams{ImageBase64: src, MaskBase64: repMask(t, w, h, tt.rects...)})
			require.NoError(t, err)

			rec.mu.Lock()
			defer rec.mu.Unlock()
			var seq []string
			for _, op := range rec.ops {
				if op == "img2img" || strings.HasPrefix(op, "progress:") {
					seq = append(seq, op)
				}
			}
			assert.Equal(t, tt.want, seq)
		})
	}
}

func TestRepairImage_AddsToSession(t *testing.T) {
	t.Parallel()
	svc, _, sessions := repSetup(t)

	result, err := svc.RepairImage(RepairParams{
		ImageBase64: repSource(t, 600, 400),
		MaskBase64:  repMask(t, 600, 400, image.Rect(250, 150, 350, 250)),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Image)

	var info map[string]any
	raw := result.Info
	var wrapped string
	if json.Unmarshal(raw, &wrapped) == nil {
		raw = json.RawMessage(wrapped)
	}
	require.NoError(t, json.Unmarshal(raw, &info))
	assert.EqualValues(t, 600, info["width"])
	assert.EqualValues(t, 400, info["height"])

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	require.Len(t, sessions.items, 1)
	item := sessions.items[0]
	assert.Equal(t, "repair", item.source)
	assert.False(t, item.isPreview)
	require.NotNil(t, item.meta)
	assert.Equal(t, "Ремонт", item.meta.Label)
	assert.Equal(t, kleinLookModel, item.meta.Model)
	sessImg := objTestDecodeImage(t, item.imageBase64)
	assert.Equal(t, 600, sessImg.Bounds().Dx())
	assert.Equal(t, 400, sessImg.Bounds().Dy())
}
