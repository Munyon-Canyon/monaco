package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	socialport "github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/notifyapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type stateRow struct {
	read, cleared, kept *time.Time
	reason              *string
	snapshot            []byte
}

func (s stateRow) clearedAs(reason string, at time.Time) bool {
	return s.reason != nil && *s.reason == reason && s.cleared != nil && s.cleared.Equal(at)
}

func (w *catchUpWorld) stateOf(t *testing.T, user ids.UserID, key string) (stateRow, bool) {
	t.Helper()
	var s stateRow
	err := w.r.pool.QueryRow(t.Context(), `SELECT read_at, cleared_at, kept_at, cleared_reason, snapshot
		FROM catch_up_state WHERE user_id = $1 AND item_key = $2`, user.UUID(), key).
		Scan(&s.read, &s.cleared, &s.kept, &s.reason, &s.snapshot)
	return s, err == nil
}

func (w *catchUpWorld) writes(hints app.Hints) *app.CatchUpWrites {
	return app.NewCatchUpWrites(w.r.uow, w.reads(), hints, w.r.clock)
}

func (w *catchUpWorld) invite(name string) (cabalport.PendingInvite, string) {
	quant := ids.CabalIDFrom(w.r.ids.NewV7())
	inv := cabalport.PendingInvite{
		ID: w.r.ids.NewV7(), CabalID: quant, CabalName: name, MemberCount: 5, InviterName: "Sam",
		CreatedAt: w.now.Add(-30 * time.Minute), ExpiresAt: w.now.Add(24 * time.Hour),
	}
	w.invites.invites = append(w.invites.invites, inv)
	w.holdings.top[quant] = chain.SolanaAddress(openAIMint)
	return inv, domain.ItemKeyInvitePrefix + inv.ID.String()
}

func (w *catchUpWorld) reply(t *testing.T) (socialCommentID uuid.UUID, key string) {
	t.Helper()
	c := w.comment(w.person("Mia"), "hi", nil, w.now.Add(-time.Hour))
	key = domain.ItemKeyCommentPrefix + c.CommentID.String()
	w.row(t, "comment_reply", key, map[string]string{}, c.CreatedAt)
	return c.CommentID, key
}

func (w *catchUpWorld) read(t *testing.T) domain.CatchUp {
	t.Helper()
	got, err := w.reads().Read(t.Context(), w.viewer, w.now.Add(-12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCatchUpWrites_ClearStoresTheItemAsItsSnapshotAndListsItUnderCleared(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	_, key := w.invite("Quant Kids")
	hints := &hintRecorder{}

	err := w.writes(hints).Clear(t.Context(), app.ClearItem{User: w.viewer, Key: key})

	got := w.read(t)
	state, found := w.stateOf(t, w.viewer, key)
	if err != nil || got.Count != 0 || len(got.Cleared) != 1 || got.Cleared[0].ClearedReason != domain.ClearedManual ||
		got.Cleared[0].Cabal.Name != "Quant Kids" || got.Cleared[0].Invite.TopHoldingName != "OpenAI" ||
		!found || state.snapshot == nil {
		t.Fatalf("clear = %v, read %+v, state %+v, want one cleared invite with its snapshot", err, got, state)
	}
	if want := "user." + w.viewer.String() + ".catch_up_changed"; len(hints.published()) != 1 ||
		hints.published()[0] != want {
		t.Fatalf("hints = %v, want [%s]", hints.published(), want)
	}
}

func TestCatchUpWrites_ANewerSourceRowReopensAClearedItemUnread(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	_, key := w.reply(t)
	if err := w.writes(&hintRecorder{}).Clear(t.Context(), app.ClearItem{User: w.viewer, Key: key}); err != nil {
		t.Fatal(err)
	}
	if got := w.read(t); got.Count != 0 {
		t.Fatalf("count after clear = %d, want 0", got.Count)
	}
	second := w.comment(w.person("Mia"), "again", nil, w.now.Add(time.Hour))
	w.row(t, "comment_reply", "comment:"+second.CommentID.String(), map[string]string{}, second.CreatedAt)
	w.row(t, "comment_reply", key, map[string]string{}, w.now.Add(2*time.Hour))

	got := w.read(t)

	if got.Count != 2 || !got.Replies[0].Unread {
		t.Fatalf("after a newer row: %+v, want both replies active and unread", got.Replies)
	}
}

func TestCatchUpWrites_ReadMarksAnItemReadAndKeepKeepsItActive(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	_, readKey := w.reply(t)
	_, keepKey := w.reply(t)
	hints := &hintRecorder{}
	writes := w.writes(hints)

	readErr := writes.Read(t.Context(), app.ReadItem{User: w.viewer, Key: readKey})
	keepErr := writes.Keep(t.Context(), app.KeepItem{User: w.viewer, Key: keepKey})

	got := w.read(t)
	read, _ := w.stateOf(t, w.viewer, readKey)
	kept, _ := w.stateOf(t, w.viewer, keepKey)
	if readErr != nil || keepErr != nil || got.Count != 2 || got.Replies[0].Unread || got.Replies[1].Unread ||
		read.read == nil || read.kept != nil || kept.kept == nil || kept.read == nil || kept.cleared != nil ||
		len(hints.published()) != 2 {
		t.Fatalf("read %v keep %v: replies %+v, read %+v, kept %+v", readErr, keepErr, got.Replies, read, kept)
	}
}

func TestCatchUpWrites_AnInactiveOrForeignKeyIsNotFound(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	writes := w.writes(&hintRecorder{})
	key := domain.ItemKeyCommentPrefix + w.r.ids.NewV7().String()
	for name, run := range map[string]func() error{
		"read":  func() error { return writes.Read(t.Context(), app.ReadItem{User: w.viewer, Key: key}) },
		"clear": func() error { return writes.Clear(t.Context(), app.ClearItem{User: w.viewer, Key: key}) },
		"keep":  func() error { return writes.Keep(t.Context(), app.KeepItem{User: w.viewer, Key: key}) },
	} {
		if got := errs.CodeOf(run()); got != errs.CodeNotFound {
			t.Errorf("%s of an unknown key = %q, want not_found", name, got)
		}
	}
}

func TestCatchUpWrites_ARenderFailureFailsTheWriteAndStoresNothing(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	_, key := w.reply(t)
	w.comments.err = errs.New(errs.CodeInternal, "test.comments")
	writes := w.writes(&hintRecorder{})

	err := writes.Clear(t.Context(), app.ClearItem{User: w.viewer, Key: key})
	w.comments.err = nil
	w.gov.err = errs.New(errs.CodeInternal, "test.governance")
	pendingErr := writes.Clear(t.Context(), app.ClearItem{User: w.viewer, Key: key})

	if _, found := w.stateOf(t, w.viewer, key); err == nil || pendingErr == nil || found {
		t.Fatalf("clear = %v, %v, state stored %v, want failures and nothing stored", err, pendingErr, found)
	}
}

func TestCatchUpWrites_AFailedWriteIsReportedAsUnavailable(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	_, key := w.reply(t)
	w.r.exec(t, `ALTER TABLE catch_up_state ADD CONSTRAINT never CHECK (false) NOT VALID`)
	writes := w.writes(&hintRecorder{})

	for name, run := range map[string]func() error{
		"read":  func() error { return writes.Read(t.Context(), app.ReadItem{User: w.viewer, Key: key}) },
		"clear": func() error { return writes.Clear(t.Context(), app.ClearItem{User: w.viewer, Key: key}) },
		"keep":  func() error { return writes.Keep(t.Context(), app.KeepItem{User: w.viewer, Key: key}) },
	} {
		if got := errs.CodeOf(run()); got != errs.CodeDBUnavailable {
			t.Errorf("%s with the table refusing writes = %q, want db_unavailable", name, got)
		}
	}
}

func (w *catchUpWorld) votedCard(t *testing.T) (events.VoteCast, governanceport.CatchUpProposal) {
	t.Helper()
	cabal := w.cabal("AC Capital")
	card := w.proposal(cabal, "AC Capital", "NVDA", governanceport.StatusPassed)
	cast := events.VoteCast{
		V: 1, ProposalID: card.ID.UUID(), CabalID: cabal.UUID(), VoterID: w.viewer.UUID(), Choice: "yes",
		CastAt: w.now.Add(-time.Minute),
	}
	return cast, card
}

func (w *catchUpWorld) clears(hints app.Hints) *app.CatchUpClears {
	return app.NewCatchUpClears(w.reads(), w.members(), hints)
}

func TestCatchUpClears_AVoteClearsTheProposalWithTheVotersChoiceAsItsSnapshot(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cast, card := w.votedCard(t)
	w.replies.counts[card.ID] = 3
	hints := &hintRecorder{}

	w.dispatch(t, w.clears(hints), cast)

	key := domain.ItemKeyProposalPrefix + card.ID.String()
	state, found := w.stateOf(t, w.viewer, key)
	var item domain.CatchUpItem
	if !found || !state.clearedAs(domain.ClearedVoted, cast.CastAt) || json.Unmarshal(state.snapshot, &item) != nil ||
		item.Proposal.YourChoice != "yes" || item.Proposal.ReplyCount != 3 || item.Cabal.Name != "AC Capital" {
		t.Fatalf("state %+v item %+v, want a voted clear snapshotting the card with the choice", state, item)
	}
	got := w.read(t)
	if len(got.Cleared) != 1 || got.Cleared[0].ClearedReason != domain.ClearedVoted || len(hints.published()) != 1 {
		t.Fatalf("cleared = %+v, hints %v, want the voted proposal and one hint", got.Cleared, hints.published())
	}
}

func TestCatchUpClears_AVoteOnAProposalTheViewerCannotSeeWritesNothing(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cast, card := w.votedCard(t)
	delete(w.gov.cards, card.ID)

	w.dispatch(t, w.clears(&hintRecorder{}), cast)

	if _, found := w.stateOf(t, w.viewer, domain.ItemKeyProposalPrefix+card.ID.String()); found {
		t.Fatal("a vote with no card stored a state row")
	}
}

func TestCatchUpClears_ARetractedVoteReopensTheProposalAndIgnoresAHandCleared(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cast, card := w.votedCard(t)
	retract := events.VoteRetracted{
		V: 1, ProposalID: cast.ProposalID, CabalID: cast.CabalID, VoterID: cast.VoterID, Choice: "yes",
		RetractedAt: w.now,
	}
	key := domain.ItemKeyProposalPrefix + card.ID.String()
	hints := &hintRecorder{}

	w.dispatch(t, w.clears(hints), cast, retract)

	state, found := w.stateOf(t, w.viewer, key)
	if !found || state.cleared != nil || state.reason != nil || state.snapshot != nil || len(hints.published()) != 2 {
		t.Fatalf("state %+v, hints %v, want the clear undone", state, hints.published())
	}

	otherID := w.r.ids.NewV7()
	other := domain.ItemKeyProposalPrefix + otherID.String()
	w.r.exec(t, `INSERT INTO catch_up_state (user_id, item_key, cleared_at, cleared_reason, snapshot, updated_at)
		VALUES ($1, $2, $3, 'cleared', '{}', $3)`, w.viewer.UUID(), other, w.now.Add(-time.Hour))
	retract.ProposalID = otherID
	w.dispatch(t, w.clears(hints), retract)
	if kept, _ := w.stateOf(t, w.viewer, other); kept.cleared == nil || *kept.reason != domain.ClearedManual {
		t.Fatalf("hand-cleared state after a retraction = %+v, want it kept cleared", kept)
	}
}

func TestCatchUpClears_ACastDeliveredAfterItsRetractionLeavesTheProposalWaiting(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cast, card := w.votedCard(t)
	retract := events.VoteRetracted{
		V: 1, ProposalID: cast.ProposalID, CabalID: cast.CabalID, VoterID: cast.VoterID, Choice: "yes",
		RetractedAt: w.now,
	}

	w.dispatch(t, w.clears(&hintRecorder{}), retract, cast)

	state, found := w.stateOf(t, w.viewer, domain.ItemKeyProposalPrefix+card.ID.String())
	if !found || state.cleared != nil {
		t.Fatalf("state %+v, want the older cast ignored behind its retraction", state)
	}
}

func TestCatchUpClears_AReplyInTheThreadClearsTheActorsCommentItemsOnly(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	threadID, thread := w.reply(t)
	_, other := w.reply(t)
	feed := w.comments.previews[threadID].FeedItemID
	w.r.exec(t, `UPDATE notifications SET data = jsonb_build_object('feed_item_id', $1::text) WHERE item_key = $2`,
		feed.String(), thread)
	hints := &hintRecorder{}
	e := events.CommentCreated{V: 1, CommentID: w.r.ids.NewV7(), FeedObjectID: feed, AuthorID: w.viewer.UUID()}

	w.dispatch(t, w.clears(hints), e)

	cleared, found := w.stateOf(t, w.viewer, thread)
	_, otherFound := w.stateOf(t, w.viewer, other)
	if !found || *cleared.reason != domain.ClearedReplied || cleared.snapshot == nil || otherFound ||
		len(hints.published()) != 1 {
		t.Fatalf("thread state %+v, other stored %v, hints %v", cleared, otherFound, hints.published())
	}
}

func TestCatchUpClears_AReplyWithNothingToClearWritesNothing(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	hints := &hintRecorder{}
	e := events.CommentCreated{
		V: 1, CommentID: w.r.ids.NewV7(), FeedObjectID: w.r.ids.NewV7(), AuthorID: w.viewer.UUID(),
	}

	w.dispatch(t, w.clears(hints), e)

	if len(hints.published()) != 0 {
		t.Fatalf("hints = %v, want none", hints.published())
	}
}

func TestCatchUpClears_AThreadItemTheReadCannotRenderIsSkipped(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	_, key := w.reply(t)
	feed := w.r.ids.NewV7()
	w.r.exec(t, `UPDATE notifications SET data = jsonb_build_object('feed_item_id', $1::text) WHERE item_key = $2`,
		feed.String(), key)
	w.comments.previews = map[uuid.UUID]socialport.CommentPreview{}
	e := events.CommentCreated{V: 1, CommentID: w.r.ids.NewV7(), FeedObjectID: feed, AuthorID: w.viewer.UUID()}

	w.dispatch(t, w.clears(&hintRecorder{}), e)

	if _, found := w.stateOf(t, w.viewer, key); found {
		t.Fatal("an item that does not render was cleared")
	}
}

func TestCatchUpClears_AcceptingAnInviteClearsItWithTheCabalAsItWasAndNoOtherDecisionDoes(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	inv, key := w.invite("Quant Kids")
	w.lookup.byID = map[uuid.UUID]cabalport.PendingInvite{inv.ID: inv}
	w.invites.invites = nil
	decided := func(direction, decision string) events.CabalAccessDecided {
		return events.CabalAccessDecided{
			V: 1, RequestID: inv.ID, CabalID: inv.CabalID.UUID(), UserID: w.viewer.UUID(), Direction: direction,
			Decision: decision,
		}
	}

	w.dispatch(t, w.clears(&hintRecorder{}), decided("invite", "declined"), decided("request", "accepted"))
	if _, found := w.stateOf(t, w.viewer, key); found {
		t.Fatal("a declined or non-invite decision cleared the invite")
	}
	w.dispatch(t, w.clears(&hintRecorder{}), decided("invite", "accepted"))

	got := w.read(t)
	if len(got.Cleared) != 1 || got.Cleared[0].ClearedReason != domain.ClearedJoined ||
		got.Cleared[0].Invite.MemberCount != 5 || got.Cleared[0].Invite.TopHoldingName != "OpenAI" {
		t.Fatalf("cleared = %+v, want the joined invite as it was", got.Cleared)
	}
}

func TestCatchUpClears_AnAcceptedInviteTheLookupDoesNotKnowWritesNothing(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	e := events.CabalAccessDecided{
		V: 1, RequestID: w.r.ids.NewV7(), UserID: w.viewer.UUID(), Direction: "invite", Decision: "accepted",
	}

	w.dispatch(t, w.clears(&hintRecorder{}), e)

	if _, found := w.stateOf(t, w.viewer, domain.ItemKeyInvitePrefix+e.RequestID.String()); found {
		t.Fatal("an unknown invite was cleared")
	}
}

func TestCatchUpClears_AClosedProposalAndANewInviteHintTheirMembersWithoutWritingState(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	other := w.r.follower(t, "bea", "Bea")
	w.cabalMembers = []ids.UserID{w.viewer, other}
	hints := &hintRecorder{}
	cabal, proposal := w.r.ids.NewV7(), w.r.ids.NewV7()

	w.dispatch(
		t,
		w.clears(hints),
		events.ProposalPassed{V: 1, ProposalID: proposal, CabalID: cabal},
		events.ProposalFailed{V: 1, ProposalID: proposal, CabalID: cabal},
		events.ProposalExpired{V: 1, ProposalID: proposal, CabalID: cabal},
		events.ProposalWithdrawn{V: 1, ProposalID: proposal, CabalID: cabal},
		events.ProposalVoided{V: 1, ProposalID: proposal, CabalID: cabal},
		events.CabalAccessRequested{
			V:         1,
			RequestID: proposal,
			CabalID:   cabal,
			UserID:    other.UUID(),
			Direction: "invite",
		},
		events.CabalAccessRequested{
			V:         1,
			RequestID: proposal,
			CabalID:   cabal,
			UserID:    other.UUID(),
			Direction: "request",
		},
	)

	var rows int
	if err := w.r.pool.QueryRow(t.Context(), `SELECT count(*) FROM catch_up_state`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if len(hints.published()) != 11 || rows != 0 {
		t.Fatalf(
			"hints = %v, state rows %d, want 5 closes to 2 members and one invite hint, no state",
			hints.published(),
			rows,
		)
	}
}

func TestCatchUpClears_APortFailureIsReturnedByEachFetch(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cast, _ := w.votedCard(t)
	clears := w.clears(&hintRecorder{})
	boom := errs.New(errs.CodeInternal, "test.down")

	w.gov.cardsErr = boom
	_, cardErr := clears.OnVoteCast(t.Context(), cast)
	w.gov.cardsErr = nil
	w.replies.err = boom
	_, replyErr := clears.OnVoteCast(t.Context(), cast)
	w.replies.err = nil
	w.holdings.err = boom
	inv, _ := w.invite("Quant Kids")
	w.lookup.byID = map[uuid.UUID]cabalport.PendingInvite{inv.ID: inv}
	_, holdingErr := clears.OnInviteAccepted(t.Context(), events.CabalAccessDecided{
		RequestID: inv.ID, UserID: w.viewer.UUID(), Direction: "invite", Decision: "accepted",
	})
	w.holdings.err = nil
	w.invites.invites = nil
	w.lookup.err = boom
	_, inviteErr := clears.OnInviteAccepted(t.Context(), events.CabalAccessDecided{
		RequestID: w.r.ids.NewV7(), UserID: w.viewer.UUID(), Direction: "invite", Decision: "accepted",
	})
	w.comments.err = boom
	_, key := w.reply(t)
	feed := w.r.ids.NewV7()
	w.r.exec(t, `UPDATE notifications SET data = jsonb_build_object('feed_item_id', $1::text) WHERE item_key = $2`,
		feed.String(), key)
	_, commentErr := clears.OnCommentCreated(t.Context(), events.CommentCreated{
		FeedObjectID: feed, AuthorID: w.viewer.UUID(),
	})

	for name, err := range map[string]error{
		"cards": cardErr, "replies": replyErr, "holdings": holdingErr, "invites": inviteErr, "comments": commentErr,
	} {
		if !errors.Is(err, boom) {
			t.Errorf("%s port failure = %v, want it returned", name, err)
		}
	}
}

func TestCatchUpClears_AFailedWriteIsReportedAsUnavailable(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	cast, _ := w.votedCard(t)
	clears := w.clears(&hintRecorder{})
	got, err := clears.OnVoteCast(t.Context(), cast)
	if err != nil || len(got) != 1 {
		t.Fatalf("fetch = %+v, %v", got, err)
	}
	w.r.exec(t, `ALTER TABLE catch_up_state ADD CONSTRAINT never CHECK (false) NOT VALID`)

	applyErr := w.r.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return clears.Apply(ctx, tx, got, w.now)
	})
	reopenErr := w.r.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return clears.Reopen(ctx, tx, events.VoteRetracted{ProposalID: cast.ProposalID, VoterID: cast.VoterID})
	})

	if errs.CodeOf(applyErr) != errs.CodeDBUnavailable || errs.CodeOf(reopenErr) != errs.CodeDBUnavailable {
		t.Fatalf("apply %v, reopen %v, want db_unavailable", applyErr, reopenErr)
	}
}

func TestCatchUpClears_AFailedThreadReadIsReportedAsUnavailable(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := w.clears(&hintRecorder{}).OnCommentCreated(ctx, events.CommentCreated{AuthorID: w.viewer.UUID()})

	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("thread read on a cancelled context = %v, want db_unavailable", err)
	}
}

func TestCatchUpWrites_HTTPRoutesWriteAnswer204AndRefuseAnAnonymousCaller(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	_, readKey := w.reply(t)
	_, keepKey := w.reply(t)
	_, clearKey := w.reply(t)
	h := adapters.HTTP{CatchUpWrites: w.writes(&hintRecorder{})}
	ctx := w.asViewer(t.Context())

	read, readErr := h.ReadCatchUpItem(ctx, api.ReadCatchUpItemRequestObject{Key: readKey})
	keep, keepErr := h.KeepCatchUpItem(ctx, api.KeepCatchUpItemRequestObject{Key: keepKey})
	cleared, clearErr := h.ClearCatchUpItem(ctx, api.ClearCatchUpItemRequestObject{Key: clearKey})

	if readErr != nil || keepErr != nil || clearErr != nil ||
		read != (api.ReadCatchUpItem204Response{}) || keep != (api.KeepCatchUpItem204Response{}) ||
		cleared != (api.ClearCatchUpItem204Response{}) {
		t.Fatalf("responses %v %v %v, errors %v %v %v", read, keep, cleared, readErr, keepErr, clearErr)
	}
	_, readDenied := h.ReadCatchUpItem(t.Context(), api.ReadCatchUpItemRequestObject{Key: readKey})
	_, keepDenied := h.KeepCatchUpItem(t.Context(), api.KeepCatchUpItemRequestObject{Key: keepKey})
	_, clearDenied := h.ClearCatchUpItem(t.Context(), api.ClearCatchUpItemRequestObject{Key: clearKey})
	for _, err := range []error{readDenied, keepDenied, clearDenied} {
		if errs.CodeOf(err) != errs.CodeUnauthorized {
			t.Errorf("anonymous write = %v, want unauthorized", err)
		}
	}
	_, missingRead := h.ReadCatchUpItem(ctx, api.ReadCatchUpItemRequestObject{Key: "comment:nope"})
	_, missingKeep := h.KeepCatchUpItem(ctx, api.KeepCatchUpItemRequestObject{Key: "comment:nope"})
	_, missingClear := h.ClearCatchUpItem(ctx, api.ClearCatchUpItemRequestObject{Key: "comment:nope"})
	for _, err := range []error{missingRead, missingKeep, missingClear} {
		if errs.CodeOf(err) != errs.CodeNotFound {
			t.Errorf("unknown key = %v, want not_found", err)
		}
	}
}

func TestCatchUpWrites_HTTPServesTheReactionChipsOnTheWire(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	id, _ := w.reply(t)
	preview := w.comments.previews[id]
	preview.Reactions = []socialport.Reaction{{Emoji: "❤️", Count: 1, Mine: true}}
	preview.ReactionCount = 1
	w.comments.previews[id] = preview
	since := w.now.Add(-time.Hour)

	res, err := w.http().GetMyCatchUp(w.asViewer(t.Context()), api.GetMyCatchUpRequestObject{
		Params: api.GetMyCatchUpParams{Since: &since},
	})

	got, ok := res.(api.GetMyCatchUp200JSONResponse)
	if err != nil || !ok || len(got.Replies) != 1 {
		t.Fatalf("response %+v, %v", res, err)
	}
	comment := got.Replies[0].Comment
	if comment.ReactionCount != 1 || len(comment.Reactions) != 1 || comment.Reactions[0].Emoji != "❤️" ||
		comment.Reactions[0].Count != 1 || !comment.Reactions[0].Mine {
		t.Fatalf("comment reactions = %+v (%d), want the heart chip", comment.Reactions, comment.ReactionCount)
	}
}

func TestCatchUpWrites_ADecorationFailureFailsTheClearAndStoresNothing(t *testing.T) {
	t.Parallel()
	w := newCatchUpWorld(t)
	pending := w.proposal(w.cabal("AC Capital"), "AC Capital", "NVDA", governanceport.StatusOpen)
	w.gov.pending = []governanceport.CatchUpProposal{pending}
	w.replies.err = errs.New(errs.CodeInternal, "test.replies")
	key := domain.ItemKeyProposalPrefix + pending.ID.String()

	err := w.writes(&hintRecorder{}).Clear(t.Context(), app.ClearItem{User: w.viewer, Key: key})

	if _, found := w.stateOf(t, w.viewer, key); err == nil || found {
		t.Fatalf("clear = %v, state stored %v, want the decoration failure and nothing stored", err, found)
	}
}
