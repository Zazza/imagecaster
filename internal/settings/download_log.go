package settings

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Zazza/imagecaster/internal/logger"
)

var percentRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)

type downloadLog struct {
	log      *logger.Logger
	what     string
	lastPct  float64
	logged   int
	lastText string
}

func newDownloadLog(log *logger.Logger, what string) *downloadLog {
	d := &downloadLog{log: log, what: what, lastPct: -1}
	if log != nil {
		log.Info("Download started: %s", what)
	}
	return d
}

func (d *downloadLog) progress(data string) {
	if d.log == nil {
		return
	}
	pct, text, ok := parseDownloadProgress(data)
	if !ok {
		if text != "" && text != d.lastText {
			d.lastText = text
			d.log.Info("Download %s: %s", d.what, text)
		}
		return
	}
	step := int(pct) / 10
	switch {
	case d.lastPct >= 0 && pct < d.lastPct:
		d.logged = step
		d.log.Info("Download %s: %s", d.what, text)
	case step > d.logged:
		d.logged = step
		d.log.Info("Download %s: %s", d.what, text)
	}
	d.lastPct = pct
}

func (d *downloadLog) finish(err error) {
	if d.log == nil {
		return
	}
	if err != nil {
		d.log.Error("Download failed: %s: %v", d.what, err)
		return
	}
	d.log.Info("Download done: %s", d.what)
}

func parseDownloadProgress(data string) (float64, string, bool) {
	s := strings.TrimSpace(data)
	if strings.HasPrefix(s, "{") {
		var p struct {
			Downloaded *float64 `json:"downloaded"`
			Total      float64  `json:"total"`
			Percent    *float64 `json:"percent"`
		}
		if json.Unmarshal([]byte(s), &p) == nil && p.Downloaded != nil {
			if p.Total <= 0 {
				return 0, "", false
			}
			pct := *p.Downloaded / p.Total * 100
			if p.Percent != nil {
				pct = *p.Percent
			}
			pct = clampPercent(pct)
			return pct, fmt.Sprintf("%d%% (%s / %s)", int(pct), sizeText(*p.Downloaded), sizeText(p.Total)), true
		}
	}
	m := percentRe.FindAllStringSubmatch(s, -1)
	if len(m) == 0 {
		return 0, s, false
	}
	pct, err := strconv.ParseFloat(m[len(m)-1][1], 64)
	if err != nil {
		return 0, s, false
	}
	return clampPercent(pct), s, true
}

func clampPercent(p float64) float64 {
	return max(0, min(100, p))
}

func sizeText(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", b/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", b/(1<<20))
	}
	return fmt.Sprintf("%.0f B", b)
}
