package sd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	faceDetectModule  = "dw_openpose_full"
	faceDetectRes     = 1024
	faceDetectTimeout = 120 * time.Second
	faceMinPoints     = 68
)

type Point struct {
	X, Y float64
}

type FaceDetector interface {
	FaceLandmarks(imageBase64 string) ([]Point, error)
}

var _ FaceDetector = (*Client)(nil)

type detectResponse struct {
	Poses []struct {
		CanvasWidth  float64 `json:"canvas_width"`
		CanvasHeight float64 `json:"canvas_height"`
		People       []struct {
			Face []float64 `json:"face_keypoints_2d"`
		} `json:"people"`
	} `json:"poses"`
}

func imageSize(b64 string) (int, int, error) {
	if i := strings.Index(b64, ","); i >= 0 && strings.HasPrefix(b64, "data:") {
		b64 = b64[i+1:]
	}
	cfg, _, err := image.DecodeConfig(base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64)))
	if err != nil {
		return 0, 0, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, errors.New("empty image")
	}
	return cfg.Width, cfg.Height, nil
}

func (c *Client) FaceLandmarks(imageBase64 string) ([]Point, error) {
	w, h, err := imageSize(imageBase64)
	if err != nil {
		return nil, fmt.Errorf("face landmarks: decode image: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"controlnet_module":        faceDetectModule,
		"controlnet_input_images":  []string{imageBase64},
		"controlnet_processor_res": faceDetectRes,
	})
	if err != nil {
		return nil, fmt.Errorf("face landmarks: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), faceDetectTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/controlnet/detect", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("face landmarks: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("face landmarks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("face landmarks: status %d", resp.StatusCode)
	}
	var out detectResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodySize)).Decode(&out); err != nil {
		return nil, fmt.Errorf("face landmarks: decode: %w", err)
	}
	if len(out.Poses) == 0 || len(out.Poses[0].People) == 0 {
		return nil, errors.New("face landmarks: no face found")
	}
	p := out.Poses[0]
	if p.CanvasWidth <= 0 || p.CanvasHeight <= 0 {
		return nil, errors.New("face landmarks: invalid canvas size")
	}
	raw := p.People[0].Face
	if len(raw)/3 < faceMinPoints {
		return nil, fmt.Errorf("face landmarks: %d points, need %d", len(raw)/3, faceMinPoints)
	}
	scale := min(p.CanvasWidth/float64(w), p.CanvasHeight/float64(h))
	cw, ch := float64(w)*scale, float64(h)*scale
	pts := make([]Point, len(raw)/3)
	for i := range pts {
		pts[i] = Point{X: raw[i*3] / cw, Y: raw[i*3+1] / ch}
	}
	return pts, nil
}
