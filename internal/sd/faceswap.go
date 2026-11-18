package sd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const (
	FaceSwapDone    = "done"
	FaceSwapSkipped = "skipped"
)

type FaceSwapParams struct {
	Image        string  `json:"image"`
	Person       string  `json:"person"`
	RestoreBlend float64 `json:"restore_blend"`
}

type FaceSwapResult struct {
	Image  string
	Status string
	Skip   string
	FacePx int
	Yaw    int
}

func faceSwapJob(imgB64, person string, blend float64) JobRequest {
	return JobRequest{Kind: KindFaceSwap, Params: FaceSwapParams{Image: imgB64, Person: person, RestoreBlend: blend}}
}

func faceSwapResult(res *JobResult) (*FaceSwapResult, error) {
	var info struct {
		FaceSwap string  `json:"face_swap"`
		Skip     string  `json:"skip"`
		FacePx   float64 `json:"face_px"`
		Yaw      float64 `json:"yaw"`
	}
	if len(res.Info) > 0 {
		if err := json.Unmarshal(res.Info, &info); err != nil {
			return nil, fmt.Errorf("face swap: read info: %w", err)
		}
	}
	out := &FaceSwapResult{Skip: info.Skip, FacePx: int(math.Round(info.FacePx)), Yaw: int(math.Round(info.Yaw))}
	switch {
	case info.FaceSwap == FaceSwapSkipped || info.Skip != "":
		out.Status = FaceSwapSkipped
	case len(res.Images) > 0 && res.Images[0] != "":
		out.Status, out.Image = FaceSwapDone, res.Images[0]
	default:
		return nil, errors.New("face swap: empty response")
	}
	return out, nil
}

func FaceSwapErrorCode(err error) string {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	var je *JobError
	if errors.As(err, &je) {
		return je.Code
	}
	return ""
}

func (c *Client) FaceSwap(ctx context.Context, imgB64, person string, blend float64) (*FaceSwapResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cctx, cancel := context.WithCancelCause(ctx)
	key := c.trackDirect(cancel)
	defer func() {
		c.untrackDirect(key)
		cancel(nil)
	}()
	res, err := c.RunJob(cctx, faceSwapJob(imgB64, person, blend), c.progressHandler())
	if err != nil {
		return nil, fmt.Errorf("face swap: %w", err)
	}
	return faceSwapResult(res)
}

func (q *queueScoped) FaceSwap(ctx context.Context, imgB64, person string, blend float64) (*FaceSwapResult, error) {
	if ctx == nil {
		ctx = q.ctx
	}
	job := faceSwapJob(imgB64, person, blend)
	job.ClientRef = QueueClientRef(q.Client.InstallID(), q.queueJobID, q.seq.Add(1), job)
	res, err := q.Client.runJob(ctx, job, q.Client.progressHandler(), false)
	if err != nil {
		return nil, fmt.Errorf("face swap: %w", err)
	}
	return faceSwapResult(res)
}
