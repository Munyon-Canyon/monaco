package market_test

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func (r *catalogRig) multiplierChanges(t *testing.T) []string {
	t.Helper()
	var out []string
	for line := range strings.Lines(string(r.logs.Bytes())) {
		var got map[string]any
		if json.Unmarshal([]byte(line), &got) != nil || got["msg"] != "market.catalog.multiplier_changed" {
			continue
		}
		out = append(out, fmt.Sprint(got["symbol"], " ", got["mint"], ": ", got["multiplier_before"], " -> ",
			got["multiplier_after"]))
	}
	return out
}

func (r *catalogRig) checkedBefore(t *testing.T, at time.Time) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(), `SELECT count(*) FROM assets WHERE chain_checked_at < $1`, at).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCatalogPoller_aScheduledMultiplierTakesOverAtItsSecondWithoutARecheck(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl := marketfake.AAPLx()
	xs.serve(nil, listed(aapl))
	rig := newRig(t, xs)
	step := rig.clock.Now().Add(30 * time.Minute).Truncate(time.Second)
	rig.facts.Put(aapl.Mint, 8, 3, 2)
	rig.facts.Schedule(aapl.Mint, 2, 1, step)
	rig.tick(t)
	if got := rig.asset(t, "AAPLx").UIMultiplierAt(rig.clock.Now()); got != (domain.Multiplier{Num: 3, Den: 2}) {
		t.Fatalf("before the step: UIMultiplierAt = %+v, want the current 3/2", got)
	}
	asked := rig.facts.Asked()
	rig.clock.Advance(time.Hour)
	rig.tick(t)
	a := rig.asset(t, "AAPLx")
	if rig.facts.Asked() != asked {
		t.Fatalf("asked %d mints after the second tick, want %d: an hour-old check is not due",
			rig.facts.Asked(), asked)
	}
	if got := a.UIMultiplierAt(step.Add(-time.Second)); got != (domain.Multiplier{Num: 3, Den: 2}) {
		t.Fatalf("a second before the step: UIMultiplierAt = %+v, want 3/2", got)
	}
	if got := a.UIMultiplierAt(step); got != (domain.Multiplier{Num: 2, Den: 1}) {
		t.Fatalf("at the step: UIMultiplierAt = %+v, want the scheduled 2/1 from the stored schedule", got)
	}
}

func TestCatalogPoller_rechecksAMintCheckedADayAgoAndLogsTheNewSchedule(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	jpst, tsla := marketfake.JPSTx(), marketfake.TSLAx()
	xs.serve(nil, listed(jpst), listed(tsla))
	rig := newRig(t, xs)
	rig.tick(t)
	if got := rig.facts.Asked(); got != 2 {
		t.Fatalf("first tick asked %d mints, want 2", got)
	}
	step := rig.clock.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	rig.facts.Schedule(jpst.Mint, 101, 100, step)
	rig.clock.Advance(25 * time.Hour)
	if got := rig.tick(t); got.Changed != 2 || rig.facts.Asked() != 4 {
		t.Fatalf("tick 25 hours on = %+v after %d asks, want both mints asked again and stored", got,
			rig.facts.Asked())
	}
	a := rig.asset(t, "JPSTx")
	want := domain.MultiplierStep{To: domain.Multiplier{Num: 101, Den: 100}, At: step}
	if !a.NextUIMultiplier.Equal(want) || a.UIMultiplierAt(step) != want.To {
		t.Fatalf("JPSTx schedule = %+v, want %+v", a.NextUIMultiplier, want)
	}
	logged := fmt.Sprintf("JPSTx %s: 1/1 -> 1/1 then 101/100 at %s", jpst.Mint, step.UTC().Format(time.RFC3339))
	if got := rig.multiplierChanges(t); !slices.Equal(got, []string{logged}) {
		t.Fatalf("multiplier changes = %v, want only JPSTx's %q", got, logged)
	}
	rig.facts.Put(jpst.Mint, jpst.Decimals, 101, 100)
	rig.clock.Advance(25 * time.Hour)
	rig.tick(t)
	stepped := fmt.Sprintf("JPSTx %s: 1/1 then 101/100 at %s -> 101/100", jpst.Mint, step.UTC().Format(time.RFC3339))
	if got := rig.multiplierChanges(t); len(got) != 2 || got[1] != stepped {
		t.Fatalf("multiplier changes = %v, want JPSTx's schedule dropped as %q", got, stepped)
	}
}

func TestCatalogPoller_fillsTheRestOfTheTickWithTheStalestCheckedMints(t *testing.T) {
	t.Parallel()
	rig := newRig(t)
	assets := make([]seeded, 200)
	for i := range assets {
		assets[i] = seeded{symbol: fmt.Sprintf("S%03dx", i), tradable: true}
	}
	rig.seed(t, assets[:150]...)
	rig.tick(t)
	rig.clock.Advance(time.Hour)
	rig.seed(t, assets[150:]...)
	rig.tick(t)
	second := rig.clock.Now()
	rig.clock.Advance(25 * time.Hour)
	rig.seed(t, seeded{symbol: "ZZZx"})
	asked := rig.facts.Asked()
	if got := rig.tick(t); got.Changed != 200 || rig.facts.Asked()-asked != 200 {
		t.Fatalf("tick = %+v after %d asks, want the unchecked ZZZx and 199 rechecks", got, rig.facts.Asked()-asked)
	}
	if got := rig.unchecked(t); len(got) != 0 {
		t.Fatalf("unchecked = %v, want ZZZx checked ahead of every recheck", got)
	}
	if got := rig.checkedBefore(t, second.Add(-time.Minute)); got != 0 {
		t.Fatalf("%d of the oldest checks left, want the 150 checked first rechecked first", got)
	}
	if got := rig.checkedBefore(t, second.Add(time.Minute)); got != 1 {
		t.Fatalf("%d stale checks left, want the one that did not fit in 200", got)
	}
}

func TestCatalogPoller_leavesARecheckWhoseRowWasCheckedMeanwhile(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl := marketfake.AAPLx()
	xs.serve(nil, listed(aapl))
	rig := newRig(t, xs)
	rig.tick(t)
	rig.clock.Advance(25 * time.Hour)
	rig.facts.Put(aapl.Mint, 8, 2, 1)
	racing := rig.asking(&interleaving{MintFacts: rig.facts, before: map[domain.Mint]func(){
		aapl.Mint: func() {
			if _, err := rig.pool.Exec(
				t.Context(),
				`UPDATE assets SET ui_multiplier_num = 3, chain_checked_at = now() WHERE symbol = 'AAPLx'`,
			); err != nil {
				t.Error(err)
			}
		},
	}})
	if got, err := racing.Tick(rig.ctx(t)); err != nil || got.Changed != 0 {
		t.Fatalf("Tick = %+v, %v, want the moved row left alone", got, err)
	}
	if a := rig.asset(t, "AAPLx"); a.UIMultiplier != (domain.Multiplier{Num: 3, Den: 1}) {
		t.Fatalf("AAPLx multiplier = %+v, want the check that landed first kept", a.UIMultiplier)
	}
	if got := rig.multiplierChanges(t); len(got) != 0 {
		t.Fatalf("multiplier changes = %v, want nothing logged for a recheck that did not store", got)
	}
}

func TestCatalogPoller_aScheduledMultiplierTooLargeToStoreLeavesThatMintUnchecked(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	tsla := marketfake.TSLAx()
	xs.serve(nil, listed(marketfake.AAPLx()), listed(tsla))
	rig := newRig(t, xs)
	rig.facts.Schedule(tsla.Mint, math.MaxUint64, 1, rig.clock.Now())
	report, err := rig.poller.Tick(rig.ctx(t))
	if errs.CodeOf(err) != errs.CodeDecodeFailed || report.Changed != 3 {
		t.Fatalf("Tick = %+v, %v, want decode_failed with AAPLx checked", report, err)
	}
	if got := rig.unchecked(t); !slices.Equal(got, []string{"TSLAx"}) {
		t.Fatalf("unchecked = %v, want only TSLAx", got)
	}
}

func TestCatalogPoller_aFailedReadOfTheDueMintsFailsTheTickWithoutAskingTheChain(t *testing.T) {
	t.Parallel()
	rig := newRig(t)
	rig.seed(t, seeded{symbol: "AAPLx", tradable: true})
	rig.exec(t, `ALTER TABLE assets RENAME COLUMN ui_multiplier_next_at TO gone`)
	if _, err := rig.poller.Tick(rig.ctx(t)); err == nil || rig.facts.Asked() != 0 {
		t.Fatalf("Tick = %v after %d asks, want the read error before asking the chain", err, rig.facts.Asked())
	}
}
