package sd

import "context"

type Service interface {
	Txt2Img(req Txt2ImgRequest) (*Txt2ImgResponse, error)
	Img2Img(req Img2ImgRequest) (*Txt2ImgResponse, error)
	UpscaleImage(base64Img string, upscaler string, scale float64) (string, error)
	GetModels() ([]SDModel, error)
	GetSamplers() ([]Sampler, error)
	GetSchedulers() ([]Scheduler, error)
	GetUpscalers() ([]Upscaler, error)
	GetVAEs() ([]VAE, error)
	GetLoRAs() ([]LoRA, error)
	State() (*ServerState, error)
	ListJobs(ctx context.Context, client string) ([]JobStatus, error)
	SetProgressHandler(fn func(JobStatus))
	Interrupt() error
	HealthCheck() error
	SetURL(baseURL string)
	MemoryInfo() (*MemoryStats, error)
}

type QueueScoper interface {
	ForQueueJob(ctx context.Context, queueJobID int64) Service
	CleanupQueueJob(ctx context.Context, queueJobID int64) error
}
