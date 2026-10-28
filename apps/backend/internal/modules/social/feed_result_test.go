package social_test

import (
	"context"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type resultRow struct {
	ID, RefID, ActorID, CabalID uuid.UUID
	RefType, Title, Body        string
	Status                      string
	Payload                     feed.Payload
}

func (r proposalRig) results(t *testing.T) []resultRow {
	t.Helper()
	rows, err := r.pool.Query(t.Context(), `SELECT id, ref_id, actor_id, cabal_id, ref_type, title,
		coalesce(body, ''), coalesce(status, ''), payload FROM feed_objects WHERE kind = 'result' ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []resultRow
	for rows.Next() {
		var (
			row resultRow
			raw []byte
		)
		err := rows.Scan(&row.ID, &row.RefID, &row.ActorID, &row.CabalID, &row.RefType, &row.Title, &row.Body,
			&row.Status, &raw)
		if err != nil {
			t.Fatal(err)
		}
		if row.Payload, err = feed.ParsePayload(raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r proposalRig) onlyResult(t *testing.T) resultRow {
	t.Helper()
	got := r.results(t)
	if len(got) != 1 {
		t.Fatalf("%d result rows, want 1", len(got))
	}
	return got[0]
}

func (r proposalRig) count(t *testing.T, kind string) int {
	t.Helper()
	var n int
	err := r.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_objects WHERE kind = $1`, kind).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (r proposalRig) createWith(t *testing.T, change func(*events.ProposalCreated)) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		e := r.created()
		change(&e)
		card, err := r.feed.FetchProposal(ctx, e)
		if err != nil {
			return err
		}
		return r.feed.ApplyProposal(ctx, tx, e, card, r.clock.Now())
	})
}

func (r proposalRig) withPreIPO() proposalRig {
	r.feed.Assets = social.NewAssets(marketfake.NewCatalog(marketfake.AAPLx(), marketfake.SPACEX()))
	return r
}

func (r proposalRig) mustCreate(t *testing.T, change func(*events.ProposalCreated)) {
	t.Helper()
	if err := r.createWith(t, change); err != nil {
		t.Fatal(err)
	}
}

func asSale(e *events.ProposalCreated) {
	e.Kind, e.USDCMicros, e.TokenAmount, e.QuoteOutAmount = "sell", money.Micros{}, 7, 480_000_000
}

func TestFeedResult_aPassedBuyMakesOneCabalHeadedResult(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	got := r.onlyResult(t)
	if got.RefID != r.proposal || got.RefType != "proposals" || got.ActorID != r.bob.UUID() || got.CabalID != r.cabal {
		t.Fatalf("result = %+v, want proposal %s by %s in %s", got, r.proposal, r.bob, r.cabal)
	}
	r.wantPassedBuyFacts(t, got.Payload)
	if want := "Alpha passed Buy Apple $500 4–1"; got.Title != want || got.Body != "buying" || got.Status != "passed" {
		t.Fatalf("title %q body %q status %q, want %q buying passed", got.Title, got.Body, got.Status, want)
	}
	if n := r.count(t, "threshold"); n != 0 {
		t.Fatalf("%d threshold rows, want none", n)
	}
	if got := r.row(t).Status; got != "passed" {
		t.Fatalf("proposal status = %q, want passed", got)
	}
}

func (r proposalRig) wantPassedBuyFacts(t *testing.T, p feed.Payload) {
	t.Helper()
	if p.Outcome != feed.OutcomePassed || p.Yes != 4 || p.No != 1 || p.Voters != 6 || p.Needed != 4 {
		t.Errorf("payload = %+v, want a pass 4-1 of 6 needing 4", p)
	}
	if p.Shape != feed.ShapeCabal || p.CallerID != r.bob.UUID() || p.CallerName != "bob" ||
		p.AssetKind != feed.AssetKindEquity {
		t.Errorf("payload = %+v, want cabal-headed, called by bob, equity", p)
	}
}

func TestFeedResult_neededIsTheTalliesPortsNeverTheProposalsPriceThreshold(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	_, err := r.pool.Exec(t.Context(), `UPDATE feed_objects SET payload = payload || '{"threshold_bps": 9999}'
		WHERE kind = 'proposal'`)
	if err != nil {
		t.Fatal(err)
	}
	r.tally(app.Tally{Yes: 4, No: 1, Voters: 6, Needed: 4, Kind: "buy"})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	if got := r.onlyResult(t).Payload.Needed; got != 4 {
		t.Fatalf("needed = %d, want the port's 4", got)
	}
}

func TestFeedResult_aPassedSaleAndAPassedPreIPOBuyAreMemberHeaded(t *testing.T) {
	t.Parallel()
	t.Run("sale", func(t *testing.T) {
		t.Parallel()
		r := newProposalRig(t)
		r.mustCreate(t, asSale)
		r.tally(app.Tally{Yes: 3, No: 1, Voters: 4, Needed: 3, Kind: "sell"})
		if err := r.passed(t); err != nil {
			t.Fatal(err)
		}
		got := r.onlyResult(t)
		if got.Payload.Shape != feed.ShapeMember || got.Payload.CallerName != "bob" || got.Body != "selling" ||
			got.Title != "bob's sale of $480 of Apple in Alpha passed 3–1" {
			t.Fatalf("result = %+v", got)
		}
	})
	t.Run("pre-ipo buy", func(t *testing.T) {
		t.Parallel()
		r := newProposalRig(t).withPreIPO()
		r.mustCreate(t, func(e *events.ProposalCreated) {
			e.Symbol, e.Mint = "SPACEX", marketfake.SPACEX().Mint.Address()
		})
		if err := r.passed(t); err != nil {
			t.Fatal(err)
		}
		got := r.onlyResult(t)
		if got.Payload.Shape != feed.ShapeMember || got.Payload.AssetKind != feed.AssetKindPreIPO ||
			got.Title != "bob's buy of $500 of SpaceX in Alpha passed" {
			t.Fatalf("result = %+v", got)
		}
	})
}

func TestFeedResult_aFailedOrExpiredVoteMakesACabalHeadedResultWithNoCaller(t *testing.T) {
	t.Parallel()
	closers := map[string]struct {
		close  func(proposalRig, *testing.T) error
		status string
	}{
		"failed":  {proposalRig.failed, "failed"},
		"expired": {proposalRig.expired, "expired"},
	}
	for name, c := range closers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t)
			r.mustCreate(t, asSale)
			r.tally(app.Tally{Yes: 2, No: 2, Voters: 5, Needed: 3, Kind: "sell"})
			if err := c.close(r, t); err != nil {
				t.Fatal(err)
			}
			wantFailedSale(t, r.onlyResult(t), c.status)
			if got := r.row(t).Status; got != c.status {
				t.Fatalf("proposal status = %q, want %s", got, c.status)
			}
		})
	}
}

func wantFailedSale(t *testing.T, got resultRow, status string) {
	t.Helper()
	p := got.Payload
	if p.Outcome != feed.OutcomeFailed || p.Yes != 2 || p.No != 2 || p.Voters != 5 || p.Needed != 3 ||
		p.Shape != feed.ShapeCabal || p.CallerID != uuid.Nil || p.CallerName != "" {
		t.Errorf("payload = %+v, want a failed cabal-headed 2-2 of 5 with no caller", p)
	}
	want := "Alpha failed Sell Apple $480 2–2"
	if got.Title != want || got.Body != "no trade" || got.Status != status {
		t.Errorf("title %q body %q status %q, want %q no trade %s", got.Title, got.Body, got.Status, want, status)
	}
}

func TestFeedResult_aRedeliveredOrReopenedCloseKeepsOneRowAndItsFillKeys(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	for range 3 {
		if err := r.passed(t); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.onlyResult(t); got.Payload.Outcome != feed.OutcomePassed {
		t.Fatalf("outcome = %q after three passes, want passed", got.Payload.Outcome)
	}
	r.setFillKeys(t)
	r.tally(app.Tally{Yes: 1, No: 3, Voters: 5, Needed: 3, Kind: "buy"})
	if err := r.failed(t); err != nil {
		t.Fatal(err)
	}
	r.tally(app.Tally{Yes: 4, No: 1, Voters: 5, Needed: 3, Kind: "buy"})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	p := r.onlyResult(t).Payload
	if p.Outcome != feed.OutcomePassed || p.Yes != 4 || p.CallerID != r.bob.UUID() {
		t.Errorf("payload = %+v, want the second close's facts", p)
	}
	wantFillKeys(t, p)
}

func (r proposalRig) setFillKeys(t *testing.T) {
	t.Helper()
	_, err := r.pool.Exec(t.Context(), `UPDATE feed_objects SET payload = payload ||
		'{"fills": [{"n": 1}], "failures": ["x"], "expected_fills": 2, "fill_state": "partial"}'
		WHERE kind = 'result'`)
	if err != nil {
		t.Fatal(err)
	}
}

func wantFillKeys(t *testing.T, p feed.Payload) {
	t.Helper()
	if string(p.Fills) != `[{"n": 1}]` || string(p.Failures) != `["x"]` {
		t.Errorf("fills %s failures %s, want the first close's keys", p.Fills, p.Failures)
	}
	if p.ExpectedFills != 2 || p.FillState != "partial" {
		t.Errorf("expected fills %d state %q, want 2 partial", p.ExpectedFills, p.FillState)
	}
}

func TestFeedResult_everyWriteRendersTheTitleAndSearchFindsIt(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	closers := []func(proposalRig, *testing.T) error{proposalRig.failed, proposalRig.passed}
	for _, close := range closers {
		if err := close(r, t); err != nil {
			t.Fatal(err)
		}
		got := r.onlyResult(t)
		if want := feed.RenderTitle(feed.KindResult, got.Payload); got.Title != want {
			t.Fatalf("title = %q, want %q", got.Title, want)
		}
		if want := feed.RenderDetail(feed.KindResult, got.Payload); got.Body != want {
			t.Fatalf("body = %q, want %q", got.Body, want)
		}
		var found int
		err := r.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_objects
			WHERE kind = 'result' AND search @@ plainto_tsquery('english', $1)`,
			string(got.Payload.Outcome)+" buy apple").Scan(&found)
		if err != nil || found != 1 {
			t.Fatalf("search for %q found %d rows, %v, want 1", got.Title, found, err)
		}
	}
}

func TestFeedResult_aRuleProposalWritesNoProposalAndNoResult(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.tally(app.Tally{Yes: 4, No: 1, Voters: 6, Needed: 4, Kind: "rule"})
	r.mustCreate(t, func(e *events.ProposalCreated) { e.Kind = "rule" })
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	if err := r.failed(t); err != nil {
		t.Fatal(err)
	}
	if err := r.expired(t); err != nil {
		t.Fatal(err)
	}
	if n := r.count(t, "proposal") + r.count(t, "result"); n != 0 {
		t.Fatalf("%d proposal and result rows for a rule proposal, want none", n)
	}
}

func TestFeedResult_aCloseBeforeTheProposalItemRetriesAndWritesNothing(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if got := errs.CodeOf(r.passed(t)); got != errs.CodeFeedItemPending {
		t.Fatalf("pass before the proposal item = %s, want %s", got, errs.CodeFeedItemPending)
	}
	if n := r.count(t, "result"); n != 0 {
		t.Fatalf("%d result rows, want none", n)
	}
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	r.onlyResult(t)
}

func TestFeedResult_aCloseReturnsEachFailure(t *testing.T) {
	t.Parallel()
	t.Run("no tally", func(t *testing.T) {
		t.Parallel()
		r := newProposalRig(t)
		r.mustCreate(t, func(*events.ProposalCreated) {})
		delete(r.tallies.votes, ids.ProposalIDFrom(r.proposal))
		if got := errs.CodeOf(r.passed(t)); got != errs.CodeProposalNotFound {
			t.Fatalf("pass without a tally = %s, want %s", got, errs.CodeProposalNotFound)
		}
	})
	t.Run("tallies down", func(t *testing.T) {
		t.Parallel()
		r := newProposalRig(t)
		r.mustCreate(t, func(*events.ProposalCreated) {})
		r.tallies.err = errs.New(errs.CodeDBUnavailable, "test")
		if got := errs.CodeOf(r.passed(t)); got != errs.CodeDBUnavailable {
			t.Fatalf("pass with tallies down = %s, want %s", got, errs.CodeDBUnavailable)
		}
	})
	for name, ddl := range map[string]string{
		"source lookup": "ALTER TABLE feed_objects RENAME COLUMN body TO thesis",
		"payload":       `UPDATE feed_objects SET payload = '{"action": 5}' WHERE kind = 'proposal'`,
		"write":         "ALTER TABLE feed_objects ADD CONSTRAINT no_result CHECK (kind <> 'result') NOT VALID",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t)
			r.mustCreate(t, func(*events.ProposalCreated) {})
			if _, err := r.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			if err := r.passed(t); err == nil {
				t.Fatal("pass = nil, want an error")
			}
		})
	}
}

func TestFeedResult_aClosePublishesOneFeedHint(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	sub := testkit.SubscribeCore(t, r.conn, hintSubject)
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	wantHints(t, r.conn.Conn, sub, 1)
}

func TestGetFeed_aResultCarriesItsOutcomeShapeCallerAndFinalTally(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t).withPreIPO()
	r.mustCreate(t, func(e *events.ProposalCreated) {
		e.Symbol, e.Mint = "SPACEX", marketfake.SPACEX().Mint.Address()
	})
	feedPage := func() map[string]api.FeedItem {
		f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
		routes := f.routesOptions(social.WithUsers(r.users), social.WithTallies(r.tallies))
		res, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{Params: api.GetFeedParams{}})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]api.FeedItem{}
		for _, item := range res.(api.GetFeed200JSONResponse).Items {
			out[item.Kind] = item
		}
		return out
	}
	open := feedPage()
	if got := open["proposal"]; got.AssetKind == nil || *got.AssetKind != api.FeedItemAssetKindPreIpo {
		t.Fatalf("pre-IPO proposal asset_kind = %v, want pre_ipo", got.AssetKind)
	}
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	r.tally(app.Tally{Yes: 0, No: 0, Voters: 9, Needed: 9, Kind: "buy"})
	got, ok := feedPage()["result"]
	if !ok {
		t.Fatal("no result item in the feed")
	}
	r.wantResultItem(t, got)
}

func (r proposalRig) wantResultItem(t *testing.T, got api.FeedItem) {
	t.Helper()
	wantResultKinds(t, got)
	if got.Caller == nil || got.Caller.Id != r.bob.UUID() || got.Caller.Name != "bob" {
		t.Errorf("result item caller = %+v, want bob", got.Caller)
	}
	if got.Tally == nil || *got.Tally != (api.FeedTally{Yes: 4, No: 1, Voters: 6, Needed: 4}) {
		t.Errorf("result item tally = %+v, want the stored 4 1 6 4", got.Tally)
	}
	if got.ProposalId == nil || *got.ProposalId != r.proposal || got.ExpiresAt != nil {
		t.Errorf("result item proposal %v expires %v, want %s and none", got.ProposalId, got.ExpiresAt, r.proposal)
	}
}

func wantResultKinds(t *testing.T, got api.FeedItem) {
	t.Helper()
	if got.Outcome == nil || *got.Outcome != api.FeedItemOutcomePassed {
		t.Errorf("result item outcome = %v, want passed", got.Outcome)
	}
	if got.Shape == nil || *got.Shape != api.FeedItemShapeMember {
		t.Errorf("result item shape = %v, want member", got.Shape)
	}
	if got.AssetKind == nil || *got.AssetKind != api.FeedItemAssetKindPreIpo {
		t.Errorf("result item asset_kind = %v, want pre_ipo", got.AssetKind)
	}
}

func TestFeedConsumerQueries_onlyTheProjectionWritesResultRows(t *testing.T) {
	t.Parallel()
	queries := os.DirFS("../../../queries/social")
	files, err := fs.Glob(queries, "*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("queries = %v, %v", files, err)
	}
	named := regexp.MustCompile(`(?m)^-- name: (\w+)`)
	allowed := map[string]bool{"LockFeedResult": true, "UpsertFeedResult": true}
	for _, file := range files {
		raw, err := fs.ReadFile(queries, file)
		if err != nil {
			t.Fatal(err)
		}
		bodies := named.Split(string(raw), -1)
		for i, m := range named.FindAllStringSubmatch(string(raw), -1) {
			if strings.Contains(bodies[i+1], "'result'") && !allowed[m[1]] {
				t.Errorf(
					"%s: query %s names the result kind, but only the projection may write result rows",
					file,
					m[1],
				)
			}
		}
	}
}

func TestFeedResult_applyClosedWithoutAProposalItemRetries(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	err := r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		tally := app.Tally{Yes: 4, No: 1, Voters: 6, Needed: 4}
		return r.feed.ApplyClosed(ctx, tx, r.proposal, feed.StatusPassed, feed.OutcomePassed, tally, r.clock.Now())
	})
	if got := errs.CodeOf(err); got != errs.CodeFeedItemPending {
		t.Fatalf("ApplyClosed without a proposal item = %s, want %s", got, errs.CodeFeedItemPending)
	}
}

func TestFeedResult_aCorruptStoredResultFailsTheNextWrite(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	_, err := r.pool.Exec(t.Context(), `UPDATE feed_objects SET payload = '{"action": 5}' WHERE kind = 'result'`)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.passed(t); err == nil {
		t.Fatal("pass over a corrupt result = nil, want an error")
	}
}

func TestFeedResult_aWriteWaitsForTheRowLockAndFailsWhenItCannotGetIt(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	var holder errgroup.Group
	holder.Go(func() error {
		return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
			_, err := tx.Queries().Exec(ctx, `SELECT 1 FROM feed_objects WHERE kind = 'result' FOR UPDATE`)
			close(held)
			<-release
			return err
		})
	})
	<-held
	t.Cleanup(func() {
		close(release)
		_ = holder.Wait()
	})
	ctx, cancel := context.WithTimeout(
		observability.WithEventID(t.Context(), ids.EventIDFrom(r.gen.NewV7())),
		time.Second,
	)
	defer cancel()
	err := db.New(r.pool, r.gen, r.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		e := events.ProposalPassed{V: 1, ProposalID: r.proposal, CabalID: r.cabal}
		closed, err := r.feed.FetchPassed(ctx, e)
		if err != nil {
			return err
		}
		return r.feed.ApplyPassed(ctx, tx, e, closed, r.clock.Now())
	})
	if err == nil {
		t.Fatal("pass against a held row lock = nil, want an error")
	}
}
