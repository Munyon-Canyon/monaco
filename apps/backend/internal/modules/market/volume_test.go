package market_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type volumeRig struct {
	*historyRig
	volumes *app.Volumes
}

func newVolumeRig(t *testing.T, assets ...market.Asset) *volumeRig {
	t.Helper()
	b := newHistoryRig(t)
	seedAssets(t, b.pool, b.clock.Now(), assets...)
	return &volumeRig{
		historyRig: b,
		volumes:    app.NewVolumes(db.New(b.pool, testkit.NewIDs(6), b.clock), b.pool, b.clock, b.history),
	}
}

func (r *volumeRig) tick(t *testing.T) (poller.Report, error) {
	t.Helper()
	return r.volumes.Tick(r.ctx(t))
}

func TestVolumes_oneCallStoresEveryListedMintsVolumeAsOfNow(t *testing.T) {
	t.Parallel()
	aapl, tsla, jpst := marketfake.AAPLx(), marketfake.TSLAx(), marketfake.JPSTx()
	jpst.IssuerTradable = false
	r := newVolumeRig(t, aapl, tsla, jpst)
	r.history.PutSpotVolume(aapl.Mint, usd(2_500_000_000_000))
	r.history.PutSpotVolume(tsla.Mint, usd(7_000_000))
	r.history.PutSpotVolume(jpst.Mint, usd(9_000_000))

	report, err := r.tick(t)
	if err != nil || report.Scanned != 2 || report.Changed != 2 {
		t.Fatalf("tick = %+v, %v, want 2 listed mints scanned and stored", report, err)
	}
	if batches := r.history.VolumeBatches(); len(batches) != 1 || len(batches[0]) != 2 {
		t.Fatalf("batches = %v, want one call for the two listed mints", batches)
	}
	got := storedVolumes(t, r.historyRig, aapl.Mint)
	if len(got) != 1 || got[0].micros != 2_500_000_000_000 || !got[0].asOf.Equal(r.clock.Now()) {
		t.Fatalf("aapl volumes = %+v, want 2.5e12 micros as of now", got)
	}
	if got := storedVolumes(t, r.historyRig, tsla.Mint); len(got) != 1 || got[0].micros != 7_000_000 {
		t.Fatalf("tsla volumes = %+v, want 7e6 micros", got)
	}
	if got := storedVolumes(t, r.historyRig, jpst.Mint); len(got) != 0 {
		t.Fatalf("jpst volumes = %+v, want none for an unlisted mint", got)
	}
}

func TestVolumes_aCatalogOfSixHundredTakesThreeCalls(t *testing.T) {
	t.Parallel()
	assets := generatedCatalog(t, 600)
	r := newVolumeRig(t, assets...)
	for _, a := range assets {
		r.history.PutSpotVolume(a.Mint, usd(1_000_000))
	}
	report, err := r.tick(t)
	if err != nil || report.Scanned != 600 || report.Changed != 600 {
		t.Fatalf("tick = %+v, %v, want 600 mints stored", report, err)
	}
	batches := r.history.VolumeBatches()
	if len(batches) != 3 || len(batches[0]) != app.VolumeBatch || len(batches[1]) != app.VolumeBatch ||
		len(batches[2]) != 100 {
		t.Fatalf("batch sizes = %d calls, want 250, 250 and 100", len(batches))
	}
}

func TestVolumes_aMintTheAnswerLeavesOutGetsNoRowAndNoError(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	r := newVolumeRig(t, aapl, tsla)
	r.history.PutSpotVolume(aapl.Mint, usd(5_000_000))
	report, err := r.tick(t)
	if err != nil || report.Scanned != 2 || report.Changed != 1 {
		t.Fatalf("tick = %+v, %v, want 2 scanned and 1 stored, no error for the missing mint", report, err)
	}
	if got := storedVolumes(t, r.historyRig, aapl.Mint); len(got) != 1 || got[0].micros != 5_000_000 {
		t.Fatalf("aapl volumes = %+v, want the reading", got)
	}
	if got := storedVolumes(t, r.historyRig, tsla.Mint); len(got) != 0 {
		t.Fatalf("tsla volumes = %+v, want none", got)
	}
}

func TestVolumes_aLaterTickReplacesTheReadingAndAnOlderOneNeverDoes(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	r := newVolumeRig(t, aapl)
	newer := r.clock.Now().Add(30 * time.Minute)
	r.exec(t, `INSERT INTO asset_volumes (mint, volume_24h_micros, as_of, updated_at) VALUES ($1, 900, $2, $2)`,
		aapl.Mint.String(), newer)
	r.history.PutSpotVolume(aapl.Mint, usd(100))
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	if got := storedVolumes(t, r.historyRig, aapl.Mint); len(got) != 1 || got[0].micros != 900 {
		t.Fatalf("volumes = %+v, want the newer reading kept", got)
	}
	r.clock.Advance(time.Hour)
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	if got := storedVolumes(t, r.historyRig, aapl.Mint); len(got) != 1 || got[0].micros != 100 {
		t.Fatalf("volumes = %+v, want the later tick to replace it", got)
	}
}

func TestVolumes_aFailedCallDoesNotStopTheNextOne(t *testing.T) {
	t.Parallel()
	assets := generatedCatalog(t, app.VolumeBatch+10)
	r := newVolumeRig(t, assets...)
	for _, a := range assets {
		r.history.PutSpotVolume(a.Mint, usd(1_000_000))
	}
	r.history.FailOnce("Volumes", errs.New(errs.CodeUpstreamUnavailable, "test"))
	report, err := r.tick(t)
	if err == nil || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("tick err = %v, want the first call's failure reported", err)
	}
	if len(r.history.VolumeBatches()) != 2 || report.Changed != 10 {
		t.Fatalf("report = %+v after %d calls, want the second call's 10 mints stored",
			report, len(r.history.VolumeBatches()))
	}
}

func TestVolumes_aRateLimitStopsTheTickAndTheCooldownHoldsTheNextOne(t *testing.T) {
	t.Parallel()
	assets := generatedCatalog(t, app.VolumeBatch+10)
	r := newVolumeRig(t, assets...)
	cool := app.NewCooldown(r.clock)
	r.volumes.WithCooldown(cool)
	r.history.FailOnce("Volumes", errs.New(errs.CodeCoinGeckoRateLimited, "test"))

	if _, err := r.tick(t); errs.CodeOf(err) != errs.CodeCoinGeckoRateLimited {
		t.Fatalf("tick err = %v, want the rate limit reported", err)
	}
	if !cool.Active() || len(r.history.VolumeBatches()) != 1 {
		t.Fatalf("cooldown %v after %d calls, want it on and no second call",
			cool.Active(), len(r.history.VolumeBatches()))
	}
	if _, err := r.tick(t); err != nil || len(r.history.VolumeBatches()) != 1 {
		t.Fatalf("a tick inside the cooldown made %d calls (err %v), want none", len(r.history.VolumeBatches())-1, err)
	}
	r.clock.Advance(61 * time.Second)
	if _, err := r.tick(t); err != nil || len(r.history.VolumeBatches()) != 3 {
		t.Fatalf("after the cooldown %d calls (err %v), want the two batches again",
			len(r.history.VolumeBatches())-1, err)
	}
}

func TestVolumes_aTickWithoutAKeyCallsNothing(t *testing.T) {
	t.Parallel()
	r := newVolumeRig(t, marketfake.AAPLx())
	r.history.WithoutKey()
	if report, err := r.tick(t); err != nil || report.Scanned != 0 || len(r.history.VolumeBatches()) != 0 {
		t.Fatalf("tick = %+v, %v with %d calls, want nothing", report, err, len(r.history.VolumeBatches()))
	}
}

func TestVolumes_aCatalogThatCannotBeReadFailsTheTick(t *testing.T) {
	t.Parallel()
	r := newVolumeRig(t, marketfake.AAPLx())
	ctx, cancel := context.WithCancel(r.ctx(t))
	cancel()
	if _, err := r.volumes.Tick(ctx); err == nil || len(r.history.VolumeBatches()) != 0 {
		t.Fatalf("tick err = %v after %d calls, want the read failure and no call", err, len(r.history.VolumeBatches()))
	}
}

func TestVolumes_aVolumeThatFailsToStoreIsReportedAndStoresNothing(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	r := newVolumeRig(t, aapl)
	r.exec(t, `ALTER TABLE asset_volumes ADD CONSTRAINT never CHECK (volume_24h_micros < 0)`)
	r.history.PutSpotVolume(aapl.Mint, usd(1_000_000))
	report, err := r.tick(t)
	if err == nil || report.Changed != 0 {
		t.Fatalf("tick = %+v, %v, want the write failure reported and no rows", report, err)
	}
}
