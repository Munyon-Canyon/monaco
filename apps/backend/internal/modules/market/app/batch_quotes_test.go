package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type quotesRead func() ([]sqlc.Asset, error)

func (r quotesRead) AssetsBySymbols(context.Context, []string) ([]sqlc.Asset, error) { return r() }

func TestBatchQuotes_reportsABadReadAndABadListing(t *testing.T) {
	t.Parallel()
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	boom := errs.New(errs.CodeInternal, "market.test")
	bad := appleRow(t, "equity")
	bad.Issuer = "nope"
	cases := []struct {
		read quotesRead
		code errs.Code
	}{
		{read: func() ([]sqlc.Asset, error) { return nil, boom }, code: errs.CodeInternal},
		{read: func() ([]sqlc.Asset, error) { return []sqlc.Asset{bad}, nil }, code: errs.CodeDecodeFailed},
	}
	for i, tc := range cases {
		q := &BatchQuotes{list: &ListAssets{clock: testkit.NewClock(when)}, read: tc.read}
		if _, err := q.Handle(t.Context(), []string{"AAPLx"}); errs.CodeOf(err) != tc.code {
			t.Fatalf("case %d = %v, want %s", i, err, tc.code)
		}
	}
}

func TestBatchQuotes_noKnownSymbolIsAnEmptyAnswer(t *testing.T) {
	t.Parallel()
	q := &BatchQuotes{
		list: &ListAssets{clock: testkit.NewClock(time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC))},
		read: quotesRead(func() ([]sqlc.Asset, error) { return nil, nil }),
	}
	got, err := q.Handle(t.Context(), []string{"NOPEx"})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Handle = %v, %v", got, err)
	}
}
