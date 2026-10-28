package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
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

	ProposerReturnBps     *int64
	ProposerReturnPrivate bool

	Stats      Stats
	MyVote     *MyVote
	ReplyCount int
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
	bps, private, err := r.proposerReturn(ctx, view.ProposerID)
	if err != nil {
		return ProposalDetail{}, err
	}
	facts, err := r.facts(ctx, row, req)
	if err != nil {
		return ProposalDetail{}, err
	}
	open := view.Status == domain.StatusOpen
	out := ProposalDetail{
		Stats: facts.stats, MyVote: facts.myVote, ReplyCount: facts.replyCount,
		Proposal: view, Voters: voters, CanVote: open && isVoter,
		CanWithdraw:       open && view.ProposerID == req.Caller && !othersVoted,
		ProposerReturnBps: bps, ProposerReturnPrivate: private,
	}
	return out, r.linkSwap(ctx, row, &out)
}

func (r *ProposalReads) linkSwap(ctx context.Context, row sqlc.GetProposalRow, out *ProposalDetail) error {
	swap, found, err := r.swaps.LatestBySource(ctx, trading.Source{Kind: "proposal", ID: row.ID})
	if err != nil || !found {
		return err
	}
	out.Swap = &swap
	out.Fill, err = r.fill(ctx, row.Mint, swap)
	return err
}

type detailFacts struct {
	stats      Stats
	myVote     *MyVote
	replyCount int
}

func (r *ProposalReads) facts(ctx context.Context, row sqlc.GetProposalRow, req GetProposal) (detailFacts, error) {
	const op = "governance.GetProposal"
	stats, err := rowStats(row)
	if err != nil {
		return detailFacts{}, err
	}
	out := detailFacts{stats: stats}
	if row.MyBallot != "" {
		order, err := r.q.BallotOrder(ctx, sqlc.BallotOrderParams{ProposalID: row.ID, VoterID: req.Caller.UUID()})
		if err != nil {
			return detailFacts{}, lookupFailed(err, op)
		}
		out.myVote = myVoteOf(order, row.DecidedBy.Valid && row.DecidedBy.Bytes == req.Caller.UUID())
	}
	replies, err := r.Replies.ReplyCounts(ctx, []ids.ProposalID{req.ID})
	if err != nil {
		return detailFacts{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out.replyCount = replies[req.ID]
	return out, nil
}

func (r *ProposalReads) fill(ctx context.Context, rawMint string, swap trading.SwapView) (Fill, error) {
	mint, err := market.ParseMint(rawMint)
	if err != nil {
		return Fill{}, err
	}
	asset, err := r.assets.AssetByMint(ctx, mint)
	if err != nil {
		return Fill{}, err
	}
	return fillOf(swap, asset.Decimals), nil
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

func (r *ProposalReads) proposerReturn(ctx context.Context, id ids.UserID) (*int64, bool, error) {
	const op = "governance.GetProposal"
	who := []ids.UserID{id}
	cards, err := r.users.UsersByID(ctx, who)
	if err != nil {
		return nil, false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	found, err := r.rets.PeopleReturns(ctx, rankingport.Range1M, who)
	if err != nil {
		return nil, false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	card, ok := cards[id]
	switch {
	case !ok || card.Deleted:
		return nil, false, nil
	case !card.SharePerformance:
		return nil, true, nil
	}
	return found[id].ReturnBps, false, nil
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
