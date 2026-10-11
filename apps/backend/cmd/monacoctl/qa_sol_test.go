package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestQAFundSOL_sendsTheExactLamportsFromThePotWithTheSystemProgram(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	user := uuid.New()
	code, stdout, stderr := h.run("fund", "--user", user.String(), "--sol", "0.002")
	if code != 0 || len(h.rpc.sent) != 1 || len(h.lookups) != 1 || h.lookups[0] != user {
		t.Fatalf("fund = %d, %d sends, lookups %v, stderr %q", code, len(h.rpc.sent), h.lookups, stderr)
	}
	checkPotSOLTransfer(t, h, stdout)
}

func checkPotSOLTransfer(t *testing.T, h *qaHarness, stdout string) {
	t.Helper()
	tx, err := chain.DecodeTransaction(h.rpc.sent[0])
	if err != nil || len(tx.Signatures) != 1 || !tx.Signed(0) {
		t.Fatalf("tx = %+v, %v; want one signature by the pot", tx, err)
	}
	header, keys, ixs := decodeQAMessage(t, tx.Message)
	want := binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint32(nil, 2), 2_000_000)
	if header != [3]byte{1, 0, 1} || len(ixs) != 1 || ixs[0].program != chain.SystemProgram ||
		!slices.Equal(ixs[0].accounts, []chain.SolanaAddress{h.pot.address, qaMember}) ||
		!bytes.Equal(ixs[0].data, want) || keys[0] != h.pot.address {
		t.Fatalf("header %v instructions %+v", header, ixs)
	}
	if !strings.Contains(stdout, "sent 0.002000000 SOL to "+string(qaMember)) ||
		!strings.Contains(stdout, string(chain.SignatureOf(tx.Signatures[0]))) {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestQAFundSOL_aHundredthOfASOLIsTheLargestTransfer(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	if code, _, stderr := h.run(
		"fund",
		"--user",
		uuid.NewString(),
		"--sol",
		"0.01",
	); code != 0 ||
		len(h.rpc.sent) != 1 {
		t.Fatalf("fund 0.01 SOL = %d, stderr %q", code, stderr)
	}
	for _, sol := range []string{"0.010000001", "1", "0", "-1", "0.0000000001", "x"} {
		h := newQAHarness(t)
		if code, _, stderr := h.run(
			"fund",
			"--user",
			uuid.NewString(),
			"--sol",
			sol,
		); code != 1 ||
			len(h.rpc.sent) != 0 ||
			len(h.lookups) != 0 {
			t.Fatalf("--sol %s = %d with %d sends, stderr %q; want a refusal before any lookup", sol, code,
				len(h.rpc.sent), stderr)
		}
	}
}

func TestQAFundSOL_refusesAPotThatWouldDropBelowItsReserve(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	h.rpc.lamports = qaMinPotLamport + 1_999_999
	code, _, stderr := h.run("fund", "--user", uuid.NewString(), "--sol", "0.002")
	if code != 1 || len(h.rpc.sent) != 0 || !strings.Contains(stderr, string(h.pot.address)) {
		t.Fatalf("fund = %d, %d sends, stderr %q", code, len(h.rpc.sent), stderr)
	}
}

func TestQAFund_takesExactlyOneOfUSDCAndSOL(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	code, _, stderr := h.run("fund", "--user", uuid.NewString(), "--usdc", "1", "--sol", "0.001")
	if code != 2 || !strings.Contains(stderr, qaUsage) || len(h.rpc.sent) != 0 {
		t.Fatalf("both = %d, stderr %q", code, stderr)
	}
}

func TestQAFundSOL_reportsEachRPCFailure(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"SOLBalance", "LatestBlockhash", "SendTransaction"} {
		h := newQAHarness(t)
		h.rpc.fail = map[string]error{method: errors.New(method + " down")}
		if code, _, stderr := h.run("fund", "--user", uuid.NewString(), "--sol", "0.001"); code != 1 ||
			!strings.Contains(stderr, method+" down") {
			t.Fatalf("%s failing: exit %d, stderr %q", method, code, stderr)
		}
	}
}

func TestQAFundSOL_namesAUserWithoutAMemberWallet(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	h.env.wallet = func(context.Context, config.Config, uuid.UUID) (chain.SolanaAddress, error) {
		return "", errNoMemberWallet
	}
	user := uuid.NewString()
	if code, _, stderr := h.run("fund", "--user", user, "--sol", "0.001"); code != 1 ||
		!strings.Contains(stderr, "no member wallet for user "+user) || len(h.rpc.sent) != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}
