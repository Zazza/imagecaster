package people

import (
	"strings"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

const defaultBase = "Z-Image"

func Key(p serverclient.Person) string {
	if d := strings.TrimSpace(p.DisplayName); d != "" {
		return d
	}
	return p.Name
}

func Find(list []serverclient.Person, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	for _, p := range list {
		if p.Name == name {
			return Key(p), true
		}
	}
	for _, p := range list {
		if strings.EqualFold(Key(p), name) {
			return Key(p), true
		}
	}
	return "", false
}

func BaseOf(p serverclient.Person) string {
	return baseOf(p)
}

func baseOf(p serverclient.Person) string {
	if p.Base == "" {
		return defaultBase
	}
	return p.Base
}

func LorasByBase(list []serverclient.Person, key string) map[string]string {
	var out map[string]string
	for _, p := range list {
		if !strings.EqualFold(Key(p), key) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		if _, ok := out[baseOf(p)]; !ok {
			out[baseOf(p)] = p.Name
		}
	}
	return out
}

func FaceSwapName(list []serverclient.Person, key string, bases []string) string {
	for _, b := range bases {
		for _, p := range list {
			if strings.EqualFold(Key(p), key) && baseOf(p) == b && p.Identity {
				return p.Name
			}
		}
	}
	return ""
}
