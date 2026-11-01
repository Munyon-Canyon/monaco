package chain_test

import (
	"context"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	adapter "github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type readerRPC struct {
	statuses    []solana.Status
	received    money.BaseUnits
	statusErr   error
	receivedErr error
	owner       platform.SolanaAddress
	mint        platform.Mint
	valid       bool
	blockhash   string
	recent      []solana.SignatureInfo
	recentErr   error
	transaction map[platform.Signature][]platform.Signature
	asked       []platform.Signature
}

func (r *readerRPC) SignaturesFor(
	_ context.Context, owner platform.SolanaAddress, opts solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
	r.owner = owner
	if opts.Limit < 1 {
		return nil, errs.New(errs.CodeInvalidInput, "test")
	}
	return r.recent, r.recentErr
}

func (r *readerRPC) TransactionSignatures(
	_ context.Context, sig platform.Signature,
) ([]platform.Signature, error) {
	r.asked = append(r.asked, sig)
	signatures, ok := r.transaction[sig]
	if !ok {
		return nil, errs.New(errs.CodeNotFound, "test")
	}
	return signatures, nil
}

func (r *readerRPC) BlockhashValid(_ context.Context, hash string) (bool, error) {
	r.blockhash = hash
	return r.valid, r.statusErr
}

func (r *readerRPC) SignatureStatuses(
	context.Context,
	[]platform.Signature,
) ([]solana.Status, error) {
	return r.statuses, r.statusErr
}

func (r *readerRPC) Received(
	_ context.Context, _ platform.Signature, owner platform.SolanaAddress, mint platform.Mint,
) (money.BaseUnits, error) {
	r.owner, r.mint = owner, mint
	return r.received, r.receivedErr
}

func TestReader_signatureStatusesMapsEveryStateAndRefusesUnknownData(t *testing.T) {
	t.Parallel()
	rpc := &readerRPC{statuses: []solana.Status{
		{State: solana.StateFinalized, Failed: true, BlockHeight: 12},
		{State: solana.StateProcessing, BlockHeight: 13},
		{State: solana.StateNotFound, BlockHeight: 14},
	}}
	got, err := adapter.NewReader(rpc).
		SignatureStatuses(t.Context(), []platform.Signature{"a", "b", "c"})
	want := []app.SigStatus{
		{State: app.SigFinalized, Failed: true, BlockHeight: 12},
		{State: app.SigProcessing, BlockHeight: 13},
		{State: app.SigNotFound, BlockHeight: 14},
	}
	if err != nil || len(got) != len(want) {
		t.Fatalf("SignatureStatuses = %+v, %v", got, err)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SignatureStatuses[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	_, err = adapter.NewReader(&readerRPC{statuses: []solana.Status{{State: 99}}}).
		SignatureStatuses(t.Context(), nil)
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("unknown status error = %v, want decode_failed", err)
	}
	_, err = adapter.NewReader(&readerRPC{
		statusErr: errs.New(errs.CodeRPCUnavailable, "test"),
	}).SignatureStatuses(t.Context(), nil)
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("status error = %v, want rpc_unavailable", err)
	}
}

func TestReader_inboundAmountIsTheTreasurysChainDeltaAndRefusesAnEmptyFill(t *testing.T) {
	t.Parallel()
	mint := aaplx()
	rpc := &readerRPC{received: money.NewBaseUnits(20, 8)}
	got, err := adapter.NewReader(rpc).InboundAmount(t.Context(), "sig", "treasury", mint)
	if err != nil || got != money.NewBaseUnits(20, 8) {
		t.Fatalf("InboundAmount = %v, %v, want 20", got, err)
	}
	if rpc.owner != "treasury" || rpc.mint != mint {
		t.Fatalf("read owner %s mint %+v, want treasury %+v", rpc.owner, rpc.mint, mint)
	}
	_, err = adapter.NewReader(&readerRPC{received: money.NewBaseUnits(0, 8)}).
		InboundAmount(t.Context(), "sig", "treasury", mint)
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("empty fill = %v, want decode_failed", err)
	}
	_, err = adapter.NewReader(&readerRPC{receivedErr: errs.New(errs.CodeRPCUnavailable, "test")}).
		InboundAmount(t.Context(), "sig", "treasury", mint)
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("read error = %v, want rpc_unavailable", err)
	}
}

func TestReader_blockhashValidityComesFromTheSignedTransaction(t *testing.T) {
	t.Parallel()
	hash := make([]byte, 32)
	hash[31] = 7
	key := make([]byte, 32)
	key[31] = 1
	message := append([]byte{1, 0, 0, 1}, key...)
	message = append(message, hash...)
	message = append(message, 0)
	signed := append(append([]byte{1}, make([]byte, 64)...), message...)
	rpc := &readerRPC{valid: true}
	valid, err := adapter.NewReader(rpc).BlockhashValid(t.Context(), signed)
	if err != nil || !valid || rpc.blockhash != platform.EncodeBase58(hash) {
		t.Fatalf("BlockhashValid = %t, %v, hash %q", valid, err, rpc.blockhash)
	}
	if _, err = adapter.NewReader(rpc).BlockhashValid(t.Context(), nil); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("invalid signed transaction = %v", err)
	}
}

func TestReader_inboundAmountPassesAnyNonZeroDeltaThrough(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		mint := aaplx()
		want := rapid.Uint64Range(1, 1<<62).Draw(t, "delta")
		got, err := adapter.NewReader(&readerRPC{received: money.NewBaseUnits(want, 8)}).InboundAmount(
			t.Context(), "sig", "treasury", mint,
		)
		if err != nil || got.Uint64() != want {
			t.Fatalf("InboundAmount = %v, %v, want %d", got, err, want)
		}
	})
}

func TestReader_findSignatureReturnsTheTransactionThatCarriesTheTreasurysSignature(t *testing.T) {
	t.Parallel()
	rpc := &readerRPC{
		recent: []solana.SignatureInfo{
			{Signature: "other-1"},
			{Signature: "gone"},
			{Signature: "real"},
			{Signature: "later"},
		},
		transaction: map[platform.Signature][]platform.Signature{
			"other-1": {"other-1", "someone"},
			"real":    {"real", "treasury-sig", "relayer-sig"},
			"later":   {"later", "treasury-sig"},
		},
	}
	got, found, err := adapter.NewReader(rpc).FindSignature(
		t.Context(), "treasury-address", platform.StandInSignature("treasury-sig"))
	if err != nil || !found || got != "real" {
		t.Fatalf("FindSignature = %q, %v, %v; want real", got, found, err)
	}
	if rpc.owner != "treasury-address" || len(rpc.asked) != 3 {
		t.Fatalf("looked at %q, read %v; want the treasury and a stop at the match", rpc.owner, rpc.asked)
	}
}

func TestReader_findSignatureSaysNotFoundWhenNoRecentTransactionCarriesIt(t *testing.T) {
	t.Parallel()
	rpc := &readerRPC{
		recent:      []solana.SignatureInfo{{Signature: "a"}},
		transaction: map[platform.Signature][]platform.Signature{"a": {"a", "x"}},
	}
	got, found, err := adapter.NewReader(rpc).FindSignature(
		t.Context(), "treasury-address", platform.StandInSignature("treasury-sig"))
	if err != nil || found || got != "" {
		t.Fatalf("FindSignature = %q, %v, %v; want nothing and no error", got, found, err)
	}
}

func TestReader_findSignatureRefusesARealSignatureAndPassesRPCFailuresThrough(t *testing.T) {
	t.Parallel()
	if _, _, err := adapter.NewReader(&readerRPC{}).
		FindSignature(t.Context(), "o", "real"); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("a real signature = %v, want invalid_input", err)
	}
	down := errs.New(errs.CodeRPCUnavailable, "test")
	_, _, err := adapter.NewReader(&readerRPC{recentErr: down}).
		FindSignature(t.Context(), "o", platform.StandInSignature("t"))
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("listing failure = %v, want rpc_unavailable", err)
	}
}

func TestReader_findSignaturePassesATransactionReadFailureThrough(t *testing.T) {
	t.Parallel()
	rpc := &readerRPC{recent: []solana.SignatureInfo{{Signature: "a"}}, transaction: nil}
	rpc.transaction = map[platform.Signature][]platform.Signature{}
	_, found, err := adapter.NewReader(&failingTransactions{rpc}).FindSignature(
		t.Context(), "o", platform.StandInSignature("t"))
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || found {
		t.Fatalf("FindSignature = %v, %v; want rpc_unavailable", found, err)
	}
}

type failingTransactions struct{ *readerRPC }

func (failingTransactions) TransactionSignatures(context.Context, platform.Signature) ([]platform.Signature, error) {
	return nil, errs.New(errs.CodeRPCUnavailable, "test")
}
