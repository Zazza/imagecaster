package promptutil

import "strings"

var proseBases = map[string]bool{"Z-Image": true, "Moss-Image": true, "Flux.1": true, "Flux.2": true}

func ProseBase(base string) bool {
	return proseBases[base]
}

func JoinProse(scene, style string) string {
	scene, style = strings.TrimSpace(scene), strings.TrimSpace(style)
	if scene == "" || style == "" {
		return scene + style
	}
	if !strings.HasSuffix(scene, ".") && !strings.HasSuffix(scene, "!") && !strings.HasSuffix(scene, "?") {
		scene += "."
	}
	return scene + " " + style
}
