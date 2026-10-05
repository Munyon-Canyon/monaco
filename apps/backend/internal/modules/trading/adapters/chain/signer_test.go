package chain_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

func swapTx(treasury string) []byte {
	return chainfake.Unsigned(chainfake.RelayerAddress(), chainfake.WalletAddress(treasury))
}

func TestSigner_namesTheRelayerAsFeePayer(t *testing.T) {
	t.Parallel()
	payer, err := chain.NewSigner(&chainfake.Signer{}, chainfake.Relayer()).FeePayer()
	if err != nil || payer != chainfake.RelayerAddress() {
		t.Fatalf("FeePayer = %q, %v, want the relayer %q", payer, err, chainfake.RelayerAddress())
	}
}

func TestSigner_returnsBothSignaturesAndTheFeePayersAsTheTransactionID(t *testing.T) {
	t.Parallel()
	s := chain.NewSigner(&chainfake.Signer{}, chainfake.Relayer())
	signed, sig, err := s.Sign(t.Context(), "wallet-a", swapTx("wallet-a"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := platform.DecodeTransaction(signed)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Signers[0] != chainfake.RelayerAddress() || !tx.Signed(0) || !tx.Signed(1) {
		t.Fatalf("signers %v, relayer signed %v, treasury signed %v; want both", tx.Signers, tx.Signed(0), tx.Signed(1))
	}
	if sig != platform.SignatureOf(tx.Signatures[0]) {
		t.Fatalf("signature = %q, want the relayer's slot 0 %q", sig, platform.SignatureOf(tx.Signatures[0]))
	}
}

func TestSigner_refusesATransactionTheRelayerDoesNotPayFor(t *testing.T) {
	t.Parallel()
	s := chain.NewSigner(&chainfake.Signer{}, chainfake.Relayer())
	_, _, err := s.Sign(t.Context(), "wallet-a", chainfake.Unsigned(chainfake.WalletAddress("wallet-a")))
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("a transaction the treasury pays for = %v, want decode_failed", err)
	}
}

type passthrough struct{}

func (passthrough) SignTransaction(_ context.Context, _ string, unsigned []byte) ([]byte, error) {
	return unsigned, nil
}

func TestSigner_refusesWhatTheTreasuryDidNotSign(t *testing.T) {
	t.Parallel()
	_, _, err := chain.NewSigner(passthrough{}, chainfake.Relayer()).Sign(t.Context(), "wallet-a", swapTx("wallet-a"))
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Privy bytes without the treasury signature = %v, want decode_failed", err)
	}
}

func TestSigner_passesPrivyFailuresThrough(t *testing.T) {
	t.Parallel()
	var fake chainfake.Signer
	fake.FailOnce("SignTransaction", errs.New(errs.CodePrivyUnavailable, "test"))
	_, _, err := chain.NewSigner(&fake, chainfake.Relayer()).Sign(t.Context(), "wallet-a", swapTx("wallet-a"))
	if errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Privy down = %v, want privy_unavailable", err)
	}
}

type junk struct{}

func (junk) SignTransaction(context.Context, string, []byte) ([]byte, error) { return []byte{0}, nil }

func TestSigner_refusesBytesThatAreNotATransaction(t *testing.T) {
	t.Parallel()
	_, _, err := chain.NewSigner(junk{}, chainfake.Relayer()).Sign(t.Context(), "wallet-a", []byte{1})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("junk signed bytes = %v, want decode_failed", err)
	}
}
