package mintfacts_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/mintfacts"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
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
	aapl := marketfake.AAPLx().Mint
	facts, failures, err := chainOverFakes(t).Facts(t.Context(), []domain.Mint{aapl})
	got := facts[aapl]
	if err != nil || len(failures) != 0 || got.Decimals != 8 ||
		got.MultiplierNum != 10_032_690_125_398_187 || got.MultiplierDen != 10_000_000_000_000_000 {
		t.Fatalf("Facts(AAPLx) = %+v, %v, %v, want 8 decimals and the chain's multiplier", facts, failures, err)
	}
}

func TestChain_anRPCFailureKeepsItsCodeAndNamesTheMint(t *testing.T) {
	t.Parallel()
	tsla := marketfake.TSLAx().Mint
	c := chainOverFakes(t, fakes.Step{Route: "/rpc/getMultipleAccounts", Action: fakes.ActionFail, Status: 500})
	_, _, err := c.Facts(t.Context(), []domain.Mint{tsla})
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Facts(TSLAx) with the RPC failing = %v, want rpc_unavailable", err)
	}
}

func TestChain_returnsPerMintFailures(t *testing.T) {
	t.Parallel()
	mint, err := domain.ParseMint("11111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	_, failures, err := chainOverFakes(t).Facts(t.Context(), []domain.Mint{mint})
	if err != nil || errs.CodeOf(failures[mint]) != errs.CodeNotFound {
		t.Fatalf("Facts = %v, %v", failures, err)
	}
}

func TestChain_keepsTheFirstChunkWhenTheSecondFails(t *testing.T) {
	t.Parallel()
	mints := make([]domain.Mint, 101)
	mints[0] = marketfake.AAPLx().Mint
	for i := 1; i < len(mints); i++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("mint-%d", i)))
		mint, err := domain.ParseMint(string(chain.AddressOf(sum[:])))
		if err != nil {
			t.Fatal(err)
		}
		mints[i] = mint
	}
	c := chainOverFakes(t,
		fakes.Step{Route: "/rpc/getMultipleAccounts", Action: fakes.ActionSucceed, Times: 1},
		fakes.Step{Route: "/rpc/getMultipleAccounts", Action: fakes.ActionFail, Status: 500, Times: 10},
	)
	facts, _, err := c.Facts(t.Context(), mints)
	if _, ok := facts[mints[0]]; !ok || errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Facts = %v, %v; want AAPLx and rpc_unavailable", facts, err)
	}
}
