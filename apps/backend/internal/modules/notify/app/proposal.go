package app

import (
	"cmp"
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	unnamedProposer = "Someone"
	ruleKind        = "rule"
	voteCategory    = "VOTE"
)

type (
	Ballots       = governanceport.Ballots
	ProposalHeads = governanceport.ProposalHeads
)

type ProposalCreated struct {
	Cabals  Cabals
	Users   Users
	Assets  Assets
	Ballots Ballots
	Heads   ProposalHeads
	Clock   clock.Clock
}

func (ProposalCreated) Name() string { return "proposal_created" }

func (k ProposalCreated) Recipients(ctx context.Context, e events.ProposalCreated) ([]ids.UserID, error) {
	if e.Kind == ruleKind {
		return nil, nil
	}
	return memberIDs(ctx, k.Cabals, &e.CabalID)
}

func (k ProposalCreated) Render(ctx context.Context, e events.ProposalCreated, to ids.UserID) (Message, error) {
	action, err := side(e.Kind)
	if err != nil {
		return Message{}, err
	}
	asset, err := labelOf(ctx, k.Assets, e.Symbol)
	if err != nil {
		return Message{}, err
	}
	view, err := k.Cabals.Cabal(ctx, ids.CabalIDFrom(e.CabalID))
	if err != nil {
		return Message{}, err
	}
	proposer, err := personSender(ctx, k.Users, ids.UserIDFrom(e.ProposerID))
	if err != nil {
		return Message{}, err
	}
	head, err := k.Heads.ProposalHead(ctx, ids.ProposalIDFrom(e.ProposalID))
	if err != nil {
		return Message{}, err
	}
	amount := e.USDCMicros
	if action == "Sell" {
		amount = money.MicrosFromUint64(e.QuoteOutAmount)
	}
	pres := ForCabal(view, proposer)
	if pres.Category, err = k.category(ctx, e.ProposalID, to); err != nil {
		return Message{}, err
	}
	return proposalMessage(
		k.Name(), e.CabalID, e.ProposalID,
		createdTitle(proposer.Name, action, asset, dollars(amount)),
		createdBody(head.Thesis, closesIn(e.ExpiresAt.Sub(k.Clock.Now()))), pres,
	), nil
}

func (k ProposalCreated) category(ctx context.Context, proposal uuid.UUID, to ids.UserID) (string, error) {
	ballots, err := k.Ballots.Ballots(ctx, ids.ProposalIDFrom(proposal))
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(ballots, func(b governanceport.Ballot) bool { return b.VoterID == to }) {
		return voteCategory, nil
	}
	return "", nil
}

func proposerName(ctx context.Context, users Users, id ids.UserID) (string, error) {
	cards, err := users.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return "", err
	}
	card := cards[id]
	if card.Deleted {
		return unnamedProposer, nil
	}
	return cmp.Or(card.DisplayName, unnamedProposer), nil
}

func proposalMessage(kind string, cabalID, proposalID uuid.UUID, title, body string, pres Presentation) Message {
	return Message{
		Title:        title,
		Body:         body,
		Data:         map[string]string{"kind": kind, "cabal_id": cabalID.String(), "proposal_id": proposalID.String()},
		CollapseID:   "proposal-" + proposalID.String(),
		Presentation: pres,
	}
}
