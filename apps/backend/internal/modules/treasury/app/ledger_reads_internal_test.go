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

func TestLedgerReads_assembleTxnRefusesAnEntryAmountThatIsNotAnInteger(t *testing.T) {
	t.Parallel()
	rows := []txnRow{{HasEntry: true, Amount: "1.5"}}
	if _, _, err := assembleTxn("test", rows); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("assembleTxn = %v, want decode_failed", err)
	}
}

func TestLedgerReads_assembleTxnKeepsEntryAmountsPastInt64(t *testing.T) {
	t.Parallel()
	rows := []txnRow{{HasEntry: true, Seq: 1, Account: "treasury", Asset: "a", Amount: "-25000000000000000000"}}
	txn, found, err := assembleTxn("test", rows)
	if err != nil || !found || len(txn.Entries) != 1 || txn.Entries[0].Amount.String() != "-25000000000000000000" {
		t.Fatalf("assembleTxn = %+v, %v, %v", txn, found, err)
	}
}
