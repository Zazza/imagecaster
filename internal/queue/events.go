package queue

const (
	EventQueueStarted   = "queue:started"
	EventQueueCompleted = "queue:completed"
	EventQueueFailed    = "queue:failed"
	EventQueuePaused    = "queue:paused"
	EventQueueChanged   = "queue:changed"
)

type EventPayload struct {
	JobID      int64      `json:"job_id"`
	Type       JobType    `json:"type,omitempty"`
	Result     *JobResult `json:"result,omitempty"`
	Error      string     `json:"error,omitempty"`
	RetryCount int        `json:"retry_count,omitempty"`
	MaxRetries int        `json:"max_retries,omitempty"`
}
