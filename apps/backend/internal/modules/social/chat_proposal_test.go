package social_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

type stubHeads struct {
	heads map[ids.ProposalID]governanceport.ProposalHead
	err   error
}

func (h stubHeads) ProposalHead(_ context.Context, id ids.ProposalID) (governanceport.ProposalHead, error) {
	if h.err != nil {
		return governanceport.ProposalHead{}, h.err
	}
	head, ok := h.heads[id]
	if !ok {
		return governanceport.ProposalHead{}, errs.New(errs.CodeProposalNotFound, "test")
	}
	return head, nil
}

func (f chatRoutes) proposalWriter(heads stubHeads) *app.WriteChatProposalMessageHandler {
	deps := module.Deps{Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock}
	return social.New(deps, social.WithUsers(f.users), social.WithRealtime(f.rt), social.WithProposalHeads(heads)).
		WriteChatProposalMessage()
}

func (f chatRoutes) proposalEvent(kind string) events.ProposalCreated {
	return events.ProposalCreated{
		V: 1, ProposalID: f.deps.IDs.NewV7(), CabalID: f.cabal.ID.UUID(), ProposerID: f.member(0).UUID(), Kind: kind,
		Symbol: "NVDA", ExpiresAt: f.clock.Now().Add(24 * time.Hour),
	}
}

func (f chatRoutes) writeProposal(t *testing.T, e events.ProposalCreated, thesis string) {
	t.Helper()
	heads := stubHeads{heads: map[ids.ProposalID]governanceport.ProposalHead{
		ids.ProposalIDFrom(e.ProposalID): {Kind: e.Kind, Symbol: e.Symbol, Thesis: thesis},
	}}
	if err := f.proposalWriter(heads).Handle(t.Context(), e); err != nil {
		t.Fatal(err)
	}
}

func (f chatRoutes) proposalRows(t *testing.T) []api.ChatMessage {
	t.Helper()
	var out []api.ChatMessage
	for _, m := range f.mustChannel(t) {
		if m.Kind == api.ChatMessageKind("proposal") {
			out = append(out, m)
		}
	}
	return out
}

func TestChatProposal_redeliveryWritesOneRowAndPublishesOnce(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	e := f.proposalEvent("buy")
	f.writeProposal(t, e, "  put it up  ")
	f.writeProposal(t, e, "  put it up  ")
	rows := f.proposalRows(t)
	if len(rows) != 1 || rows[0].ProposalId == nil || *rows[0].ProposalId != e.ProposalID ||
		rows[0].Author.Id != f.member(0).UUID() || rows[0].Body == nil || *rows[0].Body != "put it up" {
		t.Fatalf("rows = %+v, want one proposal row by the proposer with the trimmed thesis", rows)
	}
	published := f.rt.Published()
	if len(published) != 1 || published[0].Name != app.EventMessageCreated {
		t.Fatalf("published = %+v, want one message.created", published)
	}
	if f.posted(t) != 0 {
		t.Fatalf("a proposal row appended a chat.message_posted event")
	}
}

func TestChatProposal_aLongThesisIsCappedAt280Characters(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.writeProposal(t, f.proposalEvent("sell"), strings.Repeat("é", 300))
	f.clock.Advance(time.Second)
	f.writeProposal(t, f.proposalEvent("buy"), strings.Repeat("a", 279)+" bcd")
	rows := f.proposalRows(t)
	if len(rows) != 2 || len([]rune(*rows[1].Body)) != app.ChatProposalBodyCap ||
		*rows[0].Body != strings.Repeat("a", 279) {
		t.Fatalf("rows = %+v, want the first capped at 280 runes and the second cut and trimmed to 279", rows)
	}
}

func TestChatProposal_noThesisAndRuleProposalsHaveNoBody(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.writeProposal(t, f.proposalEvent("buy"), "  ")
	f.clock.Advance(time.Second)
	f.writeProposal(t, f.proposalEvent("rule"), "a thesis the rule row ignores")
	rows := f.proposalRows(t)
	if len(rows) != 2 || rows[0].Body != nil || rows[1].Body != nil {
		t.Fatalf("rows = %+v, want two proposal rows with a null body", rows)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE kind = 'proposal' AND body IS NULL`); n != 2 {
		t.Fatalf("%d stored rows have a null body, want 2", n)
	}
}

func TestChatProposal_countsAsUnreadForOthersNotTheProposer(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.writeProposal(t, f.proposalEvent("buy"), "go")
	if got := f.unread(t, f.member(1), f.cabal.ID)[f.cabal.ID]; got != 1 {
		t.Fatalf("unread for another member = %d, want 1", got)
	}
	if got := f.unread(t, f.member(0), f.cabal.ID)[f.cabal.ID]; got != 0 {
		t.Fatalf("unread for the proposer = %d, want 0", got)
	}
}

func TestChatProposal_aProposalThatIsGoneWritesNothing(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	if err := f.proposalWriter(stubHeads{}).Handle(t.Context(), f.proposalEvent("buy")); err != nil {
		t.Fatalf("err = %v, want the event acknowledged", err)
	}
	if n := f.count(
		t,
		`SELECT count(*) FROM cabal_messages WHERE kind = 'proposal'`,
	); n != 0 ||
		len(f.rt.Published()) != 0 {
		t.Fatalf("%d rows written for a missing proposal", n)
	}
}

func TestChatProposal_aFailedReadIsRetried(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	down := errs.New(errs.CodeDBUnavailable, "test")
	if err := f.proposalWriter(stubHeads{err: down}).
		Handle(t.Context(), f.proposalEvent("buy")); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("err = %v, want the read error", err)
	}
}

func TestChatProposal_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	if _, err := f.pool.Exec(
		t.Context(), `ALTER TABLE cabal_messages RENAME COLUMN dedupe_key TO dedupe_gone`,
	); err != nil {
		t.Fatal(err)
	}
	e := f.proposalEvent("buy")
	heads := stubHeads{heads: map[ids.ProposalID]governanceport.ProposalHead{ids.ProposalIDFrom(e.ProposalID): {}}}
	if err := f.proposalWriter(heads).Handle(t.Context(), e); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}
