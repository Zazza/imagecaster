package sd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

var _ FaceDetector = (*Client)(nil)

type faceDetectRequest struct {
	Module       string   `json:"controlnet_module"`
	InputImages  []string `json:"controlnet_input_images"`
	ProcessorRes int      `json:"controlnet_processor_res"`
}

func faceKeypoints(n int) []float64 {
	out := make([]float64, 0, n*3)
	for i := 0; i < n; i++ {
		out = append(out, float64(10*i+5), float64(20*i+7), 0.9)
	}
	return out
}

func facePoseResponse(width, height int, people []map[string]any) map[string]any {
	return map[string]any{
		"images": []string{"AAAA"},
		"poses": []map[string]any{{
			"canvas_width":  width,
			"canvas_height": height,
			"people":        people,
		}},
	}
}

func faceServer(t *testing.T, status int, body any, gotReq *faceDetectRequest, gotPath *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.Method + " " + r.URL.Path
		}
		raw, _ := io.ReadAll(r.Body)
		if gotReq != nil {
			_ = json.Unmarshal(raw, gotReq)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func facePNG(t *testing.T, w, h int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestFaceLandmarks_NormalizesByContentWithoutPadding(t *testing.T) {
	t.Parallel()
	const cw, ch = 1024, 1536
	const imgW, imgH = 400, 590
	img := facePNG(t, imgW, imgH)
	var req faceDetectRequest
	var path string
	body := facePoseResponse(cw, ch, []map[string]any{{"face_keypoints_2d": faceKeypoints(70)}})
	srv := faceServer(t, http.StatusOK, body, &req, &path)

	pts, err := New(srv.URL).FaceLandmarks(img)

	if err != nil {
		t.Fatalf("FaceLandmarks: %v", err)
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
	if req.ProcessorRes != 1024 {
		t.Errorf("controlnet_processor_res = %d, want 1024", req.ProcessorRes)
	}
	if len(pts) != 70 {
		t.Fatalf("got %d points, want 70", len(pts))
	}
	s := math.Min(float64(cw)/imgW, float64(ch)/imgH)
	for i, p := range pts {
		wantX := float64(10*i+5) / (imgW * s)
		wantY := float64(20*i+7) / (imgH * s)
		if math.Abs(p.X-wantX) > 1e-9 || math.Abs(p.Y-wantY) > 1e-9 {
			t.Errorf("point %d = (%v,%v), want (%v,%v)", i, p.X, p.Y, wantX, wantY)
		}
	}
}

func TestFaceLandmarks_CardExample(t *testing.T) {
	t.Parallel()
	kp := faceKeypoints(70)
	kp[0], kp[1] = 512, 768
	body := facePoseResponse(1024, 1536, []map[string]any{{"face_keypoints_2d": kp}})
	srv := faceServer(t, http.StatusOK, body, nil, nil)

	pts, err := New(srv.URL).FaceLandmarks(facePNG(t, 400, 600))

	if err != nil {
		t.Fatalf("FaceLandmarks: %v", err)
	}
	if math.Abs(pts[0].X-0.5) > 1e-9 || math.Abs(pts[0].Y-0.5) > 1e-9 {
		t.Errorf("point 0 = %+v, want (0.5,0.5)", pts[0])
	}
}

func TestFaceLandmarks_InvalidImage(t *testing.T) {
	t.Parallel()
	body := facePoseResponse(1024, 1536, []map[string]any{{"face_keypoints_2d": faceKeypoints(70)}})
	srv := faceServer(t, http.StatusOK, body, nil, nil)

	for _, in := range []string{"IMG", "", base64.StdEncoding.EncodeToString([]byte("not an image"))} {
		if pts, err := New(srv.URL).FaceLandmarks(in); err == nil {
			t.Errorf("input %q: want error, got %d points", in, len(pts))
		}
	}
}

func TestFaceLandmarks_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		body   any
	}{
		{name: "no people", status: http.StatusOK, body: facePoseResponse(1024, 1536, []map[string]any{})},
		{name: "people null", status: http.StatusOK, body: facePoseResponse(1024, 1536, nil)},
		{name: "no poses", status: http.StatusOK, body: map[string]any{"images": []string{"AAAA"}, "poses": []any{}}},
		{name: "poses missing", status: http.StatusOK, body: map[string]any{"images": []string{"AAAA"}}},
		{name: "fewer than 68 points", status: http.StatusOK, body: facePoseResponse(1024, 1536, []map[string]any{{"face_keypoints_2d": faceKeypoints(67)}})},
		{name: "no face keypoints", status: http.StatusOK, body: facePoseResponse(1024, 1536, []map[string]any{{"face_keypoints_2d": nil}})},
		{name: "zero canvas width", status: http.StatusOK, body: facePoseResponse(0, 1536, []map[string]any{{"face_keypoints_2d": faceKeypoints(70)}})},
		{name: "zero canvas height", status: http.StatusOK, body: facePoseResponse(1024, 0, []map[string]any{{"face_keypoints_2d": faceKeypoints(70)}})},
		{name: "status 500", status: http.StatusInternalServerError, body: map[string]any{"detail": "boom"}},
		{name: "status 404", status: http.StatusNotFound, body: map[string]any{"detail": "Not Found"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := faceServer(t, tt.status, tt.body, nil, nil)

			pts, err := New(srv.URL).FaceLandmarks(facePNG(t, 400, 600))

			if err == nil {
				t.Fatalf("want error, got %d points", len(pts))
			}
		})
	}
}

func TestFaceLandmarks_NotInService(t *testing.T) {
	t.Parallel()
	if _, ok := reflect.TypeOf((*Service)(nil)).Elem().MethodByName("FaceLandmarks"); ok {
		t.Error("FaceLandmarks must not be part of sd.Service")
	}
}
