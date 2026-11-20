package stylist

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/Zazza/imagecaster/internal/promptutil"
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
	"Flux.1":     {Model: "flux1-dev-fp8", Sampler: "euler", Steps: 20, CFG: 1, FaceDenoise: 0.5},
}

const (
	Width             = 1024
	Height            = 1536
	DefaultBackground = "plain light grey studio backdrop, soft even studio light"
	Quality           = "BREAK, a candid photograph with natural proportions and honest imperfections: realistic skin texture, natural light, documentary framing, no stylization"
	QualityStudio     = "BREAK, a professional studio portrait with realistic skin texture and natural pores, sharp focus, straight symmetrical framing, no stylization"
	DefaultOutfit     = "wearing simple casual everyday clothes"
)

func Prose(base string) bool {
	return promptutil.ProseBase(base)
}

func qualityFor(background string) string {
	if strings.Contains(strings.ToLower(background), "studio") {
		return QualityStudio
	}
	return Quality
}

func ComposePrompt(lora string, parts []string, background string) string {
	return composeTags(lora, parts, background, qualityFor(background))
}

func composeTags(lora string, parts []string, background, quality string) string {
	all := make([]string, 0, len(parts)+2)
	all = append(all, parts...)
	all = append(all, background, quality)
	clean := make([]string, 0, len(all))
	for _, v := range all {
		if v = strings.TrimSpace(v); v != "" {
			clean = append(clean, v)
		}
	}
	if lora == "" {
		return strings.Join(clean, ", ")
	}
	return strings.Join(clean, ", ") + fmt.Sprintf(" <lora:%s:1>", lora)
}

func sentence(v string) string {
	v = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(v), ",;"))
	if v == "" {
		return ""
	}
	r := []rune(v)
	r[0] = unicode.ToUpper(r[0])
	v = string(r)
	if !strings.HasSuffix(v, ".") && !strings.HasSuffix(v, "!") && !strings.HasSuffix(v, "?") {
		v += "."
	}
	return v
}

func Wearing(outfit string) string {
	outfit = strings.TrimSpace(outfit)
	if outfit == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(outfit), "wearing ") {
		return outfit
	}
	return "wearing " + outfit
}

func Labeled(label, v string) string {
	if v = strings.TrimSpace(v); v == "" {
		return ""
	}
	return label + ": " + v
}

func Subject(lead, appearance, action string) string {
	return subjectAs("photograph", lead, appearance, action)
}

func subjectAs(kind, lead, appearance, action string) string {
	appearance = strings.TrimSpace(appearance)
	if appearance == "" {
		appearance = "a person"
	}
	out := "a " + kind + " of " + appearance
	if kind == "image" {
		out = "an image of " + appearance
	}
	if lead = strings.TrimSpace(lead); lead != "" {
		out = "a " + lead + " " + kind + " of " + appearance
	}
	if action = strings.TrimSpace(action); action != "" {
		out += ", " + action
	}
	return out
}

func ComposeProse(lora, subject string, details []string, background string) string {
	return composeProse(lora, subject, details, background, strings.TrimPrefix(qualityFor(background), "BREAK, "))
}

func composeProse(lora, subject string, details []string, background, quality string) string {
	all := make([]string, 0, len(details)+3)
	all = append(all, subject)
	all = append(all, details...)
	all = append(all, Labeled("Background", background), quality)
	clean := make([]string, 0, len(all))
	for _, v := range all {
		if v = sentence(v); v != "" {
			clean = append(clean, v)
		}
	}
	out := strings.Join(clean, " ")
	if lora == "" {
		return out
	}
	return out + fmt.Sprintf(" <lora:%s:1>", lora)
}

func Who(gender string) string {
	switch strings.ToLower(strings.TrimSpace(gender)) {
	case "man", "male":
		return "a man"
	case "woman", "female", "":
		return "a woman"
	}
	return "a person"
}

var subjectNouns = map[string]bool{"woman": true, "man": true, "female": true, "male": true, "person": true, "girl": true, "boy": true, "lady": true, "guy": true, "teenager": true, "teen": true, "child": true, "kid": true}

func WithSubject(appearance, gender string) string {
	appearance = strings.TrimSpace(appearance)
	low := strings.ToLower(appearance)
	switch {
	case appearance == "":
		return Who(gender)
	case strings.HasPrefix(low, "a "), strings.HasPrefix(low, "an "), strings.HasPrefix(low, "the "):
		return appearance
	}
	words := strings.FieldsFunc(low, func(r rune) bool { return r == ' ' || r == ',' })
	if len(words) == 0 {
		return Who(gender)
	}
	if subjectNouns[words[0]] {
		return appearance
	}
	return Who(gender) + ", " + appearance
}
