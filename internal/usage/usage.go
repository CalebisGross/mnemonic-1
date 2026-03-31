// Package usage provides telemetry types shared across store, embedding, and API layers.
package usage

import (
	"context"
	"time"
)

// Record tracks a single operation (embedding, completion, etc.) for telemetry.
type Record struct {
	Timestamp        time.Time `json:"timestamp"`
	Operation        string    `json:"operation"` // "complete", "embed", "batch_embed"
	Caller           string    `json:"caller"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	LatencyMs        int64     `json:"latency_ms"`
	Success          bool      `json:"success"`
	ErrorMessage     string    `json:"error_message,omitempty"`
}

// Recorder persists usage records to storage.
type Recorder interface {
	RecordLLMUsage(ctx context.Context, record Record) error
}
