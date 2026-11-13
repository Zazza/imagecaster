package settings

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dlTestPercentRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)

func dlTestMessagePercent(t *testing.T, msg string) (int, bool) {
	t.Helper()
	m := dlTestPercentRe.FindStringSubmatch(msg)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	require.NoError(t, err)
	return int(math.Round(f)), true
}

func dlTestJSON(p int) string {
	return fmt.Sprintf(`{"downloaded":%d,"total":1000,"percent":%d}`, p*10, p)
}

func dlTestJSONNoPercent(p int) string {
	return fmt.Sprintf(`{"downloaded":%d,"total":200}`, p*2)
}

func dlTestText(p int) string {
	return fmt.Sprintf("pulling abc123: %d%% 1.2 GB/6.0 GB", p)
}

func dlTestSequence() []int {
	seq := []int{0, 3, 9, 10}
	for p := 15; p <= 100; p += 5 {
		seq = append(seq, p)
	}
	return seq
}

func TestDownloadLog_Start(t *testing.T) {
	t.Parallel()
	log := logger.New(nil)
	newDownloadLog(log, "model flux-2-klein.gguf")
	got := log.Entries()
	require.Len(t, got, 1)
	assert.Equal(t, logger.INFO, got[0].Level)
	assert.Contains(t, got[0].Message, "flux-2-klein.gguf")
}

func TestDownloadLog_TenPercentSteps(t *testing.T) {
	t.Parallel()
	want := []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	tests := []struct {
		name   string
		format func(int) string
	}{
		{name: "json with percent", format: dlTestJSON},
		{name: "json downloaded and total only", format: dlTestJSONNoPercent},
		{name: "text with percent", format: dlTestText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := logger.New(nil)
			d := newDownloadLog(log, "lora x.safetensors")
			for _, p := range dlTestSequence() {
				d.progress(tt.format(p))
			}
			lines := log.Entries()[1:]
			require.Len(t, lines, len(want))
			for i, e := range lines {
				p, ok := dlTestMessagePercent(t, e.Message)
				require.True(t, ok, "progress line without percent: %q", e.Message)
				assert.Equal(t, want[i], p, e.Message)
			}
		})
	}
}

func TestDownloadLog_NewLinePerStep(t *testing.T) {
	t.Parallel()
	type step struct {
		p       int
		newLine bool
	}
	tests := []struct {
		name   string
		format func(int) string
		steps  []step
	}{
		{
			name:   "reset to zero starts new stage line json",
			format: dlTestJSON,
			steps: []step{
				{0, false}, {50, true}, {100, true},
				{0, true}, {5, false}, {10, true}, {100, true},
			},
		},
		{
			name:   "reset to zero starts new stage line text",
			format: dlTestText,
			steps: []step{
				{0, false}, {40, true}, {45, false}, {100, true},
				{0, true}, {9, false}, {30, true},
			},
		},
		{
			name:   "repeated percent does not repeat line",
			format: dlTestJSON,
			steps: []step{
				{20, true}, {20, false}, {20, false}, {29, false}, {30, true}, {100, true}, {100, false},
			},
		},
		{
			name:   "jump over several steps is one line",
			format: dlTestText,
			steps: []step{
				{0, false}, {73, true}, {79, false}, {80, true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := logger.New(nil)
			d := newDownloadLog(log, "llm qwen")
			before := len(log.Entries())
			for i, s := range tt.steps {
				d.progress(tt.format(s.p))
				after := len(log.Entries())
				if s.newLine {
					assert.Equal(t, before+1, after, "step %d (%d%%) must add exactly one line", i, s.p)
				} else {
					assert.Equal(t, before, after, "step %d (%d%%) must not add a line", i, s.p)
				}
				before = after
			}
		})
	}
}

func TestDownloadLog_TextWithoutPercentDoesNotSpam(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{name: "manifest text", data: "pulling manifest"},
		{name: "empty", data: ""},
		{name: "broken json", data: `{"downloaded":`},
		{name: "json with zero total", data: `{"downloaded":0,"total":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := logger.New(nil)
			d := newDownloadLog(log, "module x")
			start := len(log.Entries())
			for i := 0; i < 20; i++ {
				d.progress(tt.data)
			}
			assert.LessOrEqual(t, len(log.Entries())-start, 1)
		})
	}
}

func TestDownloadLog_Finish(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		wantLevel logger.Level
		wantText  string
	}{
		{name: "success", err: nil, wantLevel: logger.INFO, wantText: "done"},
		{name: "error", err: errors.New("disk full: no space left on device"), wantLevel: logger.ERROR, wantText: "disk full: no space left on device"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := logger.New(nil)
			d := newDownloadLog(log, "model y")
			d.progress(dlTestJSON(50))
			before := len(log.Entries())
			d.finish(tt.err)
			got := log.Entries()
			require.Len(t, got, before+1)
			last := got[len(got)-1]
			assert.Equal(t, tt.wantLevel, last.Level)
			assert.Contains(t, strings.ToLower(last.Message), strings.ToLower(tt.wantText))
		})
	}
}

func TestDownloadLog_UnknownSizeGrowingDoesNotSpam(t *testing.T) {
	log := logger.New(nil)
	d := newDownloadLog(log, "sd/unknown.safetensors")
	for i := 1; i <= 200; i++ {
		d.progress(fmt.Sprintf(`{"downloaded":%d,"total":0}`, i<<20))
	}
	d.finish(nil)
	assert.Len(t, log.Entries(), 2)
}
