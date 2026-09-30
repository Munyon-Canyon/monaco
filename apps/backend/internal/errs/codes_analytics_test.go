package errs

import "testing"

type reading struct {
	wire      string
	kind      Kind
	retryable bool
	alert     bool
	verdict   Verdict
}

func read(code Code) reading {
	return reading{string(code), KindOf(code), Retryable(code), Alert(code), VerdictFor(code)}
}

func TestAnalyticsCodesReadTheirRows(t *testing.T) {
	t.Parallel()
	tests := map[Code]reading{
		CodePostHogUnavailable: {"post_hog_unavailable", KindUnavailable, true, false, VerdictNak},
		CodePostHogRejected:    {"post_hog_rejected", KindInternal, false, true, VerdictTerm},
		CodeAnalyticsPII:       {"analytics_pii", KindInternal, false, true, VerdictTerm},
	}
	for code, want := range tests {
		t.Run(want.wire, func(t *testing.T) {
			t.Parallel()
			if got := read(code); got != want {
				t.Errorf("read(%s) = %+v, want %+v", code, got, want)
			}
		})
	}
}
