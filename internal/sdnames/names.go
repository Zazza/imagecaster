package sdnames

import "strings"

type pair struct{ forge, sdcpp string }

var samplers = []pair{
	{"Euler", "euler"},
	{"Euler a", "euler_a"},
	{"Heun", "heun"},
	{"DPM2", "dpm2"},
	{"DPM++ 2S a", "dpm++2s_a"},
	{"DPM++ 2M", "dpm++2m"},
	{"DPM++ 2M SDE", "dpm++2m_sde"},
	{"LMS", "lms"},
	{"LCM", "lcm"},
	{"DDIM", "ddim_trailing"},
	{"TCD", "tcd"},
	{"iPNDM", "ipndm"},
	{"iPNDM_v", "ipndm_v"},
	{"Res Multistep", "res_multistep"},
	{"ER SDE", "er_sde"},
}

var schedulers = []pair{
	{"Simple", "simple"},
	{"Karras", "karras"},
	{"Exponential", "exponential"},
	{"Normal", "normal"},
	{"Uniform", "discrete"},
	{"SGM Uniform", "sgm_uniform"},
	{"Align Your Steps", "ays"},
	{"Align Your Steps GITS", "gits"},
	{"KL Optimal", "kl_optimal"},
	{"Beta", "beta"},
	{"Smoothstep", "smoothstep"},
	{"LCM", "lcm"},
	{"Bong Tangent", "bong_tangent"},
}

var upscalerAliases = map[string]string{
	"R-ESRGAN 4x+":         "RealESRGAN_x4plus",
	"R-ESRGAN 4x+ Anime6B": "RealESRGAN_x4plus_anime_6B",
	"Lanczos":              "lanczos",
	"Nearest":              "nearest",
}

func lookup(table []pair, name string) (string, bool) {
	n := strings.TrimSpace(name)
	for _, e := range table {
		if strings.EqualFold(e.forge, n) || e.sdcpp == strings.ToLower(n) {
			return e.sdcpp, true
		}
	}
	return "", false
}

func Sampler(name string) (string, bool) {
	if strings.TrimSpace(name) == "" {
		return "", true
	}
	return lookup(samplers, name)
}

func Scheduler(name string) (string, bool) {
	n := strings.TrimSpace(name)
	if n == "" || strings.EqualFold(n, "Automatic") || strings.EqualFold(n, "auto") {
		return "", true
	}
	return lookup(schedulers, n)
}

func SplitSampler(name string) (sampler string, scheduler string, ok bool) {
	if s, ok := Sampler(name); ok {
		return s, "", true
	}
	n := strings.TrimSpace(name)
	for _, sc := range schedulers {
		suffix := " " + sc.forge
		if len(n) > len(suffix) && strings.EqualFold(n[len(n)-len(suffix):], suffix) {
			if s, ok := Sampler(n[:len(n)-len(suffix)]); ok && s != "" {
				return s, sc.sdcpp, true
			}
		}
	}
	return "", "", false
}

func Pair(sampler, scheduler string) (outSampler, outScheduler string, unknown []string) {
	s, embedded, ok := SplitSampler(sampler)
	if !ok {
		unknown = append(unknown, "sampler "+sampler)
		s = ""
	}
	sc, ok := Scheduler(scheduler)
	if !ok {
		unknown = append(unknown, "scheduler "+scheduler)
		sc = ""
	}
	if strings.TrimSpace(scheduler) == "" && embedded != "" {
		sc = embedded
	}
	return s, sc, unknown
}

func Upscaler(name string) string {
	n := strings.TrimSpace(name)
	if n == "" || strings.EqualFold(n, "None") {
		return ""
	}
	if strings.HasPrefix(n, "Latent") {
		return "latent"
	}
	if s, ok := upscalerAliases[n]; ok {
		return s
	}
	return n
}
