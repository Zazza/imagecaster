package mcp

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/queue"
)

type swapEng struct {
	stubFaceEng
	mu      sync.Mutex
	inputs  []string
	persons []string
	img     string
	status  string
}

func (e *swapEng) FaceSwap(imgB64, person string) (string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inputs = append(e.inputs, imgB64)
	e.persons = append(e.persons, person)
	if e.img == "" {
		return imgB64, e.status
	}
	return e.img, e.status
}

func TestRunShootFrame_FaceSwap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		on         bool
		eng        StylistEngine
		wantStatus string
		wantSaved  string
		wantCalls  int
	}{
		{name: "default off", on: false, eng: &swapEng{img: "swapped", status: "done"}, wantStatus: shootSwapOff, wantCalls: 0},
		{name: "done", on: true, eng: &swapEng{img: "swapped", status: "done"}, wantStatus: "done", wantSaved: "swapped", wantCalls: 1},
		{name: "skipped keeps frame", on: true, eng: &swapEng{status: "skipped:лицо не найдено"}, wantStatus: "skipped:лицо не найдено", wantCalls: 1},
		{name: "engine without swap", on: true, eng: &stubFaceEng{}, wantStatus: shootSwapMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shooter := &fakeShooter{}
			sess := &fakeSessions{}
			refiner := &fakeRefiner{size: 256}
			p := sqFrameParams()
			p.FacePass = true
			p.FaceSwap = tt.on

			res, err := RunShootFrame(shooter, sess, tt.eng, refiner, p)

			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, res.FaceSwap)
			assert.Equal(t, shootFaceDone, res.FacePass)
			named := sess.namedCalls()
			require.Len(t, named, 1)
			raw := shooter.genImages()[0]
			if tt.wantSaved != "" {
				assert.Equal(t, tt.wantSaved, named[0].image)
			} else {
				assert.NotEqual(t, raw, named[0].image, "frame after face pass is saved")
			}
			if se, ok := tt.eng.(*swapEng); ok {
				require.Len(t, se.inputs, tt.wantCalls)
				if tt.wantCalls > 0 {
					assert.NotEqual(t, raw, se.inputs[0], "swap runs last, after the face pass")
					assert.Equal(t, "anna", se.persons[0])
					if tt.wantSaved == "" {
						assert.Equal(t, se.inputs[0], named[0].image, "skipped swap keeps the frame")
					}
				}
			}
		})
	}
}

func TestStylistShootQueue_FaceSwapFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		set  any
		want bool
	}{
		{name: "on", set: true, want: true},
		{name: "off", set: false, want: false},
		{name: "default off", set: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := sqNewEnv(t, nil, nil)
			args := shootArgs("anna", "front")
			delete(args, "face_swap")
			if tt.set != nil {
				args["face_swap"] = tt.set
			}

			m, _ := styCall(t, env.s, "stylist_shoot", args)

			ids := sqJobIDs(t, m)
			require.Len(t, ids, 1)
			_, p := sqJob(t, env.store, ids[0])
			assert.Equal(t, tt.want, p.FaceSwap)
		})
	}
}

func TestStylistShootStatus_FaceSwap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result string
		want   string
	}{
		{name: "skipped", result: `{"frame":{"id":7,"shot":"front","prompt":"p","face_pass":"off","face_swap":"skipped:лицо не найдено"}}`, want: "skipped:лицо не найдено"},
		{name: "done", result: `{"frame":{"id":7,"shot":"front","prompt":"p","face_pass":"off","face_swap":"done"}}`, want: "done"},
		{name: "older frame without field", result: `{"frame":{"id":7,"shot":"front","prompt":"p","face_pass":"off"}}`, want: "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := sqNewEnv(t, nil, nil)
			p := ShootFrameParams{Person: "anna", Base: "Z-Image", Model: "z-image-turbo-fp8-aio", Shot: "front", Prompt: "p", Index: 1, Total: 1}
			id, err := env.store.CreateLabeled(queue.JobStylistFrame, sqParamsJSON(t, p), "stylist", "Стилист: anna, front (1 из 1)")
			require.NoError(t, err)
			require.NoError(t, env.store.UpdateCompleted(id, tt.result))

			m, out := styCall(t, env.s, "stylist_shoot_status", map[string]any{"job_ids": sqIDsArg(id)})

			frames := sqFrames(t, m)
			require.Len(t, frames, 1, out)
			assert.Equal(t, tt.want, frames[0]["face_swap"])
		})
	}
}

func TestStylistShootSchema_FaceSwap(t *testing.T) {
	t.Parallel()
	env := sqNewEnv(t, nil, nil)
	var found bool
	for _, tool := range env.s.tools {
		if tool.Name != "stylist_shoot" {
			continue
		}
		props, _ := tool.InputSchema["properties"].(map[string]any)
		require.Contains(t, props, "face_swap")
		found = true
	}
	assert.True(t, found)
}

func TestShootFaceSwap_InterruptStopsSeries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		status      string
		wantStopped bool
	}{
		{"interrupted", generation.FaceSwapInterrupted, true},
		{"skipped", generation.FaceSwapSkipped("no_face"), false},
		{"done", generation.FaceSwapDone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := frameOut{img: "frame"}
			shootFaceSwap(&swapEng{status: tt.status}, &out, "anna", true)
			assert.Equal(t, tt.status, out.faceSwap)
			assert.Equal(t, tt.wantStopped, out.stopped)
		})
	}
}
