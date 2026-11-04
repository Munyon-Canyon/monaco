package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const proposalThesis = "put it up, small add before the print"

func TestFlow22e_WriteChatProposalMessage_OK(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	e := f.proposalEvent("buy")
	f.writeProposal(t, e, proposalThesis)
	f.writeProposal(t, e, proposalThesis)
	rows := f.proposalRows(t)
	if len(rows) != 1 || rows[0].ProposalId == nil || *rows[0].ProposalId != e.ProposalID ||
		rows[0].Body == nil || *rows[0].Body != proposalThesis || rows[0].Author.Id != f.member(0).UUID() {
		t.Fatalf("rows = %+v, want one proposal row by the proposer", rows)
	}
	published := f.rt.Published()
	if len(published) != 1 || published[0].Name != app.EventMessageCreated {
		t.Fatalf("published = %+v, want one message.created", published)
	}
	if f.posted(t) != 0 {
		t.Fatalf("a proposal row appended a chat.message_posted event")
	}
}

func TestChatProposal_AMissingProposalIsAcknowledged(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	e := f.proposalEvent("buy")
	heads := stubHeads{heads: map[ids.ProposalID]governanceport.ProposalHead{}}
	_, err := heads.ProposalHead(t.Context(), ids.ProposalIDFrom(e.ProposalID))
	if errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("head err = %v, want %s", err, errs.CodeProposalNotFound)
	}
	if err := f.proposalWriter(heads).Handle(t.Context(), e); err != nil {
		t.Fatalf("a proposal that is gone must be acknowledged, got %v", err)
	}
	if rows := f.proposalRows(t); len(rows) != 0 || len(f.rt.Published()) != 0 {
		t.Fatalf("rows = %+v, published = %d, want nothing written", rows, len(f.rt.Published()))
	}
}
