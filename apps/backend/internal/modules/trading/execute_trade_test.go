package trading_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestExecuteTrade_buyThatPassesEveryCheckSwapsAndRecordsTheDelivery(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	d := e.delivery(t, cmd)

	if err := e.handle(t, d, cmd); err != nil {
		t.Fatal(err)
	}
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 || !e.recorded(t, d) || len(e.blocked(t, cmd.ProposalID)) != 0 {
		t.Fatalf("%d swaps, recorded %v, %d blocked; want one swap, the delivery and no block",
			len(swaps), e.recorded(t, d), len(e.blocked(t, cmd.ProposalID)))
	}
	status, _, _, _ := e.row(t, swaps[0])
	var in, quote, bps int64
	var inMint string
	if err := e.pool.QueryRow(t.Context(),
		`SELECT in_amount, quote_out_amount, slippage_bps, in_mint FROM swaps WHERE id = $1`, swaps[0]).
		Scan(&in, &quote, &bps, &inMint); err != nil {
		t.Fatal(err)
	}
	if status != "confirmed" || in != buyMicros || quote != quotedOut || bps != 100 || inMint != usdcMint {
		t.Fatalf("swap %s: in %d %s, quote %d, %d bps; want confirmed 25 USDC in at the voted quote and 100 bps",
			status, in, inMint, quote, bps)
	}
}

func TestExecuteTrade_sellSpendsTheHeldTokensForUSDC(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.ledger.SetTokens(e.wallet.Address, aaplxToken(), 50_000_000)
	e.quote(aaplxToken(), usdcToken(), quotedOut, true)
	cmd := e.sell(50_000_000)

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	var in int64
	var inMint, outMint string
	if err := e.pool.QueryRow(t.Context(), `SELECT in_amount, in_mint, out_mint FROM swaps WHERE source_id = $1`,
		cmd.ProposalID.UUID()).Scan(&in, &inMint, &outMint); err != nil {
		t.Fatal(err)
	}
	if in != 50_000_000 || inMint != aaplxMint || outMint != usdcMint {
		t.Fatalf("sell swap %d %s -> %s, want 50_000_000 AAPLx base units into USDC", in, inMint, outMint)
	}
}

type refusalCase struct {
	name       string
	arrange    func(e *engineEnv, cmd *app.ExecuteTrade)
	code       errs.Code
	have, need string
}

func refusalCases() []refusalCase {
	return []refusalCase{
		{
			"symbol not in the catalog", func(_ *engineEnv, cmd *app.ExecuteTrade) { cmd.Symbol = "NOPEx" },
			errs.CodeAssetUntradable, "0", "0",
		},
		{"halted asset", func(_ *engineEnv, cmd *app.ExecuteTrade) {
			cmd.Symbol, cmd.Mint = "JPSTx", marketfake.JPSTx().Mint.Address()
		}, errs.CodeAssetUntradable, "0", "0"},
		{"mint differs from the catalog's", func(_ *engineEnv, cmd *app.ExecuteTrade) {
			cmd.Mint = marketfake.TSLAx().Mint.Address()
		}, errs.CodeAssetUntradable, "0", "0"},
		{"buy short of the amount plus the transfer fee", func(e *engineEnv, _ *app.ExecuteTrade) {
			e.ledger.SetTokens(e.wallet.Address, usdcToken(), 25_249_999)
			e.balances.setFee(aaplxMint, 100, math.MaxUint64)
		}, errs.CodeInsufficientFunds, "25249999", "25250000"},
		{"sell short of the token amount", func(e *engineEnv, cmd *app.ExecuteTrade) {
			*cmd = e.sell(50_000_000)
			e.ledger.SetTokens(e.wallet.Address, aaplxToken(), 49_999_999)
		}, errs.CodeCabalSharesShort, "49999999", "50000000"},
		{"no route at this size, routed at the probe", func(e *engineEnv, _ *app.ExecuteTrade) {
			e.quote(usdcToken(), aaplxToken(), quotedOut, true)
			e.jup.SetQuoteLimit(jupiterMint(usdcToken()), jupiterMint(aaplxToken()), 1_000_000)
		}, errs.CodeNoRoute, "0", "0"},
		{"paused: no route at the probe either", func(e *engineEnv, _ *app.ExecuteTrade) {
			e.quote(usdcToken(), aaplxToken(), 0, false)
		}, errs.CodeAssetPaused, "0", "0"},
		{"paused sell of less than one token", func(e *engineEnv, cmd *app.ExecuteTrade) {
			*cmd = e.sell(40_000_000)
			e.ledger.SetTokens(e.wallet.Address, aaplxToken(), 40_000_000)
			e.quote(aaplxToken(), usdcToken(), 0, false)
		}, errs.CodeAssetPaused, "0", "0"},
		{
			"banned cabal", func(e *engineEnv, _ *app.ExecuteTrade) { e.seedCabal(cabal.StatusBanned, 100) },
			errs.CodeCabalPaused, "0", "0",
		},
		{"funding pause", func(e *engineEnv, _ *app.ExecuteTrade) {
			e.pauses.Pause(e.cabal, funding.PauseReasonExternalDeposit)
		}, errs.CodeCabalPaused, "0", "0"},
	}
}

func TestExecuteTrade_aFailedProbeQuoteIsRetriedAndNeverBlocksAsPausedOrNoRoute(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	d := e.delivery(t, cmd)
	e.quote(usdcToken(), aaplxToken(), 0, false)
	e.jup.FailOnce("Quote", nil)
	e.jup.FailOnce("Quote", errs.New(errs.CodeJupiterUnavailable, "test"))

	err := e.handle(t, d, cmd)

	if errs.CodeOf(err) != errs.CodeJupiterUnavailable || len(e.blocked(t, cmd.ProposalID)) != 0 {
		t.Fatalf("err = %v with %d blocked, want jupiter_unavailable returned for redelivery and no block",
			err, len(e.blocked(t, cmd.ProposalID)))
	}
}

func TestExecuteTrade_eachRefusalBlocksOnceRecordsTheDeliveryAndNeverSwaps(t *testing.T) {
	t.Parallel()
	for _, tc := range refusalCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEngineEnv(t)
			cmd := e.buy()
			d := e.delivery(t, cmd)
			tc.arrange(e, &cmd)

			if err := e.handle(t, d, cmd); err != nil {
				t.Fatal(err)
			}
			if err := e.handle(t, d, cmd); err != nil {
				t.Fatal(err)
			}
			blocked := e.blocked(t, cmd.ProposalID)
			if len(blocked) != 1 || e.deliveryCode(t, d) != string(tc.code) || len(e.swapsOf(t, cmd.ProposalID)) != 0 {
				t.Fatalf("%d blocked, delivery recorded as %q, %d swaps; want one block, the delivery recorded as %s "+
					"and no swap", len(blocked), e.deliveryCode(t, d), len(e.swapsOf(t, cmd.ProposalID)), tc.code)
			}
			b := blocked[0]
			if b["code"] != string(tc.code) || b["have"] != tc.have || b["need"] != tc.need ||
				b["cabal_id"] != cmd.CabalID.String() || b["action"] != string(cmd.Action) {
				t.Fatalf("trade.blocked = %v, want %s with have %s and need %s", b, tc.code, tc.have, tc.need)
			}
		})
	}
}

func TestExecuteTrade_buyWithExactlyTheAmountPlusFeePasses(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.ledger.SetTokens(e.wallet.Address, usdcToken(), 25_001_000)
	e.balances.setFee(aaplxMint, 100, 1_000)
	e.quote(usdcToken(), aaplxToken(), 20_790_000, true)
	cmd := e.buy()

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil || len(e.blocked(t, cmd.ProposalID)) != 0 {
		t.Fatalf("err %v, %d blocked; want the capped fee and the floor quote both to pass", err,
			len(e.blocked(t, cmd.ProposalID)))
	}
}

func TestExecuteTrade_anUpstreamFailureReturnsItsCodeAndWritesNothing(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeJupiterUnavailable, "jupiter.Quote")
	rpc := errs.New(errs.CodeRPCUnavailable, "solana.TokenBalance")
	for name, arrange := range map[string]func(e *engineEnv){
		"jupiter quote": func(e *engineEnv) { e.jup.Fail("Quote", down) },
		"catalog":       func(e *engineEnv) { e.catalog.Fail("AssetBySymbol", rpc) },
		"wallet":        func(e *engineEnv) { e.cabals.Fail("TreasuryWallet", rpc) },
		"balance":       func(e *engineEnv) { e.balances.Fail("TokenBalance", rpc) },
		"mint config":   func(e *engineEnv) { e.balances.Fail("MintConfig", rpc) },
		"slippage":      func(e *engineEnv) { e.cabals.Fail("SlippageBps", rpc) },
		"cabal status":  func(e *engineEnv) { e.cabals.Fail("Status", rpc) },
		"pause":         func(e *engineEnv) { e.pauses.Fail("IsPaused", rpc) },
		"order": func(e *engineEnv) {
			e.jup.Fail("Order", down)
			advanceClock(t, e.clk)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEngineEnv(t)
			arrange(e)
			cmd := e.buy()
			d := e.delivery(t, cmd)

			err := e.handle(t, d, cmd)
			if code := errs.CodeOf(err); errs.VerdictFor(code) != errs.VerdictNak {
				t.Fatalf("err = %v, want a retryable code the bus naks", err)
			}
			if e.recorded(t, d) || len(e.blocked(t, cmd.ProposalID)) != 0 {
				t.Fatal("an upstream failure recorded the delivery or blocked the trade")
			}
		})
	}
}

func TestExecuteTrade_aBuyAmountThatOverflowsWithTheFeeFails(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.balances.setFee(aaplxMint, 100, math.MaxUint64)
	cmd := e.buy()
	cmd.USDCMicros = money.MicrosFromUint64(math.MaxUint64)

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil || e.blockedCodes(t, cmd) != "trade_not_started" {
		t.Fatalf("err = %v, blocked %q; want the overflow, which no retry fixes, blocked as trade_not_started",
			err, e.blockedCodes(t, cmd))
	}
}

func (e *engineEnv) blockedCodes(t *testing.T, cmd app.ExecuteTrade) string {
	t.Helper()
	blocked := e.blocked(t, cmd.ProposalID)
	codes := make([]string, 0, len(blocked))
	for _, b := range blocked {
		codes = append(codes, b["code"].(string))
	}
	return strings.Join(codes, ",")
}

func TestExecuteTrade_aQuoteJupiterRejectsBlocksTheTradeAsNotStarted(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.Fail("Quote", errs.New(errs.CodeJupiterRejected, "jupiter.Quote"))
	cmd := e.buy()
	d := e.delivery(t, cmd)

	if err := e.handle(t, d, cmd); err != nil {
		t.Fatal(err)
	}
	if got := e.blockedCodes(t, cmd); got != "trade_not_started" || e.deliveryCode(t, d) != "trade_not_started" ||
		len(e.swapsOf(t, cmd.ProposalID)) != 0 {
		t.Fatalf("blocked %q, delivery %q, %d swaps; want one trade_not_started block and no swap",
			got, e.deliveryCode(t, d), len(e.swapsOf(t, cmd.ProposalID)))
	}
}

func TestExecuteTrade_aRetryableCheckErrorBlocksOnlyOnTheFinalDelivery(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.Fail("Quote", errs.New(errs.CodeJupiterUnavailable, "jupiter.Quote"))
	cmd := e.buy()
	d := e.delivery(t, cmd)

	if err := e.handle(t, d, cmd); errs.CodeOf(err) != errs.CodeJupiterUnavailable ||
		e.recorded(t, d) || e.blockedCodes(t, cmd) != "" {
		t.Fatalf("err = %v, recorded %v, blocked %q; want the error returned and nothing written",
			err, e.recorded(t, d), e.blockedCodes(t, cmd))
	}
	d.Final = true
	if err := e.handle(t, d, cmd); err != nil || e.blockedCodes(t, cmd) != "trade_not_started" {
		t.Fatalf("err = %v, blocked %q; want the final delivery to block as trade_not_started",
			err, e.blockedCodes(t, cmd))
	}
}

func TestExecuteTrade_aRecordedDeliveryDoesNothing(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	d := e.delivery(t, cmd)
	e.seedCabal(cabal.StatusBanned, 100)
	if err := e.handle(t, d, cmd); err != nil {
		t.Fatal(err)
	}
	e.seedCabal(cabal.StatusActive, 100)
	if err := e.handle(t, d, cmd); err != nil || len(e.swapsOf(t, cmd.ProposalID)) != 0 {
		t.Fatalf("err %v, %d swaps; want the recorded delivery to stop a second run", err,
			len(e.swapsOf(t, cmd.ProposalID)))
	}
}

func TestExecuteTrade_aConfirmedSwapStopsARedeliveryUnderANewDeliveryID(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil || len(e.swapsOf(t, cmd.ProposalID)) != 1 {
		t.Fatalf("err %v, %d swaps; want no second swap", err, len(e.swapsOf(t, cmd.ProposalID)))
	}
}

func TestExecuteTrade_aSwapThatJupiterFailsRecordsTheDeliveryAsSwapFailed(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: 6001})
	cmd := e.buy()
	d := e.delivery(t, cmd)
	if err := e.handle(t, d, cmd); err != nil {
		t.Fatal(err)
	}
	if got := e.deliveryCode(t, d); got != string(errs.CodeSwapFailed) {
		t.Fatalf("delivery recorded as %q, want swap_failed", got)
	}
}

func TestExecuteTrade_aFailedSwapLeavesTheClaimOpen(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	e.jup.FailOnce("Order", errs.New(errs.CodeJupiterRejected, "jupiter.Order"))
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil || len(e.swapsOf(t, cmd.ProposalID)) != 2 {
		t.Fatalf(
			"err %v, %d swaps; want a second swap after the failed one",
			err,
			len(e.swapsOf(t, cmd.ProposalID)),
		)
	}
}

func TestExecuteTrade_aCancelledReadFailsAsInternal(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	ctx, cancel := context.WithCancel(actorContext(t.Context()))
	cancel()
	cmd := e.buy()
	err := e.handler().Handle(ctx, e.delivery(t, cmd), cmd, nil)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

type raceCabals struct {
	app.Cabals
	before func()
}

func (r raceCabals) Status(ctx context.Context, id ids.CabalID) (cabal.Status, error) {
	r.before()
	return r.Cabals.Status(ctx, id)
}

func TestExecuteTrade_aConcurrentWorkerThatGotThereFirstWins(t *testing.T) {
	t.Parallel()
	t.Run("its recorded delivery stops a second trade.blocked", func(t *testing.T) {
		t.Parallel()
		e := newEngineEnv(t)
		cmd := e.buy()
		d := e.delivery(t, cmd)
		e.seedCabal(cabal.StatusBanned, 100)
		ports := e.ports()
		ports.Cabals = raceCabals{Cabals: e.cabals, before: func() {
			if _, err := e.pool.Exec(t.Context(), `INSERT INTO event_deliveries (handler, event_id, code, handled_at)
				VALUES ($1, $2, 'ok', now())`, d.Handler, d.EventID.UUID()); err != nil {
				t.Error(err)
			}
		}}
		err := e.handlerWith(ports).Handle(actorContext(t.Context()), d, cmd, nil)
		if err != nil || len(e.blocked(t, cmd.ProposalID)) != 0 {
			t.Fatalf(
				"err %v, %d blocked; want the other worker's delivery to win",
				err,
				len(e.blocked(t, cmd.ProposalID)),
			)
		}
	})
	t.Run("its live swap is left to it", func(t *testing.T) {
		t.Parallel()
		e := newEngineEnv(t)
		cmd := e.buy()
		d := e.delivery(t, cmd)
		ports := e.ports()
		ports.Cabals = raceCabals{Cabals: e.cabals, before: func() {
			req := e.request(cmd.ProposalID.UUID())
			req.CabalID = e.cabal
			e.jup.FailOnce("Order", errs.New(errs.CodeInternal, "jupiter.Order"))
			if _, err := e.layer().Run(actorContext(t.Context()), req, nil); err == nil {
				t.Error("the other worker's swap should stop at created")
			}
		}}
		err := e.handlerWith(ports).Handle(actorContext(t.Context()), d, cmd, nil)
		if err != nil || e.recorded(t, d) || len(e.swapsOf(t, cmd.ProposalID)) != 1 {
			t.Fatalf("err %v, recorded %v, %d swaps; want the created swap left alone and no delivery row",
				err, e.recorded(t, d), len(e.swapsOf(t, cmd.ProposalID)))
		}
	})
}

func (e *engineEnv) handlerWith(ports app.EnginePorts) *app.ExecuteTradeHandler {
	return app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), UoW: e.uow, Reads: e.reads, Venue: e.venue, Ports: ports, USDC: usdcToken(),
	})
}

func TestExecuteTrade_aToleranceOverThePlatformCapOrdersAt300Bps(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.seedCabal(cabal.StatusActive, 900)
	cmd := e.buy()
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	if got := e.jup.Slippages(); len(got) == 0 || got[0] != 300 {
		t.Fatalf("order slippages = %v, want the first order at the 300 bps cap", got)
	}
}
