package mintfacts_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/mintfacts"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func chainOverFakes(t *testing.T, steps ...fakes.Step) *mintfacts.Chain {
	t.Helper()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	for _, step := range steps {
		body, err := json.Marshal(step)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/_script", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("script %+v: status %d", step, resp.StatusCode)
		}
	}
	cfg := config.Config{
		Solana:   config.Solana{RPCURL: srv.URL + "/rpc/"},
		Timeouts: config.Timeouts{RPC: 5 * time.Second},
	}
	return mintfacts.New(solana.New(cfg, clock.Real{}))
}

func TestChain_readsTheMintDecimalsAndTheMultiplierInForce(t *testing.T) {
	t.Parallel()
	decimals, num, den, err := chainOverFakes(t).Facts(t.Context(), marketfake.AAPLx().Mint)
	if err != nil || decimals != 8 || num != 10_032_690_125_398_187 || den != 10_000_000_000_000_000 {
		t.Fatalf("Facts(AAPLx) = %d, %d/%d, %v, want 8 decimals and the chain's multiplier", decimals, num, den, err)
	}
}

func TestChain_anRPCFailureKeepsItsCodeAndNamesTheMint(t *testing.T) {
	t.Parallel()
	tsla := marketfake.TSLAx().Mint
	c := chainOverFakes(t, fakes.Step{Route: "/rpc/getAccountInfo", Action: fakes.ActionFail, Status: 500})
	_, _, _, err := c.Facts(t.Context(), tsla)
	names := func(a slog.Attr) bool { return a.Equal(slog.String("mint", tsla.String())) }
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || !slices.ContainsFunc(errs.Detail(err), names) {
		t.Fatalf("Facts(TSLAx) with the RPC failing = %v, want rpc_unavailable naming the mint", err)
	}
}
