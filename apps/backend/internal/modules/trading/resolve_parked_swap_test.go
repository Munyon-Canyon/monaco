package trading_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const parkedActor = "operator:alice"

func (e *routeEnv) resolver() *app.ResolveParkedSwapHandler {
	return app.NewResolveParkedSwapHandler(e.uow, e.pool, e.clk, e.ids, e.hints, e.receipts())
}

func (e *routeEnv) resolveParked(t *testing.T, swap uuid.UUID, action app.ParkedAction) error {
	t.Helper()
	_, err := e.resolver().Handle(t.Context(), app.ResolveParkedSwap{
		SwapID: ids.SwapIDFrom(swap), Action: action, Actor: parkedActor, Reason: "rpc blip",
	})
	return err
}

func (e *routeEnv) park(t *testing.T, swap uuid.UUID) {
	t.Helper()
	listed := e.listings
	e.listings = listingsFake{}
	if _, err := e.tick(t.Context(), t); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("Tick err = %v, want the swap parked on the missing listing", err)
	}
	e.listings = listed
	if _, code := e.parked(t, swap); code != string(errs.CodeAssetNotFound) {
		t.Fatalf("swap parked %q, want asset_not_found", code)
	}
}

func (e *routeEnv) parkedBridged(t *testing.T) uuid.UUID {
	t.Helper()
	swap := e.bridgedThenStuck(t)
	e.park(t, swap)
	return swap
}

func (e *routeEnv) operatorActions(t *testing.T, swap uuid.UUID) []string {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT action || ' ' || actor || ' ' || reason || ' ' ||
		(detail->>'parked_code') || ' ' || (detail->>'status') FROM swap_operator_actions WHERE swap_id = $1
		ORDER BY created_at, id`, swap)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *routeEnv) mustExec(t *testing.T, stmt string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), stmt, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *routeEnv) swapState(t *testing.T, swap uuid.UUID) string {
	t.Helper()
	var state string
	if err := e.pool.QueryRow(t.Context(), `SELECT s.status || ' ' || coalesce(s.parked_code, '-') || ' ' ||
		coalesce(string_agg(t.step || ':' || t.status, ',' ORDER BY t.step), '') || ' ' ||
		(SELECT count(*) FROM events WHERE aggregate_id = s.id)
		FROM swaps AS s LEFT JOIN trade_steps AS t ON t.trade_id = s.id WHERE s.id = $1 GROUP BY s.id`, swap).
		Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestResolveParkedSwap_listsAParkedSwapWithItsStepsAndOmitsAnUnparkedOne(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedBridged(t)
	e.mustBuy(t, e.request(), domain.StatusCreated)

	parked, err := e.resolver().ListParked(t.Context())

	if err != nil || len(parked) != 1 {
		t.Fatalf("ListParked = %+v, %v, want the one parked swap", parked, err)
	}
	p := parked[0]
	if p.ID.UUID() != swap || p.CabalID != e.cabal || p.Status != domain.StatusCreated ||
		p.Code != string(errs.CodeAssetNotFound) || p.ParkedAt.IsZero() || p.BridgeOut != "succeeded" ||
		p.Swap != "" || p.BridgeBack != "" {
		t.Fatalf("parked swap = %+v", p)
	}
}

func TestResolveParkedSwap_unparkReDrivesTheSwapUnderTheSameStepKeys(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	req := e.request()
	ctx, cancel := context.WithCancel(t.Context())
	e.privy.afterTransfer = cancel
	if view, err := e.buy(ctx, req); err == nil {
		t.Fatalf("buy = %+v, want it cut off after the bridge transfer", view)
	}
	swap := e.swapIDs(t, req.Source.ID)[0]
	e.clk.Advance(app.SwapSweepAge + time.Second)
	e.park(t, swap)

	if err := e.resolveParked(t, swap, app.ParkedUnpark); err != nil {
		t.Fatal(err)
	}
	if status, code := e.parked(t, swap); status != string(domain.StatusCreated) || code != "" {
		t.Fatalf("swap %s parked %q, want created and unparked", status, code)
	}
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.clk.Advance(app.SwapSweepAge + time.Second)
	if report, err := e.tick(t.Context(), t); err != nil || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v, want the unparked swap resumed", report, err)
	}

	key := "trade:" + swap.String() + ":bridge_out"
	calls := e.privy.calls()
	if len(calls) != 2 || calls[0].key != key || calls[1].key != key || e.privy.actions() != 1 {
		t.Fatalf("transfers = %+v under %d keys, want the bridge out sent again under %s", calls,
			e.privy.actions(), key)
	}
	if status, _ := e.parked(t, swap); status != string(domain.StatusConfirmed) {
		t.Fatalf("swap %s after the resume, want confirmed", status)
	}
}

func TestResolveParkedSwap_unparkRefusesOnceTheStrandedCashHasLeft(t *testing.T) {
	t.Parallel()
	for name, mark := range map[string]string{
		"bridge back": `INSERT INTO trade_steps (id, trade_id, step, network, amount_micros, status, created_at,
			updated_at) VALUES (gen_random_uuid(), $1, 'bridge_back', 'base', 1, 'pending', now(), now())`,
		"reused": `WITH other AS (
				INSERT INTO swaps SELECT (jsonb_populate_record(NULL::swaps, to_jsonb(s) || jsonb_build_object(
					'id', gen_random_uuid(), 'source_id', gen_random_uuid(), 'parked_at', NULL, 'parked_code', NULL))).*
				FROM swaps AS s WHERE s.id = $1 RETURNING id)
			INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id, amount_micros, status, created_at,
				updated_at)
			SELECT gen_random_uuid(), other.id, 'bridge_out', 'base', 'reused:' || $1::text, 1, 'succeeded', now(), now()
			FROM other`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.parkedBridged(t)
			e.mustExec(t, mark, swap)

			if err := e.resolveParked(t, swap, app.ParkedUnpark); errs.CodeOf(err) != errs.CodeSwapNotStuck {
				t.Fatalf("unpark err = %v, want swap_not_stuck", err)
			}
			if _, code := e.parked(t, swap); code != string(errs.CodeAssetNotFound) {
				t.Fatalf("swap parked %q, want it still parked", code)
			}
			if rows := e.operatorActions(t, swap); len(rows) != 0 {
				t.Fatalf("operator actions = %q, want none", rows)
			}
		})
	}
}

func TestResolveParkedSwap_failParkedRefusesOnceAStepWasBroadcast(t *testing.T) {
	t.Parallel()
	swapStep := `INSERT INTO trade_steps (id, trade_id, step, network, amount_micros, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 'swap', 'base', 1, '%s', now(), now())`
	for name, stmt := range map[string]string{
		"submitted swap": `UPDATE swaps SET status = 'submitted', tx_signature = '0x' || md5(id::text),
			submitted_at = now() WHERE id = $1`,
		"swap step pending":    fmt.Sprintf(swapStep, "pending"),
		"swap step submitted":  fmt.Sprintf(swapStep, "submitted"),
		"swap step succeeded":  fmt.Sprintf(swapStep, "succeeded"),
		"bridge out pending":   `UPDATE trade_steps SET status = 'pending' WHERE trade_id = $1 AND step = 'bridge_out'`,
		"bridge out submitted": `UPDATE trade_steps SET status = 'submitted' WHERE trade_id = $1 AND step = 'bridge_out'`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.parkedBridged(t)
			e.mustExec(t, stmt, swap)
			before := e.swapState(t, swap)

			if err := e.resolveParked(t, swap, app.ParkedFail); errs.CodeOf(err) != errs.CodeSwapNotStuck {
				t.Fatalf("fail-parked err = %v, want swap_not_stuck", err)
			}
			if after := e.swapState(t, swap); after != before {
				t.Fatalf("swap went from %q to %q, want it unchanged", before, after)
			}
			if rows := e.operatorActions(t, swap); len(rows) != 0 {
				t.Fatalf("operator actions = %q, want none", rows)
			}
		})
	}
}

func TestResolveParkedSwap_recordsOneOperatorActionPerChange(t *testing.T) {
	t.Parallel()
	for _, action := range []app.ParkedAction{app.ParkedUnpark, app.ParkedFail} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.parkedBridged(t)

			first := e.resolveParked(t, swap, action)
			second := e.resolveParked(t, swap, action)

			if first != nil || errs.CodeOf(second) != errs.CodeSwapNotStuck {
				t.Fatalf("runs = %v, %v, want one change then swap_not_stuck", first, second)
			}
			want := string(action) + " " + parkedActor + " rpc blip asset_not_found created"
			if rows := e.operatorActions(t, swap); len(rows) != 1 || rows[0] != want {
				t.Fatalf("operator actions = %q, want one %q", rows, want)
			}
			if hints := e.hints.published(); len(hints) == 0 ||
				hints[len(hints)-1] != "cabal."+e.cabal.UUID().String()+".swap_updated" {
				t.Fatalf("hints = %q, want the cabal's swap_updated last", hints)
			}
		})
	}
}

func TestResolveParkedSwap_refusesBadInputAndAnUnknownSwap(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := ids.SwapIDFrom(e.ids.NewV7())
	for _, c := range []struct {
		cmd  app.ResolveParkedSwap
		want errs.Code
	}{
		{app.ResolveParkedSwap{SwapID: swap, Action: app.ParkedUnpark, Reason: "rpc blip"}, errs.CodeInvalidInput},
		{app.ResolveParkedSwap{SwapID: swap, Action: app.ParkedUnpark, Actor: parkedActor}, errs.CodeInvalidInput},
		{app.ResolveParkedSwap{
			SwapID: swap, Action: app.ParkedUnpark, Actor: parkedActor, Reason: strings.Repeat("é", 501),
		}, errs.CodeInvalidInput},
		{
			app.ResolveParkedSwap{SwapID: swap, Action: "retry", Actor: parkedActor, Reason: "rpc blip"},
			errs.CodeInvalidInput,
		},
		{
			app.ResolveParkedSwap{SwapID: swap, Action: app.ParkedFail, Actor: parkedActor, Reason: "rpc blip"},
			errs.CodeSwapNotFound,
		},
	} {
		if _, err := e.resolver().Handle(t.Context(), c.cmd); errs.CodeOf(err) != c.want {
			t.Errorf("Handle(%+v) err = %v, want %s", c.cmd, err, c.want)
		}
	}
}

func TestResolveParkedSwap_failParkedReleasesABridgedSwapWithoutMovingMoney(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedBridged(t)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	after, err := e.resolver().Handle(ctx, app.ResolveParkedSwap{
		SwapID: ids.SwapIDFrom(swap), Action: app.ParkedFail, Actor: parkedActor, Reason: "listing gone",
	})

	if err != nil || after.Status != domain.StatusFailed {
		t.Fatalf("Handle = %+v, %v, want the swap failed", after, err)
	}
	if code := e.failure(t, swap); code != string(domain.FailureNeverSubmitted) {
		t.Fatalf("failure code %q, want never_submitted", code)
	}
	if n := e.count(t, `SELECT count(*) FROM events WHERE type = 'trade.failed' AND aggregate_id = $1`, swap); n != 1 {
		t.Fatalf("%d trade.failed, want 1", n)
	}
	if n := len(e.privy.calls()); n != 1 {
		t.Fatalf("%d transfers, want only the original bridge out", n)
	}
	want := e.cabal.UUID().String() + " fail_parked " + parkedActor + " listing gone asset_not_found"
	if lines := logs.Lines(observability.TradingRoutedSwapResolved.Name, "cabal_id", "action", "actor", "reason",
		"parked_code"); len(lines) != 1 || lines[0] != want {
		t.Fatalf("resolved logs = %q, want one %q", lines, want)
	}

	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionSucceeded)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	if report, err := e.tick(t.Context(), t); err != nil || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v, want the cash bridged back", report, err)
	}
	if calls := e.privy.calls(); len(calls) != 2 || calls[1].key != "trade:"+swap.String()+":bridge_back" {
		t.Fatalf("transfers = %+v, want the bridge out then the bridge back", calls)
	}
}

func TestResolveParkedSwap_readsOneSwapAndReportsAFailedRead(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedBridged(t)

	got, err := e.resolver().Parked(t.Context(), ids.SwapIDFrom(swap))
	if err != nil || got.ID.UUID() != swap || got.Code != string(errs.CodeAssetNotFound) {
		t.Fatalf("Parked = %+v, %v, want the parked swap", got, err)
	}
	if _, err := e.resolver().
		Parked(t.Context(), ids.SwapIDFrom(e.ids.NewV7())); errs.CodeOf(
		err,
	) != errs.CodeSwapNotFound {
		t.Fatalf("Parked(unknown) err = %v, want swap_not_found", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.resolver().Parked(ctx, ids.SwapIDFrom(swap)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Parked(cancelled) err = %v, want internal", err)
	}
	if _, err := e.resolver().ListParked(ctx); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("ListParked(cancelled) err = %v, want internal", err)
	}
}
