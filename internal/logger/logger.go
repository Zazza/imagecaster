package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Level string

const (
	ERROR Level = "error"
	WARN  Level = "warn"
	INFO  Level = "info"
	DEBUG Level = "debug"
)

const keepEntries = 500

type Entry struct {
	ID        int64  `json:"id"`
	Level     Level  `json:"level"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type Logger struct {
	ctx     context.Context
	mu      sync.Mutex
	lastID  int64
	entries []Entry
}

func New(ctx context.Context) *Logger {
	return &Logger{ctx: ctx}
}

func (l *Logger) SetContext(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ctx = ctx
}

func (l *Logger) emit(level Level, format string, args ...interface{}) {
	e := Entry{
		Level:     level,
		Message:   fmt.Sprintf(format, args...),
		Timestamp: time.Now().Format("15:04:05"),
	}

	l.mu.Lock()
	l.lastID++
	e.ID = l.lastID
	l.entries = append(l.entries, e)
	if len(l.entries) > keepEntries {
		l.entries = append(l.entries[:0:0], l.entries[len(l.entries)-keepEntries:]...)
	}
	ctx := l.ctx
	l.mu.Unlock()

	data, _ := json.Marshal(e)

	fmt.Fprintf(os.Stderr, "[%s] %s\n", level, e.Message)

	if ctx != nil {
		runtime.EventsEmit(ctx, "log:entry", string(data))
	}
}

func (l *Logger) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Entry(nil), l.entries...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.emit(ERROR, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.emit(WARN, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.emit(INFO, format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.emit(DEBUG, format, args...)
}

func (l *Logger) APIRequest(method, url string) {
	l.Debug("API %s %s", method, shortURL(url))
}

func (l *Logger) APIResponse(method, url string, status int) {
	if status >= 400 {
		l.emit(ERROR, "API %s %s -> %d", method, shortURL(url), status)
	} else {
		l.emit(DEBUG, "API %s %s -> %d", method, shortURL(url), status)
	}
}

func (l *Logger) UserAction(action string, args ...interface{}) {
	l.Info(action, args...)
}

type logBridge struct {
	logger *Logger
	prev   io.Writer
}

func (b *logBridge) Write(p []byte) (int, error) {
	n, err := b.prev.Write(p)
	line := strings.TrimSpace(string(p))

	level := INFO
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "[error]") {
		level = ERROR
	} else if strings.Contains(lower, "warn") || strings.Contains(lower, "[warn]") {
		level = WARN
	}

	b.logger.emit(level, "%s", line)
	return n, err
}

func (l *Logger) InstallBridge() {
	lb := &logBridge{logger: l, prev: os.Stderr}
	log.SetOutput(lb)
}

func shortURL(url string) string {
	if i := strings.Index(url, "/v1/"); i >= 0 {
		return url[i:]
	}
	if i := strings.Index(url, "/api/"); i >= 0 {
		return url[i:]
	}
	return url
}
