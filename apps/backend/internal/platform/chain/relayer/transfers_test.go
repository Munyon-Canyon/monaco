package relayer_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestTransfersBuild_SignatureKnownBeforeBroadcast(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	signed, err := s.transfers.Build(t.Context(), fund(25_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if signed.Signature != chain.SignatureOf(signed.Bytes[1:65]) {
		t.Fatalf("Signature %s is not the first signature in Bytes", signed.Signature)
	}
	if slices.Contains(s.rpc.calls(), "sendTransaction") {
		t.Fatalf("Build called sendTransaction: %v", s.rpc.calls())
	}
	tx, err := chain.DecodeTransaction(signed.Bytes)
	if err != nil || !tx.Signed(0) || !tx.Signed(1) || signed.LastValidBlockHeight != 380_000_150 {
		t.Fatalf("tx = %v; relayer signed %v, member signed %v, lastValid %d",
			err, tx.Signed(0), tx.Signed(1), signed.LastValidBlockHeight)
	}
	want := []chain.SolanaAddress{s.relayer.Address(), memberWallet}
	if !slices.Equal(tx.Signers, want) {
		t.Fatalf("signers = %v, want the relayer as fee payer then the member", tx.Signers)
	}
	source, _ := chain.AssociatedTokenAccount(memberWallet, usdcMint, chain.SPLProgram)
	dest, _ := chain.AssociatedTokenAccount(treasury, usdcMint, chain.SPLProgram)
	data := binary.LittleEndian.AppendUint64([]byte{12}, 25_000_000)
	for _, part := range [][]byte{addr(source), addr(dest), addr(chain.ATAProgram), append(data, 6)} {
		if !bytes.Contains(tx.Message, part) {
			t.Fatalf("message lacks %x", part)
		}
	}
}

func addr(a chain.SolanaAddress) []byte {
	b, _ := a.Bytes()
	return b
}

func TestTransfersBroadcast_sendsTheStoredBytesAndChecksTheSignature(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	signed, err := s.transfers.Build(t.Context(), fund(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.transfers.Broadcast(t.Context(), signed); err != nil {
		t.Fatal(err)
	}
	if calls := s.rpc.calls(); calls[len(calls)-1] != "sendTransaction" {
		t.Fatalf("calls = %v", calls)
	}
	forged := signed
	forged.Signature = "other"
	wantCode(t, s.transfers.Broadcast(t.Context(), forged), errs.CodeInternal)
	script(t, s.srv, fakes.Step{Route: "/rpc/sendTransaction", Action: fakes.ActionFail, Status: 400})
	wantCode(t, s.transfers.Broadcast(t.Context(), signed), errs.CodeRPCUnavailable)
}

func TestTransfersBuild_refusesBadSpecs(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	edit := func(f func(*relayer.TransferSpec)) relayer.TransferSpec {
		spec := fund(10)
		f(&spec)
		return spec
	}
	for name, tc := range map[string]struct {
		spec relayer.TransferSpec
		want errs.Code
	}{
		"bad recipient":   {edit(func(s *relayer.TransferSpec) { s.To = "nope" }), errs.CodeInvalidAddress},
		"zero amount":     {fund(0), errs.CodeInvalidInput},
		"amount decimals": {edit(func(s *relayer.TransferSpec) { s.Amount = money.NewBaseUnits(10, 9) }), errs.CodeInvalidInput},
		"to self":         {edit(func(s *relayer.TransferSpec) { s.To = memberWallet }), errs.CodeInvalidInput},
		"to the relayer":  {edit(func(s *relayer.TransferSpec) { s.To = s0(s) }), errs.CodeInvalidInput},
		"mint decimals":   {edit(func(s *relayer.TransferSpec) { s.Mint.Decimals, s.Amount = 9, money.NewBaseUnits(10, 9) }), errs.CodeInvalidInput},
	} {
		_, err := s.transfers.Build(t.Context(), tc.spec)
		if err == nil || errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func s0(*relayer.TransferSpec) chain.SolanaAddress {
	return "CMa1GUZZLJ6goRKUyvkDKsMeyjysTaZioT3cr596KAYa"
}

func TestTransfersBuild_upstreamFailures(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"/rpc/getAccountInfo", "/rpc/getLatestBlockhash"} {
		s := overFakes(t, "relayer")
		script(t, s.srv, fakes.Step{Route: route, Action: fakes.ActionFail, Status: 400})
		_, err := s.transfers.Build(t.Context(), fund(1))
		wantCode(t, err, errs.CodeRPCUnavailable)
	}
	s := overFakes(t, "relayer")
	for name, tc := range map[string]struct {
		sign func(unsigned []byte) ([]byte, error)
		want errs.Code
	}{
		"privy down": {func([]byte) ([]byte, error) { return nil, errs.New(errs.CodePrivyUnavailable, "test") }, errs.CodePrivyUnavailable},
		"garbage":    {func([]byte) ([]byte, error) { return []byte{7}, nil }, errs.CodeInternal},
		"unsigned":   {func(b []byte) ([]byte, error) { return b, nil }, errs.CodeInternal},
		"tampered": {func(b []byte) ([]byte, error) {
			tx, _ := chain.DecodeTransaction(b)
			tx.Message = append(bytes.Clone(tx.Message[:len(tx.Message)-1]), 7)
			_ = tx.Sign(fakes.PrivyWalletKey("wallet-member"))
			return tx.Encode(), nil
		}, errs.CodeInternal},
	} {
		sign := tc.sign
		tr := relayer.NewTransfers(
			s.relayer,
			signerFunc(func(_ context.Context, _ string, b []byte) ([]byte, error) { return sign(b) }),
		)
		_, err := tr.Build(t.Context(), fund(1))
		if err == nil || errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}
