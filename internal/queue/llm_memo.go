package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/sd"
)

type memoEntry struct {
	Text string `json:"t,omitempty"`
	Err  string `json:"e,omitempty"`
}

type llmMemo struct {
	llm.Service
	path    string
	mu      sync.Mutex
	seq     int
	m       map[string]memoEntry
	pending map[string]memoEntry
}

func llmMemoPath(dataDir string, jobID int64) string {
	return filepath.Join(dataDir, "queue-llm", fmt.Sprintf("%d.json", jobID))
}

func newLLMMemo(inner llm.Service, path string) *llmMemo {
	m := &llmMemo{Service: inner, path: path, m: map[string]memoEntry{}, pending: map[string]memoEntry{}}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &m.m); err != nil {
			log.Printf("[queue] llm memo %s unreadable, starting over: %v", path, err)
			m.m = map[string]memoEntry{}
		}
	case !errors.Is(err, os.ErrNotExist):
		log.Printf("[queue] llm memo %s: %v", path, err)
	}
	return m
}

func removeLLMMemo(dataDir string, jobID int64) {
	if dataDir == "" {
		return
	}
	if err := os.Remove(llmMemoPath(dataDir, jobID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("[queue] remove llm memo of job %d: %v", jobID, err)
	}
}

func (m *llmMemo) call(method string, args []any, f func() (string, error)) (string, error) {
	b, _ := json.Marshal(append([]any{method}, args...))
	sum := sha256.Sum256(b)
	m.mu.Lock()
	m.seq++
	key := fmt.Sprintf("%d:%s", m.seq, hex.EncodeToString(sum[:8]))
	if e, ok := m.m[key]; ok {
		m.mu.Unlock()
		if e.Err != "" {
			return "", errors.New(e.Err)
		}
		return e.Text, nil
	}
	m.mu.Unlock()
	v, err := f()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.pending[key] = memoEntry{Err: err.Error()}
		return v, err
	}
	m.m[key] = memoEntry{Text: v}
	m.saveLocked()
	return v, nil
}

func (m *llmMemo) commit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pending) == 0 {
		return
	}
	for k, e := range m.pending {
		m.m[k] = e
	}
	m.pending = map[string]memoEntry{}
	m.saveLocked()
}

type memoSD struct {
	sd.Service
	memo *llmMemo
}

func (s memoSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	s.memo.commit()
	return s.Service.Txt2Img(req)
}

func (s memoSD) Img2Img(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
	s.memo.commit()
	return s.Service.Img2Img(req)
}

func (s memoSD) UpscaleImage(base64Img string, upscaler string, scale float64) (string, error) {
	s.memo.commit()
	return s.Service.UpscaleImage(base64Img, upscaler, scale)
}

func (m *llmMemo) saveLocked() {
	b, err := json.Marshal(m.m)
	if err != nil {
		log.Printf("[queue] llm memo %s: %v", m.path, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		log.Printf("[queue] llm memo %s: %v", m.path, err)
		return
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("[queue] llm memo %s: %v", m.path, err)
		return
	}
	if err := os.Rename(tmp, m.path); err != nil {
		log.Printf("[queue] llm memo %s: %v", m.path, err)
	}
}

func (m *llmMemo) Chat(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
	return m.call("Chat", []any{model, systemPrompt, userMessage, temperature, maxTokens}, func() (string, error) {
		return m.Service.Chat(model, systemPrompt, userMessage, temperature, maxTokens)
	})
}

func (m *llmMemo) ChatVision(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
	return m.call("ChatVision", []any{model, systemPrompt, userText, imageBase64, temperature, maxTokens}, func() (string, error) {
		return m.Service.ChatVision(model, systemPrompt, userText, imageBase64, temperature, maxTokens)
	})
}

func (m *llmMemo) ChatWithMessages(model string, messages []llm.Message, temperature float64, maxTokens int) (string, error) {
	return m.call("ChatWithMessages", []any{model, messages, temperature, maxTokens}, func() (string, error) {
		return m.Service.ChatWithMessages(model, messages, temperature, maxTokens)
	})
}

func (m *llmMemo) GenerateSDPrompt(systemPrompt, description, presetType, model string, maxTokens int) (string, error) {
	return m.call("GenerateSDPrompt", []any{systemPrompt, description, presetType, model, maxTokens}, func() (string, error) {
		return m.Service.GenerateSDPrompt(systemPrompt, description, presetType, model, maxTokens)
	})
}

func (m *llmMemo) AnalyzeImage(model, systemPrompt, imageBase64 string, maxTokens int) (string, error) {
	return m.call("AnalyzeImage", []any{model, systemPrompt, imageBase64, maxTokens}, func() (string, error) {
		return m.Service.AnalyzeImage(model, systemPrompt, imageBase64, maxTokens)
	})
}

func (m *llmMemo) AnalyzeImageDescribe(model, prompt, imageBase64 string, maxTokens int) (string, error) {
	return m.call("AnalyzeImageDescribe", []any{model, prompt, imageBase64, maxTokens}, func() (string, error) {
		return m.Service.AnalyzeImageDescribe(model, prompt, imageBase64, maxTokens)
	})
}

func (s memoSD) FaceLandmarks(imageBase64 string) ([]sd.Point, error) {
	det, ok := s.Service.(sd.FaceDetector)
	if !ok {
		return nil, errors.New("face detection is not available")
	}
	return det.FaceLandmarks(imageBase64)
}
