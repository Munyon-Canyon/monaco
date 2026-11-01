package solana_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestTransactionSignatures_listsEverySignatureOfTheTransaction(t *testing.T) {
	t.Parallel()
	u := result(`{"transaction":{"signatures":["first","second","third"]},"meta":{"err":null}}`)
	got, err := client(u).TransactionSignatures(t.Context(), "first")
	if want := []chain.Signature{"first", "second", "third"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("TransactionSignatures = %v, %v; want %v", got, err, want)
	}
	opts := string(u.requests()[0].params[1])
	if opts != `{"commitment":"finalized","encoding":"json","maxSupportedTransactionVersion":0}` {
		t.Fatalf("options = %s", opts)
	}
}

func TestTransactionSignatures_edges(t *testing.T) {
	t.Parallel()
	_, err := client(result(`null`)).TransactionSignatures(t.Context(), "gone")
	wantCode(t, err, errs.CodeNotFound)
	_, err = client(replying(503, "")).TransactionSignatures(t.Context(), "a")
	wantCode(t, err, errs.CodeRPCUnavailable)
}
