package generation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/sd"
)

func TestWithSD_SwapsClientSharesProgressState(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	base := &mockSD{}
	svc := newTestService(t, db, &mockLLM{}, base)
	scoped := &mockSD{}

	c := svc.WithSD(scoped)
	assert.Same(t, scoped, c.sd.(*mockSD))
	assert.Same(t, base, svc.sd.(*mockSD))
	assert.Same(t, svc.progress, c.progress)

	_ = svc.InterruptGeneration()
	assert.Error(t, c.checkSDInterrupted())
	c.StartSDPolling()
	assert.NoError(t, svc.checkSDInterrupted())
}

func TestSDProgressEvent(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		st   sd.JobStatus
		want SDProgressEvent
	}{
		{name: "queued", st: sd.JobStatus{ID: "j", Status: sd.JobQueued}, want: SDProgressEvent{Job: "j", JobCount: 1}},
		{name: "half way", st: sd.JobStatus{ID: "j", Status: sd.JobRunning, StartedAt: &start, Progress: sd.JobProgress{Step: 5, Steps: 10}}, want: SDProgressEvent{Job: "j", JobCount: 1, Steps: 10, Progress: 0.5, ETARelative: 10}},
		{name: "done", st: sd.JobStatus{ID: "j", Status: sd.JobRunning, StartedAt: &start, Progress: sd.JobProgress{Step: 10, Steps: 10}}, want: SDProgressEvent{Job: "j", JobCount: 1, Steps: 10, Progress: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, sdProgressEvent(tt.st, start.Add(10*time.Second)))
		})
	}
}

func TestOnSDProgress_EmitsOnlyWhileActive(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	sdSvc := &mockSD{}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	em := &mockEmitter{}
	svc.emitter = em

	st := sd.JobStatus{ID: "j", Status: sd.JobRunning, Progress: sd.JobProgress{Step: 1, Steps: 4}}
	svc.onSDProgress(st)
	svc.StartSDPolling()
	sdSvc.progressFn(st)
	svc.StopSDPolling()
	svc.onSDProgress(st)

	em.mu.Lock()
	defer em.mu.Unlock()
	count := 0
	for _, e := range em.events {
		if e.name == "sd:progress" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestUpscaleImage_ReadsServerInfoFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, info             string
		wantModel, wantSampler string
		wantScheduler          string
	}{
		{
			name:          "sd.cpp server info",
			info:          `{"prompt":"p","negative_prompt":"n","checkpoint":"flux/flux1-dev.safetensors","vae":"","sampler":"euler_a","scheduler":"karras","steps":20,"cfg_scale":3.5,"seed":7,"width":512,"height":512}`,
			wantModel:     "flux/flux1-dev.safetensors",
			wantSampler:   "euler_a",
			wantScheduler: "karras",
		},
		{
			name:        "old forge info in history",
			info:        `{"prompt":"p","sampler_name":"DPM++ 2M Karras","sd_model_name":"dreamshaper_8","seed":1,"width":512,"height":512}`,
			wantModel:   "dreamshaper_8",
			wantSampler: "dpm++2m", wantScheduler: "karras",
		},
		{
			name:      "no model in info sends empty checkpoint",
			info:      `{"prompt":"p","sampler":"euler","seed":1,"width":512,"height":512}`,
			wantModel: "", wantSampler: "euler",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			var got sd.Img2ImgRequest
			sdSvc := &mockSD{img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
				got = req
				return &sd.Txt2ImgResponse{Images: []string{"up"}}, nil
			}}
			svc := newTestService(t, db, &mockLLM{}, sdSvc)
			_, err := svc.UpscaleImage(UpscaleImageParams{ImageBase64: "dGVzdA==", GenInfo: tt.info})
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, got.Model.Checkpoint)
			assert.Equal(t, tt.wantSampler, got.SamplerName)
			assert.Equal(t, tt.wantScheduler, got.Scheduler)
		})
	}
}

func TestGenerateImage_DefaultModelSendsEmptyCheckpoint(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	p := makeTestPreset(t, db, nil)
	p.ModelName = ""
	require.NoError(t, db.Update(p))

	var got sd.Txt2ImgRequest
	sdSvc := &mockSD{txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
		got = req
		return &sd.Txt2ImgResponse{Images: []string{"img"}}, nil
	}}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	result, err := svc.GenerateImage(GenerateImageParams{PresetID: p.ID})
	require.NoError(t, err)
	assert.Equal(t, "img", result.Image)
	assert.Equal(t, "", got.Model.Checkpoint)
}
