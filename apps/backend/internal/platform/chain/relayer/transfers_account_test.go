package relayer_test

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type destinationAccounts struct {
	next    http.Handler
	present bool
	fail    bool
}

func (d destinationAccounts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	var call struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	var addrs []string
	if json.Unmarshal(body, &call) != nil || call.Method != "getMultipleAccounts" || len(call.Params) == 0 ||
		json.Unmarshal(call.Params[0], &addrs) != nil || false {
		d.next.ServeHTTP(w, r)
		return
	}
	if d.fail {
		http.Error(w, "rpc down", http.StatusBadRequest)
		return
	}
	value := make([]any, len(addrs))
	for i := range addrs {
		if d.present {
			value[i] = map[string]any{"owner": chain.SPLProgram, "data": map[string]any{"parsed": map[string]any{
				"info": map[string]any{
					"mint": usdcMint, "owner": treasury, "state": "initialized",
					"tokenAmount": map[string]any{"amount": "0", "decimals": 6},
				},
			}}}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": call.ID, "result": map[string]any{"context": map[string]any{"slot": 9}, "value": value},
	})
}

func overDestination(t *testing.T, present bool) stack {
	t.Helper()
	return overDestinationFailing(t, present, false)
}

func overDestinationFailing(t *testing.T, present, fail bool) stack {
	t.Helper()
	srv := fakes.New()
	rpc := &recorder{handler: destinationAccounts{next: srv, present: present, fail: fail}}
	cfg := keyConfig("relayer")
	r, err := relayer.New(cfg, solana.New(cfg, clock.Real{}, httpclient.WithTransport(rpc)))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := privy.New(cfg, clock.Real{}, httpclient.WithTransport(&recorder{handler: srv}))
	if err != nil {
		t.Fatal(err)
	}
	return stack{relayer: r, transfers: relayer.NewTransfers(r, signer), rpc: rpc, srv: srv}
}

func TestTransfersBuild_skipCreateRefusesADestinationWithoutAUSDCAccount(t *testing.T) {
	t.Parallel()
	s := overDestination(t, false)
	spec := fund(25_000_000)
	spec.SkipCreate = true
	signed, err := s.transfers.Build(t.Context(), spec)
	wantCode(t, err, errs.CodeWithdrawNoUSDCAccount)
	if signed.Bytes != nil || slices.Contains(s.rpc.calls(), "getLatestBlockhash") {
		t.Fatalf("signed = %+v, calls = %v, want nothing built for a missing account", signed, s.rpc.calls())
	}
}

func TestTransfersBuild_withoutSkipCreateNeverLooksTheAccountUp(t *testing.T) {
	t.Parallel()
	s := overDestination(t, false)
	if _, err := s.transfers.Build(t.Context(), fund(25_000_000)); err != nil {
		t.Fatal(err)
	}
	if n := len(slices.DeleteFunc(s.rpc.calls(), func(m string) bool { return m != "getMultipleAccounts" })); n != 0 {
		t.Fatalf("getMultipleAccounts calls = %d, want none without SkipCreate", n)
	}
}

func TestTransfersBuild_skipCreateSurfacesAnAccountReadFailure(t *testing.T) {
	t.Parallel()
	s := overDestinationFailing(t, true, true)
	spec := fund(25_000_000)
	spec.SkipCreate = true
	_, err := s.transfers.Build(t.Context(), spec)
	wantCode(t, err, errs.CodeRPCUnavailable)
}

func TestHasTokenAccount_readsTheAssociatedAccountOfTheOwner(t *testing.T) {
	t.Parallel()
	for _, present := range []bool{true, false} {
		s := overDestination(t, present)
		got, err := s.transfers.HasTokenAccount(t.Context(), treasury, usdc())
		if err != nil || got != present {
			t.Fatalf("HasTokenAccount with the account present=%t = %t, %v", present, got, err)
		}
	}
}

func TestHasTokenAccount_surfacesUpstreamFailures(t *testing.T) {
	t.Parallel()
	down := overDestinationFailing(t, true, true)
	if _, err := down.transfers.HasTokenAccount(
		t.Context(),
		treasury,
		usdc(),
	); errs.CodeOf(
		err,
	) != errs.CodeRPCUnavailable {
		t.Fatalf("accounts down err = %v, want %s", err, errs.CodeRPCUnavailable)
	}
	noMint := overDestination(t, true)
	script(t, noMint.srv, fakes.Step{Route: "/rpc/getAccountInfo", Action: fakes.ActionFail, Status: 400})
	if _, err := noMint.transfers.HasTokenAccount(
		t.Context(),
		treasury,
		usdc(),
	); errs.CodeOf(
		err,
	) != errs.CodeRPCUnavailable {
		t.Fatalf("mint down err = %v, want %s", err, errs.CodeRPCUnavailable)
	}
}
