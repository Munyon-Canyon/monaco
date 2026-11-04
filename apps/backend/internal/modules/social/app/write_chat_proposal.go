package app

import (
	"context"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	ChatProposalBodyCap = 280
	ruleProposalKind    = "rule"
)

type ChatProposalThesis struct {
	Missing bool
	Thesis  string
}

type WriteChatProposalMessage struct {
	d     ChatDeps
	heads governanceport.ProposalHeads
}

func NewWriteChatProposalMessage(
	d ChatDeps, heads governanceport.ProposalHeads,
) *WriteChatProposalMessage {
	return &WriteChatProposalMessage{d: d, heads: heads}
}

func (h *WriteChatProposalMessage) Fetch(
	ctx context.Context, e events.ProposalCreated,
) (ChatProposalThesis, error) {
	if e.Kind == ruleProposalKind {
		return ChatProposalThesis{}, nil
	}
	head, err := h.heads.ProposalHead(ctx, ids.ProposalIDFrom(e.ProposalID))
	switch {
	case errs.CodeOf(err) == errs.CodeProposalNotFound:
		return ChatProposalThesis{Missing: true}, nil
	case err != nil:
		return ChatProposalThesis{}, err
	}
	return ChatProposalThesis{Thesis: head.Thesis}, nil
}

func (h *WriteChatProposalMessage) Apply(
	ctx context.Context, tx db.Tx, e events.ProposalCreated, fetched ChatProposalThesis, at time.Time,
) error {
	const op = "social.WriteChatProposalMessage"
	if fetched.Missing {
		return nil
	}
	body := chatProposalBody(fetched.Thesis)
	id := h.d.IDs.NewV7()
	row, err := sqlc.New(tx.Queries()).InsertChatProposal(ctx, sqlc.InsertChatProposalParams{
		ID: id, CabalID: e.CabalID, AuthorID: e.ProposerID, Body: body,
		ProposalID: e.ProposalID,
		DedupeKey:  "proposal:" + e.ProposalID.String(), CreatedAt: at.UTC(),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	if row.ID != id {
		return nil
	}
	written := ChatMessage{
		ID: row.ID, Kind: domain.ChatKindProposal, CabalID: ids.CabalIDFrom(row.CabalID),
		AuthorID: ids.UserIDFrom(e.ProposerID), Body: body, CreatedAt: row.CreatedAt.UTC(), ProposalID: e.ProposalID,
	}
	tx.AfterCommit(func(ctx context.Context) { h.d.Publish.created(ctx, written, nil) })
	return nil
}

func (h *WriteChatProposalMessage) Handle(ctx context.Context, e events.ProposalCreated) error {
	fetched, err := h.Fetch(ctx, e)
	if err != nil {
		return err
	}
	at := h.d.Clock.Now()
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error { return h.Apply(ctx, tx, e, fetched, at) })
}

func chatProposalBody(thesis string) string {
	body := strings.TrimSpace(thesis)
	if runes := []rune(body); len(runes) > ChatProposalBodyCap {
		return strings.TrimSpace(string(runes[:ChatProposalBodyCap]))
	}
	return body
}
