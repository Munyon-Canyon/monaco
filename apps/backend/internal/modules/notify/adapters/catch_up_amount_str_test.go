package adapters

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
)

func TestWireProposal_aPastInt64SellIsNullAsAnIntegerAndExactAsAString(t *testing.T) {
	t.Parallel()
	wide := "25000000000000000000"
	got := wireProposal(domain.CatchUpProposal{TokenAmountStr: &wide})
	if got.TokenAmount != nil || got.TokenAmountStr == nil || *got.TokenAmountStr != wide {
		t.Fatalf("token amount = %v and %v", got.TokenAmount, got.TokenAmountStr)
	}
}

func TestWireProposal_aSnapshotStoredBeforeTheStringFieldDerivesIt(t *testing.T) {
	t.Parallel()
	tokens, huge := uint64(150_000_000), uint64(1<<63)
	got := wireProposal(domain.CatchUpProposal{TokenAmount: &tokens})
	if got.TokenAmount == nil || *got.TokenAmount != 150_000_000 || got.TokenAmountStr == nil ||
		*got.TokenAmountStr != "150000000" {
		t.Fatalf("token amount = %v and %v", got.TokenAmount, got.TokenAmountStr)
	}
	got = wireProposal(domain.CatchUpProposal{TokenAmount: &huge})
	if got.TokenAmount != nil || got.TokenAmountStr == nil || *got.TokenAmountStr != "9223372036854775808" {
		t.Fatalf("token amount past int64 = %v and %v", got.TokenAmount, got.TokenAmountStr)
	}
	if none := wireProposal(domain.CatchUpProposal{}); none.TokenAmount != nil || none.TokenAmountStr != nil {
		t.Fatalf("buy token amount = %v and %v, want none", none.TokenAmount, none.TokenAmountStr)
	}
}
