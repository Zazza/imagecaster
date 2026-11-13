package generation

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

func TestKleinEdit_Validation(t *testing.T) {
	t.Parallel()

	img := makePNGBase64(t, objTestSize, objTestSize)
	tests := []struct {
		name        string
		image       string
		instruction string
	}{
		{name: "empty image", image: "", instruction: "short bob haircut"},
		{name: "image too large", image: strings.Repeat("x", MaxImageBase64Len+1), instruction: "short bob haircut"},
		{name: "empty instruction", image: img, instruction: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, nil)
			seed := int64(1)

			result, err := svc.KleinEdit(tt.image, tt.instruction, &seed)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.Equal(t, 0, rec.img2imgCount())
		})
	}
}

func TestKleinEdit_Img2ImgRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		w, h int
	}{
		{name: "aligned square", w: 256, h: 256},
		{name: "unaligned", w: 300, h: 200},
		{name: "longer side over 2048", w: 2100, h: 700},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec, _ := objTestSetup(t, nil, nil)
			seed := int64(4242)
			instruction := "Give the person a short bob haircut. Keep the face unchanged."

			_, err := svc.KleinEdit(makePNGBase64(t, tt.w, tt.h), instruction, &seed)
			require.NoError(t, err)

			rec.mu.Lock()
			defer rec.mu.Unlock()
			require.Len(t, rec.reqs, 1)
			req := rec.reqs[0]
			assert.Equal(t, instruction, req.Prompt)
			assert.Equal(t, 4, req.Steps)
			assert.InDelta(t, 1.0, req.CfgScale, 1e-9)
			require.NotNil(t, req.DenoisingStrength)
			assert.InDelta(t, 1.0, *req.DenoisingStrength, 1e-9)
			assert.Equal(t, "euler", req.SamplerName)
			assert.Equal(t, "simple", req.Scheduler)
			wantW, wantH := alignLookSize(tt.w, tt.h)
			assert.Equal(t, wantW, req.Width)
			assert.Equal(t, wantH, req.Height)
			assert.Zero(t, req.Width%16)
			assert.Zero(t, req.Height%16)
			assert.LessOrEqual(t, req.Width, 2048)
			assert.LessOrEqual(t, req.Height, 2048)
			require.NotNil(t, req.Seed)
			assert.Equal(t, int64(4242), *req.Seed)
			require.NotEmpty(t, req.InitImages)
			require.NotEmpty(t, rec.models)
			assert.Equal(t, kleinLookModel, rec.models[len(rec.models)-1])
		})
	}
}

func TestKleinEdit_ReferenceToggledAroundImg2Img(t *testing.T) {
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
			seed := int64(7)

			_, err := svc.KleinEdit(makePNGBase64(t, objTestSize, objTestSize), "red hair", &seed)
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

func TestKleinEdit_KidsMode(t *testing.T) {
	t.Parallel()

	t.Run("stop word blocks without calling SD", func(t *testing.T) {
		t.Parallel()
		svc, rec, _ := objTestSetup(t, nil, nil)
		require.NoError(t, svc.kids.SetKidsMode(true, "1234"))
		seed := int64(1)

		result, err := svc.KleinEdit(makePNGBase64(t, objTestSize, objTestSize), "make the person look like a zombie", &seed)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, rec.img2imgCount())
	})

	t.Run("clean instruction gets kids negative", func(t *testing.T) {
		t.Parallel()
		svc, rec, _ := objTestSetup(t, nil, nil)
		require.NoError(t, svc.kids.SetKidsMode(true, "1234"))
		seed := int64(1)

		_, err := svc.KleinEdit(makePNGBase64(t, objTestSize, objTestSize), "short bob haircut", &seed)
		require.NoError(t, err)

		rec.mu.Lock()
		defer rec.mu.Unlock()
		require.Len(t, rec.reqs, 1)
		kidsNeg := svc.kids.ApplyNegative("")
		require.NotEmpty(t, kidsNeg)
		assert.Contains(t, rec.reqs[0].NegativePrompt, kidsNeg)
	})
}

func TestKleinEdit_ResultImageFromSD(t *testing.T) {
	t.Parallel()
	sdImage := objTestMaskBase64(t, objTestSize, objTestSize, image.Rect(10, 10, 100, 100))
	svc, _, _ := objTestSetup(t, nil, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		return &sd.Txt2ImgResponse{Images: []string{sdImage}}, nil
	})
	seed := int64(9)

	result, err := svc.KleinEdit(makePNGBase64(t, objTestSize, objTestSize), "blond hair", &seed)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, sdImage, result.Image)
}

type styVisionCall struct {
	model  string
	prompt string
	image  string
}

type styVisionRecorder struct {
	mu    sync.Mutex
	calls []styVisionCall
}

func (r *styVisionRecorder) add(c styVisionCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

func (r *styVisionRecorder) snapshot() []styVisionCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]styVisionCall(nil), r.calls...)
}

func styDescribeSetup(t *testing.T, answer string) (*Service, *styVisionRecorder) {
	t.Helper()
	rec := &styVisionRecorder{}
	llmSvc := &mockLLM{}
	llmSvc.chatVisionFn = func(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
		rec.add(styVisionCall{model: model, prompt: systemPrompt + "\n" + userText, image: imageBase64})
		return answer, nil
	}
	llmSvc.describeFn = func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
		rec.add(styVisionCall{model: model, prompt: prompt, image: imageBase64})
		return answer, nil
	}
	llmSvc.analyzeImageFn = func(model, systemPrompt, imageBase64 string, maxTokens int) (string, error) {
		rec.add(styVisionCall{model: model, prompt: systemPrompt, image: imageBase64})
		return answer, nil
	}
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("llm_analyze_model", "stylist-analyze-model"))
	svc := newTestService(t, db, llmSvc, &mockSD{})
	return svc, rec
}

func TestDescribeImage(t *testing.T) {
	t.Parallel()

	t.Run("passes prompt and image to analyze model", func(t *testing.T) {
		t.Parallel()
		svc, rec := styDescribeSetup(t, "short dark hair, round glasses")
		img := makePNGBase64(t, 64, 64)
		prompt := "Describe hair, glasses and outfit of the person in one paragraph."

		text, err := svc.DescribeImage(img, prompt)

		require.NoError(t, err)
		assert.Contains(t, text, "short dark hair, round glasses")
		calls := rec.snapshot()
		require.Len(t, calls, 1)
		assert.Contains(t, calls[0].prompt, prompt)
		assert.Equal(t, img, calls[0].image)
		assert.Equal(t, "stylist-analyze-model", calls[0].model)
	})

	tests := []struct {
		name  string
		image string
	}{
		{name: "empty image", image: ""},
		{name: "image too large", image: strings.Repeat("x", MaxImageBase64Len+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, rec := styDescribeSetup(t, "text")

			_, err := svc.DescribeImage(tt.image, "describe")

			require.Error(t, err)
			assert.Empty(t, rec.snapshot())
		})
	}
}
