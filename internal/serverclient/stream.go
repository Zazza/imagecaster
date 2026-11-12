package serverclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var ErrStreamLost = errors.New("connection to server lost")

var streamHTTPClient = &http.Client{}

func (c *Client) DownloadModelStream(modelType, downloadURL, filename string, onProgress func(string)) error {
	if err := sanitizePathSegment(filename); err != nil {
		return err
	}
	q := url.Values{"type": {modelType}, "url": {downloadURL}, "filename": {filename}}
	return c.stream("/api/server/models/download/stream?"+q.Encode(), onProgress)
}

func (c *Client) PullLLMModelStream(name string, onProgress func(string)) error {
	q := url.Values{"name": {name}}
	return c.stream("/api/server/models/llm/pull?"+q.Encode(), onProgress)
}

func (c *Client) stream(path string, onProgress func(string)) error {
	if c.baseURL == "" {
		return fmt.Errorf("server not configured")
	}
	resp, err := streamHTTPClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("server unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var errResp map[string]string
		if json.Unmarshal(body, &errResp) == nil && errResp["error"] != "" {
			return fmt.Errorf("server: %s", errResp["error"])
		}
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch {
		case data == "[DONE]":
			return nil
		case strings.HasPrefix(data, "[ERROR]"):
			return errors.New(strings.TrimSpace(strings.TrimPrefix(data, "[ERROR]")))
		case onProgress != nil:
			onProgress(data)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrStreamLost, err)
	}
	return ErrStreamLost
}
