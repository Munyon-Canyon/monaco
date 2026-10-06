package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const unnamedProposer = "Someone"

type Voters interface {
	Voters(ctx context.Context, id ids.ProposalID) ([]ids.UserID, error)
}

type ProposalCreated struct {
	Cabals Cabals
	Users  Users
	Assets Assets
}

func (ProposalCreated) Name() string { return "proposal_created" }

func (k ProposalCreated) Recipients(ctx context.Context, e events.ProposalCreated) ([]ids.UserID, error) {
	return memberIDs(ctx, k.Cabals, &e.CabalID)
}

func (k ProposalCreated) Render(ctx context.Context, e events.ProposalCreated, _ ids.UserID) (Message, error) {
	words, err := wordsFor(ctx, k.Assets, e.Kind, e.Symbol)
	if err != nil {
		return Message{}, err
	}
	view, err := k.Cabals.Cabal(ctx, ids.CabalIDFrom(e.CabalID))
	if err != nil {
		return Message{}, err
	}
	proposer, err := proposerName(ctx, k.Users, ids.UserIDFrom(e.ProposerID))
	if err != nil {
		return Message{}, err
	}
	amount := usd(e.USDCMicros)
	if words.noun == "sell" {
		amount = "about " + usd(money.MicrosFromUint64(e.QuoteOutAmount))
	}
	body := fmt.Sprintf("%s wants to %s %s of %s. Vote now.", proposer, words.noun, amount, words.asset)
	return proposalMessage(k.Name(), e.CabalID, e.ProposalID, "New proposal in "+view.Name, body), nil
}

type ProposalPassed struct {
	Cabals Cabals
	Voters Voters
	Assets Assets
}

func (ProposalPassed) Name() string { return "proposal_passed" }

func (k ProposalPassed) Recipients(ctx context.Context, e events.ProposalPassed) ([]ids.UserID, error) {
	voted, err := k.Voters.Voters(ctx, ids.ProposalIDFrom(e.ProposalID))
	if err != nil {
		return nil, err
	}
	members, err := memberIDs(ctx, k.Cabals, &e.CabalID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(members, func(user ids.UserID) bool { return !slices.Contains(voted, user) }), nil
}

func (k ProposalPassed) Render(ctx context.Context, e events.ProposalPassed, _ ids.UserID) (Message, error) {
	words, err := wordsFor(ctx, k.Assets, e.Kind, e.Symbol)
	if err != nil {
		return Message{}, err
	}
	body := fmt.Sprintf("Your cabal's buy of %s of %s passed. Buying now.", usd(e.USDCMicros), words.asset)
	if words.noun == "sell" {
		body = fmt.Sprintf("Your cabal's sell of %s passed. Selling now.", words.asset)
	}
	return proposalMessage(k.Name(), e.CabalID, e.ProposalID, "Proposal passed", body), nil
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

func proposalMessage(kind string, cabalID, proposalID uuid.UUID, title, body string) Message {
	return Message{
		Title:      title,
		Body:       body,
		Data:       map[string]string{"kind": kind, "cabal_id": cabalID.String(), "proposal_id": proposalID.String()},
		CollapseID: "proposal-" + proposalID.String(),
	}
}
