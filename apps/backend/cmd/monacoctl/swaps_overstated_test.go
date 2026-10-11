package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	soldTreasury = "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin"
	soldUSDC     = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
)

type soldSwap struct {
	action, signature string
	recorded          int64
}

func seededSale(t *testing.T, pool *pgxpool.Pool, identifiers *testkit.IDs, cabal uuid.UUID, at time.Time,
	s soldSwap,
) uuid.UUID {
	t.Helper()
	id := identifiers.NewV7()
	q := sqlc.New(pool)
	in, out := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", soldUSDC
	if s.action == "buy" {
		in, out = out, in
	}
	if err := q.InsertCreated(t.Context(), sqlc.InsertCreatedParams{
		ID: id, SourceKind: "proposal", SourceID: identifiers.NewV7(), CabalID: cabal, TreasuryAddress: soldTreasury,
		Action: s.action, Symbol: "GOOGLx", InMint: in, OutMint: out, OutDecimals: 6, InAmount: 1_000_000,
		SlippageBps: 100, SourceBatchSize: 1, CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.MarkSubmitted(t.Context(), sqlc.MarkSubmittedParams{
		ID: id, ExecuteRequestID: "req-" + id.String(), SignedTx: []byte{1}, TxSignature: s.signature, SubmittedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.FinishConfirmed(t.Context(), sqlc.FinishConfirmedParams{
		ID: id, OutAmount: s.recorded, ConfirmedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func soldOnChain(t *testing.T, received map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Params []json.RawMessage `json:"params"`
		}
		var sig string
		if json.NewDecoder(r.Body).Decode(&call) != nil || len(call.Params) == 0 ||
			json.Unmarshal(call.Params[0], &sig) != nil {
			http.Error(w, "bad call", http.StatusBadRequest)
			return
		}
		amount, ok := received[sig]
		result := "null"
		if ok {
			balance := func(v string) string {
				return `{"accountIndex":0,"mint":"` + soldUSDC + `","owner":"` + soldTreasury +
					`","uiTokenAmount":{"amount":"` + v + `","decimals":6}}`
			}
			result = `{"meta":{"err":null,"preTokenBalances":[` + balance("100") + `],"postTokenBalances":[` +
				balance(amount) + `]},"transaction":{"message":{}}}`
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestSwapsOverstatedSells_listsEachSellRecordedAboveItsChainDeltaWithPerCabalTotals(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	identifiers := testkit.NewIDs(1)
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	rpc := soldOnChain(t, map[string]string{
		"sig-googl": "4881082", "sig-nvda": "4990660", "sig-exact": "1000100", "sig-other": "2000100", "sig-buy": "100",
	})
	swaps := swapsTool([]string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(),
		"SOLANA_RPC_URL=" + rpc, "SOLANA_USDC_MINT=" + soldUSDC,
	}, testkit.NewClock(now), strings.NewReader(""))
	if code, out, stderr := runSwaps(t, swaps, "overstated-sells"); code != 0 || out != "no overstated sells\n" {
		t.Fatalf("empty overstated-sells = %d, %q, stderr %q", code, out, stderr)
	}
	first, second := uuid.MustParse("00000000-0000-7000-8000-000000000001"),
		uuid.MustParse("00000000-0000-7000-8000-000000000002")
	googl := seededSale(t, pool, identifiers, first, now, soldSwap{"sell", "sig-googl", 4_885_867})
	nvda := seededSale(t, pool, identifiers, first, now.Add(time.Minute), soldSwap{"sell", "sig-nvda", 4_995_555})
	seededSale(t, pool, identifiers, first, now, soldSwap{"sell", "sig-exact", 1_000_000})
	other := seededSale(t, pool, identifiers, second, now, soldSwap{"sell", "sig-other", 2_000_100})
	seededSale(t, pool, identifiers, second, now, soldSwap{"buy", "sig-buy", 50})
	unread := seededSale(t, pool, identifiers, second, now, soldSwap{"sell", "sig-gone", 3_000_000})

	code, out, stderr := runSwaps(t, swaps, "overstated-sells")

	want := strings.Join([]string{
		"swap=" + googl.String() + " cabal=" + first.String() +
			" symbol=GOOGLx signature=sig-googl recorded=4885867 chain=4880982 overstated=4885",
		"swap=" + nvda.String() + " cabal=" + first.String() +
			" symbol=GOOGLx signature=sig-nvda recorded=4995555 chain=4990560 overstated=4995",
		"swap=" + other.String() + " cabal=" + second.String() +
			" symbol=GOOGLx signature=sig-other recorded=2000100 chain=2000000 overstated=100",
		"cabal=" + first.String() + " sells=2 overstated_micros=9880",
		"cabal=" + second.String() + " sells=1 overstated_micros=100",
		"unread swap=" + unread.String() + " cabal=" + second.String() +
			" signature=sig-gone recorded=3000000 code=not_found",
	}, "\n") + "\n"
	if code != 0 || out != want {
		t.Fatalf("overstated-sells = %d, stderr %q\nstdout:\n%s\nwant:\n%s", code, stderr, out, want)
	}
}

func TestSwapsOverstatedSells_refusesArgumentsAndReportsAMissingDatabase(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "")
	if code, _, stderr := runSwaps(t, swaps, "overstated-sells", "--since", "x"); code != 2 ||
		!strings.Contains(stderr, "overstated-sells") {
		t.Fatalf("overstated-sells with an argument = %d, stderr %q, want the usage", code, stderr)
	}
	missing := strings.Replace(pool.Config().ConnString(), "/"+pool.Config().ConnConfig.Database,
		"/missing_"+identifiers.NewV7().String()[:8], 1)
	gone := swapsTool([]string{"MONACO_ENV=test", "DATABASE_URL=" + missing, "NATS_URL=" + testkit.NATSURL()},
		testkit.NewClock(now), strings.NewReader(""))
	if code, _, stderr := runSwaps(t, gone, "overstated-sells"); code != 1 || stderr == "" {
		t.Fatalf("overstated-sells on a missing database = %d, stderr %q, want 1", code, stderr)
	}
}
