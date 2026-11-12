package llm

type BackendConfig struct {
	KeepAlive  string
	NumCtx     int
	NumGPU     int
	NumPredict int
	NumThread  int
	TopP       float64
}
