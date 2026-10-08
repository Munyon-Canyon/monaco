package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	adminport "github.com/monaco/monaco/apps/backend/internal/modules/admin/port"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	historyLimit = 50
	actionsLimit = 20
)

type ProposalEvents interface {
	EventsByAggregate(
		ctx context.Context, aggregateType string, id uuid.UUID, types []string, limit int,
	) ([]bus.EventRow, error)
}

type Handles interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]identityport.UserCard, error)
}

type CabalReader interface {
	Cabal(ctx context.Context, id ids.CabalID) (cabalport.CabalView, error)
}

type Proposer struct {
	ID     ids.UserID
	Handle string
}

type StatusChange struct {
	Status    domain.Status
	At        time.Time
	ActorType string
}

type AdminProposal struct {
	Detail   ProposalDetail
	Proposer Proposer
	History  []StatusChange
	Actions  []adminport.Action
}

type AdminReads struct {
	reads   *ProposalReads
	events  ProposalEvents
	users   Handles
	cabals  CabalReader
	actions adminport.Actions
}

func NewAdminReads(
	reads *ProposalReads, e ProposalEvents, u Handles, c CabalReader, a adminport.Actions,
) *AdminReads {
	return &AdminReads{reads: reads, events: e, users: u, cabals: c, actions: a}
}

func eventStatuses() map[events.Type]domain.Status {
	return map[events.Type]domain.Status{
		events.TypeProposalCreated:          domain.StatusOpen,
		events.TypeProposalPassed:           domain.StatusPassed,
		events.TypeProposalFailed:           domain.StatusFailed,
		events.TypeProposalExpired:          domain.StatusExpired,
		events.TypeProposalWithdrawn:        domain.StatusWithdrawn,
		events.TypeProposalVoided:           domain.StatusVoided,
		events.TypeProposalExecuted:         domain.StatusExecuted,
		events.TypeProposalExecutionBlocked: domain.StatusExecutionBlocked,
		events.TypeProposalReopened:         domain.StatusPassed,
	}
}

func (a *AdminReads) Proposal(ctx context.Context, id ids.ProposalID) (AdminProposal, error) {
	detail, err := a.reads.Get(ctx, GetProposal{ID: id})
	if err != nil {
		return AdminProposal{}, err
	}
	cards, err := a.users.UsersByID(ctx, []ids.UserID{detail.Proposal.ProposerID})
	if err != nil {
		return AdminProposal{}, err
	}
	statuses := eventStatuses()
	rows, err := a.events.EventsByAggregate(ctx, "proposal", id.UUID(), eventTypes(statuses), historyLimit)
	if err != nil {
		return AdminProposal{}, err
	}
	actions, err := a.actions.RecentActions(ctx, string(events.AdminTargetProposal), id.String(), actionsLimit)
	if err != nil {
		return AdminProposal{}, err
	}
	out := AdminProposal{
		Detail:   detail,
		Proposer: Proposer{ID: detail.Proposal.ProposerID, Handle: cards[detail.Proposal.ProposerID].Handle},
		Actions:  actions,
	}
	for _, row := range rows {
		out.History = append(out.History, StatusChange{
			Status: statuses[events.Type(row.Type)], At: row.CreatedAt, ActorType: row.ActorType,
		})
	}
	return out, nil
}

func (a *AdminReads) CabalProposals(
	ctx context.Context, cabal ids.CabalID, filter Filter, limit int, cursor string,
) (ProposalPage, error) {
	if _, err := a.cabals.Cabal(ctx, cabal); err != nil {
		return ProposalPage{}, err
	}
	return a.reads.List(ctx, ListProposals{CabalID: cabal, Filter: filter, Limit: limit, Cursor: cursor})
}

func eventTypes(statuses map[events.Type]domain.Status) []string {
	types := make([]string, 0, len(statuses))
	for t := range statuses {
		types = append(types, string(t))
	}
	return types
}
