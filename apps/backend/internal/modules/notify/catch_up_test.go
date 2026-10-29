package notify_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	socialport "github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/notifyapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const openAIMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

type catchUpGov struct {
	pending                  []governanceport.CatchUpProposal
	cards                    map[ids.ProposalID]governanceport.CatchUpProposal
	pendingCalls, cardsCalls int
	onCall                   func()
	err                      error
	cardsErr                 error
}

func (g *catchUpGov) PendingVotes(context.Context, ids.UserID) ([]governanceport.CatchUpProposal, error) {
	g.pendingCalls++
	if g.onCall != nil {
		g.onCall()
	}
	return g.pending, g.err
}

func (g *catchUpGov) ProposalCards(
	_ context.Context, _ ids.UserID, proposals []ids.ProposalID,
) (map[ids.ProposalID]governanceport.CatchUpProposal, error) {
	g.cardsCalls++
	out := map[ids.ProposalID]governanceport.CatchUpProposal{}
	for _, id := range proposals {
		if card, ok := g.cards[id]; ok {
			out[id] = card
		}
	}
	return out, g.cardsErr
}

type catchUpComments struct {
	previews map[uuid.UUID]socialport.CommentPreview
	calls    int
	err      error
}

func (c *catchUpComments) CommentPreviews(
	_ context.Context, _ ids.UserID, comments []uuid.UUID,
) (map[uuid.UUID]socialport.CommentPreview, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	out := map[uuid.UUID]socialport.CommentPreview{}
	for _, id := range comments {
		if p, ok := c.previews[id]; ok {
			out[id] = p
			if parent, ok := c.previews[ptrValue(p.ReplyToID)]; ok {
				out[parent.CommentID] = parent
			}
		}
	}
	return out, nil
}

type catchUpReplies struct {
	counts map[ids.ProposalID]int
	calls  int
	err    error
}

func (c *catchUpReplies) ReplyCounts(_ context.Context, _ []ids.ProposalID) (map[ids.ProposalID]int, error) {
	c.calls++
	return c.counts, c.err
}

type catchUpInvites struct {
	invites []cabalport.PendingInvite
	calls   int
	err     error
	onCall  func()
}

func (c *catchUpInvites) PendingInvites(context.Context, ids.UserID, time.Time) ([]cabalport.PendingInvite, error) {
	c.calls++
	if c.onCall != nil {
		c.onCall()
	}
	return c.invites, c.err
}

type catchUpInviteLookup struct {
	byID map[uuid.UUID]cabalport.PendingInvite
	err  error
}

func (c *catchUpInviteLookup) InvitesByID(
	_ context.Context, requestIDs []uuid.UUID,
) (map[uuid.UUID]cabalport.ReceivedInvite, error) {
	out := map[uuid.UUID]cabalport.ReceivedInvite{}
	for _, id := range requestIDs {
		if inv, ok := c.byID[id]; ok {
			out[id] = inv
		}
	}
	return out, c.err
}

type catchUpCabals struct {
	joined              []ids.CabalID
	views               map[ids.CabalID]cabalport.CabalView
	ofCalls, cabalCalls int
	ofErr, cabalErr     error
}

func (c *catchUpCabals) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	c.ofCalls++
	return c.joined, c.ofErr
}

func (c *catchUpCabals) Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	c.cabalCalls++
	return c.views, c.cabalErr
}

type catchUpHoldings struct {
	top   map[ids.CabalID]chain.SolanaAddress
	calls int
	err   error
}

func (c *catchUpHoldings) TopHoldings(context.Context, []ids.CabalID) (map[ids.CabalID]chain.SolanaAddress, error) {
	c.calls++
	return c.top, c.err
}

type catchUpAssets struct {
	assets []market.Asset
	calls  int
	err    error
}

func (c *catchUpAssets) ByMints(context.Context, []string) ([]market.Asset, error) {
	c.calls++
	return c.assets, c.err
}

type catchUpWorld struct {
	r        *pushRig
	viewer   ids.UserID
	gov      *catchUpGov
	comments *catchUpComments
	replies  *catchUpReplies
	invites  *catchUpInvites
	lookup   *catchUpInviteLookup
	cabals   *catchUpCabals
	holdings *catchUpHoldings
	assets   *catchUpAssets
	now      time.Time

	cabalMembers []ids.UserID
}

func newCatchUpWorld(t *testing.T) *catchUpWorld {
	t.Helper()
	r := newPushRig(t)
	openAI, err := market.ParseMint(openAIMint)
	if err != nil {
		t.Fatal(err)
	}
	return &catchUpWorld{
		r: r, viewer: r.follower(t, "andres", "Andres"),
		gov:      &catchUpGov{cards: map[ids.ProposalID]governanceport.CatchUpProposal{}},
		comments: &catchUpComments{previews: map[uuid.UUID]socialport.CommentPreview{}},
		replies:  &catchUpReplies{counts: map[ids.ProposalID]int{}},
		invites:  &catchUpInvites{},
		lookup:   &catchUpInviteLookup{},
		cabals:   &catchUpCabals{views: map[ids.CabalID]cabalport.CabalView{}},
		holdings: &catchUpHoldings{top: map[ids.CabalID]chain.SolanaAddress{}},
		assets:   &catchUpAssets{assets: []market.Asset{{Mint: openAI, Symbol: "OPENAI", DisplayName: "OpenAI"}}},
		now:      r.clock.Now(),
	}
}

func (w *catchUpWorld) reads() *app.ReadCatchUp {
	return app.NewReadCatchUp(app.CatchUpPorts{
		Governance: w.gov,
		Comments:   w.comments,
		Replies:    w.replies,
		Invites:    w.invites,
		InviteByID: w.lookup,
		Cabals:     w.cabals,
		Holdings:   w.holdings,
		Assets:     w.assets,
		Users:      w.r.users,
		Reads:      w.r.pool,
		Clock:      w.r.clock,
	})
}

func (w *catchUpWorld) calls() []int {
	return []int{
		w.gov.pendingCalls, w.gov.cardsCalls, w.comments.calls, w.replies.calls, w.invites.calls, w.cabals.ofCalls,
		w.cabals.cabalCalls, w.holdings.calls, w.assets.calls,
	}
}

func (w *catchUpWorld) cabal(name string) ids.CabalID {
	id := ids.CabalIDFrom(w.r.ids.NewV7())
	w.cabals.joined = append(w.cabals.joined, id)
	w.cabals.views[id] = cabalport.CabalView{ID: id, Name: name}
	return id
}

func (w *catchUpWorld) person(name string) governanceport.CatchUpPerson {
	return governanceport.CatchUpPerson{ID: ids.UserIDFrom(w.r.ids.NewV7()), Name: name}
}

func (w *catchUpWorld) proposal(
	cabal ids.CabalID,
	name, symbol string,
	status governanceport.Status,
) governanceport.CatchUpProposal {
	micros := uint64(400_000_000)
	p := governanceport.CatchUpProposal{
		ID: ids.ProposalIDFrom(w.r.ids.NewV7()), CabalID: cabal, CabalName: name, Kind: "buy", Symbol: symbol,
		AssetName: symbol, USDCMicros: &micros, Proposer: w.person("Mia"), CreatedAt: w.now.Add(-2 * time.Hour),
		ExpiresAt: w.now.Add(4 * time.Hour), Status: status, Yes: 4, No: 1, Voted: 5, Eligible: 8,
	}
	if status != governanceport.StatusOpen {
		w.gov.cards[p.ID] = p
	}
	return p
}

func (w *catchUpWorld) row(t *testing.T, kind, key string, data map[string]string, at time.Time) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	w.r.exec(t, `INSERT INTO notifications
		(id, user_id, kind, source_event_id, title, body, data, collapse_id, state, created_at, item_key)
		VALUES ($1, $2, $3, $4, 't', 'b', $5, 'c', 'delivered', $6, $7)`,
		w.r.ids.NewV7(), w.viewer.UUID(), kind, w.r.ids.NewV7(), raw, at, key)
}

func (w *catchUpWorld) state(t *testing.T, key string, read, cleared *time.Time, snapshot any) {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil {
		raw = nil
	}
	reason := any(nil)
	if cleared != nil {
		reason = "cleared"
	}
	w.r.exec(
		t,
		`INSERT INTO catch_up_state (user_id, item_key, read_at, cleared_at, cleared_reason, snapshot, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		w.viewer.UUID(),
		key,
		read,
		cleared,
		reason,
		raw,
		w.now,
	)
}

func (w *catchUpWorld) comment(
	author governanceport.CatchUpPerson,
	body string,
	parent *uuid.UUID,
	at time.Time,
) socialport.CommentPreview {
	id := w.r.ids.NewV7()
	p := socialport.CommentPreview{
		FeedItemID: w.r.ids.NewV7(), CommentID: id, ParentCommentID: parent, ReplyToID: parent, RootCommentID: id,
		Author: socialport.CommentAuthor{ID: author.ID, Name: author.Name}, Body: body, CreatedAt: at,
	}
	if parent != nil {
		p.RootCommentID = *parent
	}
	w.comments.previews[id] = p
	return p
}

func (w *catchUpWorld) fixture(t *testing.T) {
	t.Helper()
	acCapital, techTitans, degens := w.cabal("AC Capital"), w.cabal("Tech Titans"), w.cabal("Degens")
	nvda := w.proposal(acCapital, "AC Capital", "NVDA", governanceport.StatusOpen)
	w.gov.pending = []governanceport.CatchUpProposal{nvda}
	w.replies.counts[nvda.ID] = 6

	nvdaProposal := w.proposal(acCapital, "AC Capital", "NVDA", governanceport.StatusOpen)
	w.gov.cards[nvdaProposal.ID] = nvdaProposal
	mention := w.comment(w.person("Christelle"), "Still out: @Andres, Jordan?", nil, w.now.Add(-20*time.Minute))
	mention.ProposalID, mention.CabalID = &nvdaProposal.ID, &acCapital
	w.comments.previews[mention.CommentID] = mention
	w.row(t, "comment_mention", "comment:"+mention.CommentID.String(), map[string]string{}, mention.CreatedAt)

	tsla := w.proposal(techTitans, "Tech Titans", "TSLA", governanceport.StatusPassed)
	mine := w.comment(
		governanceport.CatchUpPerson{ID: w.viewer, Name: "Andres"},
		"Deliveries miss is priced in.",
		nil,
		w.now.Add(-3*time.Hour),
	)
	reply := w.comment(w.person("Mia"), "Agreed, sell the rip.", &mine.CommentID, w.now.Add(-time.Hour))
	reply.ProposalID, reply.CabalID = &tsla.ID, &techTitans
	w.comments.previews[reply.CommentID] = reply
	w.row(t, "comment_reply", "comment:"+reply.CommentID.String(), map[string]string{}, reply.CreatedAt)

	quant := ids.CabalIDFrom(w.r.ids.NewV7())
	w.invites.invites = []cabalport.PendingInvite{{
		ID: w.r.ids.NewV7(), CabalID: quant, CabalName: "Quant Kids", MemberCount: 5,
		InviterID: ids.UserIDFrom(w.r.ids.NewV7()), InviterName: "Sam", CreatedAt: w.now.Add(-30 * time.Minute),
		ExpiresAt: w.now.Add(24 * time.Hour),
	}}
	w.holdings.top[quant] = chain.SolanaAddress(openAIMint)

	meta := w.proposal(degens, "Degens", "META", governanceport.StatusPassed)
	meta.YourChoice, meta.Yes, meta.No = "yes", 4, 1
	w.gov.cards[meta.ID] = meta
	w.row(
		t,
		"proposal_result",
		"proposal:"+meta.ID.String(),
		map[string]string{"outcome": "filled"},
		w.now.Add(-5*time.Minute),
	)
	tslaResult := tsla
	tslaResult.YourChoice, tslaResult.Yes, tslaResult.No = "no", 2, 2
	w.gov.cards[tsla.ID] = tslaResult
	w.row(
		t,
		"proposal_result",
		"proposal:"+tsla.ID.String(),
		map[string]string{"outcome": "failed"},
		w.now.Add(-10*time.Minute),
	)
}

func readFixture(t *testing.T) (*catchUpWorld, domain.CatchUp) {
	t.Helper()
	w := newCatchUpWorld(t)
	w.fixture(t)
	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return w, got
}

func TestCatchUp_FramesScenarioCountsSixAcrossFourSections(t *testing.T) {
	t.Parallel()
	w, got := readFixture(t)

	sections := []int{len(got.Waiting), len(got.Replies), len(got.Invites), len(got.Results)}
	if got.Count != 6 || !slices.Equal(sections, []int{1, 2, 1, 2}) {
		t.Fatalf("count %d, sections %v, want 6 and [1 2 1 2]", got.Count, sections)
	}
	count, err := w.reads().Count(t.Context(), w.viewer)
	if err != nil || count != got.Count {
		t.Fatalf("Count = %d, %v, want %d", count, err, got.Count)
	}
}

func TestCatchUp_FramesScenarioListsTheViewersCabalsInJoinOrder(t *testing.T) {
	t.Parallel()
	_, got := readFixture(t)

	names := make([]string, 0, len(got.Cabals))
	for _, c := range got.Cabals {
		names = append(names, c.Name)
	}

	if !slices.Equal(names, []string{"AC Capital", "Tech Titans", "Degens"}) {
		t.Fatalf("cabals = %v, want the viewer's three in join order", names)
	}
}

func TestCatchUp_FramesScenarioWaitingCarriesTheProposalFacts(t *testing.T) {
	t.Parallel()
	_, got := readFixture(t)

	p := got.Waiting[0].Proposal

	tally := []int{p.Yes, p.No, p.Voted, p.Eligible, p.ReplyCount}
	if !slices.Equal(tally, []int{4, 1, 5, 8, 6}) || p.Proposer.Name != "Mia" || *p.USDCMicros != 400_000_000 {
		t.Fatalf("waiting proposal = %+v", p)
	}
	if got.Waiting[0].Cabal.Name != "AC Capital" || !got.Waiting[0].Unread {
		t.Fatalf("waiting item = %+v", got.Waiting[0])
	}
}

func TestCatchUp_FramesScenarioMentionQuotesItsOwnText(t *testing.T) {
	t.Parallel()
	_, got := readFixture(t)

	mention := got.Replies[0]

	if mention.Kind != domain.CatchUpMention || mention.Actor.Name != "Christelle" || mention.Proposal == nil {
		t.Fatalf("mention = %+v", mention)
	}
	if mention.Comment.Body != "Still out: @Andres, Jordan?" || mention.Quote.Text != mention.Comment.Body ||
		mention.Quote.IsYou {
		t.Fatalf("mention comment %+v quote %+v", mention.Comment, mention.Quote)
	}
}

func TestCatchUp_FramesScenarioReplyQuotesTheViewersComment(t *testing.T) {
	t.Parallel()
	_, got := readFixture(t)

	reply := got.Replies[1]

	if reply.Kind != domain.CatchUpReply || reply.Actor.Name != "Mia" || reply.Cabal.Name != "Tech Titans" {
		t.Fatalf("reply = %+v", reply)
	}
	if reply.Comment.Body != "Agreed, sell the rip." || reply.Quote.Text != "Deliveries miss is priced in." ||
		!reply.Quote.IsYou {
		t.Fatalf("reply comment %+v quote %+v", reply.Comment, reply.Quote)
	}
}

func TestCatchUp_FramesScenarioInviteNamesTheTopHolding(t *testing.T) {
	t.Parallel()
	_, got := readFixture(t)

	invite := got.Invites[0]

	if invite.Invite.MemberCount != 5 || invite.Invite.TopHoldingName != "OpenAI" {
		t.Fatalf("invite = %+v", invite.Invite)
	}
	if invite.Actor.Name != "Sam" || invite.Cabal.Name != "Quant Kids" {
		t.Fatalf("invite item = %+v", invite)
	}
}

func TestCatchUp_FramesScenarioResultsCarryTheOutcomeAndTheViewersChoice(t *testing.T) {
	t.Parallel()
	_, got := readFixture(t)

	meta, tsla := got.Results[0].Proposal, got.Results[1].Proposal

	if meta.Symbol != "META" || meta.YourChoice != "yes" || meta.Outcome != "filled" || !*meta.Filled {
		t.Fatalf("meta = %+v", meta)
	}
	if tsla.Symbol != "TSLA" || tsla.YourChoice != "no" || tsla.Outcome != "failed" || *tsla.Filled {
		t.Fatalf("tsla = %+v", tsla)
	}
}

func TestCatchUp_ACaughtUpViewerStillGetsTheirCabals(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.cabal("AC Capital")
	w.cabal("Degens")

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || got.Count != 0 || len(got.Cabals) != 2 || got.Waiting == nil || got.Cleared == nil {
		t.Fatalf("got %+v, %v, want count 0, two cabals and empty sections", got, err)
	}
}

func TestCatchUp_AReplyOnAPostCarriesThePostAndNoProposal(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cabal := w.cabal("AC Capital")
	mine := w.comment(
		governanceport.CatchUpPerson{ID: w.viewer, Name: "Andres"},
		"Nice sale",
		nil,
		w.now.Add(-time.Hour),
	)
	reply := w.comment(w.person("Mia"), "Thanks", &mine.CommentID, w.now.Add(-time.Minute))
	reply.CabalID = &cabal
	w.comments.previews[reply.CommentID] = reply
	w.row(t, "post_reply", "comment:"+reply.CommentID.String(), map[string]string{
		"item_kind": "trade", "item_label": "sale", "item_actor_name": "Logan",
	}, reply.CreatedAt)

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Replies) != 1 {
		t.Fatalf("got %+v, %v, want one reply", got, err)
	}
	item := got.Replies[0]
	if item.Proposal != nil || item.Post == nil || item.Post.Label != "sale" || item.Post.ActorName != "Logan" ||
		item.Post.Kind != "trade" || item.Cabal.Name != "AC Capital" {
		t.Fatalf("item = %+v post %+v", item, item.Post)
	}
}

func TestCatchUp_AnItemWhoseCommentIsHiddenIsNotListedOrCounted(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.row(t, "comment_reply", "comment:"+w.r.ids.NewV7().String(), map[string]string{}, w.now.Add(-time.Minute))

	count, err := w.reads().Count(t.Context(), w.viewer)

	if err != nil || count != 0 {
		t.Fatalf("Count = %d, %v, want 0", count, err)
	}
}

func TestCatchUp_ClearedItemsStayClearedUntilANewerRowArrives(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cabal := w.cabal("Degens")
	stays, reopens, mine := w.proposal(cabal, "Degens", "META", governanceport.StatusPassed),
		w.proposal(cabal, "Degens", "AAPL", governanceport.StatusPassed),
		w.proposal(cabal, "Degens", "NVDA", governanceport.StatusPassed)
	cleared, later := w.now.Add(-time.Hour), w.now.Add(-time.Minute)
	for _, p := range []governanceport.CatchUpProposal{stays, reopens, mine} {
		w.state(t, "proposal:"+p.ID.String(), nil, &cleared, domain.CatchUpItem{Key: "proposal:" + p.ID.String()})
	}
	decider := w.viewer
	mine.DecidedBy = &decider
	w.gov.cards[mine.ID] = mine
	w.row(
		t,
		"proposal_result",
		"proposal:"+stays.ID.String(),
		map[string]string{"outcome": "filled"},
		w.now.Add(-2*time.Hour),
	)
	w.row(t, "proposal_result", "proposal:"+reopens.ID.String(), map[string]string{"outcome": "filled"}, later)
	w.row(t, "proposal_result", "proposal:"+mine.ID.String(), map[string]string{"outcome": "filled"}, later)

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || got.Count != 1 || len(got.Results) != 1 || got.Results[0].Key != "proposal:"+reopens.ID.String() {
		t.Fatalf("got count %d results %+v, %v, want only the reopened result", got.Count, got.Results, err)
	}
	if !got.Results[0].Unread {
		t.Fatalf("reopened result is read, want unread")
	}
	keys := make([]string, 0, len(got.Cleared))
	for _, item := range got.Cleared {
		keys = append(keys, item.Key)
	}
	slices.Sort(keys)
	want := []string{"proposal:" + stays.ID.String(), "proposal:" + mine.ID.String()}
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("cleared = %v, want %v: the reopened one is active, not cleared", keys, want)
	}
}

func TestCatchUp_ClearedDrawsFromTheSnapshotAndSkipsAMissingOne(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cleared := w.now.Add(-time.Minute)
	snapshot := domain.CatchUpItem{
		Kind: domain.CatchUpInvite, Invite: &domain.CatchUpInviteInfo{ID: "i", MemberCount: 5},
		Cabal: &domain.CatchUpCabal{Name: "Quant Kids"},
	}
	w.state(t, "invite:"+w.r.ids.NewV7().String(), nil, &cleared, snapshot)
	w.state(t, "invite:"+w.r.ids.NewV7().String(), nil, &cleared, nil)

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Cleared) != 1 || got.Cleared[0].Cabal.Name != "Quant Kids" ||
		got.Cleared[0].ClearedReason != "cleared" || got.Cleared[0].Unread || !got.Cleared[0].At.Equal(cleared) {
		t.Fatalf("cleared = %+v, %v, want the one item with a snapshot", got.Cleared, err)
	}
}

func TestCatchUp_AReadItemIsNotUnreadUntilANewerRowArrives(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	read := w.now.Add(-time.Minute)
	older, newer := w.r.ids.NewV7(), w.r.ids.NewV7()
	for _, id := range []uuid.UUID{older, newer} {
		p := w.comment(w.person("Mia"), "hi", nil, w.now.Add(-time.Hour))
		p.CommentID = id
		w.comments.previews[id] = p
	}
	w.row(t, "comment_reply", "comment:"+older.String(), map[string]string{}, w.now.Add(-time.Hour))
	w.row(t, "comment_reply", "comment:"+newer.String(), map[string]string{}, w.now)
	w.state(t, "comment:"+older.String(), &read, nil, nil)
	w.state(t, "comment:"+newer.String(), &read, nil, nil)

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Replies) != 2 || got.Replies[0].Unread != true || got.Replies[1].Unread != false {
		t.Fatalf("replies = %+v, %v, want the newer unread and the older read", got.Replies, err)
	}
}

func TestCatchUp_MakesOneCallToEachPortForTwoItems(t *testing.T) {
	t.Parallel()
	wantOneCallToEachPort(t, 2)
}

func TestCatchUp_MakesOneCallToEachPortForFiftyItems(t *testing.T) {
	t.Parallel()
	wantOneCallToEachPort(t, 50)
}

func wantOneCallToEachPort(t *testing.T, n int) {
	t.Helper()
	w := newCatchUpWorld(t)
	cabal := w.cabal("AC Capital")
	for range n {
		w.gov.pending = append(w.gov.pending, w.proposal(cabal, "AC Capital", "NVDA", governanceport.StatusOpen))
		result := w.proposal(cabal, "AC Capital", "TSLA", governanceport.StatusPassed)
		w.row(
			t,
			"proposal_result",
			"proposal:"+result.ID.String(),
			map[string]string{"outcome": "filled"},
			w.now.Add(-time.Minute),
		)
		c := w.comment(w.person("Mia"), "hi", nil, w.now.Add(-time.Minute))
		w.row(t, "comment_reply", "comment:"+c.CommentID.String(), map[string]string{}, c.CreatedAt)
		quant := ids.CabalIDFrom(w.r.ids.NewV7())
		w.invites.invites = append(w.invites.invites, cabalport.PendingInvite{
			ID: w.r.ids.NewV7(), CabalID: quant, CabalName: "Quant", CreatedAt: w.now.Add(-time.Minute),
		})
		w.holdings.top[quant] = chain.SolanaAddress(openAIMint)
	}

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || got.Count != 4*n {
		t.Fatalf("%d items: count %d, %v, want %d", n, got.Count, err, 4*n)
	}
	if calls := w.calls(); !slices.Equal(calls, []int{1, 1, 1, 1, 1, 1, 1, 1, 1}) {
		t.Fatalf("%d items: port calls %v, want one to each", n, calls)
	}
}

type hintRecorder struct {
	mu   sync.Mutex
	keys []string
}

func (h *hintRecorder) PublishHint(_ context.Context, key string, _ []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keys = append(h.keys, key)
}

func (h *hintRecorder) published() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.keys)
}

func TestCatchUp_ARepliesAndMentionsOffRecipientGetsAMutedRowAndNoPush(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	parent, replier := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, parent, token('a'))
	r.preference(t, parent, domain.CategoryRepliesAndMentions, false)
	e := r.reply(t, parent, replier)
	d := r.emitReply(t, e)
	hints := &hintRecorder{}

	err := handleReplyWith(t, r, hints, d, e)

	wantVerdict(t, err, "", errs.VerdictAck)
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing for a muted row", sent)
	}
	r.wantStates(t, d, map[ids.UserID]string{parent: "muted"})
	r.wantCount(
		t,
		"item key",
		1,
		`SELECT count(*) FROM notifications WHERE item_key = $1`,
		"comment:"+e.CommentID.String(),
	)
	if got := hints.published(); !slices.Equal(got, []string{"user." + parent.String() + ".catch_up_changed"}) {
		t.Fatalf("hints = %v, want the catch up hint for the recipient of the muted row", got)
	}

	wantVerdict(t, handleReplyWith(t, r, hints, d, e), "", errs.VerdictAck)

	r.wantCount(t, "rows after a redelivery", 1, `SELECT count(*) FROM notifications`)
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v after a redelivery, want nothing", sent)
	}
}

type cappedNamed struct{ namedKind }

func (cappedNamed) DailyCap() int { return 1 }

func TestCatchUp_AMutedRowDoesNotCountTowardADailyCap(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.preference(t, user, domain.CategoryRepliesAndMentions, false)
	d, e := r.trigger(t, opsActor, user)
	capped := cappedNamed{namedKind{&crowd{users: []ids.UserID{user}}, "comment_reply"}}

	wantVerdict(t, r.handle(t, r.sender, d, e, capped), "", errs.VerdictAck)
	r.exec(t, `UPDATE notification_preferences SET enabled = true WHERE user_id = $1`, user.UUID())
	d2, e2 := r.trigger(t, opsActor, user)
	wantVerdict(t, r.handle(t, r.sender, d2, e2, capped), "", errs.VerdictAck)

	r.wantRows(t, map[string]int{"comment_reply/muted": 1, "comment_reply/no_device": 1})
}

func TestCatchUp_AKeyedKindWritesItsKeyForEveryRecipient(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	parent, replier := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	e := r.reply(t, parent, replier)
	d := r.emitReply(t, e)

	wantVerdict(t, handleReplyWith(t, r, nil, d, e), "", errs.VerdictAck)

	var key string
	if err := r.pool.QueryRow(t.Context(), `SELECT item_key FROM notifications WHERE user_id = $1`, parent.UUID()).
		Scan(&key); err != nil ||
		!strings.HasPrefix(key, "comment:") ||
		key != "comment:"+e.CommentID.String() {
		t.Fatalf("item key = %q, %v, want comment:%s", key, err, e.CommentID)
	}
}

func handleReplyWith(t *testing.T, r *pushRig, hints app.Hints, d bus.Delivery, e events.CommentCreated) error {
	t.Helper()
	pusher := app.NewPusher(r.uow, r.users, r.sender, r.ids, r.clock)
	if hints != nil {
		pusher = pusher.WithHints(hints)
	}
	ctx := observability.WithLogger(observability.WithActor(t.Context(), "system:"+d.Handler),
		observability.NewLogger(config.Config{Env: config.EnvTest}, r.logs))
	return app.Notify[events.CommentCreated]{Pusher: pusher, Kinds: []app.Kind[events.CommentCreated]{
		app.CommentReply{Users: r.users, Cabals: cabalsOf(e), Blocks: blocks{}},
	}}.Handle(ctx, d, e)
}

func TestCatchUp_AChatMentionCarriesItsAuthorAndCabal(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cabal := w.cabal("Degens")
	author := w.r.follower(t, "dana", "Dana")
	w.row(t, "chat_mention", "chat:"+w.r.ids.NewV7().String(), map[string]string{
		"author_id": author.String(), "cabal_id": cabal.String(),
	}, w.now.Add(-time.Minute))

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Replies) != 1 {
		t.Fatalf("got %+v, %v, want one mention", got, err)
	}
	item := got.Replies[0]
	if item.Kind != domain.CatchUpMention || item.Actor.Name != "Dana" || item.Cabal.Name != "Degens" ||
		item.Comment != nil || item.Quote != nil {
		t.Fatalf("item = %+v", item)
	}
}

type cancellingReplies struct {
	cancel context.CancelFunc
}

func (c cancellingReplies) ReplyCounts(context.Context, []ids.ProposalID) (map[ids.ProposalID]int, error) {
	c.cancel()
	return map[ids.ProposalID]int{}, nil
}

func TestCatchUp_AFailedClearedReadFailsTheRead(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cabal := w.cabal("AC Capital")
	w.gov.pending = []governanceport.CatchUpProposal{w.proposal(cabal, "AC Capital", "NVDA", governanceport.StatusOpen)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reads := app.NewReadCatchUp(app.CatchUpPorts{
		Governance: w.gov, Comments: w.comments, Replies: cancellingReplies{cancel}, Invites: w.invites,
		Cabals: w.cabals, Holdings: w.holdings, Assets: w.assets, Users: w.r.users, Reads: w.r.pool, Clock: w.r.clock,
	})

	_, err := reads.Read(ctx, w.viewer, w.now.Add(-12*time.Hour))

	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Read error = %v, want %s", err, errs.CodeDBUnavailable)
	}
}

func TestCatchUp_AnUnknownNotificationKindHasNoCatchUpKind(t *testing.T) {
	t.Parallel()
	if kind, ok := domain.CatchUpKindOf("deposit_credited"); ok || kind != "" {
		t.Fatalf("CatchUpKindOf = %q, %v, want none", kind, ok)
	}
}

func TestCatchUp_TheModuleExposesTheCountPort(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)

	if notify.New(module.Deps{Pool: r.pool, Clock: r.clock}).CatchUp() == nil {
		t.Fatal("CatchUp() = nil")
	}
}

func ptrValue(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.UUID{}
	}
	return *id
}

func TestCatchUp_ChatItemsFromSeveralAuthorsCarryThemAndOneWithoutACabalHasNone(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cabal := w.cabal("Degens")
	dana, eli := w.r.follower(t, "dana", "Dana"), w.r.follower(t, "eli", "Eli")
	w.row(t, "chat_mention", "chat:"+w.r.ids.NewV7().String(), map[string]string{
		"author_id": dana.String(), "cabal_id": cabal.String(),
	}, w.now.Add(-time.Minute))
	w.row(t, "chat_thread_reply", "chat:"+w.r.ids.NewV7().String(), map[string]string{"author_id": eli.String()},
		w.now.Add(-2*time.Minute))

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Replies) != 2 {
		t.Fatalf("got %+v, %v, want two chat items", got, err)
	}
	mention, thread := got.Replies[0], got.Replies[1]
	if mention.Actor.Name != "Dana" || mention.Cabal.Name != "Degens" || thread.Actor.Name != "Eli" ||
		thread.Cabal != nil {
		t.Fatalf("mention %+v, thread %+v", mention, thread)
	}
}

func TestCatchUp_AFailedAuthorReadFailsTheRead(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	author := w.r.follower(t, "dana", "Dana")
	w.row(t, "chat_mention", "chat:"+w.r.ids.NewV7().String(), map[string]string{"author_id": author.String()},
		w.now.Add(-time.Minute))
	reads := app.NewReadCatchUp(app.CatchUpPorts{
		Governance: w.gov, Comments: w.comments, Replies: w.replies, Invites: w.invites, Cabals: w.cabals,
		Holdings: w.holdings, Assets: w.assets, Users: users{err: errs.New(errs.CodeDBUnavailable, "test.users")},
		Reads: w.r.pool, Clock: w.r.clock,
	})

	_, err := reads.Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Read error = %v, want %s", err, errs.CodeDBUnavailable)
	}
}

func TestCatchUp_AResultFromACabalTheViewerLeftNamesItFromTheCard(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	left := ids.CabalIDFrom(w.r.ids.NewV7())
	result := w.proposal(left, "Left Club", "META", governanceport.StatusPassed)
	w.row(
		t,
		"proposal_result",
		"proposal:"+result.ID.String(),
		map[string]string{"outcome": "filled"},
		w.now.Add(-time.Minute),
	)

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Results) != 1 || got.Results[0].Cabal.Name != "Left Club" ||
		got.Results[0].Cabal.PictureURL != "" {
		t.Fatalf("got %+v, %v, want the result under the cabal's card name", got.Results, err)
	}
}

func TestCatchUp_AnInviteFallsBackToTheSymbolWhenTheAssetHasNoName(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.assets.assets[0].DisplayName = ""
	quant := ids.CabalIDFrom(w.r.ids.NewV7())
	w.invites.invites = []cabalport.PendingInvite{{
		ID: w.r.ids.NewV7(), CabalID: quant, CabalName: "Quant Kids", CreatedAt: w.now.Add(-time.Minute),
	}}
	w.holdings.top[quant] = chain.SolanaAddress(openAIMint)

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Invites) != 1 || got.Invites[0].Invite.TopHoldingName != "OPENAI" {
		t.Fatalf("got %+v, %v, want the symbol as the holding name", got.Invites, err)
	}
}

func TestCatchUp_AReplyWhoseParentIsHiddenHasNoQuote(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.cabal("AC Capital")
	hidden := w.r.ids.NewV7()
	reply := w.comment(w.person("Mia"), "Agreed", &hidden, w.now.Add(-time.Minute))
	w.row(t, "comment_reply", "comment:"+reply.CommentID.String(), map[string]string{}, reply.CreatedAt)

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Replies) != 1 || got.Replies[0].Quote != nil {
		t.Fatalf("got %+v, %v, want one reply without a quote", got.Replies, err)
	}
}

func TestCatchUp_AFailingPortFailsTheRead(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.port")
	for name, fail := range map[string]func(*catchUpWorld){
		"replies":   func(w *catchUpWorld) { w.replies.err = down },
		"cabals of": func(w *catchUpWorld) { w.cabals.ofErr = down },
		"cabals":    func(w *catchUpWorld) { w.cabals.cabalErr = down },
		"holdings":  func(w *catchUpWorld) { w.holdings.err = down },
		"assets":    func(w *catchUpWorld) { w.assets.err = down },
		"invites":   func(w *catchUpWorld) { w.invites.err = down },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newCatchUpWorld(t)
			cabal := w.cabal("AC Capital")
			w.gov.pending = []governanceport.CatchUpProposal{
				w.proposal(cabal, "AC Capital", "NVDA", governanceport.StatusOpen),
			}
			quant := ids.CabalIDFrom(w.r.ids.NewV7())
			w.invites.invites = []cabalport.PendingInvite{{ID: w.r.ids.NewV7(), CabalID: quant, CreatedAt: w.now}}
			w.holdings.top[quant] = chain.SolanaAddress(openAIMint)
			fail(w)

			_, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

			if errs.CodeOf(err) != errs.CodeDBUnavailable {
				t.Fatalf("Read error = %v, want %s", err, errs.CodeDBUnavailable)
			}
		})
	}
}

func TestCatchUp_ACabalMissingFromTheViewsIsSkippedAndAnEmptyHoldingMapNamesNothing(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.cabals.joined = []ids.CabalID{ids.CabalIDFrom(w.r.ids.NewV7())}
	quant := ids.CabalIDFrom(w.r.ids.NewV7())
	w.invites.invites = []cabalport.PendingInvite{
		{ID: w.r.ids.NewV7(), CabalID: quant, CabalName: "Quant", CreatedAt: w.now},
	}

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || len(got.Cabals) != 0 || len(got.Invites) != 1 || got.Invites[0].Invite.TopHoldingName != "" {
		t.Fatalf("got %+v, %v, want no cabals and an invite without a holding", got, err)
	}
}

func (w *catchUpWorld) readCancelling(t *testing.T, hook func(cancel context.CancelFunc)) error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	hook(cancel)
	_, err := w.reads().Read(ctx, w.viewer, w.now.Add(-12*time.Hour))
	return err
}

func TestCatchUp_AFailedRowsReadFailsTheRead(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)

	err := w.readCancelling(t, func(cancel context.CancelFunc) { w.gov.onCall = cancel })

	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Read error = %v, want %s", err, errs.CodeDBUnavailable)
	}
}

func TestCatchUp_AFailedStateReadFailsTheRead(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cabal := w.cabal("AC Capital")
	w.gov.pending = []governanceport.CatchUpProposal{
		w.proposal(cabal, "AC Capital", "NVDA", governanceport.StatusOpen),
	}

	err := w.readCancelling(t, func(cancel context.CancelFunc) { w.invites.onCall = cancel })

	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Read error = %v, want %s", err, errs.CodeDBUnavailable)
	}
}

func TestCatchUp_AFailedCommentReadFailsTheRead(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.comments.err = errs.New(errs.CodeDBUnavailable, "test.comments")
	w.row(t, "comment_reply", "comment:"+w.r.ids.NewV7().String(), map[string]string{}, w.now.Add(-time.Minute))

	_, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Read error = %v, want %s", err, errs.CodeDBUnavailable)
	}
}

func TestCatchUp_ArowWithUndecodableDataFailsTheRead(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.r.exec(t, `INSERT INTO notifications
		(id, user_id, kind, source_event_id, title, body, data, collapse_id, state, created_at, item_key)
		VALUES ($1, $2, 'chat_mention', $3, 't', 'b', '{"a": 1}', 'c', 'delivered', $4, 'chat:x')`,
		w.r.ids.NewV7(), w.viewer.UUID(), w.r.ids.NewV7(), w.now.Add(-time.Minute))

	_, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Read error = %v, want %s", err, errs.CodeDecodeFailed)
	}
}

func TestCatchUp_TwoRowsWithOneItemKeyAreOneItem(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	key := "chat:" + w.r.ids.NewV7().String()
	w.row(t, "chat_mention", key, map[string]string{}, w.now.Add(-time.Minute))
	w.row(t, "chat_thread_reply", key, map[string]string{}, w.now.Add(-2*time.Minute))

	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))

	if err != nil || got.Count != 1 {
		t.Fatalf("count %d, %v, want one item for one key", got.Count, err)
	}
}

func (w *catchUpWorld) http() adapters.HTTP {
	return adapters.HTTP{CatchUp: w.reads(), Clock: w.r.clock}
}

func (w *catchUpWorld) asViewer(ctx context.Context) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: w.viewer.String()})
}

func wireFixture(t *testing.T) api.GetMyCatchUp200JSONResponse {
	t.Helper()
	w := newCatchUpWorld(t)
	w.fixture(t)
	cleared := w.now.Add(-time.Minute)
	w.state(t, "invite:"+w.r.ids.NewV7().String(), nil, &cleared, domain.CatchUpItem{
		Kind: domain.CatchUpInvite, Invite: &domain.CatchUpInviteInfo{ID: uuid.NewString(), MemberCount: 3},
	})
	since := w.now.Add(-time.Hour)
	res, err := w.http().GetMyCatchUp(w.asViewer(t.Context()), api.GetMyCatchUpRequestObject{
		Params: api.GetMyCatchUpParams{Since: &since},
	})
	got, ok := res.(api.GetMyCatchUp200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("response %+v, %v", res, err)
	}
	return got
}

func TestCatchUp_HTTPServesTheCountsAndTheWaitingProposalOnTheWire(t *testing.T) {
	t.Parallel()
	got := wireFixture(t)

	if got.Count != 6 || len(got.Cabals) != 3 || len(got.Cleared) != 1 || got.Cleared[0].ClearedReason == nil {
		t.Fatalf("count %d, cabals %d, cleared %+v", got.Count, len(got.Cabals), got.Cleared)
	}
	waiting := got.Waiting[0].Proposal
	if waiting == nil || *waiting.UsdcMicros != 400_000_000 || waiting.TokenAmount != nil || waiting.Yes != 4 {
		t.Fatalf("waiting proposal = %+v", waiting)
	}
}

func TestCatchUp_HTTPServesTheReplyCommentAndQuoteOnTheWire(t *testing.T) {
	t.Parallel()
	reply := wireFixture(t).Replies[1]

	if reply.Comment == nil || reply.Comment.ParentCommentId == nil || reply.Comment.ProposalId == nil ||
		reply.Comment.CabalId == nil || !reply.Quote.IsYou || reply.Cabal == nil {
		t.Fatalf("reply = %+v comment %+v", reply, reply.Comment)
	}
}

func TestCatchUp_HTTPServesTheResultAndInviteOnTheWire(t *testing.T) {
	t.Parallel()
	got := wireFixture(t)

	result, invite := got.Results[0].Proposal, got.Invites[0]

	if result.Outcome == nil || *result.Outcome != api.Filled || result.YourChoice == nil || result.Filled == nil ||
		!*result.Filled {
		t.Fatalf("result proposal = %+v", result)
	}
	if invite.Invite.TopHoldingName == nil || *invite.Invite.TopHoldingName != "OpenAI" || invite.Actor == nil {
		t.Fatalf("invite = %+v", invite)
	}
}

func TestCatchUp_HTTPDefaultsSinceToADayAgoAndServesTheCount(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.fixture(t)

	res, err := w.http().GetMyCatchUp(w.asViewer(t.Context()), api.GetMyCatchUpRequestObject{})
	count, countErr := w.http().GetMyCatchUpCount(w.asViewer(t.Context()), api.GetMyCatchUpCountRequestObject{})

	if _, ok := res.(api.GetMyCatchUp200JSONResponse); err != nil || !ok {
		t.Fatalf("response %+v, %v", res, err)
	}
	if got, ok := count.(api.GetMyCatchUpCount200JSONResponse); countErr != nil || !ok || got.Count != 6 {
		t.Fatalf("count %+v, %v, want 6", count, countErr)
	}
}

func TestCatchUp_HTTPRefusesAnAnonymousCallerAndSurfacesAReadFailure(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	w.gov.err = errs.New(errs.CodeDBUnavailable, "test.gov")

	_, anon := w.http().GetMyCatchUp(t.Context(), api.GetMyCatchUpRequestObject{})
	_, anonCount := w.http().GetMyCatchUpCount(t.Context(), api.GetMyCatchUpCountRequestObject{})
	_, readErr := w.http().GetMyCatchUp(w.asViewer(t.Context()), api.GetMyCatchUpRequestObject{})
	_, countErr := w.http().GetMyCatchUpCount(w.asViewer(t.Context()), api.GetMyCatchUpCountRequestObject{})

	if errs.CodeOf(anon) != errs.CodeUnauthorized || errs.CodeOf(anonCount) != errs.CodeUnauthorized {
		t.Fatalf("anonymous errors = %v / %v, want %s", anon, anonCount, errs.CodeUnauthorized)
	}
	if errs.CodeOf(readErr) != errs.CodeDBUnavailable || errs.CodeOf(countErr) != errs.CodeDBUnavailable {
		t.Fatalf("read errors = %v / %v, want %s", readErr, countErr, errs.CodeDBUnavailable)
	}
}

func TestCatchUp_HTTPServesAPostReplyWithItsPost(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	you := governanceport.CatchUpPerson{ID: w.viewer, Name: "Andres"}
	mine := w.comment(you, "Nice sale", nil, w.now.Add(-time.Hour))
	reply := w.comment(w.person("Mia"), "Thanks", &mine.CommentID, w.now.Add(-time.Minute))
	w.row(t, "post_reply", "comment:"+reply.CommentID.String(), map[string]string{
		"item_kind": "trade", "item_label": "sale", "item_actor_name": "Logan",
	}, reply.CreatedAt)

	res, err := w.http().GetMyCatchUp(w.asViewer(t.Context()), api.GetMyCatchUpRequestObject{})

	got, ok := res.(api.GetMyCatchUp200JSONResponse)
	if err != nil || !ok || len(got.Replies) != 1 {
		t.Fatalf("response %+v, %v, want one reply", res, err)
	}
	if got.Replies[0].Post == nil || got.Replies[0].Post.Label != "sale" || got.Replies[0].Proposal != nil {
		t.Fatalf("response %+v, %v, want one reply on a post", res, err)
	}
}
