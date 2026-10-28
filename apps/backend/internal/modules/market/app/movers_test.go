package app

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fixedAssets struct {
	assets []domain.Asset
	err    error
}

func (f fixedAssets) ListAll(context.Context) ([]domain.Asset, error) { return f.assets, f.err }

func moverRig(t *testing.T, r reader, all fixedAssets, hot ...HotMints) *ListAssets {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC))
	return &ListAssets{read: r, volumes: r, clock: clk, catalog: all, hot: hot}
}

func moverAsset(t *testing.T) domain.Asset {
	t.Helper()
	asset, err := toAsset(appleRow(t, "equity"))
	if err != nil {
		t.Fatal(err)
	}
	asset.ChainChecked, asset.PopularRank = true, 1
	return asset
}

func TestMovers_reportsEachReadFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	sample := []sqlc.NewestSamplesRow{{Mint: appleRow(t, "equity").Mint, Ts: when, PriceMicros: 1}}
	calls := 0
	secondReadFails := func() ([]sqlc.NewestSamplesRow, error) {
		calls++
		if calls == 1 {
			return sample, nil
		}
		return nil, boom
	}
	badHot := func(context.Context) ([]chain.SolanaAddress, error) { return nil, boom }
	assets := fixedAssets{assets: []domain.Asset{moverAsset(t)}}
	cases := map[string]*ListAssets{
		"catalog": moverRig(t, reader{}, fixedAssets{err: boom}),
		"hot set": moverRig(t, reader{}, assets, badHot),
		"newest":  moverRig(t, reader{newest: func() ([]sqlc.NewestSamplesRow, error) { return nil, boom }}, assets),
		"summary": moverRig(t, reader{newest: secondReadFails}, assets),
	}
	for name, l := range cases {
		_, err := l.Handle(t.Context(), ListRequest{Filter: FilterMovers})
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("%s = %v, want internal", name, err)
		}
	}
}

func TestMovers_aSecondReadInTheSameTickServesTheSortedList(t *testing.T) {
	t.Parallel()
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	reads := 0
	r := reader{
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			reads++
			return []sqlc.NewestSamplesRow{{Mint: appleRow(t, "equity").Mint, Ts: when, PriceMicros: 1}}, nil
		},
	}
	l := moverRig(t, r, fixedAssets{assets: []domain.Asset{moverAsset(t)}})
	perCall := make([]int, 0, 2)
	for range 2 {
		before := reads
		if _, err := l.Handle(t.Context(), ListRequest{Filter: FilterMovers}); err != nil {
			t.Fatal(err)
		}
		perCall = append(perCall, reads-before)
	}
	if perCall[0] <= perCall[1] || perCall[1] != 1 {
		t.Fatalf("newest reads per call = %v, want the second to read the tick only", perCall)
	}
}

func TestMovers_rejectsABadCursor(t *testing.T) {
	t.Parallel()
	id := "01920000-0000-7000-8000-000000000001"
	for _, head := range []string{"m5", "mx,AAPLx", "m-5,AAPLx", "m5,"} {
		raw := base64.RawURLEncoding.EncodeToString([]byte(head + "\x00" + id))
		_, err := normalize(ListRequest{Filter: FilterMovers, Cursor: raw})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("cursor %q = %v, want invalid_input", head, err)
		}
	}
	symbol := base64.RawURLEncoding.EncodeToString([]byte("sAAPLx\x00" + id))
	_, err := normalize(ListRequest{Filter: FilterMovers, Cursor: symbol})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("symbol cursor on movers = %v, want invalid_input", err)
	}
}

func TestListSort_rejectsABadCursorAndReportsAFailedNameRead(t *testing.T) {
	t.Parallel()
	id := "01920000-0000-7000-8000-000000000001"
	for sort, heads := range map[Sort][]string{
		SortPrice: {"p5", "px,AAPLx", "p-5,AAPLx", "p5,", "sAAPLx"},
		SortName:  {"nAAPLx", "sAAPLx"},
	} {
		for _, head := range heads {
			raw := base64.RawURLEncoding.EncodeToString([]byte(head + "\x00" + id))
			_, err := normalize(ListRequest{Sort: sort, Cursor: raw})
			if errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("%s cursor %q = %v, want invalid_input", sort, head, err)
			}
		}
	}
	boom := errs.New(errs.CodeInternal, "market.test")
	l := &ListAssets{read: reader{name: func() ([]sqlc.Asset, error) { return nil, boom }}}
	if _, err := l.Handle(t.Context(), ListRequest{Sort: SortName}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("name read failure = %v, want internal", err)
	}
}

func TestListSort_aStockWithoutAPriceFollowsEveryPricedOne(t *testing.T) {
	t.Parallel()
	if got := comparePrice(false, 0, "A", true, 5, "B"); got != 1 {
		t.Fatalf("unpriced against priced = %d, want 1", got)
	}
	if got := comparePrice(true, 5, "B", false, 0, "A"); got != -1 {
		t.Fatalf("priced against unpriced = %d, want -1", got)
	}
}
