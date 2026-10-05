package trading_test

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

const nvdaxMint = "Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh"

func tslaxToken() platform.Mint { return platform.Mint{Address: tslaxMint, Decimals: 8} }

func nvdaxToken() platform.Mint { return platform.Mint{Address: nvdaxMint, Decimals: 8} }

type cashOutEnv struct {
	*layerEnv
	cabal ids.CabalID
	job   uuid.UUID

	mu           sync.Mutex
	holdings     []app.Holding
	positions    int
	positionsErr error
	walletErr    error
}

func newCashOutEnv(t *testing.T) *cashOutEnv {
	t.Helper()
	c := &cashOutEnv{layerEnv: newLayerEnv(t)}
	c.cabal, c.job = ids.CabalIDFrom(c.ids.NewV7()), c.ids.NewV7()
	c.holdings = []app.Holding{
		{Mint: usdcToken(), Symbol: "USDC", Units: 900_000_000},
		{Mint: tslaxToken(), Symbol: "TSLAx", Units: 300},
		{Mint: nvdaxToken(), Symbol: "NVDAx", Units: 7},
		{Mint: aaplxToken(), Symbol: "AAPLx", Units: 1_000},
		{Mint: platform.Mint{Address: "XsEmpty", Decimals: 8}, Symbol: "EMPTYx", Units: 0},
	}
	c.price(aaplxToken(), 50_000_000)
	c.price(tslaxToken(), 30_000_000)
	for i, m := range []platform.Mint{aaplxToken(), tslaxToken()} {
		tx := swapTx()
		tx[len(tx)-2] = byte(i + 1)
		c.jup.SetOrder(
			jupiterMint(m),
			jupiterMint(usdcToken()),
			jupiter.Order{RequestID: "req-" + string(m.Address), Transaction: tx},
		)
	}
	c.fill(aaplxToken(), jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 49_800_000})
	c.fill(tslaxToken(), jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 10_550_000})
	return c
}

func (c *cashOutEnv) price(m platform.Mint, usdc uint64) {
	c.jup.SetQuote(jupiterMint(m), jupiterMint(usdcToken()), jupiter.Quote{
		OutAmount: money.NewBaseUnits(usdc, 6), Routable: true,
	})
}

func (c *cashOutEnv) fill(m platform.Mint, r jupiter.ExecuteResult) {
	c.jup.SetExecute("req-"+string(m.Address), r)
}

func (c *cashOutEnv) Positions(_ context.Context, cabal ids.CabalID) ([]app.Holding, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.positions++
	if cabal != c.cabal {
		return nil, errs.New(errs.CodeInternal, "test: positions read for the wrong cabal")
	}
	return c.holdings, c.positionsErr
}

func (c *cashOutEnv) TreasuryWallet(_ context.Context, cabal ids.CabalID) (app.TreasuryWallet, error) {
	if cabal != c.cabal {
		return app.TreasuryWallet{}, errs.New(errs.CodeInternal, "test: treasury wallet read for the wrong cabal")
	}
	return app.TreasuryWallet{
		PrivyWalletID: treasuryWallet, Address: chainfake.WalletAddress(treasuryWallet),
	}, c.walletErr
}

func (c *cashOutEnv) handler() *app.SellForCashOutHandler {
	return app.NewSellForCashOutHandler(app.SellForCashOutDeps{
		Layer: c.layer(), UoW: c.uow, Reads: c.reads, Clock: c.clk, Venue: c.venue,
		Holdings: c, Wallets: c, USDC: usdcToken(),
	})
}

func (c *cashOutEnv) sell(t *testing.T, needed uint64) error {
	t.Helper()
	return c.handler().Handle(actorContext(t.Context()), app.SellForCashOut{
		CabalID: c.cabal, Source: domain.Source{Kind: domain.SourceCashout, ID: c.job},
		USDCNeeded: money.MicrosFromUint64(needed),
	}, nil)
}

type soldLeg struct {
	Symbol      string
	Units       int64
	QuoteUSDC   int64
	Status      string
	SlippageBps int32
}

func (c *cashOutEnv) legs(t *testing.T) []soldLeg {
	t.Helper()
	rows, err := c.pool.Query(t.Context(), `SELECT symbol, in_amount, quote_out_amount, status, slippage_bps FROM swaps
		WHERE source_kind = 'cashout' AND source_id = $1 AND action = 'sell' AND out_mint = $2 AND cabal_id = $3
		ORDER BY created_at, id`, c.job, usdcMint, c.cabal.UUID())
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[soldLeg])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (c *cashOutEnv) plan(t *testing.T) string {
	t.Helper()
	var legs string
	err := c.pool.QueryRow(t.Context(), `SELECT legs::text FROM cashout_sell_plans WHERE job_id = $1`, c.job).
		Scan(&legs)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return legs
}

func assertLegs(t *testing.T, got []soldLeg, want ...soldLeg) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("legs = %+v\nwant %+v", got, want)
	}
}

func confirmedLeg(symbol string, units, quote int64) soldLeg {
	return soldLeg{Symbol: symbol, Units: units, QuoteUSDC: quote, Status: "confirmed", SlippageBps: 100}
}

func TestSellForCashOut_OneHoldingCovers(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	if err := c.sell(t, 20_000_000); err != nil {
		t.Fatal(err)
	}
	assertLegs(t, c.legs(t), confirmedLeg("AAPLx", 404, 20_200_000))
}

func TestSellForCashOut_SpansTwoHoldings(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	if err := c.sell(t, 60_000_000); err != nil {
		t.Fatal(err)
	}
	assertLegs(t, c.legs(t), confirmedLeg("AAPLx", 1_000, 50_000_000), confirmedLeg("TSLAx", 106, 10_600_000))
}

func TestSellForCashOut_AllHoldingsShort_SellsEverything(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	if err := c.sell(t, 100_000_000); err != nil {
		t.Fatal(err)
	}
	assertLegs(t, c.legs(t), confirmedLeg("AAPLx", 1_000, 50_000_000), confirmedLeg("TSLAx", 300, 30_000_000))
}

func TestSellForCashOut_ZeroNeeded_NoOp(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	if err := c.sell(t, 0); err != nil {
		t.Fatal(err)
	}
	if legs, plan := c.legs(t), c.plan(t); len(legs) != 0 || plan != "" || c.positions != 0 {
		t.Fatalf("zero needed sold %+v, stored plan %q and read positions %d times", legs, plan, c.positions)
	}
}

func TestSellForCashOut_Redelivery_SamePlan(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	c.walletErr = errs.New(errs.CodeUpstreamTimeout, "test")
	if err := c.sell(t, 60_000_000); errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("first delivery err = %v, want the wallet read to fail", err)
	}
	planned := c.plan(t)
	if planned == "" || len(c.legs(t)) != 0 {
		t.Fatalf("first delivery stored plan %q and sold %+v; want a plan and no swap", planned, c.legs(t))
	}
	c.walletErr = nil
	c.price(tslaxToken(), 90_000_000)
	c.price(aaplxToken(), 5_000_000)
	for range 2 {
		if err := c.sell(t, 60_000_000); err != nil {
			t.Fatal(err)
		}
	}
	if c.plan(t) != planned || c.positions != 1 {
		t.Fatalf("plan after redelivery = %s, first %s, positions read %d times", c.plan(t), planned, c.positions)
	}
	assertLegs(t, c.legs(t), confirmedLeg("AAPLx", 1_000, 50_000_000), confirmedLeg("TSLAx", 106, 10_600_000))
}

func TestSellForCashOut_FailedLegNotRetried(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	c.fill(aaplxToken(), jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: -1})
	for range 2 {
		if err := c.sell(t, 60_000_000); err != nil {
			t.Fatal(err)
		}
	}
	failed := soldLeg{Symbol: "AAPLx", Units: 1_000, QuoteUSDC: 50_000_000, Status: "failed", SlippageBps: 100}
	assertLegs(t, c.legs(t), failed, confirmedLeg("TSLAx", 106, 10_600_000))
	if sent := c.jup.Sent("req-" + aaplxMint); len(sent) != 1 {
		t.Fatalf("the failed leg was sent %d times, want 1", len(sent))
	}
}

func TestSellForCashOut_TradeEventsGolden(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	if err := c.sell(t, 60_000_000); err != nil {
		t.Fatal(err)
	}
	got := c.tradeEvents(t)
	want, err := os.ReadFile("testdata/cashout_sell_events.golden")
	if err != nil || got != string(want) {
		t.Fatalf("trade events differ from testdata/cashout_sell_events.golden (%v):\n%s", err, got)
	}
}

func (c *cashOutEnv) tradeEvents(t *testing.T) string {
	t.Helper()
	rows, err := c.pool.Query(t.Context(), `SELECT e.type, e.payload FROM events e JOIN swaps s ON s.id = e.aggregate_id
		WHERE s.source_id = $1 ORDER BY e.id`, c.job)
	if err != nil {
		t.Fatal(err)
	}
	type event struct {
		Type    string
		Payload map[string]any
	}
	events, err := pgx.CollectRows(rows, pgx.RowToStructByPos[event])
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range events {
		source, _ := e.Payload["source"].(map[string]any)
		if source["id"] != c.job.String() {
			t.Fatalf("%s source = %v, want the cash out job %s", e.Type, source, c.job)
		}
		for _, k := range []string{"swap_id", "cabal_id", "tx_signature", "confirmed_at"} {
			delete(e.Payload, k)
		}
		delete(source, "id")
		line, _ := json.Marshal(map[string]any{"type": e.Type, "payload": e.Payload})
		b.Write(append(line, '\n'))
	}
	return b.String()
}

func TestSellForCashOut_Failures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func(c *cashOutEnv)
		sql     string
		kind    domain.SourceKind
		need    uint64
		want    errs.Code
	}{
		{name: "proposal source", kind: domain.SourceProposal, need: 1, want: errs.CodeInvalidInput},
		{name: "target overflows", need: math.MaxUint64, want: errs.CodeInvalidInput},
		{name: "positions fail", need: 1, want: errs.CodeUpstreamTimeout, arrange: func(c *cashOutEnv) {
			c.positionsErr = errs.New(errs.CodeUpstreamTimeout, "test")
		}},
		{name: "quote fails", need: 1, want: errs.CodeJupiterUnavailable, arrange: func(c *cashOutEnv) {
			c.jup.Fail("Quote", errs.New(errs.CodeJupiterUnavailable, "test"))
		}},
		{name: "order fails", need: 1, want: errs.CodeInternal, arrange: func(c *cashOutEnv) {
			c.jup.Fail("Order", errs.New(errs.CodeInternal, "test"))
		}},
		{name: "plan read fails", need: 1, want: errs.CodeInternal, sql: "DROP TABLE cashout_sell_plans"},
		{name: "swap read fails", need: 1, want: errs.CodeInternal, sql: "ALTER TABLE swaps RENAME in_mint TO mint"},
		{
			name: "plan insert fails", need: 1, want: errs.CodeInternal,
			sql: "ALTER TABLE cashout_sell_plans ADD CONSTRAINT refuse CHECK (false) NOT VALID",
		},
		{
			name: "stored plan undecodable", need: 1, want: errs.CodeDecodeFailed,
			sql: `INSERT INTO cashout_sell_plans SELECT $1, $2, '[{"units": -1}]', now()`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newCashOutEnv(t)
			if tc.arrange != nil {
				tc.arrange(c)
			}
			c.exec(t, tc.sql)
			err := c.handler().Handle(actorContext(t.Context()), app.SellForCashOut{
				CabalID: c.cabal, Source: domain.Source{Kind: cmp.Or(tc.kind, domain.SourceCashout), ID: c.job},
				USDCNeeded: money.MicrosFromUint64(tc.need),
			}, nil)
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func (c *cashOutEnv) exec(t *testing.T, statement string) {
	t.Helper()
	if statement == "" {
		return
	}
	args := []any{c.job, c.cabal.UUID()}[:strings.Count(statement, "$")]
	if _, err := c.pool.Exec(t.Context(), statement, args...); err != nil {
		t.Fatal(err)
	}
}

func TestSellForCashOut_NothingSellable_BlocksOnce(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	c.holdings = []app.Holding{
		{Mint: usdcToken(), Symbol: "USDC", Units: 900_000_000},
		{Mint: nvdaxToken(), Symbol: "NVDAx", Units: 7},
	}
	for range 2 {
		if err := c.sell(t, 20_000_000); err != nil {
			t.Fatal(err)
		}
	}
	var payloads []string
	rows, err := c.pool.Query(t.Context(), `SELECT payload::text FROM events WHERE type = 'trade.blocked'
		AND aggregate_id = $1`, c.job)
	if err != nil {
		t.Fatal(err)
	}
	if payloads, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 || len(c.legs(t)) != 0 || c.plan(t) != "[]" {
		t.Fatalf("blocked events %v, legs %+v, plan %q; want one block, no swap and an empty plan", payloads,
			c.legs(t), c.plan(t))
	}
	var got busevents.TradeBlocked
	if err := json.Unmarshal([]byte(payloads[0]), &got); err != nil {
		t.Fatal(err)
	}
	want := busevents.TradeBlocked{
		V: 1, CabalID: c.cabal.UUID(), Source: busevents.TradeSource{Kind: "cashout", ID: c.job}, Action: "sell",
		Code: errs.CodeAssetUntradable,
	}
	if got != want {
		t.Fatalf("blocked = %+v, want %+v", got, want)
	}
}
