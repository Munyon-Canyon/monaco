package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func TestEVMWithdrawalResolver_resolveRefusesAnAmountItCannotRead(t *testing.T) {
	t.Parallel()
	_, err := (&EVMWithdrawalResolver{}).resolve(t.Context(), ResolveEVMWithdrawal{},
		sqlc.EVMWithdrawalForResolveRow{AmountMicros: "1.5"}, EVMWithdrawalFacts{})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("resolve err = %v, want decode_failed", err)
	}
}

func TestSupportEntry_refusesAnAmountItCannotRead(t *testing.T) {
	t.Parallel()
	_, err := supportEntry(sqlc.ListSupportCandidateEVMWithdrawalsRow{AmountMicros: "1.5"},
		multichain.ActionStatus{}, time.Time{})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("supportEntry err = %v, want decode_failed", err)
	}
}
