package doctor

import "testing"

func TestSummaryCountsFailedWarningsAndErrors(t *testing.T) {
	checks := []Check{
		{Name: "ok", Severity: SeverityError, OK: true},
		{Name: "warn", Severity: SeverityWarn, OK: false},
		{Name: "err", Severity: SeverityError, OK: false},
	}
	if got := Summary(checks); got != "1 error(s), 1 warning(s)" {
		t.Fatalf("Summary() = %q", got)
	}
	if !HasErrors(checks) {
		t.Fatal("HasErrors() = false, want true")
	}
}
