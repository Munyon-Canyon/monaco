package relayer_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"strconv"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/swaptx"
)

func TestCheckFloor_RefusesAtThreshold(t *testing.T) {
	t.Parallel()
	at := overFakes(t, "relayer-at-floor")
	err := at.relayer.CheckFloor(t.Context())
	wantCode(t, err, errs.CodeRelayerUnderfunded)
	if !errs.Alert(errs.CodeOf(err)) || errs.Retryable(errs.CodeOf(err)) {
		t.Fatal("an underfunded relayer alerts and is not retryable")
	}
	if err := overFakes(t, "relayer-above-floor").relayer.CheckFloor(t.Context()); err != nil {
		t.Fatalf("1,000,001 lamports = %v, want boot to pass", err)
	}
	script(t, at.srv, fakes.Step{Route: "/rpc/getBalance", Action: fakes.ActionFail, Status: 400})
	wantCode(t, at.relayer.CheckFloor(t.Context()), errs.CodeRPCUnavailable)
}

func TestCheckBoot_onlyStagingAndProductionCheckTheFloor(t *testing.T) {
	t.Parallel()
	srv := fakes.New()
	for env, want := range map[config.Env]errs.Code{
		config.EnvLocal: "", config.EnvTest: "",
		config.EnvStaging: errs.CodeRelayerUnderfunded, config.EnvProduction: errs.CodeRelayerUnderfunded,
	} {
		cfg := keyConfig("relayer-at-floor")
		cfg.Env = env
		err := relayer.CheckBoot(t.Context(), cfg, httpclient.WithTransport(&recorder{handler: srv}))
		if want == "" && err != nil || want != "" && errs.CodeOf(err) != want {
			t.Fatalf("%s: CheckBoot = %v, want %q", env, err, want)
		}
	}
	cfg := keyConfig("relayer")
	cfg.Relayer.PrivateKey = ""
	wantCode(t, relayer.CheckBoot(t.Context(), cfg), errs.CodeInvalidInput)
}

func TestNew_acceptsBase58AndSolanaCLIJSONForTheSameKey(t *testing.T) {
	t.Parallel()
	key := fakes.FixtureKey("relayer")
	nums := make([]string, len(key))
	for i, b := range key {
		nums[i] = strconv.Itoa(int(b))
	}
	asJSON := "[" + strings.Join(nums, ",") + "]"
	want, err := relayer.New(keyConfig("relayer"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"json":            asJSON,
		"json whitespace": " \n[ " + strings.Join(nums, " , ") + " ]\n",
	} {
		cfg := keyConfig("relayer")
		cfg.Relayer.PrivateKey = raw
		got, err := relayer.New(cfg, nil)
		if err != nil || got.Address() != want.Address() {
			t.Fatalf("%s: Address = %v, %v, want %s", name, got, err, want.Address())
		}
	}
	bad := map[string]string{
		"63 numbers": "[" + strings.Join(nums[:63], ",") + "]",
		"256":        "[256," + strings.Join(nums[1:], ",") + "]",
		"garbage":    "[not json",
		"not base58": "0OIl-not-base58",
		"negative":   "[-1," + strings.Join(nums[1:], ",") + "]",
	}
	for name, raw := range bad {
		cfg := keyConfig("relayer")
		cfg.Relayer.PrivateKey = raw
		_, err := relayer.New(cfg, nil)
		wantCode(t, err, errs.CodeInvalidInput)
		if strings.Contains(err.Error(), nums[1]+","+nums[2]) {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
}

func TestNew_refusesAKeyThatIsNotABase58Keypair(t *testing.T) {
	t.Parallel()
	good := fakes.FixtureKey("relayer")
	mismatched := append(bytes.Clone(good[:32]), fakes.FixtureKey("other")[32:]...)
	for name, raw := range map[string]string{
		"empty":      "",
		"json array": "[1,2,3]",
		"seed only":  chain.EncodeBase58(good[:32]),
		"mismatched": chain.EncodeBase58(mismatched),
	} {
		cfg := keyConfig("relayer")
		cfg.Relayer.PrivateKey = raw
		_, err := relayer.New(cfg, nil)
		wantCode(t, err, errs.CodeInvalidInput)
		if raw != "" && strings.Contains(err.Error(), raw) {
			t.Fatalf("%s: the error echoes the key", name)
		}
	}
	r, err := relayer.New(keyConfig("relayer"), nil)
	if err != nil || r.Address() != "CMa1GUZZLJ6goRKUyvkDKsMeyjysTaZioT3cr596KAYa" {
		t.Fatalf("Address = %v, %v", r, err)
	}
}

func TestCoSign_addsTheFeePayerSignature(t *testing.T) {
	t.Parallel()
	cfg := keyConfig("relayer")
	r, _ := relayer.New(cfg, solana.New(cfg, clock.Real{}))
	payer := fakes.FixtureKey("relayer").Public().(ed25519.PublicKey)
	other := fakes.FixtureKey("other").Public().(ed25519.PublicKey)
	build := func(keys ...ed25519.PublicKey) []byte {
		msg := append(chain.CompactU16(len(keys)), 0, 0)
		msg = append(msg, chain.CompactU16(len(keys))...)
		for _, k := range keys {
			msg = append(msg, k...)
		}
		msg = append(append(msg, make([]byte, 32)...), 0)
		return append(append(chain.CompactU16(len(keys)), make([]byte, 64*len(keys))...), msg...)
	}
	signed, err := r.CoSign(t.Context(), build(payer, other))
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := chain.DecodeTransaction(signed)
	if !tx.Signed(0) || tx.Signed(1) {
		t.Fatal("CoSign must sign the fee payer slot only")
	}
	_, err = r.CoSign(t.Context(), build(other, payer))
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = r.CoSign(t.Context(), []byte{9})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = r.CoSign(t.Context(), build())
	wantCode(t, err, errs.CodeInvalidInput)
	signed, err = relayer.WithKey(r, fakes.FixtureKey("stranger")).CoSign(t.Context(), build(payer, other))
	if signed != nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("CoSign with a key that cannot sign = %x, %v; want no bytes and internal", signed, err)
	}
}

type tablesRPC struct {
	relayer.RPC
	tables chain.LookupTables
	err    error
	calls  int
}

func (r *tablesRPC) LookupTables(context.Context, []chain.SolanaAddress) (chain.LookupTables, error) {
	r.calls++
	return r.tables, r.err
}

func TestCoSign_refusesAnythingOutsideTheSwapPrograms(t *testing.T) {
	t.Parallel()
	cfg := keyConfig("relayer")
	probe, _ := relayer.New(cfg, solana.New(cfg, clock.Real{}))
	spec := swaptx.Spec{
		Payer: probe.Address(), Wallet: swaptx.Address("treasury"), In: usdc(),
		Out: chain.Mint{Address: feeMint, Decimals: 8}, InAmount: 25_000_000, QuotedOut: 11_000_000, SlippageBps: 50,
	}
	honest := swaptx.HonestTx(spec)
	with := func(ix swaptx.Ix) swaptx.Tx {
		tx := honest
		tx.Ixs = append(append([]swaptx.Ix(nil), honest.Ixs...), ix)
		return tx
	}
	token := func(tag byte) swaptx.Ix {
		return swaptx.Ix{Program: chain.SPLProgram, Data: []byte{tag}, Accounts: []chain.SolanaAddress{spec.Wallet}}
	}
	rpc := &tablesRPC{}
	r, _ := relayer.New(cfg, rpc)
	signed, err := r.CoSign(t.Context(), honest.Bytes())
	if tx, _ := chain.DecodeTransaction(signed); err != nil || !tx.Signed(0) || rpc.calls != 0 {
		t.Fatalf("CoSign of an honest swap = %v, %d table reads", err, rpc.calls)
	}
	for name, tx := range map[string]swaptx.Tx{
		"unknown program": with(swaptx.Ix{Program: swaptx.Address("drainer"), Data: []byte{1}}),
		"set authority":   with(token(6)),
		"close account":   with(token(9)),
		"transfer":        with(token(3)),
	} {
		signed, err := r.CoSign(t.Context(), tx.Bytes())
		if signed != nil || errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: CoSign = %x, %v; want no bytes and invalid_input", name, signed, err)
		}
	}
}

func TestCoSign_readsTheLookupTablesBeforeJudgingTheInstructions(t *testing.T) {
	t.Parallel()
	cfg := keyConfig("relayer")
	probe, _ := relayer.New(cfg, solana.New(cfg, clock.Real{}))
	spec := swaptx.Spec{
		Payer: probe.Address(), Wallet: swaptx.Address("treasury"), In: usdc(),
		Out: chain.Mint{Address: feeMint, Decimals: 8}, InAmount: 25_000_000, QuotedOut: 11_000_000, SlippageBps: 50,
	}
	honest := swaptx.HonestTx(spec)
	honest.Table, honest.Tabled = swaptx.Address("table"), []chain.SolanaAddress{
		spec.Out.Address, usdc().Address, swaptx.Address("event-authority"),
	}
	rpc := &tablesRPC{tables: honest.Tables()}
	r, _ := relayer.New(cfg, rpc)
	if _, err := r.CoSign(t.Context(), honest.Bytes()); err != nil || rpc.calls != 1 {
		t.Fatalf("CoSign with a table = %v after %d table reads, want one read and success", err, rpc.calls)
	}
	rpc = &tablesRPC{err: errs.New(errs.CodeRPCUnavailable, "test")}
	r, _ = relayer.New(cfg, rpc)
	if signed, err := r.CoSign(
		t.Context(),
		honest.Bytes(),
	); signed != nil ||
		errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("CoSign when the table cannot be read = %x, %v; want no bytes and rpc_unavailable", signed, err)
	}
	rpc = &tablesRPC{tables: chain.LookupTables{honest.Table: nil}}
	r, _ = relayer.New(cfg, rpc)
	if signed, err := r.CoSign(
		t.Context(),
		honest.Bytes(),
	); signed != nil ||
		errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("CoSign with an empty table = %x, %v; want no bytes and invalid_input", signed, err)
	}
}
