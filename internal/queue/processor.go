package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/sd"
)

type Processor interface {
	ProcessJob(ctx context.Context, job *Job) (*JobResult, error)
}

type EventEmitter interface {
	Emit(event string, data ...any)
}

type processor struct {
	gen     *generation.Service
	scoper  sd.QueueScoper
	store   *Store
	dataDir string
	emit    EventEmitter
}

func NewProcessor(gen *generation.Service, scoper sd.QueueScoper, store *Store, dataDir string, emit EventEmitter) Processor {
	return &processor{
		gen:     gen,
		scoper:  scoper,
		store:   store,
		dataDir: dataDir,
		emit:    emit,
	}
}

func (p *processor) genFor(ctx context.Context, job *Job) (*generation.Service, func() bool) {
	if p.scoper == nil || p.gen == nil {
		return p.gen, func() bool { return false }
	}
	sdSvc := p.scoper.ForQueueJob(ctx, job.ID)
	if p.dataDir == "" || p.gen.LLM() == nil {
		return p.gen.ForQueue(ctx, sdSvc, nil)
	}
	memo := newLLMMemo(p.gen.LLM(), llmMemoPath(p.dataDir, job.ID))
	return p.gen.ForQueue(ctx, memoSD{Service: sdSvc, memo: memo}, memo)
}

func (p *processor) Forget(jobID int64) {
	removeLLMMemo(p.dataDir, jobID)
}

func (p *processor) ProcessJob(ctx context.Context, job *Job) (*JobResult, error) {
	gen, dropped := p.genFor(ctx, job)
	res, err := p.dispatch(gen, job)
	if err == nil && dropped() {
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		return nil, context.Canceled
	}
	return res, err
}

func (p *processor) dispatch(gen *generation.Service, job *Job) (*JobResult, error) {
	switch job.Type {
	case JobTxt2Img:
		return p.processTxt2Img(gen, job)
	case JobFromImage:
		return p.processFromImage(gen, job)
	case JobCompound:
		return p.processCompound(gen, job)
	case JobLook:
		return p.processLook(gen, job)
	case JobEdit:
		return p.processEdit(gen, job)
	default:
		return nil, fmt.Errorf("unknown job type: %s", job.Type)
	}
}

func (p *processor) processGeneration(job *Job, gen func() (*generation.GenerateImageResult, error)) (*JobResult, error) {
	p.emit.Emit("sd:progress:start")
	result, err := gen()
	p.emit.Emit("sd:progress:stop")
	if err != nil {
		return nil, err
	}
	return &JobResult{
		ImageBase64:             result.Image,
		Info:                    string(result.Info),
		SessionItemID:           result.SessionItemID,
		QualityUpscaleSkipped:   result.QualityUpscaleSkipped,
		QualityRedenoiseSkipped: result.QualityRedenoiseSkipped,
	}, nil
}

func (p *processor) processTxt2Img(gen *generation.Service, job *Job) (*JobResult, error) {
	return p.processGeneration(job, func() (*generation.GenerateImageResult, error) {
		var params generation.GenerateImageParams
		if err := json.Unmarshal([]byte(job.Params), &params); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return gen.GenerateImage(params)
	})
}

func (p *processor) processFromImage(gen *generation.Service, job *Job) (*JobResult, error) {
	return p.processGeneration(job, func() (*generation.GenerateImageResult, error) {
		var params generation.GenerateFromImageParams
		if err := json.Unmarshal([]byte(job.Params), &params); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return gen.GenerateFromImage(params)
	})
}

func (p *processor) processLook(gen *generation.Service, job *Job) (*JobResult, error) {
	return p.processGeneration(job, func() (*generation.GenerateImageResult, error) {
		var params generation.ApplyLookParams
		if err := json.Unmarshal([]byte(job.Params), &params); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return gen.ApplyLookDirect(params.ImageBase64, params.Text, params.Strength, params.Seed)
	})
}

func (p *processor) processEdit(gen *generation.Service, job *Job) (*JobResult, error) {
	return p.processGeneration(job, func() (*generation.GenerateImageResult, error) {
		var params generation.EditImageParams
		if err := json.Unmarshal([]byte(job.Params), &params); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return gen.EditImage(params)
	})
}

func (p *processor) processCompound(gen *generation.Service, job *Job) (*JobResult, error) {
	return p.processGeneration(job, func() (*generation.GenerateImageResult, error) {
		var params generation.GenerateCompoundImageParams
		if err := json.Unmarshal([]byte(job.Params), &params); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return gen.GenerateCompoundImage(params)
	})
}
