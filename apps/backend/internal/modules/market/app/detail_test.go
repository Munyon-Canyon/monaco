package app

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type detailRead struct {
	asset  func() (sqlc.Asset, error)
	others func() ([]sqlc.Asset, error)
}

func (r detailRead) AssetBySymbol(context.Context, string) (sqlc.Asset, error) {
	return r.asset()
}

func (r detailRead) SiblingAssets(context.Context, sqlc.SiblingAssetsParams) ([]sqlc.Asset, error) {
	if r.others == nil {
		return []sqlc.Asset{}, nil
	}
	return r.others()
}

func TestDetail_reportsAMissingAssetAndABadListing(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	boom := errs.New(errs.CodeInternal, "market.test")
	ok := func() (sqlc.Asset, error) { return apple, nil }
	cases := []struct {
		read detailRead
		list reader
		code errs.Code
	}{
		{
			read: detailRead{asset: func() (sqlc.Asset, error) { return sqlc.Asset{}, sql.ErrNoRows }},
			code: errs.CodeAssetNotFound,
		},
		{read: detailRead{asset: func() (sqlc.Asset, error) { return sqlc.Asset{}, boom }}, code: errs.CodeInternal},
		{
			read: detailRead{asset: ok},
			list: reader{
				symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil },
				newest: func() ([]sqlc.NewestSamplesRow, error) { return nil, boom },
			},
			code: errs.CodeInternal,
		},
		{
			read: detailRead{asset: ok, others: func() ([]sqlc.Asset, error) { return nil, boom }},
			code: errs.CodeInternal,
		},
		{
			read: detailRead{asset: ok, others: func() ([]sqlc.Asset, error) {
				bad := apple
				bad.Symbol, bad.Issuer = "AAPLy", "nope"
				return []sqlc.Asset{bad}, nil
			}},
			code: errs.CodeDecodeFailed,
		},
	}
	for i, tc := range cases {
		d := &Detail{list: &ListAssets{read: tc.list, clock: testkit.NewClock(when)}, read: tc.read}
		if _, err := d.Handle(t.Context(), "AAPLx"); errs.CodeOf(err) != tc.code {
			t.Fatalf("case %d = %v, want %s", i, err, tc.code)
		}
	}
}
