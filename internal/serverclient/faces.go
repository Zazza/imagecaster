package serverclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

type FaceInfo struct {
	Box        [4]float64 `json:"box"`
	HeightPx   int        `json:"height_px"`
	Score      float64    `json:"score"`
	Yaw        float64    `json:"yaw"`
	Sharpness  float64    `json:"sharpness"`
	Embedding  []float32  `json:"embedding"`
	Similarity *float64   `json:"similarity,omitempty"`
}

type FacesResult struct {
	Faces  []FaceInfo `json:"faces"`
	Width  int        `json:"width"`
	Height int        `json:"height"`
}

func (c *Client) AnalyzeFaces(ctx context.Context, imageBase64 string, refs [][]float32) (*FacesResult, error) {
	body, err := json.Marshal(map[string]any{"image_base64": imageBase64, "refs": refs})
	if err != nil {
		return nil, err
	}
	var out FacesResult
	if err := c.trainDo(ctx, http.MethodPost, "/api/train/faces", bytes.NewReader(body), "application/json", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
