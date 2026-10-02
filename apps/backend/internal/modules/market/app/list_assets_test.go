package app

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
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
	popular := encodeCursor(FilterPopular, asset)
	symbol := encodeCursor(FilterAll, asset)
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
	rank   func() ([]sqlc.Asset, error)
}

func (r reader) ListAssetsBySymbol(context.Context, sqlc.ListAssetsBySymbolParams) ([]sqlc.Asset, error) {
	return r.symbol()
}

func (r reader) ListAssetsByRank(context.Context, sqlc.ListAssetsByRankParams) ([]sqlc.Asset, error) {
	return r.rank()
}

func (r reader) list(t *testing.T, req ListRequest) (Page, error) {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC))
	return (&ListAssets{read: r, clock: clk}).Handle(t.Context(), req)
}

func TestListAssets_reportsEachReadFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	cases := []struct {
		read   reader
		filter Filter
	}{
		{reader{symbol: func() ([]sqlc.Asset, error) { return nil, boom }}, FilterAll},
		{reader{rank: func() ([]sqlc.Asset, error) { return nil, boom }}, FilterPopular},
	}
	for i, tc := range cases {
		if _, err := tc.read.list(t, ListRequest{Filter: tc.filter}); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("case %d = %v, want internal", i, err)
		}
	}
}
