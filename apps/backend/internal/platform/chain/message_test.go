package chain_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/swaptx"
)

func messageOf(t *testing.T, raw []byte) chain.Message {
	t.Helper()
	tx, err := chain.DecodeTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := chain.ParseMessage(tx.Message)
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestParseMessage_readsLegacyAndVersionedMessages(t *testing.T) {
	t.Parallel()
	honest := swaptx.HonestTx(spec())
	legacy := messageOf(t, honest.Bytes())
	if legacy.Versioned || len(legacy.Tables()) != 0 || len(legacy.Instructions) != len(honest.Ixs) ||
		legacy.Signing()[0] != payer() || legacy.Signing()[1] != wallet() {
		t.Fatalf("legacy message = %+v", legacy)
	}
	withTable := tabled(honest)
	versioned := messageOf(t, withTable.Bytes())
	if !versioned.Versioned || len(versioned.Tables()) != 1 || versioned.Tables()[0] != withTable.Table {
		t.Fatalf("versioned message = %+v", versioned)
	}
	ixs, err := versioned.Resolve(withTable.Tables())
	if err != nil {
		t.Fatal(err)
	}
	sameInstructions(t, ixs, honest.Ixs)
}

func sameInstructions(t *testing.T, got []chain.Instruction, want []swaptx.Ix) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d instructions, want %d", len(got), len(want))
	}
	for i, ix := range got {
		if ix.Program != want[i].Program || !slices.Equal(ix.Accounts, want[i].Accounts) {
			t.Fatalf("instruction %d = %+v, want %+v", i, ix, want[i])
		}
	}
}

func TestParseMessage_refusesMalformedBytes(t *testing.T) {
	t.Parallel()
	good := messageOf(t, swaptx.Honest(spec()))
	tx, _ := chain.DecodeTransaction(swaptx.Honest(spec()))
	raw := tx.Message
	versioned, _ := chain.DecodeTransaction(tabled(swaptx.HonestTx(spec())).Bytes())
	cases := map[string][]byte{
		"empty":                nil,
		"unknown version":      append([]byte{0x81}, raw[1:]...),
		"truncated":            raw[:len(raw)-1],
		"trailing bytes":       append(append([]byte(nil), raw...), 0),
		"truncated in a key":   raw[:40],
		"truncated lookups":    versioned.Message[:len(versioned.Message)-1],
		"key count cut off":    append(append([]byte(nil), raw[:3]...), 0x80, 0x80, 0x80),
		"no signers":           append([]byte{0}, raw[1:]...),
		"all signers readonly": append([]byte{raw[0], raw[0]}, raw[2:]...),
	}
	for name, msg := range cases {
		if _, err := chain.ParseMessage(msg); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid_input", name, err)
		}
	}
	if len(good.Keys) == 0 {
		t.Fatal("no keys")
	}
}

func TestResolve_refusesUnloadedTablesAndBadIndexes(t *testing.T) {
	t.Parallel()
	tx := tabled(swaptx.HonestTx(spec()))
	msg := messageOf(t, tx.Bytes())
	if _, err := msg.Resolve(nil); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Resolve without the table = %v, want invalid_input", err)
	}
	short := chain.LookupTables{tx.Table: tx.Tabled[:1]}
	if _, err := msg.Resolve(short); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Resolve with a short table = %v, want invalid_input", err)
	}
	writable := msg
	writable.Lookups = []chain.AddressLookup{{Table: tx.Table, Writable: []int{len(tx.Tabled)}}}
	if _, err := writable.Resolve(tx.Tables()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Resolve with a writable index past the table = %v, want invalid_input", err)
	}
	program := msg
	program.Instructions = []chain.CompiledInstruction{{Program: len(msg.Keys)}}
	if _, err := program.Resolve(tx.Tables()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Resolve with a program loaded from a table = %v, want invalid_input", err)
	}
	account := msg
	account.Instructions = []chain.CompiledInstruction{{Program: 0, Accounts: []int{1 << 12}}}
	if _, err := account.Resolve(tx.Tables()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Resolve with an account past the list = %v, want invalid_input", err)
	}
	writableOK := msg
	writableOK.Lookups = []chain.AddressLookup{{Table: tx.Table, Writable: []int{0}, Readonly: []int{0}}}
	writableOK.Instructions = []chain.CompiledInstruction{
		{Program: 0, Accounts: []int{len(msg.Keys), len(msg.Keys) + 1}},
	}
	ixs, err := writableOK.Resolve(tx.Tables())
	if err != nil || ixs[0].Accounts[0] != tx.Tabled[0] || ixs[0].Accounts[1] != tx.Tabled[0] {
		t.Fatalf("Resolve of writable then readonly = %+v, %v", ixs, err)
	}
}

func TestResolveInstructions_failsOnBadTransactionsAndMissingTables(t *testing.T) {
	t.Parallel()
	if _, _, err := chain.ResolveInstructions([]byte{9}, nil); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("garbage = %v, want invalid_input", err)
	}
	tx := swaptx.HonestTx(spec()).Bytes()
	bad := append(append([]byte(nil), tx...), 0)
	if _, _, err := chain.ResolveInstructions(bad, nil); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("trailing byte = %v, want invalid_input", err)
	}
	withTable := tabled(swaptx.HonestTx(spec()))
	_, _, err := chain.ResolveInstructions(withTable.Bytes(), func([]chain.SolanaAddress) (chain.LookupTables, error) {
		return nil, errs.New(errs.CodeRPCUnavailable, "test")
	})
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("table fetch failure = %v, want rpc_unavailable", err)
	}
}
