package sd

import (
	"errors"
	"fmt"
)

const bodyPoints = 18

var ErrNoBody = errors.New("body keypoints: no person found")

type BodyDetector interface {
	BodyKeypoints(imageBase64 string) ([]Point, error)
}

var _ BodyDetector = (*Client)(nil)

func (c *Client) BodyKeypoints(imageBase64 string) ([]Point, error) {
	out, w, h, err := c.detectPose(imageBase64)
	if err != nil {
		return nil, fmt.Errorf("body keypoints: %w", err)
	}
	if len(out.Poses) == 0 || len(out.Poses[0].People) == 0 {
		return nil, ErrNoBody
	}
	p := out.Poses[0]
	if p.CanvasWidth <= 0 || p.CanvasHeight <= 0 {
		return nil, errors.New("body keypoints: invalid canvas size")
	}
	raw := p.People[0].Pose
	scale := min(p.CanvasWidth/float64(w), p.CanvasHeight/float64(h))
	cw, ch := float64(w)*scale, float64(h)*scale
	pts := make([]Point, bodyPoints)
	found := false
	for i := range pts {
		pts[i] = Point{X: -1, Y: -1}
		if (i+1)*3 > len(raw) || raw[i*3+2] <= 0 {
			continue
		}
		pts[i] = Point{X: raw[i*3] / cw, Y: raw[i*3+1] / ch}
		found = true
	}
	if !found {
		return nil, ErrNoBody
	}
	return pts, nil
}
