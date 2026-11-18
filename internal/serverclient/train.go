package serverclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type TrainProgress struct {
	Step  int `json:"step"`
	Steps int `json:"steps"`
}

type TrainError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TrainJob struct {
	ID         string        `json:"id"`
	Client     string        `json:"client"`
	ClientRef  string        `json:"client_ref"`
	Name       string        `json:"name"`
	Trigger    string        `json:"trigger"`
	Gender     string        `json:"gender"`
	Base       string        `json:"base"`
	Photos     int           `json:"photos"`
	Status     string        `json:"status"`
	Progress   TrainProgress `json:"progress"`
	Error      *TrainError   `json:"error"`
	CreatedAt  *time.Time    `json:"created_at"`
	StartedAt  *time.Time    `json:"started_at"`
	FinishedAt *time.Time    `json:"finished_at"`
}

type Person struct {
	Name      string     `json:"name"`
	Trigger   string     `json:"trigger"`
	Gender    string     `json:"gender"`
	Base      string     `json:"base"`
	Lora      string     `json:"lora"`
	Steps     int        `json:"steps"`
	CreatedAt *time.Time `json:"created_at"`
}

type TrainRequest struct {
	Name      string
	Gender    string
	Base      string
	Client    string
	ClientRef string
	Photos    []string
}

type TrainAPIError struct {
	Status  int
	Code    string
	Message string
}

func (e *TrainAPIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("training: %s (%s)", e.Message, e.Code)
	}
	return fmt.Sprintf("training: HTTP %d %s", e.Status, e.Code)
}

var uploadHTTPClient = &http.Client{}

func (c *Client) trainDo(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	hc := c.httpClient
	if body != nil && contentType != "application/json" {
		hc = uploadHTTPClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxServerResponseBodySize))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error TrainError `json:"error"`
		}
		apiErr := &TrainAPIError{Status: resp.StatusCode}
		if json.Unmarshal(data, &e) == nil {
			apiErr.Code, apiErr.Message = e.Error.Code, e.Error.Message
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) SubmitTraining(ctx context.Context, r TrainRequest) (*TrainJob, error) {
	for _, p := range r.Photos {
		if _, err := os.Stat(p); err != nil {
			return nil, err
		}
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		pw.CloseWithError(writeTrainForm(mw, r))
	}()
	var job TrainJob
	err := c.trainDo(ctx, http.MethodPost, "/api/train/jobs", pr, mw.FormDataContentType(), &job)
	pr.Close()
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func writeTrainForm(mw *multipart.Writer, r TrainRequest) error {
	for _, f := range [][2]string{{"name", r.Name}, {"gender", r.Gender}, {"base", r.Base}, {"client", r.Client}, {"client_ref", r.ClientRef}} {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			return err
		}
	}
	for _, p := range r.Photos {
		if err := writePhotoPart(mw, p); err != nil {
			return err
		}
	}
	return mw.Close()
}

func writePhotoPart(mw *multipart.Writer, path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	part, err := mw.CreateFormFile("photos", filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, in)
	return err
}

func (c *Client) TrainingJob(ctx context.Context, id string) (*TrainJob, error) {
	var job TrainJob
	if err := c.trainDo(ctx, http.MethodGet, "/api/train/jobs/"+url.PathEscape(id), nil, "", &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) TrainingJobs(ctx context.Context, client string) ([]TrainJob, error) {
	var out struct {
		Jobs []TrainJob `json:"jobs"`
	}
	if err := c.trainDo(ctx, http.MethodGet, "/api/train/jobs?client="+url.QueryEscape(client), nil, "", &out); err != nil {
		return nil, err
	}
	if out.Jobs == nil {
		out.Jobs = []TrainJob{}
	}
	return out.Jobs, nil
}

func (c *Client) CancelTraining(ctx context.Context, id string) (*TrainJob, error) {
	var job TrainJob
	if err := c.trainDo(ctx, http.MethodPost, "/api/train/jobs/"+url.PathEscape(id)+"/cancel", nil, "", &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) PauseTraining(ctx context.Context, id string) (*TrainJob, error) {
	return c.trainJobAction(ctx, id, "pause")
}

func (c *Client) ResumeTraining(ctx context.Context, id string) (*TrainJob, error) {
	return c.trainJobAction(ctx, id, "resume")
}

func (c *Client) trainJobAction(ctx context.Context, id, action string) (*TrainJob, error) {
	var job TrainJob
	if err := c.trainDo(ctx, http.MethodPost, "/api/train/jobs/"+url.PathEscape(id)+"/"+action, nil, "", &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) RenamePerson(ctx context.Context, oldName, newName string) (*Person, error) {
	body, err := json.Marshal(map[string]string{"name": newName})
	if err != nil {
		return nil, err
	}
	var out struct {
		Person Person `json:"person"`
	}
	if err := c.trainDo(ctx, http.MethodPost, "/api/train/people/"+url.PathEscape(oldName)+"/rename", bytes.NewReader(body), "application/json", &out); err != nil {
		return nil, err
	}
	return &out.Person, nil
}

func (c *Client) People(ctx context.Context) ([]Person, error) {
	var out struct {
		People []Person `json:"people"`
	}
	if err := c.trainDo(ctx, http.MethodGet, "/api/train/people", nil, "", &out); err != nil {
		return nil, err
	}
	if out.People == nil {
		out.People = []Person{}
	}
	return out.People, nil
}

func (c *Client) DeletePerson(ctx context.Context, name string) error {
	return c.trainDo(ctx, http.MethodDelete, "/api/train/people/"+url.PathEscape(name), nil, "", nil)
}

func (c *Client) TrainingBases(ctx context.Context) ([]string, error) {
	var out struct {
		Bases []string `json:"bases"`
	}
	if err := c.trainDo(ctx, http.MethodGet, "/api/train/bases", nil, "", &out); err != nil {
		return nil, err
	}
	if out.Bases == nil {
		out.Bases = []string{}
	}
	return out.Bases, nil
}
