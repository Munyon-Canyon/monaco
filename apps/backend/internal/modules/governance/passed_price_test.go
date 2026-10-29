package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const passedPrice = 709_800_000

type pricedVote struct {
	voteDB
	catalog *marketfake.CatalogFake
	prices  *marketfake.PricesFake
}

func newPricedVote(t *testing.T) pricedVote {
	t.Helper()
	d := pricedVote{
		voteDB:  newVoteDB(t),
		catalog: marketfake.NewCatalog(marketfake.Fixtures()...),
		prices:  &marketfake.PricesFake{},
	}
	d.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(passedPrice), d.clk.Now().Add(-time.Minute))
	return d
}

func (d pricedVote) handler() *app.CastVoteHandler {
	return d.voteDB.handler().Priced(d.catalog, d.prices)
}

func (d pricedVote) passedPrice(t *testing.T, proposal uuid.UUID) pgtype.Int8 {
	t.Helper()
	var got pgtype.Int8
	if err := d.pool.QueryRow(t.Context(),
		`SELECT passed_price_micros FROM proposals WHERE id = $1`, proposal).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func wantStoredPrice(t *testing.T, got pgtype.Int8) {
	t.Helper()
	if !got.Valid || got.Int64 != passedPrice {
		t.Fatalf("passed_price_micros = %+v, want %d", got, passedPrice)
	}
}

func wantNull(t *testing.T, got pgtype.Int8) {
	t.Helper()
	if got.Valid {
		t.Fatalf("passed_price_micros = %+v, want null", got)
	}
}

func warnings(buf *bytes.Buffer) []map[string]any {
	var out []map[string]any
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		var rec map[string]any
		if len(line) > 0 && json.Unmarshal(line, &rec) == nil && rec["level"] == "WARN" {
			out = append(out, rec)
		}
	}
	return out
}

func loggedCtx(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	return observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(buf, nil))), buf
}

func TestCastVote_decidingVoteStoresThePriceReadBeforeTheTransaction(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	p, v := d.open(t, 2)
	h := d.handler()
	d.mustCast(t, h, p.ID, v[0], domain.ChoiceYes)
	wantNull(t, d.passedPrice(t, p.ID))
	d.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(passedPrice+1), d.clk.Now().Add(time.Minute))
	d.mustCast(t, h, p.ID, v[1], domain.ChoiceYes)
	wantStoredPrice(t, d.passedPrice(t, p.ID))
	if asked := d.prices.Asked(); len(asked) != 2 || len(asked[1]) != 1 || asked[1][0] != marketfake.AAPLx().ID {
		t.Fatalf("PricesAsOf asked %v, want the proposal's asset once per vote", asked)
	}
}

func TestCastVote_anAssetWithNoPriceStoresNullAndTheVotePasses(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	d.prices = &marketfake.PricesFake{}
	ctx, buf := loggedCtx(t)
	p, v := d.open(t, 1)
	got, err := d.cast(ctx, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if err != nil || got.Status != domain.StatusPassed {
		t.Fatalf("CastVote = %+v, %v, want passed", got, err)
	}
	wantNull(t, d.passedPrice(t, p.ID))
	if w := warnings(buf); len(w) != 0 {
		t.Fatalf("warnings = %v, want none for a missing price", w)
	}
}

func TestCastVote_aPriceBeyondBigintStoresNullAndTheVotePasses(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	d.prices = &marketfake.PricesFake{}
	d.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(math.MaxInt64+1), d.clk.Now().Add(-time.Minute))
	p, v := d.open(t, 1)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	wantNull(t, d.passedPrice(t, p.ID))
}

func TestCastVote_aFailingMintLookupStoresNullLogsOnceAndTheVotePasses(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	d.catalog.Fail("AssetByMint", errs.New(errs.CodeUpstreamUnavailable, "test"))
	ctx, buf := loggedCtx(t)
	p, v := d.open(t, 1)
	got, err := d.cast(ctx, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if err != nil || got.Status != domain.StatusPassed {
		t.Fatalf("CastVote = %+v, %v, want passed", got, err)
	}
	wantNull(t, d.passedPrice(t, p.ID))
	wantOneWarning(t, buf, p.ID, errs.CodeUpstreamUnavailable)
}

func TestCastVote_aFailingPriceReadStoresNullLogsOnceAndTheVotePasses(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	d.prices.Fail("PricesAsOf", errs.New(errs.CodeUpstreamUnavailable, "test"))
	ctx, buf := loggedCtx(t)
	p, v := d.open(t, 1)
	got, err := d.cast(ctx, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if err != nil || got.Status != domain.StatusPassed {
		t.Fatalf("CastVote = %+v, %v, want passed", got, err)
	}
	wantNull(t, d.passedPrice(t, p.ID))
	wantOneWarning(t, buf, p.ID, errs.CodeUpstreamUnavailable)
}

func TestCastVote_anUnreadableMintStoresNullLogsOnceAndTheVotePasses(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	ctx, buf := loggedCtx(t)
	p, v := d.open(t, 1)
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET mint = 'not-a-mint' WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := d.cast(ctx, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if err != nil || got.Status != domain.StatusPassed {
		t.Fatalf("CastVote = %+v, %v, want passed", got, err)
	}
	wantNull(t, d.passedPrice(t, p.ID))
	if w := warnings(buf); len(w) != 1 {
		t.Fatalf("warnings = %v, want one", w)
	}
}

func TestCastVote_aMissingProposalAnswersNotFoundAndLogsNoDegradedPrice(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	ctx, buf := loggedCtx(t)
	_, err := d.cast(ctx, d.handler(), d.ids.NewV7(), d.ids.NewV7(), domain.ChoiceYes)
	if errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("CastVote err = %v, want proposal_not_found", err)
	}
	if w := warnings(buf); len(w) != 0 {
		t.Fatalf("warnings = %v, want none", w)
	}
}

func wantOneWarning(t *testing.T, buf *bytes.Buffer, proposal uuid.UUID, code errs.Code) {
	t.Helper()
	w := warnings(buf)
	if len(w) != 1 || w[0]["proposal_id"] != proposal.String() || w[0]["code"] != string(code) {
		t.Fatalf("warnings = %v, want one for proposal %s with code %s", w, proposal, code)
	}
}

func TestCastVote_aFailedOrUndecidedProposalStoresNoPrice(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	h := d.handler()
	failing, fv := d.openUnder(t, 2, "unanimous")
	d.mustCast(t, h, failing.ID, fv[0], domain.ChoiceNo)
	if r := d.row(t, failing.ID); r.status.String != "failed" {
		t.Fatalf("status = %s, want failed", r.status.String)
	}
	wantNull(t, d.passedPrice(t, failing.ID))
	undecided, uv := d.open(t, 3)
	d.mustCast(t, h, undecided.ID, uv[0], domain.ChoiceYes)
	wantNull(t, d.passedPrice(t, undecided.ID))
}

func TestCastVote_withoutPricePortsTheVoteStillPasses(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 1)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	wantNull(t, pricedVote{voteDB: d}.passedPrice(t, p.ID))
}

func TestCastVote_theDecidingVoteReadsNothingThroughThePoolInsideItsTransaction(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	pool, guard := testkit.TracedPool(t, d.pool)
	d.uow = db.New(pool, d.ids, d.clk)
	p, v := d.open(t, 1)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	wantStoredPrice(t, d.passedPrice(t, p.ID))
	if found := guard.Found(); len(found) != 0 {
		t.Fatalf("pool queries inside the vote's transaction = %+v, want none", found)
	}
}

func TestTransition_aReplayNeverOverwritesThePassedPrice(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	p, v := d.open(t, 1)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	n, err := d.q.Transition(t.Context(), sqlc.TransitionParams{
		ID: p.ID, FromStatus: string(domain.StatusOpen), ToStatus: string(domain.StatusPassed), At: d.clk.Now(),
		PassedPriceMicros: pgtype.Int8{Int64: 1, Valid: true},
	})
	if err != nil || n != 0 {
		t.Fatalf("second Transition = %d rows, %v, want 0", n, err)
	}
	wantStoredPrice(t, d.passedPrice(t, p.ID))
	_, err = d.cast(t.Context(), d.handler(), p.ID, v[0], domain.ChoiceYes)
	if errs.CodeOf(err) != errs.CodeProposalClosed {
		t.Fatalf("a vote after the pass err = %v, want proposal_closed", err)
	}
	wantStoredPrice(t, d.passedPrice(t, p.ID))
}

func TestCastVote_aFailedEventAppendRollsBackTheStatusAndThePrice(t *testing.T) {
	t.Parallel()
	d := newPricedVote(t)
	p, v := d.open(t, 1)
	const guardEvents = `
CREATE FUNCTION refuse_passed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'append refused'; END $$;
CREATE TRIGGER refuse_passed BEFORE INSERT ON events FOR EACH ROW WHEN (NEW.type = 'proposal.passed')
EXECUTE FUNCTION refuse_passed();`
	if _, err := d.pool.Exec(t.Context(), guardEvents); err != nil {
		t.Fatal(err)
	}
	if _, err := d.cast(t.Context(), d.handler(), p.ID, v[0], domain.ChoiceYes); err == nil {
		t.Fatal("CastVote succeeded with the event append refused")
	}
	if r := d.row(t, p.ID); r.status.String != "open" {
		t.Fatalf("status = %s, want open after the rollback", r.status.String)
	}
	wantNull(t, d.passedPrice(t, p.ID))
}

func TestGetProposal_returnsThePassedPriceAndTheProposerForEverySwapState(t *testing.T) {
	t.Parallel()
	photo := "https://img.example/christelle.png"
	for name, swap := range map[string]*trading.SwapView{
		"no swap": nil,
		"pending": {Status: "pending"},
		"failed":  {Status: "failed", FailureCode: "jupiter_failed"},
		"confirmed": {
			Status: "confirmed", InAmount: 300_000_000, OutAmount: 42_000_000,
			ConfirmedAt: time.Date(2026, 3, 2, 9, 41, 0, 0, time.UTC),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newReadsDB(t)
			proposer := d.ids.NewV7()
			p := d.proposedBy(t, proposer)
			d.setStatus(t, p, string(domain.StatusPassed), "")
			if _, err := d.pool.Exec(t.Context(),
				`UPDATE proposals SET passed_price_micros = $2 WHERE id = $1`, p, int64(passedPrice)); err != nil {
				t.Fatal(err)
			}
			if swap != nil {
				swap.ID, swap.Source = ids.SwapIDFrom(d.ids.NewV7()), trading.Source{Kind: "proposal", ID: p}
				d.swaps.Put(*swap)
			}
			cards := &cardBook{cards: map[ids.UserID]identityport.UserCard{
				ids.UserIDFrom(proposer): {ID: ids.UserIDFrom(proposer), DisplayName: "Christelle", PhotoURL: photo},
			}}
			reads := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, cards, &returnBook{}, &replyBook{})
			got, err := reads.Get(
				t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(p), Caller: ids.UserIDFrom(d.caller)},
			)
			if err != nil {
				t.Fatal(err)
			}
			wantCard := app.ProposerCard{ID: ids.UserIDFrom(proposer), DisplayName: "Christelle", PhotoURL: photo}
			if got.Proposer != wantCard || got.PassedPriceMicros == nil || *got.PassedPriceMicros != passedPrice {
				t.Fatalf("detail proposer = %+v, passed price = %v, want %+v and %d",
					got.Proposer, got.PassedPriceMicros, wantCard, passedPrice)
			}
		})
	}
}

func TestGetProposal_anOpenOrOldProposalHasNoPassedPriceAndADeletedProposerKeepsOnlyTheirID(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	proposer := d.ids.NewV7()
	p := d.proposedBy(t, proposer)
	cards := &cardBook{cards: map[ids.UserID]identityport.UserCard{
		ids.UserIDFrom(proposer): {ID: ids.UserIDFrom(proposer), DisplayName: "Gone", Deleted: true},
	}}
	reads := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, cards, &returnBook{}, &replyBook{})
	got, err := reads.Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(p), Caller: ids.UserIDFrom(d.caller)})
	if err != nil || got.PassedPriceMicros != nil || got.Proposer != (app.ProposerCard{ID: ids.UserIDFrom(proposer)}) {
		t.Fatalf("detail = %+v, %v, want no passed price and the bare proposer id", got, err)
	}
}

func TestProposerCard_readsTheNameAndPhotoAndWrapsAFailingRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	who := ids.UserIDFrom(d.caller)
	cards := &cardBook{cards: map[ids.UserID]identityport.UserCard{who: {ID: who, DisplayName: "Mia", PhotoURL: "p"}}}
	reads := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, cards, &returnBook{}, &replyBook{})
	want := app.ProposerCard{ID: who, DisplayName: "Mia", PhotoURL: "p"}
	if got, err := reads.ProposerCard(t.Context(), who); err != nil || got != want {
		t.Fatalf("ProposerCard = %+v, %v, want Mia with her photo", got, err)
	}
	failing := app.NewProposalReads(
		d.pool,
		d.swaps,
		d.names,
		d.assets,
		&cardBook{err: errs.New(errs.CodeInternal, "test")},
		&returnBook{},
		&replyBook{},
	)
	if _, err := failing.ProposerCard(t.Context(), who); err == nil {
		t.Fatal("ProposerCard with a failing read err = nil")
	}
}
