package social_test

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type listedAssets struct{}

func (listedAssets) AssetByMint(context.Context, string) (app.AssetCard, error) {
	return app.AssetCard{ID: marketfake.AAPLx().ID.UUID(), Symbol: "AAPLx", Decimals: 1}, nil
}

func (f chatFixture) settledMoments(votes map[ids.ProposalID]app.Tally) adapters.ChatMoments {
	return adapters.ChatMoments{
		Assets:  listedAssets{},
		Tallies: &fakeTallies{votes: votes},
		Reads:   f.pool,
	}
}

func (f chatFixture) seedProposalFeed(t *testing.T, proposal uuid.UUID, actor ids.UserID, p feed.Payload) {
	t.Helper()
	p.Status, p.AssetKind = "open", feed.AssetKindEquity
	_, err := f.pool.Exec(t.Context(), `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, symbol, title, payload, status, created_at, updated_at)
		VALUES ($1, 'proposal', 'proposals', $2, $3, 'Alpha', $4, 'NVDA', 't', $5, 'open', now(), now())`,
		f.deps.IDs.NewV7(), proposal, f.cabal.ID.UUID(), actor.UUID(), p.JSON())
	if err != nil {
		t.Fatalf("seed the proposal feed item: %v", err)
	}
}

func (f chatFixture) seedProposalCard(t *testing.T, proposal uuid.UUID, author ids.UserID) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_messages
		(id, cabal_id, author_id, kind, proposal_id, dedupe_key, created_at)
		VALUES ($1, $2, $3, 'proposal', $4, $5, now())`,
		f.deps.IDs.NewV7(), f.cabal.ID.UUID(), author.UUID(), proposal, "proposal:"+proposal.String())
	if err != nil {
		t.Fatalf("seed the proposal card: %v", err)
	}
}

func buyPayload() feed.Payload {
	return feed.Payload{Symbol: "NVDA", Action: feed.ActionBuy, USDCMicros: money.MicrosFromUint64(300_000_000)}
}

func votesOf(proposal uuid.UUID, tally app.Tally) map[ids.ProposalID]app.Tally {
	return map[ids.ProposalID]app.Tally{ids.ProposalIDFrom(proposal): tally}
}

func TestChatMoments_FetchFailed_ReadsTheTallyTheProposerAndTheAmount(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	bought, sold, plain := f.deps.IDs.NewV7(), f.deps.IDs.NewV7(), f.deps.IDs.NewV7()
	f.seedProposalFeed(t, bought, f.member(1), buyPayload())
	f.seedProposalCard(t, bought, f.member(0))
	sell := feed.Payload{
		Symbol: "AAPLx", Action: feed.ActionSell, QuoteMicros: money.MicrosFromUint64(41_000_000),
		TokenAmount: money.UnitsFromUint64(2_000_000),
	}
	f.seedProposalFeed(t, sold, f.member(1), sell)
	f.seedProposalFeed(t, plain, f.member(1), feed.Payload{Symbol: "NVDA", Action: feed.ActionBuy})
	tally := app.Tally{Yes: 2, No: 4, Voters: 6, Needed: 4, Kind: "buy"}
	h := f.settledMoments(map[ids.ProposalID]app.Tally{
		ids.ProposalIDFrom(bought): tally, ids.ProposalIDFrom(sold): tally, ids.ProposalIDFrom(plain): tally,
	})
	cases := map[string]struct {
		fetch func() (adapters.SettledFetch, error)
		want  domain.ProposalSettled
	}{
		"failed buy by the card's author": {
			fetch: func() (adapters.SettledFetch, error) {
				return h.FetchFailed(t.Context(), events.ProposalFailed{V: 1, ProposalID: bought, Kind: "buy"})
			},
			want: domain.ProposalSettled{
				Outcome: domain.SettledFailed, ProposalID: bought, Side: domain.TradeBuy, Symbol: "NVDA",
				USDCMicros: 300_000_000, Yes: 2, No: 4, Voters: 6, ProposerID: f.member(0),
			},
		},
		"expired sell by the feed item's actor": {
			fetch: func() (adapters.SettledFetch, error) {
				return h.FetchExpired(t.Context(), events.ProposalExpired{V: 1, ProposalID: sold, Kind: "sell"})
			},
			want: domain.ProposalSettled{
				Outcome: domain.SettledExpired, ProposalID: sold, Side: domain.TradeSell, Symbol: "AAPLx",
				USDCMicros: 41_000_000, TokenAmount: 2_000_000, Yes: 2, No: 4, Voters: 6, ProposerID: f.member(1),
			},
		},
		"an event with no kind reads the side from the feed item": {
			fetch: func() (adapters.SettledFetch, error) {
				return h.FetchFailed(t.Context(), events.ProposalFailed{V: 1, ProposalID: plain})
			},
			want: domain.ProposalSettled{
				Outcome: domain.SettledFailed, ProposalID: plain, Side: domain.TradeBuy, Symbol: "NVDA",
				Yes: 2, No: 4, Voters: 6, ProposerID: f.member(1),
			},
		},
	}
	for name, c := range cases {
		got, err := c.fetch()
		if err != nil || got.Skip || got.Settled != c.want {
			t.Errorf("%s: fetched (%+v, %v), want %+v", name, got, err, c.want)
		}
	}
}

func TestChatMoments_FetchFailed_ClampsAnAmountPastInt64(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	vast := f.deps.IDs.NewV7()
	f.seedProposalFeed(t, vast, f.member(0), feed.Payload{
		Symbol: "NVDA", Action: feed.ActionBuy, USDCMicros: money.MicrosFromUint64(1 << 63),
	})
	h := f.settledMoments(votesOf(vast, app.Tally{Yes: 1, No: 3, Voters: 4, Kind: "buy"}))
	got, err := h.FetchFailed(t.Context(), events.ProposalFailed{V: 1, ProposalID: vast, Kind: "buy"})
	if err != nil || got.Settled.USDCMicros != math.MaxInt64 {
		t.Fatalf("fetched (%+v, %v), want the amount clamped to int64", got, err)
	}
}

func TestChatMoments_FetchFailed_SkipsWhatTheChatDoesNotSay(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	rule, unknown, ruleTally := f.deps.IDs.NewV7(), f.deps.IDs.NewV7(), f.deps.IDs.NewV7()
	f.seedProposalFeed(t, ruleTally, f.member(0), buyPayload())
	h := f.settledMoments(votesOf(ruleTally, app.Tally{Yes: 1, No: 3, Voters: 4, Kind: "rule"}))
	for name, e := range map[string]events.ProposalFailed{
		"a rule change by event kind": {V: 1, ProposalID: rule, Kind: "rule"},
		"an unknown proposal":         {V: 1, ProposalID: unknown, Kind: "buy"},
		"a rule change by tally kind": {V: 1, ProposalID: ruleTally},
	} {
		got, err := h.FetchFailed(t.Context(), e)
		if err != nil || !got.Skip {
			t.Errorf("%s: fetched (%+v, %v), want a skip", name, got, err)
		}
	}
	got, err := h.FetchExpired(t.Context(), events.ProposalExpired{V: 1, ProposalID: rule, Kind: "rule"})
	if err != nil || !got.Skip {
		t.Errorf("an expired rule change fetched (%+v, %v), want a skip", got, err)
	}
	if err := h.ApplyFailed(t.Context(), db.Tx{}, events.ProposalFailed{V: 1}, got, f.now); err != nil {
		t.Errorf("applying a skip returned %v, want nil", err)
	}
	if err := h.ApplyExpired(t.Context(), db.Tx{}, events.ProposalExpired{V: 1}, got, f.now); err != nil {
		t.Errorf("applying a skip returned %v, want nil", err)
	}
}

func TestChatMoments_FetchFailed_SurfacesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	pending, broken := f.deps.IDs.NewV7(), f.deps.IDs.NewV7()
	f.seedProposalFeed(t, broken, f.member(0), buyPayload())
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE feed_objects SET payload = '{"status":7}' WHERE ref_id = $1`,
		broken,
	); err != nil {
		t.Fatal(err)
	}
	tally := app.Tally{Yes: 1, No: 3, Voters: 4, Kind: "buy"}
	votes := map[ids.ProposalID]app.Tally{ids.ProposalIDFrom(pending): tally, ids.ProposalIDFrom(broken): tally}
	h := f.settledMoments(votes)
	if _, err := h.FetchFailed(
		t.Context(),
		events.ProposalFailed{V: 1, ProposalID: pending, Kind: "buy"},
	); errs.CodeOf(
		err,
	) != errs.CodeFeedItemPending {
		t.Errorf("a proposal with no feed item returned %v, want feed_item_pending", err)
	}
	if _, err := h.FetchFailed(t.Context(), events.ProposalFailed{V: 1, ProposalID: broken, Kind: "buy"}); err == nil {
		t.Error("a feed item with an unreadable payload fetched without an error")
	}
	down := adapters.ChatMoments{Tallies: &fakeTallies{err: errs.New(errs.CodeInternal, "test")}, Reads: f.pool}
	if _, err := down.FetchFailed(
		t.Context(),
		events.ProposalFailed{V: 1, ProposalID: pending, Kind: "buy"},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Errorf("failing tallies returned %v, want internal", err)
	}
}

func TestChatMoments_Fetch_SurfacesAFailingRead(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	good := f.deps.IDs.NewV7()
	f.seedProposalFeed(t, good, f.member(0), buyPayload())
	h := f.settledMoments(votesOf(good, app.Tally{Yes: 1, No: 3, Voters: 4, Kind: "buy"}))
	for _, c := range []struct{ table, column string }{{"feed_objects", "actor_id"}, {"cabal_messages", "author_id"}} {
		if _, err := f.pool.Exec(
			t.Context(),
			`ALTER TABLE `+c.table+` RENAME COLUMN `+c.column+` TO gone`,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := h.FetchFilled(t.Context(), proposalFill(good)); errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("a fill with %s down returned %v, want internal", c.table, err)
		}
		if _, err := h.FetchFailed(
			t.Context(),
			events.ProposalFailed{V: 1, ProposalID: good, Kind: "buy"},
		); errs.CodeOf(
			err,
		) != errs.CodeInternal {
			t.Errorf("a failed vote with %s down returned %v, want internal", c.table, err)
		}
		if _, err := f.pool.Exec(t.Context(), `ALTER TABLE `+c.table+` RENAME COLUMN gone TO `+c.column); err != nil {
			t.Fatal(err)
		}
	}
}

func TestChatMoments_ApplyFailed_RefusesAnUnwritableMoment(t *testing.T) {
	t.Parallel()
	err := adapters.ChatMoments{}.ApplyFailed(
		t.Context(), db.Tx{}, events.ProposalFailed{V: 1}, adapters.SettledFetch{}, time.Time{})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("an empty settled fetch returned %v, want invalid_input", err)
	}
}

func proposalFill(proposal uuid.UUID) events.TradeConfirmed {
	return events.TradeConfirmed{
		V: 1, SwapID: uuid.NewSHA1(uuid.Nil, []byte("swap")), Action: "buy",
		Source:  events.TradeSource{Kind: "proposal", ID: proposal},
		OutMint: marketfake.AAPLx().Mint.Address(), OutAmount: money.UnitsFromUint64(20),
		USDCMicros: money.MicrosFromUint64(393_500_000), ConfirmedAt: time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC),
	}
}

func TestChatMoments_FetchFilled_CarriesTheTallyAndTheProposerOfAPassedVote(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	carded, acted, bare, untallied := f.deps.IDs.NewV7(), f.deps.IDs.NewV7(), f.deps.IDs.NewV7(), f.deps.IDs.NewV7()
	f.seedProposalFeed(t, carded, f.member(1), buyPayload())
	f.seedProposalCard(t, carded, f.member(0))
	f.seedProposalFeed(t, acted, f.member(1), buyPayload())
	tally := app.Tally{Yes: 5, No: 1, Voters: 6, Needed: 4, Kind: "buy"}
	h := f.settledMoments(map[ids.ProposalID]app.Tally{
		ids.ProposalIDFrom(carded): tally, ids.ProposalIDFrom(acted): tally, ids.ProposalIDFrom(bare): tally,
	})
	cases := map[string]struct {
		proposal uuid.UUID
		want     adapters.FilledFetch
	}{
		"the card's author":    {carded, adapters.FilledFetch{Tallied: true, Yes: 5, No: 1, Proposer: f.member(0)}},
		"the feed's actor":     {acted, adapters.FilledFetch{Tallied: true, Yes: 5, No: 1, Proposer: f.member(1)}},
		"no card, no feed":     {bare, adapters.FilledFetch{Tallied: true, Yes: 5, No: 1}},
		"a proposal untallied": {untallied, adapters.FilledFetch{}},
	}
	for name, c := range cases {
		got, err := h.FetchFilled(t.Context(), proposalFill(c.proposal))
		if err != nil || got.Asset.Symbol != "AAPLx" {
			t.Fatalf("%s: fetched (%+v, %v), want the asset card", name, got, err)
		}
		got.Asset = app.AssetCard{}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: fetched %+v, want %+v", name, got, c.want)
		}
	}
	manual := proposalFill(carded)
	manual.Source = events.TradeSource{}
	got, err := h.FetchFilled(t.Context(), manual)
	if err != nil || got.Tallied || got.Proposer != (ids.UserID{}) {
		t.Errorf("a fill with no proposal fetched (%+v, %v), want no tally and no proposer", got, err)
	}
	down := adapters.ChatMoments{
		Assets: listedAssets{}, Tallies: &fakeTallies{err: errs.New(errs.CodeInternal, "test")},
		Reads: f.pool,
	}
	if _, err := down.FetchFilled(t.Context(), proposalFill(carded)); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("failing tallies returned %v, want internal", err)
	}
}

func (f chatRoutes) writeSettled(t *testing.T, p domain.ProposalSettled, key string) {
	t.Helper()
	moment, err := domain.NewProposalSettledMoment(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.writeMoment(t, moment, key); err != nil {
		t.Fatal(err)
	}
}

func TestGetChatMessages_proposalSettledReachesTheWire(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	proposal := f.deps.IDs.NewV7()
	f.writeSettled(t, domain.ProposalSettled{
		Outcome: domain.SettledFailed, ProposalID: proposal, Side: domain.TradeBuy, Symbol: "NVDA",
		USDCMicros: 300_000_000, Yes: 2, No: 4, Voters: 6, ProposerID: f.member(0),
	}, "settled:buy")
	if hidden, err := f.channel(t, f.member(1), api.GetChatMessagesParams{}); err != nil || len(hidden) != 0 {
		t.Fatalf("a read without moments = %+v, %v, want none", hidden, err)
	}
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	kai := api.ChatAuthor{
		Id: f.member(0).
			UUID(),
		Handle:      ptr("kai"),
		DisplayName: "Kai",
		PhotoUrl:    ptr("https://cdn.example.com/kai.jpg"),
	}
	failed, err := got[0].Moment.AsProposalSettledMoment()
	want := api.ProposalSettledMoment{
		Kind: api.ProposalSettled, Outcome: api.ProposalSettledMomentOutcomeFailed, ProposalId: proposal,
		Side: api.ProposalSettledMomentSideBuy, Symbol: "NVDA", UsdcMicros: 300_000_000, Yes: 2, No: 4, Voters: 6,
		Proposer: kai,
	}
	if err != nil || !reflect.DeepEqual(failed, want) {
		t.Fatalf("failed = %+v, %v, want %+v", failed, err, want)
	}
	if published := f.rt.Published(); len(published) != 1 || published[0].Name != app.EventMomentCreated {
		t.Fatalf("published = %+v, want one moment.created", published)
	}
}

func TestGetChatMessages_anExpiredSellCarriesItsTokenAmount(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.writeSettled(t, domain.ProposalSettled{
		Outcome: domain.SettledExpired, ProposalID: f.deps.IDs.NewV7(), Side: domain.TradeSell, Symbol: "NVDA",
		USDCMicros: 41_000_000, TokenAmount: 2_000_000, Voters: 3, ProposerID: f.member(0),
	}, "settled:sell")
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	expired, err := got[0].Moment.AsProposalSettledMoment()
	if err != nil || expired.Outcome != api.ProposalSettledMomentOutcomeExpired || expired.TokenAmount == nil ||
		*expired.TokenAmount != 2_000_000 || expired.Side != api.ProposalSettledMomentSideSell {
		t.Fatalf("expired = %+v, %v, want an expired sell of 2000000 tokens", expired, err)
	}
}

func TestGetChatMessages_aPassedVoteFillCarriesItsTallyAndProposer(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	trade, err := domain.NewTradeFilledMoment(domain.TradeFilled{
		Side: domain.TradeBuy, Symbol: "NVDA", AssetID: f.deps.IDs.NewV7(), PriceMicros: 196_750_000,
		USDCMicros: 300_000_000, FilledAt: momentFilledAt(), SwapID: f.deps.IDs.NewV7(), ProposalID: f.deps.IDs.NewV7(),
		Tallied: true, Yes: 5, No: 1, ProposerID: f.member(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.writeMoment(t, trade, "swap"); err != nil {
		t.Fatal(err)
	}
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	filled, err := got[0].Moment.AsTradeFilledMoment()
	if err != nil || !reflect.DeepEqual(filled.Yes, ptr(5)) || !reflect.DeepEqual(filled.No, ptr(1)) ||
		filled.Proposer == nil || filled.Proposer.Id != f.member(0).UUID() || filled.Proposer.DisplayName != "Kai" {
		t.Fatalf("filled = %+v, %v, want 5 to 1 by Kai", filled, err)
	}
}
