// Package ownerattention exposes a bounded, credential-free view of active
// conditions that require the signed-in owner's attention. It is read-only and
// has no provider, order, approval, or execution dependency.
package ownerattention

import "time"

type Severity string

const (
	SeverityAttention Severity = "ATTENTION"
	SeverityStopped   Severity = "STOPPED"
)

type Status string

const (
	StatusClear     Status = "CLEAR"
	StatusAttention Status = "ATTENTION"
	StatusStopped   Status = "STOPPED"
)

type Item struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	Severity     Severity  `json:"severity"`
	ResourceType string    `json:"resource_type"`
	ResourceID   *string   `json:"resource_id,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
	Count        int64     `json:"count"`
}

type Overview struct {
	GeneratedAt            time.Time `json:"generated_at"`
	Status                 Status    `json:"status"`
	Items                  []Item    `json:"items"`
	Total                  int       `json:"total"`
	AttentionCount         int       `json:"attention_count"`
	StoppedCount           int       `json:"stopped_count"`
	LiveExecutionAvailable bool      `json:"live_execution_available"`
	BrokerActionRequested  bool      `json:"broker_action_requested"`
}
