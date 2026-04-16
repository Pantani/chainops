package doctor

import "testing"

func TestReportStatusFlags(t *testing.T) {
	t.Parallel()

	r := NewReport()
	r.Add("spec.validation", StatusPass, "ok", "")
	r.Add("state.snapshot", StatusWarn, "missing", "run apply")

	if r.HasFailures() {
		t.Fatalf("expected no failures")
	}
	if !r.HasWarnings() {
		t.Fatalf("expected warnings")
	}

	r.Add("backend.resolve", StatusFail, "missing", "fix backend")
	if !r.HasFailures() {
		t.Fatalf("expected failures")
	}
}
