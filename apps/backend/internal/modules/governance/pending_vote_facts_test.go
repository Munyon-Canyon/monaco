package governance_test

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type replyBook struct {
	mu     sync.Mutex
	counts map[ids.ProposalID]int
	calls  [][]ids.ProposalID
	err    error
}

func (r *replyBook) ReplyCounts(_ context.Context, proposalIDs []ids.ProposalID) (map[ids.ProposalID]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, slices.Clone(proposalIDs))
	return r.counts, r.err
}

type mintBook struct {
	*marketfake.CatalogFake

	mu    sync.Mutex
	calls [][]string
}

func (m *mintBook) ByMints(ctx context.Context, mints []string) ([]market.Asset, error) {
	m.mu.Lock()
	m.calls = append(m.calls, slices.Clone(mints))
	m.mu.Unlock()
	return m.CatalogFake.ByMints(ctx, mints)
}

type pendingFacts struct {
	readsDB

	cards   *cardBook
	replies *replyBook
	mints   *mintBook
}

func newPendingFacts(t *testing.T) pendingFacts {
	t.Helper()
	d := newReadsDB(t)
	return pendingFacts{
		readsDB: d, cards: &cardBook{cards: map[ids.UserID]identityport.UserCard{}},
		replies: &replyBook{counts: map[ids.ProposalID]int{}}, mints: &mintBook{CatalogFake: d.assets},
	}
}

func (f pendingFacts) pendingReads() *app.PendingVoteReads {
	reads := app.NewProposalReads(f.pool, f.swaps, f.names, f.assets, f.cards, &returnBook{})
	return app.NewPendingVoteReads(reads, app.PendingVotePorts{Assets: f.mints, Replies: f.replies})
}

func (f pendingFacts) open(
	t *testing.T, cabal, proposer uuid.UUID, in time.Duration, voters ...uuid.UUID,
) sqlc.InsertProposalParams {
	t.Helper()
	p := f.buy(voters...)
	p.CabalID, p.ProposerID = cabal, proposer
	p.ExpiresAt, p.CreatedAt = f.now.Add(in), f.now.Add(-2*time.Hour)
	p.UsdcMicros = pgtype.Int8{Int64: 400_000_000, Valid: true}
	return p
}

func (f pendingFacts) card(id uuid.UUID, handle, name string) {
	f.cards.cards[ids.UserIDFrom(id)] = identityport.UserCard{
		ID: ids.UserIDFrom(id), Handle: handle, DisplayName: name, PhotoURL: "https://cdn.example.com/" + handle,
	}
}

func TestMe_PendingVotes_CarriesEveryFactTheCardDraws(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	mia, kai := f.ids.NewV7(), f.ids.NewV7()
	voters := []uuid.UUID{f.caller, mia, kai, f.ids.NewV7(), f.ids.NewV7(), f.ids.NewV7(), f.ids.NewV7(), f.ids.NewV7()}
	nvda := f.open(t, f.ids.NewV7(), mia, time.Hour, voters...)
	f.insert(t, nvda)
	for _, yes := range voters[1:5] {
		f.ballot(t, nvda.ID, yes, "yes")
	}
	f.ballot(t, nvda.ID, voters[5], "no")
	sell := f.open(t, f.ids.NewV7(), kai, 2*time.Hour, f.caller, mia, kai)
	sell.Kind, sell.Symbol, sell.Mint = "sell", "TSLAx", marketfake.TSLAx().Mint.String()
	sell.UsdcMicros, sell.TokenAmount = pgtype.Int8{}, pgtype.Int8{Int64: 150_000_000, Valid: true}
	sell.Threshold = "unanimous"
	f.insert(t, sell)
	f.names.set(nvda.CabalID, "AC Capital", port.StatusActive)
	f.names.set(sell.CabalID, "Degens", port.StatusActive)
	f.card(mia, "mia", "Mia")
	f.card(kai, "kai", "Kai")
	f.replies.counts[ids.ProposalIDFrom(nvda.ID)] = 6

	got, err := f.pendingReads().PendingVotes(t.Context(), ids.UserIDFrom(f.caller))
	if err != nil || len(got) != 2 {
		t.Fatalf("PendingVotes = %+v, %v, want two votes", got, err)
	}
	assertPendingVote(t, got[0], app.PendingVote{
		ProposalID: ids.ProposalIDFrom(nvda.ID), CabalID: ids.CabalIDFrom(nvda.CabalID), CabalName: "AC Capital",
		Kind: "buy", Symbol: "AAPLx", USDC: money.MicrosFromUint64(400_000_000), AssetName: "Apple",
		TokenAmount: money.NewBaseUnits(0, 8), AssetLogo: marketfake.AAPLx().LogoURL,
		Proposer: app.PendingProposer{
			ID: ids.UserIDFrom(mia), Handle: "mia", DisplayName: "Mia", PhotoURL: "https://cdn.example.com/mia",
		},
		Tally: app.Tally{Yes: 4, No: 1, Voters: 8, Needed: 5}, VotedCount: 5, ReplyCount: 6,
		CreatedAt: nvda.CreatedAt, ExpiresAt: nvda.ExpiresAt,
	})
	assertPendingVote(t, got[1], app.PendingVote{
		ProposalID: ids.ProposalIDFrom(sell.ID), CabalID: ids.CabalIDFrom(sell.CabalID), CabalName: "Degens",
		Kind: "sell", Symbol: "TSLAx", TokenAmount: money.NewBaseUnits(150_000_000, 8), AssetName: "Tesla",
		AssetLogo: marketfake.TSLAx().LogoURL,
		Proposer: app.PendingProposer{
			ID: ids.UserIDFrom(kai), Handle: "kai", DisplayName: "Kai", PhotoURL: "https://cdn.example.com/kai",
		},
		Tally:     app.Tally{Voters: 3, Needed: 3},
		CreatedAt: sell.CreatedAt, ExpiresAt: sell.ExpiresAt,
	})
}

func assertPendingVote(t *testing.T, got, want app.PendingVote) {
	t.Helper()
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("%s times = %s and %s, want %s and %s",
			want.Symbol, got.CreatedAt, got.ExpiresAt, want.CreatedAt, want.ExpiresAt)
	}
	got.CreatedAt, got.ExpiresAt, want.CreatedAt, want.ExpiresAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	if got != want {
		t.Errorf("pending vote = %+v, want %+v", got, want)
	}
}

func TestMe_PendingVotes_ReadsEachPortOncePerRequest(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	proposer := f.ids.NewV7()
	f.insert(t, f.open(t, f.ids.NewV7(), proposer, time.Hour, f.caller))
	f.insert(t, f.open(t, f.ids.NewV7(), proposer, 2*time.Hour, f.caller))
	got, err := f.pendingReads().PendingVotes(t.Context(), ids.UserIDFrom(f.caller))
	if err != nil || len(got) != 2 {
		t.Fatalf("PendingVotes = %+v, %v, want two votes", got, err)
	}
	if n := len(f.names.calls); n != 1 {
		t.Errorf("cabal reads = %d, want 1", n)
	}
	if n := len(f.mints.calls); n != 1 {
		t.Errorf("asset reads = %d, want 1", n)
	}
	if n := len(f.cards.calls); n != 1 {
		t.Errorf("proposer reads = %d, want 1", n)
	}
	if n := len(f.replies.calls); n != 1 || len(f.replies.calls[0]) != 2 {
		t.Errorf("reply reads = %v, want one call for both proposals", f.replies.calls)
	}
}

func TestMe_PendingVotes_FallsBackWhenAssetOrProposerIsUnknown(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	gone := f.ids.NewV7()
	p := f.open(t, f.ids.NewV7(), gone, time.Hour, f.caller)
	p.Mint = marketfake.JPSTx().Mint.String()
	f.mints.CatalogFake = marketfake.NewCatalog()
	f.insert(t, p)
	f.cards.cards[ids.UserIDFrom(gone)] = identityport.UserCard{ID: ids.UserIDFrom(gone), Deleted: true}
	got, err := f.pendingReads().PendingVotes(t.Context(), ids.UserIDFrom(f.caller))
	if err != nil || len(got) != 1 {
		t.Fatalf("PendingVotes = %+v, %v, want one vote", got, err)
	}
	if got[0].AssetName != "AAPLx" || got[0].AssetLogo != "" || got[0].Proposer.ID.UUID() != gone ||
		got[0].Proposer.Handle != "" || got[0].Proposer.DisplayName != "" {
		t.Errorf("vote = %+v, want the symbol as the name, no logo and a blank proposer", got[0])
	}
}

func TestMe_PendingVotes_AFailingPortFailsTheRead(t *testing.T) {
	t.Parallel()
	for name, breakIt := range map[string]func(pendingFacts){
		"assets":   func(f pendingFacts) { f.mints.Fail("ByMints", errs.New(errs.CodeDBUnavailable, "t")) },
		"proposer": func(f pendingFacts) { f.cards.err = errs.New(errs.CodeDBUnavailable, "t") },
		"replies":  func(f pendingFacts) { f.replies.err = errs.New(errs.CodeDBUnavailable, "t") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPendingFacts(t)
			f.insert(t, f.open(t, f.ids.NewV7(), f.ids.NewV7(), time.Hour, f.caller))
			breakIt(f)
			if _, err := f.pendingReads().PendingVotes(t.Context(), ids.UserIDFrom(f.caller)); errs.CodeOf(
				err) != errs.CodeDBUnavailable {
				t.Errorf("PendingVotes with the %s read down err = %v, want db_unavailable", name, err)
			}
		})
	}
}

func TestHTTP_GetMyPendingVotes_WiresTheCardFacts(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	mia := f.ids.NewV7()
	p := f.open(t, f.ids.NewV7(), mia, time.Hour, f.caller, mia)
	f.insert(t, p)
	f.ballot(t, p.ID, mia, "yes")
	f.card(mia, "mia", "Mia")
	f.replies.counts[ids.ProposalIDFrom(p.ID)] = 6
	h := adapters.HTTP{Pending: f.pendingReads()}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.caller.String()})
	res, err := h.GetMyPendingVotes(ctx, api.GetMyPendingVotesRequestObject{})
	got, ok := res.(api.GetMyPendingVotes200JSONResponse)
	if err != nil || !ok || len(got) != 1 {
		t.Fatalf("GetMyPendingVotes = %#v, %v, want one vote", res, err)
	}
	logo, handle, photo := marketfake.AAPLx().LogoURL, "mia", "https://cdn.example.com/mia"
	usdc := int64(400_000_000)
	if !got[0].CreatedAt.Equal(p.CreatedAt) {
		t.Errorf("created_at = %s, want %s", got[0].CreatedAt, p.CreatedAt)
	}
	got[0].CreatedAt, got[0].ExpiresAt = time.Time{}, time.Time{}
	want := api.PendingVote{
		ProposalId: p.ID, CabalId: p.CabalID, Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc,
		AssetName: "Apple", AssetLogoUrl: &logo,
		Proposer: api.PendingVoteProposer{UserId: mia, Handle: &handle, DisplayName: "Mia", PhotoUrl: &photo},
		Tally:    api.Tally{Yes: 1, Voters: 2, Needed: 2}, VotedCount: 1, ReplyCount: 6,
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("pending vote = %+v, want %+v", got[0], want)
	}
}

func TestMe_PendingVotes_NothingWaitingReadsNoPort(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	got, err := f.pendingReads().PendingVotes(t.Context(), ids.UserIDFrom(f.caller))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("PendingVotes = %#v, %v, want an empty list", got, err)
	}
	if calls := len(f.names.calls) + len(f.mints.calls) + len(f.cards.calls) + len(f.replies.calls); calls != 0 {
		t.Errorf("ports read %d times for an empty list, want 0", calls)
	}
}
