package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
)

func TestParseWithdrawalRequest(t *testing.T) {
	t.Parallel()
	const to = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
	got, err := domain.ParseWithdrawalRequest("1000000", to)
	if err != nil || got.Amount.String() != "1000000" || string(got.To) != to {
		t.Fatalf("Parse = %+v, %v", got, err)
	}
	for name, c := range map[string]struct {
		amount, to string
		want       errs.Code
	}{
		"below minimum": {"999999", to, errs.CodeInvalidInput},
		"not a number":  {"1.5", to, errs.CodeInvalidInput},
		"negative":      {"-1000000", to, errs.CodeInvalidInput},
		"bad address":   {"1000000", "0OIl", errs.CodeInvalidAddress},
	} {
		if _, err := domain.ParseWithdrawalRequest(c.amount, c.to); errs.CodeOf(err) != c.want {
			t.Errorf("%s: err = %v, want %s", name, err, c.want)
		}
	}
}
