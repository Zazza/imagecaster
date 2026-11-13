package generation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

const objTestSize = 256

func objTestPNGBase64(t *testing.T, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func objTestDecodeImage(t *testing.T, s string) image.Image {
	t.Helper()
	if i := strings.Index(s, ","); strings.HasPrefix(s, "data:") && i >= 0 {
		s = s[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	img, _, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	return img
}

func objTestMaskBase64(t *testing.T, w, h int, r image.Rectangle) string {
	t.Helper()
	m := image.NewGray(image.Rect(0, 0, w, h))
	r = r.Intersect(m.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			m.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	return objTestPNGBase64(t, m)
}

func objValidMask(t *testing.T) string {
	return objTestMaskBase64(t, objTestSize, objTestSize, image.Rect(96, 96, 160, 160))
}

type objRecorder struct {
	mu          sync.Mutex
	ops         []string
	reqs        []sd.Img2ImgRequest
	models      []string
	visionCalls int
	visionImage string
}

func (r *objRecorder) img2imgCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *objRecorder) visionCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.visionCalls
}

func objTestSetup(t *testing.T, vision func() (string, error), img2img func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error)) (*Service, *objRecorder, *mockSessions) {
	t.Helper()
	rec := &objRecorder{}
	llmSvc := &mockLLM{}
	llmSvc.chatVisionFn = func(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
		rec.mu.Lock()
		rec.visionCalls++
		rec.visionImage = imageBase64
		rec.mu.Unlock()
		if vision == nil {
			return "", fmt.Errorf("vision not expected")
		}
		return vision()
	}
	sdSvc := &mockSD{}
	sdSvc.setOption = func(key string, value any) error {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.ops = append(rec.ops, fmt.Sprintf("option:%s=%v", key, value))
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
		rec.mu.Unlock()
		if img2img != nil {
			return img2img(req)
		}
		return &sd.Txt2ImgResponse{Images: []string{makePNGBase64(t, objTestSize, objTestSize)}}, nil
	}
	db := openTestDB(t)
	svc := newTestService(t, db, llmSvc, sdSvc)
	sessions := &mockSessions{}
	svc.sessions = sessions
	return svc, rec, sessions
}

func TestEditObject_Validation(t *testing.T) {
	t.Parallel()

	img := makePNGBase64(t, objTestSize, objTestSize)
	mask := objValidMask(t)
	blackMask := objTestMaskBase64(t, objTestSize, objTestSize, image.Rectangle{})
	transparentMask := objTestPNGBase64(t, image.NewNRGBA(image.Rect(0, 0, objTestSize, objTestSize)))

	tests := []struct {
		name   string
		params ObjectEditParams
	}{
		{name: "empty image", params: ObjectEditParams{ImageBase64: "", MaskBase64: mask, Op: "remove", Object: "cup"}},
		{name: "image too large", params: ObjectEditParams{ImageBase64: strings.Repeat("x", MaxImageBase64Len+1), MaskBase64: mask, Op: "remove", Object: "cup"}},
		{name: "empty mask", params: ObjectEditParams{ImageBase64: img, MaskBase64: "", Op: "remove", Object: "cup"}},
		{name: "mask not an image", params: ObjectEditParams{ImageBase64: img, MaskBase64: base64.StdEncoding.EncodeToString([]byte("not a png")), Op: "remove", Object: "cup"}},
		{name: "mask all black", params: ObjectEditParams{ImageBase64: img, MaskBase64: blackMask, Op: "remove", Object: "cup"}},
		{name: "mask all transparent", params: ObjectEditParams{ImageBase64: img, MaskBase64: transparentMask, Op: "remove", Object: "cup"}},
		{name: "bad op", params: ObjectEditParams{ImageBase64: img, MaskBase64: mask, Op: "inpaint", Object: "cup"}},
		{name: "empty op", params: ObjectEditParams{ImageBase64: img, MaskBase64: mask, Op: "", Object: "cup"}},
		{name: "replace with empty replacement", params: ObjectEditParams{ImageBase64: img, MaskBase64: mask, Op: "replace", Replacement: "", Object: "cup"}},
		{name: "replace with whitespace replacement", params: ObjectEditParams{ImageBase64: img, MaskBase64: mask, Op: "replace", Replacement: "   \t\n", Object: "cup"}},
		{name: "replacement over 500 runes", params: ObjectEditParams{ImageBase64: img, MaskBase64: mask, Op: "replace", Replacement: strings.Repeat("ж", 501), Object: "cup"}},
		{name: "object over 200 runes", params: ObjectEditParams{ImageBase64: img, MaskBase64: mask, Op: "remove", Object: strings.Repeat("ж", 201)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, func() (string, error) { return "cup", nil }, nil)

			result, err := svc.EditObject(tt.params)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.Equal(t, 0, rec.img2imgCount())
			assert.Equal(t, 0, rec.visionCount())
		})
	}
}

func TestEditObject_LimitsAreInclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params func(t *testing.T) ObjectEditParams
	}{
		{name: "replacement exactly 500 runes", params: func(t *testing.T) ObjectEditParams {
			return ObjectEditParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), MaskBase64: objValidMask(t), Op: "replace", Replacement: strings.Repeat("ж", 500), Object: "cup"}
		}},
		{name: "object exactly 200 runes", params: func(t *testing.T) ObjectEditParams {
			return ObjectEditParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), MaskBase64: objValidMask(t), Op: "remove", Object: strings.Repeat("ж", 200)}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, nil)

			_, err := svc.EditObject(tt.params(t))

			require.NoError(t, err)
			assert.Equal(t, 1, rec.img2imgCount())
		})
	}
}

func TestEditObject_KidsModeBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params func(t *testing.T) ObjectEditParams
	}{
		{name: "replacement", params: func(t *testing.T) ObjectEditParams {
			return ObjectEditParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), MaskBase64: objValidMask(t), Op: "replace", Replacement: "zombie", Object: "cup"}
		}},
		{name: "object", params: func(t *testing.T) ObjectEditParams {
			return ObjectEditParams{ImageBase64: makePNGBase64(t, objTestSize, objTestSize), MaskBase64: objValidMask(t), Op: "remove", Object: "zombie"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, nil)
			require.NoError(t, svc.kids.SetKidsMode(true, "1234"))

			_, err := svc.EditObject(tt.params(t))

			require.Error(t, err)
			assert.Equal(t, 0, rec.img2imgCount())
		})
	}
}

func TestEditObject_DetectsObjectViaVision(t *testing.T) {
	t.Parallel()
	svc, rec, _ := objTestSetup(t, func() (string, error) { return "  a red coffee mug \n", nil }, nil)

	result, err := svc.EditObject(ObjectEditParams{
		ImageBase64: makePNGBase64(t, objTestSize, objTestSize),
		MaskBase64:  objValidMask(t),
		Op:          "remove",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, rec.visionCount())
	assert.Equal(t, "a red coffee mug", result.DetectedObject)
	require.Equal(t, 1, rec.img2imgCount())
	assert.Contains(t, rec.reqs[0].Prompt, "a red coffee mug")
}

func TestEditObject_VisionReceivesRedOutlineNotFill(t *testing.T) {
	t.Parallel()
	svc, rec, _ := objTestSetup(t, func() (string, error) { return "mug", nil }, nil)

	_, err := svc.EditObject(ObjectEditParams{
		ImageBase64: makePNGBase64(t, objTestSize, objTestSize),
		MaskBase64:  objValidMask(t),
		Op:          "remove",
	})
	require.NoError(t, err)

	rec.mu.Lock()
	sent := rec.visionImage
	rec.mu.Unlock()
	require.NotEmpty(t, sent)
	img := objTestDecodeImage(t, sent)
	b := img.Bounds()

	isRed := func(x, y int) bool {
		r, g, bl, _ := img.At(x, y).RGBA()
		return r>>8 >= 180 && g>>8 <= 80 && bl>>8 <= 80
	}
	redCount := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isRed(x, y) {
				redCount++
			}
		}
	}
	assert.Greater(t, redCount, 0, "vision image must contain a red outline of the mask")
	cx, cy := b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2
	assert.False(t, isRed(cx, cy), "mask interior must not be filled red")
	assert.False(t, isRed(b.Min.X+b.Dx()/10, b.Min.Y+b.Dy()/10), "area far from mask must not be red")
}

func TestEditObject_ObjectGivenSkipsVision(t *testing.T) {
	t.Parallel()
	svc, rec, _ := objTestSetup(t, func() (string, error) { return "should not be used", nil }, nil)

	result, err := svc.EditObject(ObjectEditParams{
		ImageBase64: makePNGBase64(t, objTestSize, objTestSize),
		MaskBase64:  objValidMask(t),
		Op:          "remove",
		Object:      "the blue vase",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, rec.visionCount())
	assert.Equal(t, "the blue vase", result.DetectedObject)
	require.Equal(t, 1, rec.img2imgCount())
	assert.Contains(t, rec.reqs[0].Prompt, "the blue vase")
	assert.NotContains(t, rec.reqs[0].Prompt, "should not be used")
}

func TestEditObject_VisionFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		vision func() (string, error)
	}{
		{name: "empty answer", vision: func() (string, error) { return "", nil }},
		{name: "whitespace answer", vision: func() (string, error) { return "  \n ", nil }},
		{name: "vision error", vision: func() (string, error) { return "", fmt.Errorf("vision down") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, tt.vision, nil)

			result, err := svc.EditObject(ObjectEditParams{
				ImageBase64: makePNGBase64(t, objTestSize, objTestSize),
				MaskBase64:  objValidMask(t),
				Op:          "remove",
			})

			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "не удалось определить объект")
			assert.Equal(t, 0, rec.img2imgCount())
		})
	}
}

func TestEditObject_Instruction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		op          string
		replacement string
		want        string
		mustContain []string
		labelPrefix string
	}{
		{
			name:        "remove",
			op:          "remove",
			want:        "Remove red car completely. Fill the area with the natural background that would be behind it. Keep everything else in the image unchanged.",
			mustContain: []string{"Remove", "red car"},
			labelPrefix: "Удаление:",
		},
		{
			name:        "replace",
			op:          "replace",
			replacement: "  a green bicycle ",
			want:        "Replace red car with a green bicycle. Keep everything else in the image unchanged.",
			mustContain: []string{"Replace", "red car", "a green bicycle"},
			labelPrefix: "Замена:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, sessions := objTestSetup(t, nil, nil)

			result, err := svc.EditObject(ObjectEditParams{
				ImageBase64: makePNGBase64(t, objTestSize, objTestSize),
				MaskBase64:  objValidMask(t),
				Op:          tt.op,
				Replacement: tt.replacement,
				Object:      "red car",
			})

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, rec.img2imgCount())
			prompt := rec.reqs[0].Prompt
			for _, s := range tt.mustContain {
				assert.Contains(t, prompt, s)
			}
			assert.Contains(t, prompt, tt.want)
			assert.Equal(t, tt.want, result.EffectivePrompt)

			sessions.mu.Lock()
			defer sessions.mu.Unlock()
			require.Len(t, sessions.items, 1)
			require.NotNil(t, sessions.items[0].meta)
			assert.True(t, strings.HasPrefix(sessions.items[0].meta.Label, tt.labelPrefix), "label %q", sessions.items[0].meta.Label)
			assert.Equal(t, kleinLookModel, sessions.items[0].meta.Model)
		})
	}
}

func TestEditObject_Img2ImgRequest(t *testing.T) {
	t.Parallel()
	svc, rec, _ := objTestSetup(t, nil, nil)
	seed := int64(777)

	_, err := svc.EditObject(ObjectEditParams{
		ImageBase64: makePNGBase64(t, objTestSize, objTestSize),
		MaskBase64:  objValidMask(t),
		Op:          "remove",
		Object:      "cup",
		Seed:        &seed,
	})
	require.NoError(t, err)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.reqs, 1)
	req := rec.reqs[0]
	require.NotNil(t, req.DenoisingStrength)
	assert.InDelta(t, 1.0, *req.DenoisingStrength, 1e-9)
	assert.Equal(t, 4, req.Steps)
	assert.InDelta(t, 1.0, req.CfgScale, 1e-9)
	assert.Equal(t, "euler", req.SamplerName)
	assert.Equal(t, "simple", req.Scheduler)
	wantW, wantH := alignLookSize(objTestSize, objTestSize)
	assert.Equal(t, wantW, req.Width)
	assert.Equal(t, wantH, req.Height)
	require.NotNil(t, req.Seed)
	assert.Equal(t, int64(777), *req.Seed)
	require.NotEmpty(t, req.InitImages)

	require.NotEmpty(t, rec.models)
	assert.Equal(t, kleinLookModel, rec.models[len(rec.models)-1])
}

func TestEditObject_ReferenceToggledAroundImg2Img(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		img2img func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error)
		wantErr bool
	}{
		{name: "success", img2img: nil, wantErr: false},
		{name: "img2img error", img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			return nil, fmt.Errorf("sd boom")
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, tt.img2img)

			_, err := svc.EditObject(ObjectEditParams{
				ImageBase64: makePNGBase64(t, objTestSize, objTestSize),
				MaskBase64:  objValidMask(t),
				Op:          "remove",
				Object:      "cup",
			})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			rec.mu.Lock()
			defer rec.mu.Unlock()
			var refOps []string
			for _, op := range rec.ops {
				if op == "img2img" || strings.HasPrefix(op, "option:"+kleinReferenceOption+"=") {
					refOps = append(refOps, op)
				}
			}
			assert.Equal(t, []string{
				"option:" + kleinReferenceOption + "=true",
				"img2img",
				"option:" + kleinReferenceOption + "=false",
			}, refOps)
		})
	}
}

func TestEditObject_ResultKeepsOriginalSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sdW, sdH     int
		origW, origH int
	}{
		{name: "sd returns smaller image", sdW: 128, sdH: 128, origW: objTestSize, origH: objTestSize},
		{name: "sd returns same size", sdW: objTestSize, sdH: objTestSize, origW: objTestSize, origH: objTestSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := objTestSetup(t, nil, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
				return &sd.Txt2ImgResponse{Images: []string{makePNGBase64(t, tt.sdW, tt.sdH)}}, nil
			})

			result, err := svc.EditObject(ObjectEditParams{
				ImageBase64: makePNGBase64(t, tt.origW, tt.origH),
				MaskBase64:  objTestMaskBase64(t, tt.origW, tt.origH, image.Rect(96, 96, 160, 160)),
				Op:          "remove",
				Object:      "cup",
			})

			require.NoError(t, err)
			require.NotNil(t, result)
			img := objTestDecodeImage(t, result.Image)
			assert.Equal(t, tt.origW, img.Bounds().Dx())
			assert.Equal(t, tt.origH, img.Bounds().Dy())
		})
	}
}

func TestPatchInfoSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"object", `{"seed":7,"width":512,"height":384}`, `{"height":768,"seed":7,"width":1024}`},
		{"string-wrapped", `"{\"seed\":7,\"width\":512}"`, `"{\"height\":768,\"seed\":7,\"width\":1024}"`},
		{"garbage kept", `not json`, `not json`},
		{"empty kept", ``, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := string(patchInfoSize(json.RawMessage(tt.in), 1024, 768)); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
