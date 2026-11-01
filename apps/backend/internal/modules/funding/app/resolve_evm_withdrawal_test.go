package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
)

func TestEVMWithdrawalResolver_resolveRefusesAnAmountItCannotRead(t *testing.T) {
	t.Parallel()
	_, err := (&EVMWithdrawalResolver{}).resolve(t.Context(), ResolveEVMWithdrawal{},
		sqlc.EVMWithdrawalForResolveRow{AmountMicros: "1.5"}, EVMWithdrawalFacts{})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("resolve err = %v, want decode_failed", err)
	}
}
