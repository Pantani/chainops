package doctor

import "time"

// Status classifies doctor check outcomes.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Check is a single doctor check item.
type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Report aggregates doctor checks for one command execution.
type Report struct {
	GeneratedAt time.Time `json:"generatedAt"`
	ClusterName string    `json:"clusterName,omitempty"`
	Backend     string    `json:"backend,omitempty"`
	Checks      []Check   `json:"checks"`
}

// NewReport initializes an empty report with UTC generation timestamp.
func NewReport() Report {
	return Report{
		GeneratedAt: time.Now().UTC(),
		Checks:      make([]Check, 0),
	}
}

// Add appends a check entry to the report.
func (r *Report) Add(name string, status Status, message, hint string) {
	r.Checks = append(r.Checks, Check{Name: name, Status: status, Message: message, Hint: hint})
}

// HasFailures reports whether report contains at least one fail status.
func (r Report) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// HasWarnings reports whether report contains at least one warning status.
func (r Report) HasWarnings() bool {
	for _, c := range r.Checks {
		if c.Status == StatusWarn {
			return true
		}
	}
	return false
}
