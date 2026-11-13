package queue

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
)

func xrPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

type xrImgSD struct {
	xrSD
	out      string
	imgMu    sync.Mutex
	imgPrmts []string
	onImg    func()
}

func (f *xrImgSD) Img2Img(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
	f.imgMu.Lock()
	f.imgPrmts = append(f.imgPrmts, req.Prompt)
	hook := f.onImg
	f.imgMu.Unlock()
	if hook != nil {
		hook()
	}
	return &sd.Txt2ImgResponse{Images: []string{f.out}, Info: json.RawMessage(`{"prompt":"x"}`)}, nil
}

func (f *xrImgSD) img2imgPrompts() []string {
	f.imgMu.Lock()
	defer f.imgMu.Unlock()
	return append([]string(nil), f.imgPrmts...)
}

type xrFaceSD struct {
	xrSD
	mu    sync.Mutex
	calls []string
	pts   []sd.Point
}

func (f *xrFaceSD) FaceLandmarks(imageBase64 string) ([]sd.Point, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, imageBase64)
	return f.pts, nil
}

type xrFailLLM struct {
	xrLLM
	errMu sync.Mutex
	err   error
}

func (l *xrFailLLM) reply() (string, error) {
	l.errMu.Lock()
	e := l.err
	l.errMu.Unlock()
	if e != nil {
		l.next()
		return "", e
	}
	return l.answer(), nil
}

func (l *xrFailLLM) Chat(string, string, string, float64, int) (string, error) {
	return l.reply()
}
func (l *xrFailLLM) ChatVision(string, string, string, string, float64, int) (string, error) {
	return l.reply()
}
func (l *xrFailLLM) ChatWithMessages(string, []llm.Message, float64, int) (string, error) {
	return l.reply()
}
func (l *xrFailLLM) GenerateSDPrompt(string, string, string, string, int) (string, error) {
	return l.reply()
}
func (l *xrFailLLM) AnalyzeImage(string, string, string, int) (string, error) {
	return l.reply()
}
func (l *xrFailLLM) AnalyzeImageDescribe(string, string, string, int) (string, error) {
	return l.reply()
}

func (e *xrEnv) processorWithLog(t *testing.T, l llm.Service, s sd.Service, sessions *xrSessions) (Processor, *logger.Logger) {
	t.Helper()
	log := logger.New(nil)
	gen := generation.New(
		e.pdb,
		l,
		s,
		&config.Config{SDPromptModel: "test-model", VisionModel: "test-vision"},
		t.TempDir(),
		&mockEmitter{},
		kids.NewManager(e.pdb),
		sessions,
		editFakeSettings{},
		nil,
		log,
	)
	gen.SetContext(context.Background())
	return NewProcessor(gen, &xrScoper{svc: s}, e.store, e.dataDir, &mockEmitter{}), log
}

func (e *xrEnv) lookJob(t *testing.T, img, text string) int64 {
	t.Helper()
	seed := int64(5)
	raw, err := json.Marshal(generation.ApplyLookParams{ImageBase64: img, Text: text, Strength: 0.5, Seed: &seed})
	require.NoError(t, err)
	id, err := e.store.Create(JobLook, string(raw), "test")
	require.NoError(t, err)
	return id
}

func xrWarnsWith(log *logger.Logger, sub string) []string {
	var out []string
	for _, en := range log.Entries() {
		if en.Level == logger.WARN && strings.Contains(en.Message, sub) {
			out = append(out, en.Message)
		}
	}
	return out
}

func TestLLMMemo_FailureBeforeSDWorkIsReplayedXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	img := xrPNG(t, 64, 64)
	const lookText = "neon night look with rain"
	id := env.lookJob(t, img, lookText)
	llmErr := errors.New("llm backend said: model overloaded #7731")

	sdA := &xrImgSD{out: img}
	llmA := &xrFailLLM{xrLLM: xrLLM{variant: "A"}, err: llmErr}
	procA, logA := env.processorWithLog(t, llmA, sdA, &xrSessions{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sdA.onImg = cancel
	NewWorker(env.store, procA, &mockEmitter{}).processNext(ctx)

	promptsA := sdA.img2imgPrompts()
	require.Len(t, promptsA, 1)
	require.GreaterOrEqual(t, llmA.count(), 1)
	require.Contains(t, promptsA[0], lookText, "run 1 takes the fallback path")
	warnsA := xrWarnsWith(logA, "look: LLM rewrite failed")
	require.Len(t, warnsA, 1)
	require.Contains(t, warnsA[0], llmErr.Error())
	job, err := env.store.Get(id)
	require.NoError(t, err)
	require.Equal(t, StatusPending, job.Status)
	require.True(t, xrExists(xrMemoFile(env.dataDir, id)))

	sdB := &xrImgSD{out: img}
	llmB := &xrFailLLM{xrLLM: xrLLM{variant: "B"}}
	sessionsB := &xrSessions{}
	procB, logB := env.processorWithLog(t, llmB, sdB, sessionsB)
	NewWorker(env.store, procB, &mockEmitter{}).processNext(context.Background())

	assert.Equal(t, promptsA, sdB.img2imgPrompts(), "same SD request after restart")
	assert.Equal(t, 0, llmB.count(), "failed call replayed, llm not called")
	assert.Equal(t, warnsA, xrWarnsWith(logB, "look: LLM rewrite failed"), "same error text replayed")
	job, err = env.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status)
	assert.Equal(t, 1, sessionsB.count())
}

func TestLLMMemo_FailureWithoutSDWorkIsNotReplayedXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	id := env.ideaJob(t)
	llmErr := errors.New("dial tcp 127.0.0.1:11434: connect: connection refused")

	sdA := &xrSD{}
	llmA := &xrFailLLM{xrLLM: xrLLM{variant: "A"}, err: llmErr}
	procA, _ := env.processorWithLog(t, llmA, sdA, &xrSessions{})
	NewWorker(env.store, procA, &mockEmitter{}).processNext(context.Background())

	require.Empty(t, sdA.txtPrompts(), "no SD work in run 1")
	require.Equal(t, 1, llmA.count())
	job, err := env.store.Get(id)
	require.NoError(t, err)
	require.Equal(t, StatusPending, job.Status, "retryable llm failure requeues the job")
	require.Equal(t, 1, job.RetryCount)
	_, err = env.store.db.Exec(`UPDATE job_queue SET next_retry_at = NULL WHERE id = ?`, id)
	require.NoError(t, err)

	sdB := &xrSD{}
	llmB := &xrFailLLM{xrLLM: xrLLM{variant: "B"}}
	sessionsB := &xrSessions{}
	procB, _ := env.processorWithLog(t, llmB, sdB, sessionsB)
	NewWorker(env.store, procB, &mockEmitter{}).processNext(context.Background())

	assert.GreaterOrEqual(t, llmB.count(), 1, "failed call goes to the llm again")
	promptsB := sdB.txtPrompts()
	require.Len(t, promptsB, 1)
	assert.Contains(t, promptsB[0], "B-tags-1")
	job, err = env.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status)
	assert.Equal(t, 1, sessionsB.count())
}

func TestLLMMemo_SuccessIsReplayedForLookXReview(t *testing.T) {
	t.Parallel()
	env := newXREnv(t)
	img := xrPNG(t, 64, 64)
	id := env.lookJob(t, img, "golden hour look")

	sdA := &xrImgSD{out: img}
	llmA := &xrFailLLM{xrLLM: xrLLM{variant: "A"}}
	procA, _ := env.processorWithLog(t, llmA, sdA, &xrSessions{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sdA.onImg = cancel
	NewWorker(env.store, procA, &mockEmitter{}).processNext(ctx)

	promptsA := sdA.img2imgPrompts()
	require.Len(t, promptsA, 1)
	require.Contains(t, promptsA[0], "A-tags-1")
	job, err := env.store.Get(id)
	require.NoError(t, err)
	require.Equal(t, StatusPending, job.Status)

	sdB := &xrImgSD{out: img}
	llmB := &xrFailLLM{xrLLM: xrLLM{variant: "B"}}
	procB, _ := env.processorWithLog(t, llmB, sdB, &xrSessions{})
	NewWorker(env.store, procB, &mockEmitter{}).processNext(context.Background())

	assert.Equal(t, promptsA, sdB.img2imgPrompts())
	assert.Equal(t, 0, llmB.count())
	job, err = env.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status)
}

func TestQueueJobSD_FaceDetectionXReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		svc     func() (sd.Service, *xrFaceSD)
		wantErr bool
	}{
		{"face detector delegates", func() (sd.Service, *xrFaceSD) {
			f := &xrFaceSD{pts: []sd.Point{{X: 1.5, Y: 2.5}, {X: 3, Y: 4}}}
			return f, f
		}, false},
		{"no face detector", func() (sd.Service, *xrFaceSD) { return &xrSD{}, nil }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newXREnv(t)
			id := env.ideaJob(t)
			job, err := env.store.Get(id)
			require.NoError(t, err)
			svc, face := tt.svc()
			proc, _ := env.processorWithLog(t, &xrLLM{variant: "A"}, &xrSD{}, &xrSessions{})
			p := proc.(*processor)
			p.scoper = &xrScoper{svc: svc}

			gen, _ := p.genFor(context.Background(), job)
			pts, err := gen.FaceLandmarks("face-image-b64")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, pts, 2)
			assert.Equal(t, 1.5, pts[0].X)
			assert.Equal(t, 2.5, pts[0].Y)
			assert.Equal(t, 3.0, pts[1].X)
			assert.Equal(t, 4.0, pts[1].Y)
			face.mu.Lock()
			assert.Equal(t, []string{"face-image-b64"}, face.calls)
			face.mu.Unlock()
		})
	}
}
