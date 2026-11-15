package llm

const (
	ModeGenerate = "generate"
	ModeAnalyze  = "analyze"
)

type BackendConfig struct {
	KeepAlive string
	NumCtx    int
	NumThread int
	TopP      float64
}

type ConfigSource func(mode string) BackendConfig

func (c BackendConfig) ollamaOptions(opts map[string]any) map[string]any {
	if opts == nil {
		opts = map[string]any{}
	}
	if c.NumCtx > 0 {
		opts["num_ctx"] = c.NumCtx
	}
	if c.TopP > 0 {
		opts["top_p"] = c.TopP
	}
	if c.NumThread > 0 {
		opts["num_thread"] = c.NumThread
	}
	return opts
}
