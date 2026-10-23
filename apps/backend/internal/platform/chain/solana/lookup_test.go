package solana_test

import (
	"encoding/base64"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

func tableAccount(owner string, entries ...chain.SolanaAddress) string {
	raw := make([]byte, 0, 56+len(entries)*32)
	raw = append(raw, make([]byte, 56)...)
	for _, e := range entries {
		b, _ := e.Bytes()
		raw = append(raw, b...)
	}
	return `{"owner":"` + owner + `","data":["` + base64.StdEncoding.EncodeToString(raw) + `","base64"],"lamports":1}`
}

func TestLookupTables_readsTheAddressesOfEachTable(t *testing.T) {
	t.Parallel()
	body := `{"context":{"slot":9},"value":[` +
		tableAccount(string(solana.LookupTableProgram), member, sender) + `,` +
		tableAccount(string(solana.LookupTableProgram)) + `]}`
	u := result(body)
	c := client(u)
	got, err := c.LookupTables(t.Context(), []chain.SolanaAddress{feeMint, usdcMint})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got[feeMint], []chain.SolanaAddress{member, sender}) || len(got[usdcMint]) != 0 || len(got) != 2 {
		t.Fatalf("LookupTables = %+v", got)
	}
	request := u.requests()[0]
	if request.method != "getMultipleAccounts" ||
		string(request.params[1]) != `{"commitment":"confirmed","encoding":"base64"}` {
		t.Fatalf("request = %+v", request)
	}
}

func TestLookupTables_refusesWhatTheRuntimeWouldNotLoad(t *testing.T) {
	t.Parallel()
	good := tableAccount(string(solana.LookupTableProgram), member)
	for name, value := range map[string]string{
		"missing":          `null`,
		"owned by another": tableAccount(string(chain.SystemProgram), member),
		"wrong encoding":   `{"owner":"` + string(solana.LookupTableProgram) + `","data":["AAAA","base58"]}`,
		"short header":     `{"owner":"` + string(solana.LookupTableProgram) + `","data":["AAAA","base64"]}`,
		"ragged entries":   `{"owner":"` + string(solana.LookupTableProgram) + `","data":["` + base64.StdEncoding.EncodeToString(make([]byte, 60)) + `","base64"]}`,
		"bad base64":       `{"owner":"` + string(solana.LookupTableProgram) + `","data":["%%%","base64"]}`,
	} {
		c := client(result(`{"context":{"slot":9},"value":[` + value + `]}`))
		if _, err := c.LookupTables(
			t.Context(),
			[]chain.SolanaAddress{feeMint},
		); errs.CodeOf(
			err,
		) != errs.CodeDecodeFailed {
			t.Fatalf("%s: err = %v, want decode_failed", name, err)
		}
	}
	c := client(result(`{"context":{"slot":9},"value":[` + good + `,` + good + `]}`))
	if _, err := c.LookupTables(
		t.Context(),
		[]chain.SolanaAddress{feeMint},
	); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("reply with an extra account: err = %v, want decode_failed", err)
	}
	if _, err := c.LookupTables(
		t.Context(),
		[]chain.SolanaAddress{"not-an-address"},
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidAddress {
		t.Fatalf("bad address: err = %v, want invalid_address", err)
	}
	if _, err := c.LookupTables(
		t.Context(),
		make([]chain.SolanaAddress, 101),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("101 tables: err = %v, want invalid_input", err)
	}
	down := client(&upstream{transport: errs.New(errs.CodeUpstreamUnavailable, "test")})
	if _, err := down.LookupTables(
		t.Context(),
		[]chain.SolanaAddress{feeMint},
	); errs.CodeOf(
		err,
	) != errs.CodeRPCUnavailable {
		t.Fatalf("rpc down: err = %v, want rpc_unavailable", err)
	}
}
