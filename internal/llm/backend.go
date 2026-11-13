package llm

type BackendConfig struct {
	KeepAlive  string
	NumCtx     int
	NumPredict int
	NumThread  int
	TopP       float64
}
