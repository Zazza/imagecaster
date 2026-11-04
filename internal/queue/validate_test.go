package queue

import (
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
)

func TestValidateJobParams(t *testing.T) {
	resID := int64(2)

	tests := []struct {
		name    string
		jobType JobType
		params  any
		wantErr string
	}{
		{"txt2img without preset", JobTxt2Img, generation.GenerateImageParams{}, "preset is required"},
		{"txt2img with preset", JobTxt2Img, generation.GenerateImageParams{PresetID: 5}, ""},
		{"compound without preset", JobCompound, generation.GenerateCompoundImageParams{}, "compound preset is required"},
		{"compound with preset", JobCompound, generation.GenerateCompoundImageParams{CompoundPresetID: 3}, ""},
		{"from image preset mode without preset", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset"}, "preset is required"},
		{"from image preset mode with preset", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset", PresetID: 5}, ""},
		{"from image compound mode without preset", JobFromImage, generation.GenerateFromImageParams{GenMode: "compound"}, "compound preset is required"},
		{"from image compound mode with preset", JobFromImage, generation.GenerateFromImageParams{GenMode: "compound", CompoundPresetID: 3}, ""},
		{"from image quality valid", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset", Mode: "img2img", PresetID: 5, OutputMode: "quality", ResolutionID: &resID}, ""},
		{"from image quality without resolution", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset", Mode: "img2img", PresetID: 5, OutputMode: "quality"}, "quality output mode requires img2img preset mode with resolution"},
		{"from image quality with txt2img mode", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset", Mode: "txt2img", PresetID: 5, OutputMode: "quality", ResolutionID: &resID}, "quality output mode requires img2img preset mode with resolution"},
		{"from image quality with compound gen mode", JobFromImage, generation.GenerateFromImageParams{GenMode: "compound", Mode: "img2img", CompoundPresetID: 3, OutputMode: "quality", ResolutionID: &resID}, "quality output mode requires img2img preset mode with resolution"},
		{"from image quality with txt2img mode", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset", Mode: "txt2img", PresetID: 5, OutputMode: "quality", ResolutionID: &resID}, "quality output mode requires img2img preset mode with resolution"},
		{"from image bad output mode", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset", PresetID: 5, OutputMode: "bad"}, "output_mode must be standard or quality"},
		{"from image empty gen mode", JobFromImage, generation.GenerateFromImageParams{}, ""},
		{"from image unknown gen mode", JobFromImage, generation.GenerateFromImageParams{GenMode: "analyze"}, ""},
		{"from image legacy inpaint mode rejected", JobFromImage, generation.GenerateFromImageParams{GenMode: "preset", Mode: "inpaint", PresetID: 5}, "mode must be txt2img or img2img"},
		{"legacy compare item passes validation", JobType("compare_item"), nil, ""},
		{"unknown params type untouched", JobTxt2Img, nil, ""},
		{"pointer params type untouched", JobTxt2Img, &generation.GenerateImageParams{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJobParams(tt.jobType, tt.params)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEnqueue_RejectsInvalidParams(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	emitter := &mockEmitter{}
	proc := &mockProcessor{}
	store := NewStore(db)
	svc := NewService(store, proc, emitter)

	_, err := svc.Enqueue(JobTxt2Img, generation.GenerateImageParams{}, "generate")
	if err == nil || err.Error() != "preset is required" {
		t.Fatalf("error = %v, want %q", err, "preset is required")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_queue`).Scan(&count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Errorf("job count = %d, want 0", count)
	}

	if emitter.hasEvent(EventQueueChanged) {
		t.Error("expected no EventQueueChanged for rejected job")
	}
}

func TestValidateJobParams_IdeaLength(t *testing.T) {
	ok := strings.Repeat("я", 4000)
	tooLong := strings.Repeat("я", 4001)

	tests := []struct {
		name    string
		jobType JobType
		params  any
		wantErr string
	}{
		{"txt2img idea 4000 runes", JobTxt2Img, generation.GenerateImageParams{PresetID: 5, Idea: ok, IdeaNegative: ok}, ""},
		{"txt2img idea 4001 runes", JobTxt2Img, generation.GenerateImageParams{PresetID: 5, Idea: tooLong}, "idea too long"},
		{"txt2img idea negative 4001 runes", JobTxt2Img, generation.GenerateImageParams{PresetID: 5, Idea: "cat", IdeaNegative: tooLong}, "idea"},
		{"compound idea 4000 runes", JobCompound, generation.GenerateCompoundImageParams{CompoundPresetID: 3, Idea: ok, IdeaNegative: ok}, ""},
		{"compound idea 4001 runes", JobCompound, generation.GenerateCompoundImageParams{CompoundPresetID: 3, Idea: tooLong}, "idea too long"},
		{"compound idea negative 4001 runes", JobCompound, generation.GenerateCompoundImageParams{CompoundPresetID: 3, Idea: "cat", IdeaNegative: tooLong}, "idea"},
		{"compound custom steps idea 4001 runes", JobCompound, generation.GenerateCompoundImageParams{Steps: []generation.CustomStep{{PresetID: 5}}, Idea: tooLong}, "idea too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJobParams(tt.jobType, tt.params)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEnqueue_RejectsTooLongIdea(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	emitter := &mockEmitter{}
	store := NewStore(db)
	svc := NewService(store, &mockProcessor{}, emitter)

	_, err := svc.Enqueue(JobTxt2Img, generation.GenerateImageParams{PresetID: 5, Idea: strings.Repeat("я", 4001)}, "generate")
	if err == nil || !strings.Contains(err.Error(), "idea too long") {
		t.Fatalf("error = %v, want it to contain %q", err, "idea too long")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_queue`).Scan(&count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Errorf("job count = %d, want 0", count)
	}
}
