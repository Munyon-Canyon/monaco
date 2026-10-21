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

func TestTransfersBuild_token2022MintUsesTheToken2022Program(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	spec := fund(0)
	spec.Mint, spec.Amount = chain.Mint{Address: feeMint, Decimals: 8}, money.NewBaseUnits(1_000, 8)
	signed, err := s.transfers.Build(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := chain.DecodeTransaction(signed.Bytes)
	keys, ixs := compiled(t, tx.Message)
	if len(ixs) != 2 {
		t.Fatalf("%d instructions, want the ATA create then TransferChecked", len(ixs))
	}
	create, transfer := ixs[0], ixs[1]
	source, _ := chain.AssociatedTokenAccount(memberWallet, feeMint, chain.SPL2022Program)
	dest, _ := chain.AssociatedTokenAccount(treasury, feeMint, chain.SPL2022Program)
	for name, pair := range map[string][2]chain.SolanaAddress{
		"create program":       {keys[create.program], chain.ATAProgram},
		"create token program": {keys[create.accounts[5]], chain.SPL2022Program},
		"create ata":           {keys[create.accounts[1]], dest},
		"transfer program":     {keys[transfer.program], chain.SPL2022Program},
		"transfer source":      {keys[transfer.accounts[0]], source},
		"transfer dest":        {keys[transfer.accounts[2]], dest},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
	if slices.Contains(keys, chain.SPLProgram) {
		t.Fatal("a Token-2022 transfer references the SPL Token program")
	}
}

type compiledInstruction struct {
	program  byte
	accounts []byte
}

func compiled(t *testing.T, msg []byte) ([]chain.SolanaAddress, []compiledInstruction) {
	t.Helper()
	next := func(n int) []byte {
		if len(msg) < n {
			t.Fatalf("message cut short")
		}
		out := msg[:n]
		msg = msg[n:]
		return out
	}
	next(3)
	keyCount := next(1)[0]
	keys := make([]chain.SolanaAddress, 0, keyCount)
	for range keyCount {
		keys = append(keys, chain.AddressOf(next(32)))
	}
	next(32)
	ixCount := next(1)[0]
	ixs := make([]compiledInstruction, 0, ixCount)
	for range ixCount {
		ix := compiledInstruction{program: next(1)[0]}
		ix.accounts = next(int(next(1)[0]))
		next(int(next(1)[0]))
		ixs = append(ixs, ix)
	}
	return keys, ixs
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

func TestTransfersBuild_relayerSignFailureReturnsNoBytes(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	member := signerFunc(func(_ context.Context, _ string, b []byte) ([]byte, error) {
		tx, _ := chain.DecodeTransaction(b)
		_ = tx.Sign(fakes.PrivyWalletKey("wallet-member"))
		return tx.Encode(), nil
	})
	stranger := relayer.WithKey(s.relayer, fakes.FixtureKey("stranger"))
	signed, err := relayer.NewTransfers(stranger, member).Build(t.Context(), fund(1))
	wantCode(t, err, errs.CodeInternal)
	if signed.Bytes != nil || signed.Signature != "" || signed.LastValidBlockHeight != 0 {
		t.Fatalf("signed = %+v, want the zero SignedTx", signed)
	}
}

func TestTransfersBuild_skipCreateSendsOnlyTheTransfer(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	spec := fund(25_000_000)
	spec.SkipCreate = true
	signed, err := s.transfers.Build(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := chain.DecodeTransaction(signed.Bytes)
	if err != nil || !tx.Signed(0) || !tx.Signed(1) {
		t.Fatalf("tx = %v; relayer signed %v, member signed %v", err, tx.Signed(0), tx.Signed(1))
	}
	keys, ixs := compiled(t, tx.Message)
	if len(ixs) != 1 || keys[ixs[0].program] != chain.SPLProgram {
		t.Fatalf("instructions = %d, want only the TransferChecked and no ATA create", len(ixs))
	}
}

const strandedAccount = treasury

func consolidate(amount uint64) relayer.ConsolidationSpec {
	return relayer.ConsolidationSpec{
		Wallet: member(), Source: strandedAccount, Mint: usdc(), Amount: money.NewBaseUnits(amount, 6),
	}
}

func TestTransfersBuildConsolidation_movesTheStrandedAccountIntoTheAssociatedOne(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	signed, err := s.transfers.BuildConsolidation(t.Context(), consolidate(15_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(s.rpc.calls(), "sendTransaction") {
		t.Fatalf("BuildConsolidation called sendTransaction: %v", s.rpc.calls())
	}
	tx, err := chain.DecodeTransaction(signed.Bytes)
	if err != nil || !tx.Signed(0) || !tx.Signed(1) || signed.Signature != chain.SignatureOf(tx.Signatures[0]) {
		t.Fatalf("tx = %v, signature %s", err, signed.Signature)
	}
	if want := []chain.SolanaAddress{s.relayer.Address(), memberWallet}; !slices.Equal(tx.Signers, want) {
		t.Fatalf("signers = %v, want the relayer as fee payer then the member", tx.Signers)
	}
	ata, _ := chain.AssociatedTokenAccount(memberWallet, usdcMint, chain.SPLProgram)
	keys, ixs := compiled(t, tx.Message)
	if len(ixs) != 2 {
		t.Fatalf("%d instructions, want the ATA create then TransferChecked", len(ixs))
	}
	create, transfer := ixs[0], ixs[1]
	wantKeys(t, map[string][2]chain.SolanaAddress{
		"create program":   {keys[create.program], chain.ATAProgram},
		"create payer":     {keys[create.accounts[0]], s.relayer.Address()},
		"create ata":       {keys[create.accounts[1]], ata},
		"create owner":     {keys[create.accounts[2]], memberWallet},
		"create mint":      {keys[create.accounts[3]], usdcMint},
		"create system":    {keys[create.accounts[4]], chain.SystemProgram},
		"create token":     {keys[create.accounts[5]], chain.SPLProgram},
		"transfer program": {keys[transfer.program], chain.SPLProgram},
		"transfer source":  {keys[transfer.accounts[0]], strandedAccount},
		"transfer mint":    {keys[transfer.accounts[1]], usdcMint},
		"transfer dest":    {keys[transfer.accounts[2]], ata},
		"transfer owner":   {keys[transfer.accounts[3]], memberWallet},
	})
	data := binary.LittleEndian.AppendUint64([]byte{12}, 15_000_000)
	if !bytes.Contains(tx.Message, append(data, 6)) {
		t.Fatal("message lacks TransferChecked for 15 USDC")
	}
	if got := slices.Compact(slices.Sorted(slices.Values(keys))); len(got) != len(keys) {
		t.Fatalf("message repeats an account: %v", keys)
	}
}

func wantKeys(t *testing.T, pairs map[string][2]chain.SolanaAddress) {
	t.Helper()
	for name, pair := range pairs {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
}

func TestTransfersBuildConsolidation_aRetryRebuildsTheSameTransactionAndRebroadcastsIt(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	first, err := s.transfers.BuildConsolidation(t.Context(), consolidate(15_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.transfers.Broadcast(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	again, err := s.transfers.BuildConsolidation(t.Context(), consolidate(15_000_000))
	if err != nil || again.Signature != first.Signature || !bytes.Equal(again.Bytes, first.Bytes) {
		t.Fatalf("rebuild = %s, %v, want the signature %s of the first build", again.Signature, err, first.Signature)
	}
	if err := s.transfers.Broadcast(t.Context(), first); err != nil {
		t.Fatalf("rebroadcast of the stored transaction: %v", err)
	}
	if err := s.transfers.Broadcast(t.Context(), again); err != nil {
		t.Fatalf("broadcast of the rebuilt transaction: %v", err)
	}
}

func TestTransfersBuildConsolidation_refusesBadSpecs(t *testing.T) {
	t.Parallel()
	s := overFakes(t, "relayer")
	ata, _ := chain.AssociatedTokenAccount(memberWallet, usdcMint, chain.SPLProgram)
	edit := func(f func(*relayer.ConsolidationSpec)) relayer.ConsolidationSpec {
		spec := consolidate(10)
		f(&spec)
		return spec
	}
	for name, tc := range map[string]struct {
		spec relayer.ConsolidationSpec
		want errs.Code
	}{
		"bad source":      {edit(func(s *relayer.ConsolidationSpec) { s.Source = "nope" }), errs.CodeInvalidAddress},
		"bad wallet":      {edit(func(s *relayer.ConsolidationSpec) { s.Wallet.Address = "nope" }), errs.CodeInvalidAddress},
		"zero amount":     {consolidate(0), errs.CodeInvalidInput},
		"amount decimals": {edit(func(s *relayer.ConsolidationSpec) { s.Amount = money.NewBaseUnits(10, 9) }), errs.CodeInvalidInput},
		"the associated account itself": {
			edit(func(s *relayer.ConsolidationSpec) { s.Source = ata }), errs.CodeInvalidInput,
		},
		"the relayer's wallet": {
			edit(func(spec *relayer.ConsolidationSpec) { spec.Wallet.Address = s.relayer.Address() }),
			errs.CodeInvalidInput,
		},
		"mint decimals": {
			edit(func(s *relayer.ConsolidationSpec) { s.Mint.Decimals, s.Amount = 9, money.NewBaseUnits(10, 9) }),
			errs.CodeInvalidInput,
		},
	} {
		_, err := s.transfers.BuildConsolidation(t.Context(), tc.spec)
		if err == nil || errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestTransfersBuildConsolidation_upstreamFailures(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"/rpc/getAccountInfo", "/rpc/getLatestBlockhash"} {
		s := overFakes(t, "relayer")
		script(t, s.srv, fakes.Step{Route: route, Action: fakes.ActionFail, Status: 400})
		_, err := s.transfers.BuildConsolidation(t.Context(), consolidate(1))
		wantCode(t, err, errs.CodeRPCUnavailable)
	}
	s := overFakes(t, "relayer")
	down := signerFunc(func(context.Context, string, []byte) ([]byte, error) {
		return nil, errs.New(errs.CodePrivyUnavailable, "test")
	})
	_, err := relayer.NewTransfers(s.relayer, down).BuildConsolidation(t.Context(), consolidate(1))
	wantCode(t, err, errs.CodePrivyUnavailable)
}
