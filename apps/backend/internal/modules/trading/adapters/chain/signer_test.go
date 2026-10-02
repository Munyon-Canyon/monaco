package chain_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

func TestSigner_returnsTheSignedBytesAndTheirFirstSignature(t *testing.T) {
	t.Parallel()
	var fake chainfake.Signer
	s := chain.NewSigner(&fake)
	unsigned := chainfake.Unsigned(chainfake.WalletAddress("wallet-a"))
	signed, sig, err := s.Sign(t.Context(), "wallet-a", unsigned)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := platform.DecodeTransaction(signed)
	if err != nil || sig != platform.SignatureOf(tx.Signatures[0]) || !tx.Signed(0) {
		t.Fatalf("signature = %q, decode %v, first slot signed %v", sig, err, tx.Signed(0))
	}
}

func TestSigner_refusesWhatIsNotSignedInTheFirstSlot(t *testing.T) {
	t.Parallel()
	var fake chainfake.Signer
	s := chain.NewSigner(&fake)
	feePayer := chainfake.WalletAddress("fee-payer")
	_, _, err := s.Sign(t.Context(), "wallet-a", chainfake.Unsigned(feePayer, chainfake.WalletAddress("wallet-a")))
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("a transaction whose first slot stays empty = %v, want decode_failed", err)
	}
	down := errs.New(errs.CodePrivyUnavailable, "test")
	fake.FailOnce("SignTransaction", down)
	_, _, err = s.Sign(t.Context(), "wallet-a", chainfake.Unsigned(chainfake.WalletAddress("wallet-a")))
	if errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Privy down = %v, want privy_unavailable", err)
	}
}

type junk struct{}

func (junk) SignTransaction(context.Context, string, []byte) ([]byte, error) { return []byte{0}, nil }

func TestSigner_refusesBytesThatAreNotATransaction(t *testing.T) {
	t.Parallel()
	_, _, err := chain.NewSigner(junk{}).Sign(t.Context(), "wallet-a", []byte{1})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("junk signed bytes = %v, want decode_failed", err)
	}
}
