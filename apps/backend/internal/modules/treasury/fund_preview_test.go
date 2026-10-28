package treasury_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type previewCase struct {
	*settleHarness
}

func newPreviewCase(t *testing.T) previewCase {
	t.Helper()
	return previewCase{newSettleHarness(t)}
}

func (c previewCase) srv() adapters.HTTP {
	return adapters.HTTP{
		FundReads: c.pool, Pot: c.stubs.owner,
		FundPreview: app.NewPreviewFundHandler(app.PreviewFundDeps{
			Cabals: c.stubs, Balances: c.stubs, Pauses: c.stubs, Pot: c.stubs.owner,
			History: adapters.FundHistory{Reads: c.pool},
		}),
	}
}

func (c previewCase) holds(t *testing.T, user ids.UserID, units int64) {
	t.Helper()
	u, cabalTxn, err := c.fixture.fund(user, c.cabal, units, units, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.postPair(u, cabalTxn); err != nil {
		t.Fatal(err)
	}
}

func (c previewCase) preview(t *testing.T, amount *string) (api.FundPreview, error) {
	t.Helper()
	res, err := c.srv().GetFundPreview(c.actorCtx(), api.GetFundPreviewRequestObject{
		Id: c.cabal.UUID(), Params: api.GetFundPreviewParams{AmountMicros: amount},
	})
	if err != nil {
		return api.FundPreview{}, err
	}
	return api.FundPreview(res.(api.GetFundPreview200JSONResponse)), nil
}

func (c previewCase) mustPreview(t *testing.T, micros string) api.FundPreview {
	t.Helper()
	got, err := c.preview(t, &micros)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (c previewCase) fundAndSettle(t *testing.T, micros uint64) api.GetFundTransfer200JSONResponse {
	t.Helper()
	id, sig := c.submitted(t, micros)
	c.finalize(sig)
	c.tick(t)
	res, err := c.srv().GetFundTransfer(c.actorCtx(), api.GetFundTransferRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	return res.(api.GetFundTransfer200JSONResponse)
}

func (c previewCase) potSlice(t *testing.T) int32 {
	t.Helper()
	pot, err := c.stubs.owner.CabalPot(c.ctx(), c.cabal, c.user, true)
	if err != nil {
		t.Fatal(err)
	}
	return pot.Me.SliceBps
}

func (c previewCase) assertRealFundMatches(t *testing.T, micros uint64, want int32) {
	t.Helper()
	before := c.mustPreview(t, money.MicrosFromUint64(micros).String())
	if before.SliceBpsAfter == nil || *before.SliceBpsAfter != want {
		t.Fatalf("preview after = %v, want %d", before.SliceBpsAfter, want)
	}
	settled := c.fundAndSettle(t, micros)
	if settled.SliceBps == nil || *settled.SliceBps != want {
		t.Fatalf("settled slice_bps = %v, want %d", settled.SliceBps, want)
	}
	if got := c.potSlice(t); got != want {
		t.Fatalf("pot me.slice_bps = %d, want %d", got, want)
	}
}

func TestFundPreview_withoutAnAmountGivesTheCurrentShare(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	c.holds(t, c.fixture.user(t), 65_000_000)
	c.holds(t, c.user, 35_000_000)
	got, err := c.preview(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.AvailableMicros != "100000000" || got.MinMicros != "1000000" || got.SliceBpsNow != 3500 ||
		got.SliceBpsAfter != nil || got.OverMicros != "0" || got.Paused || got.FirstFundedAt != nil {
		t.Fatalf("preview = %+v, want 100000000 available, 3500 now, no after", got)
	}
}

func TestFundPreview_matchesTheRealFund(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		others []int64
		mine   int64
		fund   uint64
		want   int32
	}{
		"clean split": {others: []int64{65_000_000}, mine: 35_000_000, fund: 1_562_500, want: 3600},
		"caller ends largest and takes the remainder": {
			others: []int64{33_333_333, 33_333_333}, mine: 33_333_333, fund: 1_000_000, want: 3400,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newPreviewCase(t)
			for _, units := range tt.others {
				c.holds(t, c.fixture.user(t), units)
			}
			c.holds(t, c.user, tt.mine)
			c.assertRealFundMatches(t, tt.fund, tt.want)
		})
	}
}

func TestFundPreview_tieGoesToTheLowerUserID(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		caller int
		want   int32
	}{"caller sorts first": {caller: 0, want: 3334}, "caller sorts last": {caller: 2, want: 3333}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newPreviewCase(t)
			users := []ids.UserID{c.fixture.user(t), c.fixture.user(t), c.fixture.user(t)}
			sort.Slice(users, func(i, j int) bool { return users[i].String() < users[j].String() })
			for i, user := range users {
				if i != tt.caller {
					c.holds(t, user, 1_000_000)
				}
			}
			c.user = users[tt.caller]
			c.assertRealFundMatches(t, 1_000_000, tt.want)
		})
	}
}

func TestFundPreview_overTheCashPromisesOnlyTheCash(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	c.holds(t, c.fixture.user(t), 65_000_000)
	c.holds(t, c.user, 35_000_000)
	exact := c.mustPreview(t, "100000000")
	over := c.mustPreview(t, "150000000")
	if over.OverMicros != "50000000" || over.SliceBpsAfter == nil || exact.SliceBpsAfter == nil ||
		*over.SliceBpsAfter != *exact.SliceBpsAfter || exact.OverMicros != "0" {
		t.Fatalf("over = %+v, exact = %+v, want 50000000 over and the same slice", over, exact)
	}
	c.stubs.onChain = money.MicrosFromUint64(500_000)
	if broke := c.mustPreview(t, "5000000"); broke.OverMicros != "4500000" || broke.SliceBpsAfter != nil {
		t.Fatalf("below the minimum cash = %+v, want 4500000 over and no after", broke)
	}
}

func TestFundPreview_belowTheMinimumHasNoAfter(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	c.holds(t, c.user, 35_000_000)
	for _, amount := range []string{"0", "999999"} {
		if got := c.mustPreview(t, amount); got.SliceBpsAfter != nil || got.OverMicros != "0" {
			t.Fatalf("amount %s = %+v, want no after and no over", amount, got)
		}
	}
}

func TestFundPreview_pausedAnswersOK(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	c.stubs.paused = true
	if got := c.mustPreview(t, "5000000"); !got.Paused {
		t.Fatalf("preview = %+v, want paused", got)
	}
}

func TestFundPreview_refusals(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	bad := "1.5"
	_, err := c.preview(t, &bad)
	wantCode(t, err, errs.CodeInvalidInput)
	c.stubs.status = cabalport.StatusBanned
	_, err = c.preview(t, nil)
	wantCode(t, err, errs.CodeCabalBanned)
	c.stubs.member = false
	_, err = c.preview(t, nil)
	wantCode(t, err, errs.CodeNotCabalMember)
	_, err = c.srv().GetFundPreview(c.ctx(), api.GetFundPreviewRequestObject{Id: c.cabal.UUID()})
	wantCode(t, err, errs.CodeUnauthorized)
	_, err = adapters.HTTP{}.GetFundPreview(c.actorCtx(), api.GetFundPreviewRequestObject{Id: c.cabal.UUID()})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

func TestFundPreview_firstFundedAtIsTheEarliestConfirmedFund(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	if got, err := c.preview(t, nil); err != nil || got.FirstFundedAt != nil {
		t.Fatalf("never funded = %+v, %v, want no first_funded_at", got, err)
	}
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	rows := []struct {
		kind, status string
		user         ids.UserID
		cabal        ids.CabalID
		at           time.Time
	}{
		{"fund", "pending", c.user, c.cabal, base.Add(-48 * time.Hour)},
		{"fund", "confirmed", c.fixture.user(t), c.cabal, base.Add(-36 * time.Hour)},
		{"fund", "confirmed", c.user, c.fixture.cabal(t), base.Add(-30 * time.Hour)},
		{"cash_out", "confirmed", c.user, c.cabal, base.Add(-24 * time.Hour)},
		{"fund", "confirmed", c.user, c.cabal, base},
		{"fund", "confirmed", c.user, c.cabal, base.Add(72 * time.Hour)},
	}
	for _, r := range rows {
		if _, err := c.pool.Exec(t.Context(), `INSERT INTO cabal_activity
			(id, cabal_id, kind, status, actor_user_id, usdc_micros, occurred_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 1000000, $6, $6)`,
			c.ids.NewV7(), r.cabal.UUID(), r.kind, r.status, r.user.UUID(), r.at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := c.preview(t, nil)
	if err != nil || got.FirstFundedAt == nil || !got.FirstFundedAt.Equal(base) {
		t.Fatalf("first_funded_at = %v, %v, want %v", got.FirstFundedAt, err, base)
	}
}

func TestFundPreview_theFirstFunderOfAnEmptyPotGetsEverything(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	got := c.mustPreview(t, "5000000")
	if got.SliceBpsNow != 0 || got.SliceBpsAfter == nil || *got.SliceBpsAfter != 10000 {
		t.Fatalf("preview = %+v, want 0 now and 10000 after", got)
	}
}

func TestFundPreview_aWorthlessPotRefusesAnAmount(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	c.holds(t, c.user, 1_000_000)
	drain, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: c.ids.NewV7(), CabalID: c.cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled, SwapID: c.ids.NewV7(),
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: c.usdc(), Amount: amount(-1_000_000)},
		{Account: domain.CabalVenue, Asset: c.usdc(), Amount: amount(1_000_000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.postCabal(drain); err != nil {
		t.Fatal(err)
	}
	if got, err := c.preview(t, nil); err != nil || got.SliceBpsNow != 10000 {
		t.Fatalf("no amount = %+v, %v, want 10000 now", got, err)
	}
	amountMicros := "5000000"
	_, err = c.preview(t, &amountMicros)
	wantCode(t, err, errs.CodePotValueZero)
}

func TestGetFundTransfer_sliceBpsOnlyOnceSettled(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	read := func(id uuid.UUID) api.GetFundTransfer200JSONResponse {
		res, err := c.srv().GetFundTransfer(c.actorCtx(), api.GetFundTransferRequestObject{Id: id})
		if err != nil {
			t.Fatal(err)
		}
		return res.(api.GetFundTransfer200JSONResponse)
	}
	for _, status := range []domain.FundStatus{domain.FundSubmitted, domain.FundLanded, domain.FundFailed} {
		if got := read(c.fundTransferIn(t, c.user, status, 5_000_000)); got.SliceBps != nil {
			t.Fatalf("%s transfer slice_bps = %d, want null", status, *got.SliceBps)
		}
	}
	if got := read(c.fundTransferIn(t, c.user, domain.FundSettled, 5_000_000)); got.SliceBps == nil {
		t.Fatal("settled transfer slice_bps = null, want a share")
	}
}

func TestGetFundTransfer_sliceBpsWaitsForAPrice(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	p.buy(t, 40_000_000, 200_000_000)
	id := p.f.fundTransferIn(t, p.funder, domain.FundSettled, 5_000_000)
	if _, err := p.f.pool.Exec(t.Context(), `UPDATE fund_transfers SET cabal_id = $1 WHERE id = $2`,
		p.cabal.UUID(), id); err != nil {
		t.Fatal(err)
	}
	actor := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: p.funder.String()})
	srv := adapters.HTTP{FundReads: p.f.pool, Pot: p.q}
	res, err := srv.GetFundTransfer(actor, api.GetFundTransferRequestObject{Id: id})
	if err != nil || res.(api.GetFundTransfer200JSONResponse).SliceBps != nil {
		t.Fatalf("without a price = %#v, %v, want settled with no slice_bps", res, err)
	}
	broken := adapterQueriesWithReader(
		p.f,
		func(context.Context, []uuid.UUID, time.Time) (map[uuid.UUID]app.Price, error) {
			return nil, errs.New(errs.CodeInternal, "stub")
		},
	)
	_, err = adapters.HTTP{FundReads: p.f.pool, Pot: broken}.GetFundTransfer(
		actor, api.GetFundTransferRequestObject{Id: id})
	wantCode(t, err, errs.CodeInternal)
	p.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), p.f.clock.Now())
	res, err = srv.GetFundTransfer(actor, api.GetFundTransferRequestObject{Id: id})
	if got := res.(api.GetFundTransfer200JSONResponse); err != nil || got.SliceBps == nil || *got.SliceBps != 10000 {
		t.Fatalf("with a price = %#v, %v, want 10000", got, err)
	}
}

func TestFundHistory_readFailureIsInternal(t *testing.T) {
	t.Parallel()
	c := newPreviewCase(t)
	_, _, err := adapters.FundHistory{Reads: c.pool}.FirstFundedAt(canceled(c.ctx()), c.cabal, c.user)
	wantCode(t, err, errs.CodeInternal)
}
