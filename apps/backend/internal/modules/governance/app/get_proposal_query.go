package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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
	Fill        Fill
}

type Fill struct {
	USDCMicros    uint64
	TokenAmount   uint64
	TokenDecimals uint8
	PriceMicros   uint64
	Confirmed     bool
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
	view, err := proposalView(listRow(row), tally)
	if err != nil {
		return ProposalDetail{}, err
	}
	view.CabalName, err = r.CabalName(ctx, view.CabalID)
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
		mint, err := market.ParseMint(row.Mint)
		if err != nil {
			return ProposalDetail{}, err
		}
		asset, err := r.assets.AssetByMint(ctx, mint)
		if err != nil {
			return ProposalDetail{}, err
		}
		out.Fill = fillOf(swap, asset.Decimals)
	}
	return out, nil
}

func fillOf(s trading.SwapView, decimals uint8) Fill {
	f := Fill{USDCMicros: s.InAmount, TokenAmount: s.OutAmount, TokenDecimals: decimals}
	if string(s.Action) == string(domain.KindSell) {
		f.USDCMicros, f.TokenAmount = f.TokenAmount, f.USDCMicros
	}
	if f.Confirmed = s.Status == "confirmed"; f.Confirmed {
		f.PriceMicros = money.PricePerUnit(money.MicrosFromUint64(f.USDCMicros), f.TokenAmount, decimals).Uint64()
	}
	return f
}

func listRow(r sqlc.GetProposalRow) sqlc.ListProposalsRow {
	return sqlc.ListProposalsRow{
		ID: r.ID, CabalID: r.CabalID, ProposerID: r.ProposerID, Kind: r.Kind, Symbol: r.Symbol,
		UsdcMicros: r.UsdcMicros, TokenAmount: r.TokenAmount, QuoteOutAmount: r.QuoteOutAmount, Thesis: r.Thesis,
		Threshold: r.Threshold, Status: r.Status, StatusReason: r.StatusReason, ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt, MyBallot: r.MyBallot, PriceComparison: r.PriceComparison, Network: r.Network,
	}
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
