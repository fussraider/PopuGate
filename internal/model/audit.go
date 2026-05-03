package model

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	ID        int64  `json:"id"`
	Timestamp int64  `json:"timestamp"`
	User      string `json:"user"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
}
