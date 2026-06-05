package model

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	ID        int64  `json:"id"`
	Timestamp int64  `json:"timestamp"`
	User      string `json:"user"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
}

// AuditFilter represents query filters for listing audit entries.
type AuditFilter struct {
	Users   []string `json:"users"`
	Actions []string `json:"actions"`
	From    int64    `json:"from"`
	To      int64    `json:"to"`
}
