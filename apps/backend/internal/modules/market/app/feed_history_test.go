package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func feedID(seed uint64) uuid.UUID { return testkit.NewIDs(seed).NewV7() }

func feedNow() time.Time { return time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC) }

type feedAsset struct {
	id   uuid.UUID
	mint string
}

func seedFeedAsset(t *testing.T, db sqlc.DBTX, seed uint64, symbol, mint string) feedAsset {
	t.Helper()
	id := domain.NewAssetID(testkit.NewIDs(seed))
	_, err := db.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, $2, $3, 8, 'xstocks', 'equity', $2, true, $2, $4, $4)`,
		id.UUID(), symbol, mint, feedNow())
	if err != nil {
		t.Fatal(err)
	}
	return feedAsset{id: id.UUID(), mint: mint}
}

func seedFeedPoint(t *testing.T, db sqlc.DBTX, a feedAsset, at time.Time, micros int64) {
	t.Helper()
	_, err := db.Exec(t.Context(),
		`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
		a.mint, at, micros)
	if err != nil {
		t.Fatal(err)
	}
}

type feedRig struct {
	h  *FeedHistory
	db sqlc.DBTX
}

func feedHistory(t *testing.T) feedRig {
	t.Helper()
	pool := testkit.DB(t)
	return feedRig{h: NewFeedHistory(pool, testkit.NewClock(feedNow())), db: pool}
}

func TestFeedHistoryLatest_returnsTheAcceptedPriceNotAHeldSpike(t *testing.T) {
	t.Parallel()
	rig := feedHistory(t)
	h, db := rig.h, rig.db
	spiked := seedFeedAsset(t, db, 1, "SPKx", "mintSPK")
	steady := seedFeedAsset(t, db, 2, "STDx", "mintSTD")
	unpriced := seedFeedAsset(t, db, 3, "NOPx", "mintNOP")
	at := feedNow()
	seedFeedPoint(t, db, spiked, at.Add(-3*time.Minute), 100_000_000)
	seedFeedPoint(t, db, spiked, at.Add(-2*time.Minute), 100_000_000)
	seedFeedPoint(t, db, spiked, at.Add(-time.Minute), 500_000_000)
	seedFeedPoint(t, db, steady, at.Add(-time.Minute), 724_400_000)

	got, err := h.Latest(t.Context(), []uuid.UUID{spiked.id, steady.id, unpriced.id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[spiked.id].Uint64() != 100_000_000 || got[steady.id].Uint64() != 724_400_000 {
		t.Fatalf("Latest = %v, want spiked held at 100 and steady at 724.40 with the unpriced asset absent", got)
	}
}

func TestFeedHistoryLatest_noAssetsReadsNothing(t *testing.T) {
	t.Parallel()
	h := feedHistory(t).h
	got, err := h.Latest(t.Context(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("Latest(nil) = %v, %v, want empty", got, err)
	}
}

func TestFeedHistoryLatest_reportsAReadFailure(t *testing.T) {
	t.Parallel()
	h := feedHistory(t).h
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.Latest(ctx, []uuid.UUID{feedID(101)}); err == nil {
		t.Fatal("Latest on a cancelled context returned no error")
	}
}

func TestFeedHistoryCloses_binsEachWindowSeparatelyAndKeepsTheLastClose(t *testing.T) {
	t.Parallel()
	rig := feedHistory(t)
	h, db := rig.h, rig.db
	a := seedFeedAsset(t, db, 1, "METx", "mintMET")
	start := feedNow()
	for i := range 10 {
		seedFeedPoint(t, db, a, start.Add(time.Duration(i)*time.Minute), int64(i+1)*1_000_000)
	}
	whole, tail := feedID(102), feedID(103)
	got, err := h.Closes(t.Context(), []port.Window{
		{Key: whole, AssetID: a.id, From: start, To: start.Add(9 * time.Minute)},
		{Key: tail, AssetID: a.id, From: start.Add(5 * time.Minute), To: start.Add(9 * time.Minute)},
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uuid.UUID][]uint64{
		whole: {3_000_000, 6_000_000, 10_000_000},
		tail:  {7_000_000, 8_000_000, 10_000_000},
	}
	for key, prices := range want {
		series := got[key]
		if len(series) != len(prices) {
			t.Fatalf("window %v has %d points, want %d: %v", key, len(series), len(prices), series)
		}
		for i, p := range series {
			if p.PriceMicros.Uint64() != prices[i] {
				t.Fatalf("window %v point %d = %v, want %d", key, i, p.PriceMicros, prices[i])
			}
			if i > 0 && !p.At.After(series[i-1].At) {
				t.Fatalf("window %v points are not in time order: %v", key, series)
			}
		}
	}
	if len(got) != 2 {
		t.Fatalf("Closes keyed %d windows, want 2", len(got))
	}
}

func TestFeedHistoryCloses_leavesOutWindowsWithNothingToDraw(t *testing.T) {
	t.Parallel()
	rig := feedHistory(t)
	h, db := rig.h, rig.db
	a := seedFeedAsset(t, db, 1, "METx", "mintMET")
	start := feedNow()
	seedFeedPoint(t, db, a, start, 1_000_000)
	empty, unknown, flat := feedID(104), feedID(105), feedID(106)
	got, err := h.Closes(t.Context(), []port.Window{
		{Key: empty, AssetID: a.id, From: start.Add(time.Hour), To: start.Add(2 * time.Hour)},
		{Key: unknown, AssetID: feedID(107), From: start, To: start.Add(time.Hour)},
		{Key: flat, AssetID: a.id, From: start, To: start},
	}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Closes = %v, want no series", got)
	}
}

func TestFeedHistoryCloses_noWindowsOrNoBucketsReadsNothing(t *testing.T) {
	t.Parallel()
	h := feedHistory(t).h
	window := port.Window{Key: feedID(108), AssetID: feedID(109), From: feedNow(), To: feedNow().Add(time.Hour)}
	for _, tc := range []struct {
		windows []port.Window
		buckets int
	}{{nil, 32}, {[]port.Window{window}, 0}} {
		got, err := h.Closes(t.Context(), tc.windows, tc.buckets)
		if err != nil || len(got) != 0 {
			t.Fatalf("Closes(%d windows, %d buckets) = %v, %v, want empty", len(tc.windows), tc.buckets, got, err)
		}
	}
}

func TestFeedHistoryCloses_reportsAReadFailure(t *testing.T) {
	t.Parallel()
	h := feedHistory(t).h
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	window := port.Window{Key: feedID(110), AssetID: feedID(111), From: feedNow(), To: feedNow().Add(time.Hour)}
	if _, err := h.Closes(ctx, []port.Window{window}, 4); err == nil {
		t.Fatal("Closes on a cancelled context returned no error")
	}
}

func TestFeedHistoryCloses_rejectsANegativeStoredPrice(t *testing.T) {
	t.Parallel()
	rig := feedHistory(t)
	h, db := rig.h, rig.db
	a := seedFeedAsset(t, db, 1, "METx", "mintMET")
	seedFeedPoint(t, db, a, feedNow(), -5)
	window := port.Window{
		Key: feedID(112), AssetID: a.id, From: feedNow().Add(-time.Hour), To: feedNow().Add(time.Hour),
	}
	if _, err := h.Closes(t.Context(), []port.Window{window}, 4); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Closes = %v, want decode_failed", err)
	}
}
