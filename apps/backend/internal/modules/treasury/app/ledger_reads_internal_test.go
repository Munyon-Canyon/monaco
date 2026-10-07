package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestLedgerReads_assembleTxnRefusesAMalformedUserID(t *testing.T) {
	t.Parallel()
	if _, _, err := assembleTxn("test", []txnRow{{UserID: "not-a-uuid"}}); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("assembleTxn = %v, want decode_failed", err)
	}
}

func TestLedgerReads_assembleTxnOfNoRowsIsAbsent(t *testing.T) {
	t.Parallel()
	if _, found, err := assembleTxn("test", nil); found || err != nil {
		t.Fatalf("assembleTxn = %v, %v", found, err)
	}
}
