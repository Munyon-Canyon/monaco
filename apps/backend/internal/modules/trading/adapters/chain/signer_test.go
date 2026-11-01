package chain_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

func swapTx() []byte {
	return chainfake.Unsigned(chainfake.RelayerAddress(), chainfake.WalletAddress("wallet-a"))
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
	signed, sig, err := s.Sign(t.Context(), "wallet-a", swapTx())
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
	_, _, err := chain.NewSigner(passthrough{}, chainfake.Relayer()).Sign(t.Context(), "wallet-a", swapTx())
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Privy bytes without the treasury signature = %v, want decode_failed", err)
	}
}

func TestSigner_passesPrivyFailuresThrough(t *testing.T) {
	t.Parallel()
	var fake chainfake.Signer
	fake.FailOnce("SignTransaction", errs.New(errs.CodePrivyUnavailable, "test"))
	_, _, err := chain.NewSigner(&fake, chainfake.Relayer()).Sign(t.Context(), "wallet-a", swapTx())
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

type junkCoSigner struct{}

func (junkCoSigner) Address() platform.SolanaAddress { return chainfake.RelayerAddress() }

func (junkCoSigner) CoSign(context.Context, []byte) ([]byte, error) { return []byte{0}, nil }

func TestSigner_refusesCoSignedBytesThatAreNotATransaction(t *testing.T) {
	t.Parallel()
	_, _, err := chain.NewSigner(&chainfake.Signer{}, junkCoSigner{}).Sign(t.Context(), "wallet-a", swapTx())
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("junk co-signed bytes = %v, want decode_failed", err)
	}
}

func marketMaker() platform.SolanaAddress { return platform.AddressOf(make([]byte, 32)) }

func TestSigner_sendsAMarketMakerRouteTheRelayerIsNotPartOfWithoutCoSigning(t *testing.T) {
	t.Parallel()
	tx := chainfake.Unsigned(marketMaker(), chainfake.WalletAddress("wallet-a"))
	signed, sig, err := chain.NewSigner(&chainfake.Signer{}, chainfake.Relayer()).Sign(t.Context(), "wallet-a", tx)
	if err != nil {
		t.Fatalf("Sign = %v, want the Privy-signed transaction", err)
	}
	got, err := platform.DecodeTransaction(signed)
	if err != nil {
		t.Fatal(err)
	}
	if got.Signed(0) || !got.Signed(1) {
		t.Fatalf(
			"market maker slot signed %v, treasury slot signed %v; want only the treasury",
			got.Signed(0),
			got.Signed(1),
		)
	}
	want := platform.StandInSignature(platform.SignatureOf(got.Signatures[1]))
	if sig != want {
		t.Fatalf("signature = %q, want the stand-in for the treasury signature %q", sig, want)
	}
}

func TestSigner_coSignsAMarketMakerRouteThatNamesTheRelayerAsFunder(t *testing.T) {
	t.Parallel()
	relayerAddr, wallet := chainfake.RelayerAddress(), chainfake.WalletAddress("wallet-a")
	tx := chainfake.Unsigned(marketMaker(), wallet, relayerAddr)
	signed, sig, err := chain.NewSigner(&chainfake.Signer{}, chainfake.Relayer()).Sign(t.Context(), "wallet-a", tx)
	if err != nil {
		t.Fatalf("Sign = %v, want both signatures", err)
	}
	got, _ := platform.DecodeTransaction(signed)
	if got.Signed(0) || !got.Signed(1) || !got.Signed(2) {
		t.Fatalf("slots signed %v %v %v, want market maker unsigned, treasury and relayer signed",
			got.Signed(0), got.Signed(1), got.Signed(2))
	}
	if !platform.IsStandInSignature(sig) {
		t.Fatalf("signature = %q, want a stand-in until Jupiter signs the fee payer slot", sig)
	}
}

func TestSigner_refusesAMarketMakerRouteTheTreasuryDidNotSign(t *testing.T) {
	t.Parallel()
	tx := chainfake.Unsigned(marketMaker(), chainfake.WalletAddress("wallet-a"))
	_, _, err := chain.NewSigner(passthrough{}, chainfake.Relayer()).Sign(t.Context(), "wallet-a", tx)
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("a market maker route with no treasury signature = %v, want decode_failed", err)
	}
}

func transferOutOfSecondSigner(relayerAddr, wallet platform.SolanaAddress) []byte {
	key := func(a platform.SolanaAddress) []byte { b, _ := a.Bytes(); return b }
	msg := make([]byte, 0, 256)
	msg = append(msg, 3, 0, 1)
	msg = append(msg, platform.CompactU16(4)...)
	for _, a := range []platform.SolanaAddress{marketMaker(), wallet, relayerAddr, platform.SystemProgram} {
		msg = append(msg, key(a)...)
	}
	msg = append(msg, make([]byte, 32)...)
	msg = append(msg, 1, 3, 2, 2, 0, 1)
	msg = append(msg, 12, 2, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0)
	return append(append(platform.CompactU16(3), make([]byte, 3*64)...), msg...)
}

func TestSigner_refusesARouteThatMakesTheRelayerPayOut(t *testing.T) {
	t.Parallel()
	tx := transferOutOfSecondSigner(chainfake.RelayerAddress(), chainfake.WalletAddress("wallet-a"))
	_, _, err := chain.NewSigner(&chainfake.Signer{}, chainfake.Relayer()).Sign(t.Context(), "wallet-a", tx)
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("a transfer out of the relayer on a market maker route = %v, want decode_failed", err)
	}
}
