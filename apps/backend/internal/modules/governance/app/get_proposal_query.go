package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Swaps interface {
	LatestBySource(ctx context.Context, src trading.Source) (trading.SwapView, bool, error)
}

type GetProposal struct {
	ID     ids.ProposalID
	Caller ids.UserID
}

type Voter struct {
	UserID ids.UserID
	Choice domain.Choice
	CastAt time.Time
}

type ProposalDetail struct {
	Proposal    ProposalView
	Voters      []Voter
	CanVote     bool
	CanWithdraw bool
	Swap        *trading.SwapView
}

func (r *ProposalReads) Get(ctx context.Context, req GetProposal) (ProposalDetail, error) {
	const op = "governance.GetProposal"
	row, err := r.q.GetProposal(ctx, sqlc.GetProposalParams{ID: req.ID.UUID(), CallerID: req.Caller.UUID()})
	if err != nil {
		return ProposalDetail{}, lookupFailed(err, op)
	}
	ballots, err := r.q.ProposalVoters(ctx, row.ID)
	if err != nil {
		return ProposalDetail{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	voters, tally, isVoter, othersVoted := tallyVoters(ballots, req.Caller)
	tally.Needed = domain.ThresholdRule(row.Threshold).Needed(tally.Voters)
	view, err := proposalView(sqlc.ListProposalsRow(row), tally)
	if err != nil {
		return ProposalDetail{}, err
	}
	swap, found, err := r.swaps.LatestBySource(ctx, trading.Source{Kind: "proposal", ID: row.ID})
	if err != nil {
		return ProposalDetail{}, err
	}
	open := view.Status == domain.StatusOpen
	out := ProposalDetail{
		Proposal: view, Voters: voters, CanVote: open && isVoter,
		CanWithdraw: open && view.ProposerID == req.Caller && !othersVoted,
	}
	if found {
		out.Swap = &swap
	}
	return out, nil
}

func tallyVoters(rows []sqlc.ProposalVotersRow, caller ids.UserID) ([]Voter, Tally, bool, bool) {
	voters := make([]Voter, len(rows))
	tally := Tally{Voters: len(rows)}
	isVoter, othersVoted := false, false
	for i, row := range rows {
		voter := Voter{
			UserID: ids.UserIDFrom(row.VoterID), Choice: domain.Choice(row.Choice.String), CastAt: row.CastAt.Time,
		}
		switch voter.Choice {
		case domain.ChoiceYes:
			tally.Yes++
		case domain.ChoiceNo:
			tally.No++
		}
		isVoter = isVoter || voter.UserID == caller
		othersVoted = othersVoted || (voter.Choice != "" && voter.UserID != caller)
		voters[i] = voter
	}
	return voters, tally, isVoter, othersVoted
}
