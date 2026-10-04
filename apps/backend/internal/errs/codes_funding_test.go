package errs

import "testing"

func TestFundingCodes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		code      Code
		name      string
		retryable bool
	}{
		{CodeCabalPaused, "CabalPaused", false},
		{CodeNotADeposit, "NotADeposit", false},
		{CodeMonacoSigned, "MonacoSigned", false},
		{CodeUnresolved, "Unresolved", true},
	} {
		if got := Name(tc.code); got != tc.name {
			t.Fatalf("Name(%q) = %q, want %q", tc.code, got, tc.name)
		}
		if got := Retryable(tc.code); got != tc.retryable {
			t.Fatalf("Retryable(%q) = %t, want %t", tc.code, got, tc.retryable)
		}
	}
}
