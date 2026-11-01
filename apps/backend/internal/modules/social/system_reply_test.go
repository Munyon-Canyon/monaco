package social_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func systemFillAt() time.Time { return time.Date(2026, 10, 22, 9, 41, 0, 0, time.UTC) }

type systemRig struct {
	commentRoutes
	treasury *fakes.Treasury
	tallies  *fakeTallies
	thread   adapters.Thread
	proposal uuid.UUID
	item     uuid.UUID
	unit     uint64
}

func newSystemRig(t *testing.T) systemRig {
	t.Helper()
	f := newCommentRoutes(t)
	r := systemRig{
		commentRoutes: f,
		treasury:      fakes.NewTreasury(),
		tallies:       &fakeTallies{votes: map[ids.ProposalID]app.Tally{}},
	}
	assets := social.NewAssets(marketfake.NewCatalog(marketfake.AAPLx()))
	card, err := assets.AssetByMint(t.Context(), string(marketfake.AAPLx().Mint.Address()))
	if err != nil {
		t.Fatal(err)
	}
	r.unit = 1
	for range card.Decimals {
		r.unit *= 10
	}
	r.proposal, r.item = f.proposalItem(t)
	r.tallies.votes[ids.ProposalIDFrom(r.proposal)] = app.Tally{Kind: "buy"}
	r.thread = adapters.Thread{
		Hints:    f.hints,
		Assets:   assets,
		Holdings: social.PositionHoldings{Positions: r.treasury, Swaps: r.treasury},
		Tallies:  r.tallies,
	}
	return r
}

func (r systemRig) hold(hundredths uint64) {
	units := hundredths * (r.unit / 100)
	r.treasury.SetPositions(ids.CabalIDFrom(r.cabal.ID.UUID()), []treasury.Position{
		{Mint: chain.SolanaAddress("other"), Units: money.NewBaseUnits(7, 0)},
		{Mint: marketfake.AAPLx().Mint.Address(), Units: money.NewBaseUnits(units, r.decimals())},
	})
}

func (r systemRig) decimals() uint8 {
	var d uint8
	for u := r.unit; u > 1; u /= 10 {
		d++
	}
	return d
}

func (r systemRig) confirmed(action string, hundredths uint64, source string) events.TradeConfirmed {
	amount := hundredths * (r.unit / 100)
	e := events.TradeConfirmed{
		V:           1,
		SwapID:      r.gen.NewV7(),
		CabalID:     r.cabal.ID.UUID(),
		Source:      events.TradeSource{Kind: source, ID: r.proposal},
		Action:      action,
		Symbol:      "AAPLx",
		ConfirmedAt: systemFillAt(),
	}
	if action == "sell" {
		e.InMint, e.InAmount = marketfake.AAPLx().Mint.Address(), amount
	} else {
		e.OutMint, e.OutAmount = marketfake.AAPLx().Mint.Address(), amount
	}
	return e
}

func (r systemRig) deliver(t *testing.T, fn func(context.Context, db.Tx) error) error {
	t.Helper()
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(r.gen.NewV7()))
	return db.New(r.pool, r.gen, r.clock).Do(ctx, fn)
}

func (r systemRig) confirm(t *testing.T, e events.TradeConfirmed) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		fill, err := r.thread.FetchConfirmed(ctx, e)
		if err != nil {
			return err
		}
		return r.thread.ApplyConfirmed(ctx, tx, e, fill, r.clock.Now())
	})
}

func (r systemRig) fail(t *testing.T) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		e := events.ProposalFailed{V: 1, ProposalID: r.proposal, CabalID: r.cabal.ID.UUID()}
		closed, err := r.thread.FetchClosed(ctx, e)
		if err != nil {
			return err
		}
		return r.thread.ApplyFailed(ctx, tx, e, closed, systemFillAt())
	})
}

func (r systemRig) expire(t *testing.T) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		e := events.ProposalExpired{V: 1, ProposalID: r.proposal, CabalID: r.cabal.ID.UUID()}
		closed, err := r.thread.FetchExpired(ctx, e)
		if err != nil {
			return err
		}
		return r.thread.ApplyExpired(ctx, tx, e, closed, systemFillAt())
	})
}

func (r systemRig) tradeFailed(t *testing.T, action, source string) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		return r.thread.TradeFailed(ctx, tx, events.TradeFailed{
			V: 1, CabalID: r.cabal.ID.UUID(), Source: events.TradeSource{Kind: source, ID: r.proposal}, Action: action,
			Symbol: "AAPLx",
		}, r.clock.Now())
	})
}

func (r systemRig) replies(t *testing.T) []string {
	t.Helper()
	rows, err := r.pool.Query(
		t.Context(),
		`SELECT body FROM feed_comments WHERE feed_object_id = $1 AND author_kind = 'system' ORDER BY created_at`,
		r.item,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		out = append(out, body)
	}
	return out
}

func (r systemRig) wantReplies(t *testing.T, want ...string) {
	t.Helper()
	if got := r.replies(t); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("system replies = %q, want %q", got, want)
	}
}

func (r systemRig) rows(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r systemRig) commentCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(), `SELECT comment_count FROM feed_objects WHERE id = $1`, r.item).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSystemReply_FilledBuyWritesOneReplyTimedAtTheFillEvenWhenDeliveredTwice(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	r.hold(3982)
	e := r.confirmed("buy", 182, "proposal")
	for range 2 {
		if err := r.confirm(t, e); err != nil {
			t.Fatal(err)
		}
	}
	r.wantReplies(t, "Passed and filled. 38 → 39.82 sh AAPLx in the treasury.")
	var at time.Time
	if err := r.pool.QueryRow(
		t.Context(),
		`SELECT created_at FROM feed_comments WHERE feed_object_id = $1 AND author_kind = 'system'`,
		r.item,
	).Scan(&at); err != nil || !at.Equal(systemFillAt()) {
		t.Fatalf("created_at = %v, %v, want %v", at, err, systemFillAt())
	}
	if n := r.commentCount(t); n != 1 {
		t.Fatalf("comment_count = %d, want 1", n)
	}
	if got := r.rows(t, `SELECT count(*) FROM notifications`); got != 0 {
		t.Fatalf("%d notifications, want 0", got)
	}
	if got := r.rows(t, `SELECT count(*) FROM events WHERE type = 'comment.created'`); got != 0 {
		t.Fatalf("%d comment.created events, want 0", got)
	}
}

func TestSystemReply_FilledBuyFromNothingOmitsTheBefore(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	r.hold(42)
	if err := r.confirm(t, r.confirmed("buy", 42, "proposal")); err != nil {
		t.Fatal(err)
	}
	r.wantReplies(t, "Passed and filled. 0.42 sh AAPLx in the treasury.")
}

func TestSystemReply_FilledBuyBeforeTheHoldingLandsReadsAsFromNothing(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	if err := r.confirm(t, r.confirmed("buy", 200, "proposal")); err != nil {
		t.Fatal(err)
	}
	r.wantReplies(t, "Passed and filled. 0 sh AAPLx in the treasury.")
}

func TestSystemReply_FilledSellShowsTheSmallerNumberAfter(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	r.hold(400)
	if err := r.confirm(t, r.confirmed("sell", 650, "proposal")); err != nil {
		t.Fatal(err)
	}
	r.wantReplies(t, "Passed and filled. 10.5 → 4 sh AAPLx in the treasury.")
}

func TestSystemReply_ATradeNotFromAProposalWritesNothing(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	if err := r.confirm(t, r.confirmed("buy", 100, "order")); err != nil {
		t.Fatal(err)
	}
	if err := r.tradeFailed(t, "buy", "order"); err != nil {
		t.Fatal(err)
	}
	r.wantReplies(t)
}

func (r systemRig) wantClosedOnce(t *testing.T, action feed.Action, want string) {
	t.Helper()
	if _, err := r.pool.Exec(
		t.Context(),
		`UPDATE feed_objects SET payload = jsonb_set(payload, '{action}', to_jsonb($2::text))
		WHERE id = $1`,
		r.item,
		string(action),
	); err != nil {
		t.Fatal(err)
	}
	for _, step := range []func(*testing.T) error{r.fail, r.expire, r.fail} {
		if err := step(t); err != nil {
			t.Fatal(err)
		}
	}
	r.wantReplies(t, want)
	if n := r.commentCount(t); n != 1 {
		t.Fatalf("comment_count = %d, want 1", n)
	}
}

func TestSystemReply_ClosedBuyWritesOnceWhetherTheVoteFailedOrExpired(t *testing.T) {
	t.Parallel()
	newSystemRig(t).wantClosedOnce(t, feed.ActionBuy, "Closed without passing. Nothing was bought.")
}

func TestSystemReply_ClosedSellWritesOnceWhetherTheVoteFailedOrExpired(t *testing.T) {
	t.Parallel()
	newSystemRig(t).wantClosedOnce(t, feed.ActionSell, "Closed without passing. Nothing was sold.")
}

func TestSystemReply_ARuleProposalWritesNothing(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	r.tallies.votes[ids.ProposalIDFrom(r.proposal)] = app.Tally{Kind: "rule"}
	if err := r.fail(t); err != nil {
		t.Fatal(err)
	}
	r.wantReplies(t)
}

func TestSystemReply_ATradeThatFailedAfterThePassWritesItsOwnLine(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	for range 2 {
		if err := r.tradeFailed(t, "sell", "proposal"); err != nil {
			t.Fatal(err)
		}
	}
	r.wantReplies(t, "Passed, but the trade didn't go through. Nothing was sold.")
}

func TestSystemReply_AMissingFeedItemNaksRetryably(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	r.proposal = r.gen.NewV7()
	r.tallies.votes[ids.ProposalIDFrom(r.proposal)] = app.Tally{Kind: "buy"}
	for name, err := range map[string]error{
		"confirmed": r.confirm(t, r.confirmed("buy", 100, "proposal")),
		"failed":    r.fail(t),
		"trade":     r.tradeFailed(t, "buy", "proposal"),
	} {
		if errs.CodeOf(err) != errs.CodeFeedItemPending {
			t.Errorf("%s: err = %v, want feed_item_pending", name, err)
		}
	}
}

type brokenHoldings struct{}

func (brokenHoldings) Posted(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func (brokenHoldings) Holding(context.Context, ids.CabalID, chain.SolanaAddress) (uint64, error) {
	return 0, errs.New(errs.CodeInternal, "test.positions")
}

type brokenPositions struct{}

func (brokenPositions) SwapPosted(context.Context, uuid.UUID) (bool, error) {
	return false, errs.New(errs.CodeInternal, "test.posted")
}

func (brokenPositions) SolanaPositions(context.Context, ids.CabalID) ([]treasury.Position, error) {
	return nil, errs.New(errs.CodeInternal, "test.positions")
}

func TestSystemReply_ReadFailuresAbortBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	e := r.confirmed("buy", 100, "proposal")
	r.thread.Holdings = brokenHoldings{}
	if err := r.confirm(t, e); err == nil {
		t.Fatal("confirm with broken holdings succeeded")
	}
	if _, err := (social.PositionHoldings{Positions: brokenPositions{}, Swaps: brokenPositions{}}).Holding(
		t.Context(),
		ids.CabalIDFrom(r.cabal.ID.UUID()),
		e.OutMint,
	); err == nil {
		t.Fatal("holding with broken positions succeeded")
	}
	e.OutMint = chain.SolanaAddress("unknown")
	if err := r.confirm(t, e); err == nil {
		t.Fatal("confirm with an unknown mint succeeded")
	}
	r.tallies.votes = map[ids.ProposalID]app.Tally{}
	if err := r.fail(t); errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("fail without a tally = %v, want proposal_not_found", err)
	}
	r.wantReplies(t)
}

func (r systemRig) wantRefused(t *testing.T, what string, err error) {
	t.Helper()
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("%s: err = %v, want invalid_input", what, err)
	}
	var reason string
	for _, attr := range errs.Detail(err) {
		if attr.Key == "reason" {
			reason = attr.Value.String()
		}
	}
	if reason != domain.ReasonSystemReply {
		t.Fatalf("%s: reason = %q, want %q (attrs %v)", what, reason, domain.ReasonSystemReply, errs.Detail(err))
	}
}

func (r systemRig) systemReply(t *testing.T) api.Comment {
	t.Helper()
	if err := r.fail(t); err != nil {
		t.Fatal(err)
	}
	page := r.read(t, r.cabal.Members[0].ID, r.item, api.GetFeedCommentsParams{})
	if len(page.Items) == 0 {
		t.Fatal("no threads")
	}
	return page.Items[0].Comment
}

func TestSystemReply_ListsFirstOnTheFirstPageOnly(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	kai := r.cabal.Members[0].ID
	first := r.say(t, r.item, kai, "first", nil)
	second := r.say(t, r.item, kai, "second", nil)
	if err := r.fail(t); err != nil {
		t.Fatal(err)
	}
	page := r.read(t, kai, r.item, api.GetFeedCommentsParams{Limit: ptr(1)})
	if len(page.Items) != 2 {
		t.Fatalf("first page has %d threads, want the system reply and one member reply", len(page.Items))
	}
	wantMonaco(t, page.Items[0])
	if page.Items[1].Comment.Id != first.Id || page.NextCursor == nil {
		t.Fatalf(
			"second item = %+v next = %v, want the first member reply and a cursor",
			page.Items[1].Comment,
			page.NextCursor,
		)
	}
	next := r.read(t, kai, r.item, api.GetFeedCommentsParams{Limit: ptr(1), Cursor: page.NextCursor})
	if len(next.Items) != 1 || next.Items[0].Comment.Id != second.Id {
		t.Fatalf("next page = %+v, want only the second member reply", next.Items)
	}
}

func TestSystemReply_TakesNoReplyReactionOrReport(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	kai, lee := r.cabal.Members[0].ID, r.cabal.Members[1].ID
	monaco := r.systemReply(t)
	_, err := r.post(t, r.item, kai, &api.CreateCommentRequest{Body: ptr("hi"), ParentCommentId: &monaco.Id})
	r.wantRefused(t, "reply", err)
	handlers := app.ReactionDeps{UoW: r.uow, Reads: r.pool, Members: membersOn(r.pool), Clock: r.clock, Hints: r.hints}
	_, err = app.NewReactCommentHandler(handlers).Handle(
		asUser(t.Context(), lee), app.ReactComment{CommentID: monaco.Id, Viewer: lee, Emoji: "👍"})
	r.wantRefused(t, "react", err)
	report := app.NewCreateReportHandler(app.CreateReportDeps{
		UoW:     r.uow,
		Reads:   r.pool,
		Users:   r.users,
		Members: cabal.New(module.Deps{Pool: r.pool}).Queries(),
		IDs:     r.gen,
		Clock:   r.clock,
	})
	_, err = report.Handle(asUser(t.Context(), lee), app.CreateReport{
		Reporter: lee, Kind: domain.ReportComment, TargetID: monaco.Id, Reason: domain.ReasonAbuse,
	})
	r.wantRefused(t, "report", err)
	if got := r.rows(t, `SELECT count(*) FROM notifications`); got != 0 {
		t.Fatalf("%d notifications, want 0", got)
	}
}

func TestSystemReply_IsNotCountedAsAReply(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	r.say(t, r.item, r.cabal.Members[0].ID, "one", nil)
	if err := r.fail(t); err != nil {
		t.Fatal(err)
	}
	counts, err := social.New(module.Deps{Pool: r.pool}).
		ReplyCounts(t.Context(), []ids.ProposalID{ids.ProposalIDFrom(r.proposal)})
	if err != nil || counts[ids.ProposalIDFrom(r.proposal)] != 1 {
		t.Fatalf("ReplyCounts = %v, %v, want 1", counts, err)
	}
}

func TestSystemReply_TheTableRejectsAUserRowWithoutAuthorAndASystemRowWithOne(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	for name, query := range map[string]string{
		"user without author": `INSERT INTO feed_comments (id, feed_object_id, author_kind, body, created_at)
			VALUES (gen_random_uuid(), $1, 'user', 'x', now())`,
		"system with author": `INSERT INTO feed_comments (id, feed_object_id, author_id, author_kind, system_outcome, body, created_at)
			VALUES (gen_random_uuid(), $1, gen_random_uuid(), 'system', 'closed', 'x', now())`,
	} {
		if _, err := r.pool.Exec(t.Context(), query, r.item); err == nil {
			t.Errorf("%s: insert succeeded, want a check violation", name)
		}
	}
}

func wantMonaco(t *testing.T, thread api.CommentThread) {
	t.Helper()
	monaco := thread.Comment
	if monaco.Author.Kind != api.CommentAuthorKindSystem || monaco.Author.DisplayName != "Monaco" ||
		monaco.Author.Id != uuid.Nil || monaco.Author.Handle != nil || monaco.Author.PhotoUrl != nil {
		t.Fatalf("system author = %+v", monaco.Author)
	}
	if monaco.IsMine || monaco.ReplyTo != nil || monaco.ParentCommentId != nil || len(thread.Replies) != 0 ||
		monaco.BodyDisplay != "Closed without passing. Nothing was bought." || !monaco.CreatedAt.Equal(systemFillAt()) {
		t.Fatalf("system reply = %+v", monaco)
	}
}

func TestSystemReply_ABrokenPayloadOrTableFailsWithoutAReply(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	if _, err := r.pool.Exec(
		t.Context(),
		`UPDATE feed_objects SET payload = '"broken"' WHERE id = $1`,
		r.item,
	); err != nil {
		t.Fatal(err)
	}
	if err := r.fail(t); err == nil {
		t.Fatal("close with a broken payload succeeded")
	}
	if _, err := r.pool.Exec(
		t.Context(),
		`UPDATE feed_objects SET payload = '{"action":"buy"}' WHERE id = $1`,
		r.item,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(
		t.Context(),
		`ALTER TABLE feed_objects RENAME COLUMN comment_count TO comment_total`,
	); err != nil {
		t.Fatal(err)
	}
	if err := r.fail(t); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("close without a counter = %v, want internal", err)
	}
	if _, err := r.pool.Exec(t.Context(), `ALTER TABLE feed_comments RENAME TO feed_comments_moved`); err != nil {
		t.Fatal(err)
	}
	if err := r.fail(t); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("close without a table = %v, want internal", err)
	}
}

func TestSystemReply_ModuleTakesInjectedHoldings(t *testing.T) {
	t.Parallel()
	m := social.New(module.Deps{}, social.WithHoldings(brokenHoldings{}))
	if len(m.Consumers()) != 6 {
		t.Fatalf("consumers = %d, want 6", len(m.Consumers()))
	}
}

func TestSystemReply_AFillTreasuryHasNotPostedNaksRetryablyUntilItDoes(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	r.hold(3982)
	e := r.confirmed("buy", 182, "proposal")
	r.treasury.SetSwapUnposted(e.SwapID)
	err := r.confirm(t, e)
	if errs.CodeOf(err) != errs.CodeFeedItemPending || !errs.Retryable(errs.CodeOf(err)) {
		t.Fatalf("confirm before the post = %v, want a retryable feed_item_pending", err)
	}
	r.wantReplies(t)
	if _, err := (social.PositionHoldings{Positions: brokenPositions{}, Swaps: brokenPositions{}}).Posted(
		t.Context(),
		e.SwapID,
	); err == nil {
		t.Fatal("posted with a broken reader succeeded")
	}
	r.thread.Holdings = social.PositionHoldings{Positions: brokenPositions{}, Swaps: brokenPositions{}}
	if err := r.confirm(t, e); err == nil {
		t.Fatal("confirm with a broken posted check succeeded")
	}
}

func TestSystemReply_ListingFailsWhenTheSystemRepliesCannotBeRead(t *testing.T) {
	t.Parallel()
	r := newSystemRig(t)
	if _, err := r.pool.Exec(t.Context(), `INSERT INTO feed_comments
		(id, feed_object_id, author_kind, system_outcome, body, created_at)
		VALUES ($1, $2, 'system', 'closed', 'x', 'infinity')`, r.gen.NewV7(), r.item); err != nil {
		t.Fatal(err)
	}
	_, err := app.ListComments(t.Context(), r.pool, app.CommentsQuery{FeedObjectID: r.item, Limit: 5})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("ListComments = %v, want internal", err)
	}
}
