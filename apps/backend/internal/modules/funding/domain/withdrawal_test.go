package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
)

const (
	wallet      = "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	usdcAccount = "FGETo8T8wMcN2wCjav8VK6eh3dLk63evNDPxzLSJra8B"
)

func TestParseWithdrawalRequest(t *testing.T) {
	t.Parallel()
	got, err := domain.ParseWithdrawalRequest("1000000", wallet)
	if err != nil || got.Amount.String() != "1000000" || string(got.To) != wallet {
		t.Fatalf("Parse = %+v, %v", got, err)
	}
	for name, c := range map[string]struct {
		amount, to string
		want       errs.Code
	}{
		"below minimum": {"999999", wallet, errs.CodeInvalidInput},
		"not a number":  {"1.5", wallet, errs.CodeInvalidInput},
		"negative":      {"-1000000", wallet, errs.CodeInvalidInput},
		"bad address":   {"1000000", "0OIl", errs.CodeInvalidAddress},
		"token account": {"1000000", usdcAccount, errs.CodeInvalidAddress},
	} {
		if _, err := domain.ParseWithdrawalRequest(c.amount, c.to); errs.CodeOf(err) != c.want {
			t.Errorf("%s: err = %v, want %s", name, err, c.want)
		}
	}
}
