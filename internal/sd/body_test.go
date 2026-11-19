package sd

import (
	"errors"
	"math"
	"net/http"
	"testing"
)

func bodyKeypoints18(set map[int][2]float64) []float64 {
	out := make([]float64, 0, 18*3)
	for i := 0; i < 18; i++ {
		if p, ok := set[i]; ok {
			out = append(out, p[0], p[1], 0.9)
		} else {
			out = append(out, 0, 0, 0)
		}
	}
	return out
}

func bodyPoseResponse(width, height int, people []map[string]any) map[string]any {
	return map[string]any{
		"images": []string{"AAAA"},
		"poses": []map[string]any{{
			"canvas_width":  width,
			"canvas_height": height,
			"people":        people,
		}},
	}
}

func TestBodyKeypoints_FractionsAndMissing(t *testing.T) {
	t.Parallel()
	const cw, ch = 1024, 1536
	const imgW, imgH = 400, 590
	img := facePNG(t, imgW, imgH)
	kp := bodyKeypoints18(map[int][2]float64{1: {512, 300}, 2: {400, 320}, 5: {620, 320}, 8: {450, 800}, 11: {580, 800}})
	var req faceDetectRequest
	var path string
	srv := faceServer(t, http.StatusOK, bodyPoseResponse(cw, ch, []map[string]any{{"pose_keypoints_2d": kp}}), &req, &path)

	pts, err := New(srv.URL).BodyKeypoints(img)

	if err != nil {
		t.Fatalf("BodyKeypoints: %v", err)
	}
	if path != "POST /controlnet/detect" {
		t.Errorf("request = %q, want POST /controlnet/detect", path)
	}
	if req.Module != "dw_openpose_full" {
		t.Errorf("controlnet_module = %q, want dw_openpose_full", req.Module)
	}
	if len(req.InputImages) != 1 || req.InputImages[0] != img {
		t.Errorf("controlnet_input_images must be [input image], got %d images", len(req.InputImages))
	}
	if len(pts) != 18 {
		t.Fatalf("got %d points, want 18", len(pts))
	}
	s := math.Min(float64(cw)/imgW, float64(ch)/imgH)
	present := map[int][2]float64{1: {512, 300}, 2: {400, 320}, 5: {620, 320}, 8: {450, 800}, 11: {580, 800}}
	for i, p := range pts {
		want, ok := present[i]
		if !ok {
			if p.X != -1 || p.Y != -1 {
				t.Errorf("missing point %d = %+v, want {-1,-1}", i, p)
			}
			continue
		}
		wx, wy := want[0]/(imgW*s), want[1]/(imgH*s)
		if math.Abs(p.X-wx) > 1e-9 || math.Abs(p.Y-wy) > 1e-9 {
			t.Errorf("point %d = (%v,%v), want (%v,%v)", i, p.X, p.Y, wx, wy)
		}
	}
}

func TestBodyKeypoints_CardExample(t *testing.T) {
	t.Parallel()
	kp := bodyKeypoints18(map[int][2]float64{1: {512, 768}})
	srv := faceServer(t, http.StatusOK, bodyPoseResponse(1024, 1536, []map[string]any{{"pose_keypoints_2d": kp}}), nil, nil)

	pts, err := New(srv.URL).BodyKeypoints(facePNG(t, 400, 600))

	if err != nil {
		t.Fatalf("BodyKeypoints: %v", err)
	}
	if math.Abs(pts[1].X-0.5) > 1e-9 || math.Abs(pts[1].Y-0.5) > 1e-9 {
		t.Errorf("neck = %+v, want (0.5,0.5)", pts[1])
	}
}

func TestBodyKeypoints_FirstPerson(t *testing.T) {
	t.Parallel()
	first := bodyKeypoints18(map[int][2]float64{1: {100, 150}})
	second := bodyKeypoints18(map[int][2]float64{1: {900, 1400}})
	srv := faceServer(t, http.StatusOK, bodyPoseResponse(1024, 1536, []map[string]any{{"pose_keypoints_2d": first}, {"pose_keypoints_2d": second}}), nil, nil)

	pts, err := New(srv.URL).BodyKeypoints(facePNG(t, 400, 600))

	if err != nil {
		t.Fatalf("BodyKeypoints: %v", err)
	}
	if math.Abs(pts[1].X-100.0/1024) > 1e-9 || math.Abs(pts[1].Y-150.0/1536) > 1e-9 {
		t.Errorf("neck = %+v, want the first person", pts[1])
	}
}

func TestBodyKeypoints_NoBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body any
	}{
		{name: "no people", body: bodyPoseResponse(1024, 1536, []map[string]any{})},
		{name: "people null", body: bodyPoseResponse(1024, 1536, nil)},
		{name: "no poses", body: map[string]any{"images": []string{"AAAA"}, "poses": []any{}}},
		{name: "all points zero", body: bodyPoseResponse(1024, 1536, []map[string]any{{"pose_keypoints_2d": bodyKeypoints18(nil)}})},
		{name: "no body keypoints", body: bodyPoseResponse(1024, 1536, []map[string]any{{"face_keypoints_2d": faceKeypoints(70)}})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := faceServer(t, http.StatusOK, tc.body, nil, nil)
			pts, err := New(srv.URL).BodyKeypoints(facePNG(t, 400, 600))
			if !errors.Is(err, ErrNoBody) {
				t.Fatalf("want ErrNoBody, got %v (%d points)", err, len(pts))
			}
		})
	}
}

func TestBodyKeypoints_ServerErrorIsNotNoBody(t *testing.T) {
	t.Parallel()
	srv := faceServer(t, http.StatusInternalServerError, map[string]any{"detail": "boom"}, nil, nil)
	_, err := New(srv.URL).BodyKeypoints(facePNG(t, 400, 600))
	if err == nil {
		t.Fatal("want error")
	}
	if errors.Is(err, ErrNoBody) {
		t.Errorf("server error must not be ErrNoBody: %v", err)
	}
}

func TestBodyKeypoints_InvalidImage(t *testing.T) {
	t.Parallel()
	srv := faceServer(t, http.StatusOK, bodyPoseResponse(1024, 1536, []map[string]any{{"pose_keypoints_2d": bodyKeypoints18(map[int][2]float64{1: {1, 1}})}}), nil, nil)
	for _, in := range []string{"", "IMG"} {
		if _, err := New(srv.URL).BodyKeypoints(in); err == nil {
			t.Errorf("input %q: want error", in)
		}
	}
}
