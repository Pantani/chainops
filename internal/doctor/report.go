package doctor

import "time"

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Report struct {
	GeneratedAt time.Time `json:"generatedAt"`
	ClusterName string    `json:"clusterName,omitempty"`
	Backend     string    `json:"backend,omitempty"`
	Checks      []Check   `json:"checks"`
}

func NewReport() Report {
	return Report{
		GeneratedAt: time.Now().UTC(),
		Checks:      make([]Check, 0),
	}
}

func (r *Report) Add(name string, status Status, message, hint string) {
	r.Checks = append(r.Checks, Check{Name: name, Status: status, Message: message, Hint: hint})
}

func (r Report) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

func (r Report) HasWarnings() bool {
	for _, c := range r.Checks {
		if c.Status == StatusWarn {
			return true
		}
	}
	return false
}
