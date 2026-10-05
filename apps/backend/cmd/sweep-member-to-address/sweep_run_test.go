package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type stubRPC struct {
	*chainfake.Ledger
	sent    []chain.Signature
	sendErr error
}

func (*stubRPC) LatestBlockhash(context.Context) (solana.Blockhash, error) {
	return solana.Blockhash{Hash: [32]byte{7}, LastValidBlockHeight: 100}, nil
}

func (*stubRPC) MintConfig(_ context.Context, mint chain.SolanaAddress) (solana.MintConfig, error) {
	return solana.MintConfig{
		Mint:         chain.Mint{Address: mint, Decimals: 6},
		TokenProgram: chain.SPLProgram,
	}, nil
}

func (r *stubRPC) SendTransaction(_ context.Context, raw []byte) (chain.Signature, error) {
	tx, err := chain.DecodeTransaction(raw)
	if err != nil {
		return "", err
	}
	if r.sendErr != nil {
		return "", r.sendErr
	}
	sig := chain.SignatureOf(tx.Signatures[0])
	r.sent = append(r.sent, sig)
	return sig, nil
}

type recordingTransfers struct {
	*relayer.Transfers
	specs []relayer.TransferSpec
}

func (r *recordingTransfers) Build(ctx context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	r.specs = append(r.specs, spec)
	return r.Transfers.Build(ctx, spec)
}

type harness struct {
	ledger    *chainfake.Ledger
	rpc       *stubRPC
	signer    *chainfake.Signer
	transfers *recordingTransfers
	payer     chain.SolanaAddress
	out       bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{ledger: chainfake.NewLedger(clock.Real{}), signer: &chainfake.Signer{}}
	h.rpc = &stubRPC{Ledger: h.ledger}
	cfg := config.Config{Relayer: config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey("relayer"))}}
	payer, err := relayer.New(cfg, h.rpc)
	if err != nil {
		t.Fatal(err)
	}
	h.payer = payer.Address()
	h.transfers = &recordingTransfers{Transfers: relayer.NewTransfers(payer, h.signer)}
	return h
}

func (h *harness) run(dryRun bool, sources ...sweepSource) []walletOutcome {
	r := sweepRunner{
		out: &h.out, dest: dest, dryRun: dryRun, relayer: h.payer, usdc: usdc,
		balances: h.ledger, transfers: h.transfers,
	}
	return r.sweep(context.Background(), sources)
}

func (h *harness) wantLines(t *testing.T, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(h.out.String(), line) {
			t.Fatalf("output lacks %q:\n%s", line, h.out.String())
		}
	}
}

func statuses(outcomes []walletOutcome) []outcomeStatus {
	out := make([]outcomeStatus, len(outcomes))
	for i, o := range outcomes {
		out[i] = o.status
	}
	return out
}

func TestSweep_dryRunListsEveryBalanceAndSendsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ledger.SetTokens(member.Address, usdc, 1_500_000)
	h.ledger.SetTokens(stray.Address, usdc, 2_000_000)
	h.ledger.SetTokens(dest, usdc, 9_000_000)
	outcomes := h.run(true,
		sweepSource{kindMember, member},
		sweepSource{kindTreasury, cabal},
		sweepSource{kindExplicit, chain.Wallet{Address: stray.Address}},
		sweepSource{kindExplicit, chain.Wallet{ID: "dest", Address: dest}},
	)
	want := []outcomeStatus{statusDryRun, statusSkipped, statusFail, statusSkipped}
	if !slices.Equal(statuses(outcomes), want) {
		t.Fatalf("statuses = %v, want %v", statuses(outcomes), want)
	}
	h.wantLines(t,
		"dry-run would sweep member from="+string(member.Address)+" dest="+string(dest)+
			" mint="+string(usdc.Address)+" amount=1500000 no_tx_sent",
		"skip treasury "+string(cabal.Address)+" balance=0.000000 USDC (zero USDC)",
		"fail usdc-sweep explicit from="+string(stray.Address)+" err=not a Privy wallet of this app",
		"skip explicit "+string(dest)+" balance=9.000000 USDC (same as destination)",
	)
	if len(h.transfers.specs) != 0 || len(h.rpc.sent) != 0 {
		t.Fatalf("dry run built %d and sent %d transactions", len(h.transfers.specs), len(h.rpc.sent))
	}
}

func TestSweep_liveRunMovesEachWholeBalanceToTheDestination(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ledger.SetTokens(member.Address, usdc, 1_500_000)
	h.ledger.SetTokens(cabal.Address, usdc, 2_000_000)
	outcomes := h.run(false,
		sweepSource{kindMember, member},
		sweepSource{kindTreasury, cabal},
		sweepSource{kindPrivy, chain.Wallet{Address: h.payer}},
	)
	if want := []outcomeStatus{statusOK, statusOK, statusSkipped}; !slices.Equal(statuses(outcomes), want) {
		t.Fatalf("statuses = %v, want %v\n%s", statuses(outcomes), want, h.out.String())
	}
	wantSpecs := []relayer.TransferSpec{
		{FromWallet: member, To: dest, Mint: usdc, Amount: money.NewBaseUnits(1_500_000, 6)},
		{FromWallet: cabal, To: dest, Mint: usdc, Amount: money.NewBaseUnits(2_000_000, 6)},
	}
	if !slices.Equal(h.transfers.specs, wantSpecs) || len(h.rpc.sent) != 2 {
		t.Fatalf("specs = %+v, sent = %v", h.transfers.specs, h.rpc.sent)
	}
	h.wantLines(t,
		"ok usdc-sweep member from="+string(member.Address)+" dest="+string(dest)+
			" mint="+string(usdc.Address)+" amount=1500000 tx="+string(h.rpc.sent[0]),
		"ok usdc-sweep treasury from="+string(cabal.Address)+" dest="+string(dest)+
			" mint="+string(usdc.Address)+" amount=2000000 tx="+string(h.rpc.sent[1]),
		"skip privy "+string(h.payer)+" balance=0.000000 USDC (fee payer; do not drain)",
	)
	if outcomes[0].tx != h.rpc.sent[0] {
		t.Fatalf("outcome tx = %s, want %s", outcomes[0].tx, h.rpc.sent[0])
	}
}

func TestSweep_aFailedWalletDoesNotStopTheRest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ledger.SetTokens(member.Address, usdc, 1_000_000)
	h.ledger.SetTokens(cabal.Address, usdc, 1_000_000)
	h.ledger.FailOnce("TokenBalance", errs.New(errs.CodeUpstreamUnavailable, "test.rpc"))
	h.signer.FailOnce("SignTransaction", errs.New(errs.CodePrivyUnavailable, "test.sign"))
	h.ledger.SetTokens(stray.Address, usdc, 1_000_000)
	outcomes := h.run(false,
		sweepSource{kindMember, member},
		sweepSource{kindTreasury, cabal},
		sweepSource{kindPrivy, stray},
	)
	if want := []outcomeStatus{statusFail, statusFail, statusOK}; !slices.Equal(statuses(outcomes), want) {
		t.Fatalf("statuses = %v, want %v\n%s", statuses(outcomes), want, h.out.String())
	}
	h.wantLines(t,
		"fail usdc-sweep member from="+string(member.Address)+" err=balance:",
		"fail usdc-sweep treasury from="+string(cabal.Address)+" err=build:",
	)
	if len(h.rpc.sent) != 1 {
		t.Fatalf("sent %d transactions, want only the third wallet's", len(h.rpc.sent))
	}
	h.rpc.sendErr = errs.New(errs.CodeUpstreamUnavailable, "test.send")
	if got := h.run(false, sweepSource{kindMember, member}); got[0].status != statusFail {
		t.Fatalf("broadcast failure = %+v", got[0])
	}
	h.wantLines(t, "fail usdc-sweep member from="+string(member.Address)+" err=broadcast:")
}
