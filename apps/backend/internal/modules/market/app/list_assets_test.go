package app

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestListAssets_rejectsABadRequest(t *testing.T) {
	t.Parallel()
	id := "01920000-0000-7000-8000-000000000001"
	assetID, err := domain.ParseAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	asset := domain.Asset{ID: assetID, Symbol: "AAPLx", PopularRank: 1}
	popular := encodeCursor(FilterPopular, SortSymbol, asset)
	symbol := encodeCursor(FilterAll, SortSymbol, asset)
	cases := []ListRequest{
		{Filter: "nope"},
		{Limit: -1},
		{Limit: 51},
		{Cursor: "%%%"},
		{Cursor: base64.RawURLEncoding.EncodeToString([]byte("noseparator"))},
		{Cursor: base64.RawURLEncoding.EncodeToString([]byte("sA\x00not-a-uuid"))},
		{Cursor: base64.RawURLEncoding.EncodeToString([]byte("s\x00" + id))},
		{Filter: FilterPopular, Cursor: base64.RawURLEncoding.EncodeToString([]byte("r99999\x00" + id))},
		{Filter: FilterPopular, Cursor: symbol},
		{Cursor: popular},
	}
	for _, req := range cases {
		if _, err := normalize(req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("normalize(%+v) = %v, want invalid_input", req, err)
		}
	}
	got, err := normalize(ListRequest{})
	if err != nil || got.limit != listDefault || got.filter != FilterAll || got.cursor.ok {
		t.Fatalf("normalize(empty) = %+v, %v", got, err)
	}
}

type reader struct {
	symbol func() ([]sqlc.Asset, error)
	name   func() ([]sqlc.Asset, error)
	rank   func() ([]sqlc.Asset, error)
	newest func() ([]sqlc.NewestSamplesRow, error)
	first  func() ([]sqlc.FirstSamplesSinceRow, error)
	spark  func() ([]sqlc.SparklineClosesRow, error)
	vols   func(sqlc.VolumesByMintsParams) ([]sqlc.VolumesByMintsRow, error)
}

func (r reader) ListAssetsBySymbol(context.Context, sqlc.ListAssetsBySymbolParams) ([]sqlc.Asset, error) {
	if r.symbol == nil {
		return []sqlc.Asset{}, nil
	}
	return r.symbol()
}

func (r reader) ListAssetsByName(context.Context, sqlc.ListAssetsByNameParams) ([]sqlc.Asset, error) {
	if r.name == nil {
		return []sqlc.Asset{}, nil
	}
	return r.name()
}

func (r reader) ListAssetsByRank(context.Context, sqlc.ListAssetsByRankParams) ([]sqlc.Asset, error) {
	if r.rank == nil {
		return []sqlc.Asset{}, nil
	}
	return r.rank()
}

func (r reader) NewestSamples(context.Context, sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error) {
	if r.newest == nil {
		return []sqlc.NewestSamplesRow{}, nil
	}
	return r.newest()
}

func (r reader) FirstSamplesSince(context.Context, sqlc.FirstSamplesSinceParams) ([]sqlc.FirstSamplesSinceRow, error) {
	if r.first == nil {
		return []sqlc.FirstSamplesSinceRow{}, nil
	}
	return r.first()
}

func (r reader) VolumesByMints(_ context.Context, p sqlc.VolumesByMintsParams) ([]sqlc.VolumesByMintsRow, error) {
	if r.vols == nil {
		return []sqlc.VolumesByMintsRow{}, nil
	}
	return r.vols(p)
}

func (r reader) SparklineCloses(context.Context, sqlc.SparklineClosesParams) ([]sqlc.SparklineClosesRow, error) {
	if r.spark == nil {
		return []sqlc.SparklineClosesRow{}, nil
	}
	return r.spark()
}

func appleRow(t *testing.T, kind string) sqlc.Asset {
	t.Helper()
	id, err := uuid.Parse("01920000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	return sqlc.Asset{
		ID: id, Symbol: "AAPLx", Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Decimals: 8,
		Issuer: "xstocks", Kind: kind, DisplayName: "Apple xStock", UiMultiplierNum: 1, UiMultiplierDen: 1,
		IssuerTradable: true, CompanyKey: "apple",
		FirstSeenAt: time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC),
	}
}

func (r reader) list(t *testing.T, req ListRequest) (Page, error) {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC))
	return (&ListAssets{read: r, volumes: r, clock: clk}).Handle(t.Context(), req)
}

func TestListAssets_reportsEachReadFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	apple := appleRow(t, "equity")
	pre := appleRow(t, "pre_ipo")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	sample := []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when, PriceMicros: 1}}
	calls := 0
	cases := []reader{
		{symbol: func() ([]sqlc.Asset, error) { return nil, boom }},
		{rank: func() ([]sqlc.Asset, error) { return nil, boom }},
		{
			symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil },
			newest: func() ([]sqlc.NewestSamplesRow, error) { return nil, boom },
		},
		{
			symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil },
			newest: func() ([]sqlc.NewestSamplesRow, error) {
				calls++
				if calls == 1 {
					return sample, nil
				}
				return nil, boom
			},
		},
		{
			symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{pre}, nil },
			newest: func() ([]sqlc.NewestSamplesRow, error) { return sample, nil },
			first:  func() ([]sqlc.FirstSamplesSinceRow, error) { return nil, boom },
		},
		{
			symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil },
			newest: func() ([]sqlc.NewestSamplesRow, error) { return sample, nil },
			spark:  func() ([]sqlc.SparklineClosesRow, error) { return nil, boom },
		},
	}
	filters := []Filter{FilterAll, FilterPopular, FilterAll, FilterAll, FilterAll, FilterAll}
	for i, r := range cases {
		if _, err := r.list(t, ListRequest{Filter: filters[i]}); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("case %d = %v, want internal", i, err)
		}
	}
}

func TestListAssets_reportsAPriceThatDoesNotDecode(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	pre := appleRow(t, "pre_ipo")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	good := func() ([]sqlc.NewestSamplesRow, error) {
		return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when, PriceMicros: 1}}, nil
	}
	cases := []reader{
		{
			symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil },
			newest: func() ([]sqlc.NewestSamplesRow, error) {
				return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when, PriceMicros: -1}}, nil
			},
		},
		{
			symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{pre}, nil }, newest: good,
			first: func() ([]sqlc.FirstSamplesSinceRow, error) {
				return []sqlc.FirstSamplesSinceRow{{Mint: pre.Mint, Ts: when, PriceMicros: -1}}, nil
			},
		},
		{
			symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil }, newest: good,
			spark: func() ([]sqlc.SparklineClosesRow, error) {
				return []sqlc.SparklineClosesRow{{Mint: apple.Mint, Bucket: when, CloseMicros: -1}}, nil
			},
		},
	}
	for i, r := range cases {
		if _, err := r.list(t, ListRequest{}); errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("case %d = %v, want decode_failed", i, err)
		}
	}
}

func TestGroupSpark_keepsTheBucketSoAHeldSpikeCannotCrossIt(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	at := func(hour, minute int) time.Time {
		return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}
	const mint = "mint"
	spark, err := groupSpark([]sqlc.SparklineClosesRow{
		{Mint: mint, Bucket: at(13, 30), CloseMicros: 300},
		{Mint: mint, Bucket: at(14, 30), CloseMicros: 200},
		{Mint: mint, Bucket: at(15, 0), CloseMicros: 300},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := domain.LastSparkline(spark[mint], []domain.Sample{
		{Micros: money.MicrosFromUint64(300), ObservedAt: at(15, 0)},
		{Micros: money.MicrosFromUint64(200), ObservedAt: at(14, 58)},
		{Micros: money.MicrosFromUint64(100), ObservedAt: at(14, 56)},
	})
	if len(got) != 2 || got[0].Uint64() != 300 || got[1].Uint64() != 100 {
		t.Fatalf("sparkline = %v", got)
	}
}

func TestListAssets_makesOneVolumeQueryForTheWholePage(t *testing.T) {
	t.Parallel()
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	apple := appleRow(t, "equity")
	other := apple
	other.ID, other.Symbol = testkit.NewIDs(2).NewV7(), "TSLAx"
	other.Mint = "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB"
	var asks []sqlc.VolumesByMintsParams
	r := reader{
		symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple, other}, nil },
		vols: func(p sqlc.VolumesByMintsParams) ([]sqlc.VolumesByMintsRow, error) {
			asks = append(asks, p)
			return []sqlc.VolumesByMintsRow{{Mint: other.Mint, Volume24hMicros: 42, AsOf: when.Add(-time.Hour)}}, nil
		},
	}
	page, err := r.list(t, ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(asks) != 1 || len(asks[0].Mints) != 2 || !asks[0].Since.Equal(when.Add(-VolumeMaxAge)) {
		t.Fatalf("volume queries = %+v, want one for both mints since now-36h", asks)
	}
	second := page.Items[1].Volume
	if page.Items[0].Volume != nil || second == nil || second.USD != money.MicrosFromUint64(42) ||
		!second.At.Equal(when.Add(-time.Hour)) {
		t.Fatalf("items = %+v, want the volume on the second only", page.Items)
	}
}

func TestListAssets_reportsAVolumeReadFailureAndAVolumeThatDoesNotDecode(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	one := func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil }
	for want, vols := range map[errs.Code]func(sqlc.VolumesByMintsParams) ([]sqlc.VolumesByMintsRow, error){
		errs.CodeInternal: func(sqlc.VolumesByMintsParams) ([]sqlc.VolumesByMintsRow, error) {
			return nil, errs.New(errs.CodeInternal, "market.test")
		},
		errs.CodeDecodeFailed: func(sqlc.VolumesByMintsParams) ([]sqlc.VolumesByMintsRow, error) {
			return []sqlc.VolumesByMintsRow{{Mint: apple.Mint, Volume24hMicros: -1}}, nil
		},
	} {
		if _, err := (reader{symbol: one, vols: vols}).list(t, ListRequest{}); errs.CodeOf(err) != want {
			t.Fatalf("err = %v, want %s", err, want)
		}
	}
}
