package relayer_test

import (
	"bytes"
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
