package queue

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
)

func TestProcessGeneration_QualityFlagsInJobResult(t *testing.T) {
	cases := []struct {
		name              string
		result            generation.GenerateImageResult
		wantUpscaleSkip   bool
		wantRedenoiseSkip bool
	}{
		{
			name: "no skips",
		},
		{
			name:            "upscale skipped",
			result:          generation.GenerateImageResult{QualityUpscaleSkipped: true},
			wantUpscaleSkip: true,
		},
		{
			name:              "redenoise skipped",
			result:            generation.GenerateImageResult{QualityRedenoiseSkipped: true},
			wantRedenoiseSkip: true,
		},
		{
			name:              "both skipped",
			result:            generation.GenerateImageResult{QualityUpscaleSkipped: true, QualityRedenoiseSkipped: true},
			wantUpscaleSkip:   true,
			wantRedenoiseSkip: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &processor{emit: &mockEmitter{}}
			res, err := p.processGeneration(&Job{}, func() (*generation.GenerateImageResult, error) {
				return &tc.result, nil
			})
			if err != nil {
				t.Fatalf("processGeneration returned error: %v", err)
			}
			if res.QualityUpscaleSkipped != tc.wantUpscaleSkip {
				t.Errorf("QualityUpscaleSkipped = %v, want %v", res.QualityUpscaleSkipped, tc.wantUpscaleSkip)
			}
			if res.QualityRedenoiseSkipped != tc.wantRedenoiseSkip {
				t.Errorf("QualityRedenoiseSkipped = %v, want %v", res.QualityRedenoiseSkipped, tc.wantRedenoiseSkip)
			}
		})
	}
}

func TestEventPayload_QualityFlagsSerialization(t *testing.T) {
	cases := []struct {
		name   string
		result *JobResult
		want   []string
	}{
		{
			name:   "flags present",
			result: &JobResult{QualityUpscaleSkipped: true, QualityRedenoiseSkipped: true},
			want:   []string{`"quality_upscale_skipped":true`, `"quality_redenoise_skipped":true`},
		},
		{
			name:   "flags omitted",
			result: &JobResult{ImageBase64: "img"},
			want:   []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(EventPayload{JobID: 1, Result: tc.result})
			if err != nil {
				t.Fatalf("marshal EventPayload: %v", err)
			}
			encoded := string(data)
			for _, w := range tc.want {
				if !strings.Contains(encoded, w) {
					t.Errorf("payload JSON %s does not contain %s", encoded, w)
				}
			}
			if tc.name == "flags omitted" {
				if strings.Contains(encoded, "quality_upscale_skipped") || strings.Contains(encoded, "quality_redenoise_skipped") {
					t.Errorf("payload JSON %s contains omitted quality flags", encoded)
				}
			}
		})
	}
}

func TestProcessJob_LegacyCompareItemIsUnknownJobType(t *testing.T) {
	p := NewProcessor(nil, nil, t.TempDir(), &mockEmitter{})
	job := &Job{Type: JobType("compare_item"), Params: `{"mode":"compounds"}`}

	var res *JobResult
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ProcessJob panicked on legacy compare_item job: %v", r)
			}
		}()
		res, err = p.ProcessJob(context.Background(), job)
	}()

	if err == nil {
		t.Fatal("expected error for legacy compare_item job, got nil")
	}
	if !strings.Contains(err.Error(), "unknown job type") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown job type")
	}
	if res != nil {
		t.Errorf("result = %+v, want nil", res)
	}
}
