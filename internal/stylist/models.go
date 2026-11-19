package stylist

import (
	"fmt"
	"strings"
)

type Model struct {
	Model       string
	Sampler     string
	Steps       int
	CFG         float64
	FaceDenoise float64
}

var Models = map[string]Model{
	"Z-Image":    {Model: "z-image-turbo-fp8-aio", Sampler: "euler", Steps: 8, CFG: 1, FaceDenoise: 0.4},
	"Moss-Image": {Model: "moss_image_sft_bf16", Sampler: "euler", Steps: 30, CFG: 3, FaceDenoise: 0.5},
	"SDXL":       {Model: "juggernautXL_ragnarokBy", Sampler: "dpm++2m_sde", Steps: 30, CFG: 6, FaceDenoise: 0.5},
	"Flux.2":     {Model: "flux-2-klein-4b", Sampler: "euler", Steps: 4, CFG: 1, FaceDenoise: 0.6},
}

const (
	Width             = 1024
	Height            = 1536
	DefaultBackground = "plain light grey studio backdrop, soft even studio light"
	Quality           = "BREAK, a candid photograph with natural proportions and honest imperfections: realistic skin texture, natural light, documentary framing, no stylization"
	QualityStudio     = "BREAK, a professional studio portrait with realistic skin texture and natural pores, sharp focus, straight symmetrical framing, no stylization"
	DefaultOutfit     = "wearing simple casual everyday clothes"
)

func ComposePrompt(lora string, parts []string, background string) string {
	quality := Quality
	if strings.Contains(strings.ToLower(background), "studio") {
		quality = QualityStudio
	}
	all := make([]string, 0, len(parts)+2)
	all = append(all, parts...)
	all = append(all, background, quality)
	clean := make([]string, 0, len(all))
	for _, v := range all {
		if v = strings.TrimSpace(v); v != "" {
			clean = append(clean, v)
		}
	}
	return strings.Join(clean, ", ") + fmt.Sprintf(" <lora:%s:1>", lora)
}
