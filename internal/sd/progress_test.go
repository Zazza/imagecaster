package sd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetProgress_Steps(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantTotal   int
		wantCurrent int
	}{
		{
			name:        "forge format sampling_step and sampling_steps",
			body:        `{"progress":0.5,"eta_relative":3.2,"state":{"job":"job1","job_count":1,"job_no":0,"sampling_step":10,"sampling_steps":20},"current_image":""}`,
			wantTotal:   20,
			wantCurrent: 10,
		},
		{
			name:        "legacy format state.sampling.steps",
			body:        `{"state":{"sampling":{"steps":30}}}`,
			wantTotal:   30,
			wantCurrent: 0,
		},
		{
			name:        "both formats prefer sampling_steps",
			body:        `{"state":{"sampling_step":5,"sampling_steps":20,"sampling":{"steps":30}}}`,
			wantTotal:   20,
			wantCurrent: 5,
		},
		{
			name:        "empty state",
			body:        `{"progress":0,"state":{}}`,
			wantTotal:   0,
			wantCurrent: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/sdapi/v1/progress" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			resp, err := New(srv.URL).GetProgress()
			if err != nil {
				t.Fatalf("GetProgress: %v", err)
			}
			if resp == nil {
				t.Fatal("GetProgress returned nil response")
			}
			if got := resp.TotalSteps(); got != tc.wantTotal {
				t.Errorf("TotalSteps() = %d, want %d", got, tc.wantTotal)
			}
			if got := resp.CurrentStep(); got != tc.wantCurrent {
				t.Errorf("CurrentStep() = %d, want %d", got, tc.wantCurrent)
			}
		})
	}
}
