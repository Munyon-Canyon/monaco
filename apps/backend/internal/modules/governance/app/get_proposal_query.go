package app

import (
	"context"
	"math"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Swaps interface {
	LatestBySource(ctx context.Context, src trading.Source) (trading.SwapView, bool, error)
}

type GetProposal struct {
	ID       ids.ProposalID
	Caller   ids.UserID
	AllSides bool
}

type Voter struct {
	UserID      ids.UserID
	Voted       bool
	Choice      domain.Choice
	CastAt      time.Time
	DisplayName string
	PhotoURL    string
}

type ProposalDetail struct {
	Proposal     ProposalView
	Voters       []Voter
	SidesVisible bool
	CanVote      bool
	CanWithdraw  bool
	Swap         *trading.SwapView
	Fill         Fill

	Proposer              ProposerCard
	ProposerReturnBps     *int64
	ProposerReturnPrivate bool
	PassedPriceMicros     *int64

	Stats      Stats
	MyVote     *MyVote
	ReplyCount int

	ReminderSet bool
}

type ProposerCard struct {
	ID          ids.UserID
	DisplayName string
	PhotoURL    string
}

type Fill struct {
	USDCMicros    uint64
	TokenAmount   uint64
	TokenDecimals uint8
	PriceMicros   uint64
	Confirmed     bool
	FeeMicros     *money.Micros
	FilledAt      time.Time
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
	reveal := req.AllSides || sidesVisible(domain.Status(row.Status), row.SidesAfterClose)
	voters, tally, isVoter, othersVoted := tallyVoters(ballots, req.Caller, reveal)
	tally.Needed = domain.ThresholdRule(row.Threshold).Needed(tally.Voters)
	view, err := proposalView(listRow(row), tally)
	if err != nil {
		return ProposalDetail{}, err
	}
	if err := r.nameView(ctx, &view); err != nil {
		return ProposalDetail{}, err
	}
	cards, err := r.nameVoters(ctx, view.ProposerID, voters)
	if err != nil {
		return ProposalDetail{}, err
	}
	proposer, err := r.proposerFacts(ctx, view.ProposerID, cards)
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
		Proposal: view, Voters: voters, SidesVisible: reveal, CanVote: open && isVoter,
		CanWithdraw: open && view.ProposerID == req.Caller && !othersVoted,
		Proposer:    proposer.card, ProposerReturnBps: proposer.bps, ProposerReturnPrivate: proposer.private,
		PassedPriceMicros: passedPrice(row), ReminderSet: row.ReminderSet,
	}
	return out, r.priced(ctx, row, &out)
}

func (r *ProposalReads) nameView(ctx context.Context, view *ProposalView) error {
	name, err := r.CabalName(ctx, view.CabalID)
	view.CabalName = name
	if err != nil {
		return err
	}
	return r.nameRuleUsers(ctx, []*RuleView{view.Rule})
}

func (r *ProposalReads) priced(ctx context.Context, row sqlc.GetProposalRow, out *ProposalDetail) error {
	if domain.Kind(row.Kind) == domain.KindRule {
		return nil
	}
	asset, err := r.assetOf(ctx, row.Mint.String)
	if err != nil {
		return err
	}
	decimals, err := r.tokenDecimals(ctx, asset, chain.Network(row.Network))
	if err != nil {
		return err
	}
	if row.TokenAmount.Valid && chain.Network(row.Network).IsEVM() {
		out.Proposal.TokenDecimals = decimals
	}
	live, err := r.livePrice(ctx, asset.ID)
	if err != nil {
		return err
	}
	if err := r.linkSwap(ctx, row, decimals, out); err != nil {
		return err
	}
	if out.Swap != nil {
		decimals = out.Fill.TokenDecimals
	}
	out.Stats = withPrices(ctx, out.Stats, priceInputs{
		id: out.Proposal.ID, kind: row.Kind, usdc: money.MicrosFromUint64(unsigned(row.UsdcMicros.Int64)),
		token: unsigned(row.TokenAmount.Int64), decimals: decimals, live: live, fill: out.Fill,
	})
	return nil
}

func (r *ProposalReads) linkSwap(
	ctx context.Context, row sqlc.GetProposalRow, decimals uint8, out *ProposalDetail,
) error {
	swap, found, err := r.swaps.LatestBySource(ctx, trading.Source{Kind: "proposal", ID: row.ID})
	if err != nil || !found {
		return err
	}
	out.Swap = &swap
	out.Fill = fillOf(swap, decimals)
	return nil
}

func (r *ProposalReads) assetOf(ctx context.Context, rawMint string) (market.Asset, error) {
	mint, err := market.ParseMint(rawMint)
	if err != nil {
		return market.Asset{}, err
	}
	return r.assets.AssetByMint(ctx, mint)
}

func (r *ProposalReads) livePrice(ctx context.Context, id market.AssetID) (money.Micros, error) {
	if r.prices == nil {
		return money.Micros{}, nil
	}
	prices, err := r.prices.PricesAsOf(ctx, []market.AssetID{id}, r.clock.Now())
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), "governance.GetProposal")
	}
	return prices[id].Micros, nil
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

func fillOf(s trading.SwapView, decimals uint8) Fill {
	if string(s.Action) == string(domain.KindBuy) && s.OutDecimals > 0 && s.OutDecimals <= math.MaxUint8 {
		decimals = uint8(s.OutDecimals)
	}
	f := Fill{USDCMicros: s.InAmount, TokenAmount: s.OutAmount, TokenDecimals: decimals}
	if string(s.Action) == string(domain.KindSell) {
		f.USDCMicros, f.TokenAmount = f.TokenAmount, f.USDCMicros
	}
	if f.Confirmed = s.Status == "confirmed"; f.Confirmed {
		f.PriceMicros = money.PricePerUnit(money.MicrosFromUint64(f.USDCMicros), f.TokenAmount, decimals).Uint64()
		f.FeeMicros, f.FilledAt = s.FeeMicros, s.ConfirmedAt
	}
	return f
}

func listRow(r sqlc.GetProposalRow) sqlc.ListProposalsRow {
	return sqlc.ListProposalsRow{
		ID: r.ID, CabalID: r.CabalID, ProposerID: r.ProposerID, Kind: r.Kind, Symbol: r.Symbol,
		UsdcMicros: r.UsdcMicros, TokenAmount: r.TokenAmount, QuoteOutAmount: r.QuoteOutAmount, Thesis: r.Thesis,
		Threshold: r.Threshold, Status: r.Status, StatusReason: r.StatusReason, ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt, MyBallot: r.MyBallot, PriceComparison: r.PriceComparison, Network: r.Network,
		RuleChange: r.RuleChange,
	}
}

func passedPrice(row sqlc.GetProposalRow) *int64 {
	if !row.PassedPriceMicros.Valid {
		return nil
	}
	return &row.PassedPriceMicros.Int64
}

type proposerFacts struct {
	card    ProposerCard
	bps     *int64
	private bool
}

func (r *ProposalReads) ProposerCard(ctx context.Context, id ids.UserID) (ProposerCard, error) {
	cards, err := r.users.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return ProposerCard{}, errs.Wrap(err, errs.CodeOf(err), "governance.GetProposal")
	}
	return cardOf(id, cards), nil
}

func cardOf(id ids.UserID, cards map[ids.UserID]identityport.UserCard) ProposerCard {
	card, ok := cards[id]
	if !ok || card.Deleted {
		return ProposerCard{ID: id}
	}
	return ProposerCard{ID: id, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL}
}

func (r *ProposalReads) nameVoters(
	ctx context.Context, proposer ids.UserID, voters []Voter,
) (map[ids.UserID]identityport.UserCard, error) {
	cards, err := r.cardsOf(ctx, proposer, voters)
	if err != nil {
		return nil, err
	}
	for i := range voters {
		card := cardOf(voters[i].UserID, cards)
		voters[i].DisplayName, voters[i].PhotoURL = card.DisplayName, card.PhotoURL
	}
	return cards, nil
}

func (r *ProposalReads) cardsOf(
	ctx context.Context, proposer ids.UserID, voters []Voter,
) (map[ids.UserID]identityport.UserCard, error) {
	who := make([]ids.UserID, 0, len(voters)+1)
	who = append(who, proposer)
	for _, v := range voters {
		if v.UserID != proposer {
			who = append(who, v.UserID)
		}
	}
	cards, err := r.users.UsersByID(ctx, who)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "governance.GetProposal")
	}
	return cards, nil
}

func (r *ProposalReads) proposerFacts(
	ctx context.Context, id ids.UserID, cards map[ids.UserID]identityport.UserCard,
) (proposerFacts, error) {
	const op = "governance.GetProposal"
	found, err := r.rets.PeopleReturns(ctx, rankingport.Range1M, []ids.UserID{id})
	if err != nil {
		return proposerFacts{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out := proposerFacts{card: cardOf(id, cards)}
	card, ok := cards[id]
	switch {
	case !ok || card.Deleted:
	case !card.SharePerformance:
		out.private = true
	default:
		out.bps = found[id].ReturnBps
	}
	return out, nil
}

func sidesVisible(status domain.Status, rule bool) bool {
	switch status {
	case domain.StatusPassed, domain.StatusFailed, domain.StatusExpired, domain.StatusExecuted,
		domain.StatusExecutionBlocked:
		return rule
	case domain.StatusOpen, domain.StatusWithdrawn, domain.StatusVoided:
		return false
	}
	return false
}

func tallyVoters(rows []sqlc.ProposalVotersRow, caller ids.UserID, reveal bool) ([]Voter, Tally, bool, bool) {
	voters := make([]Voter, len(rows))
	tally := Tally{Voters: len(rows)}
	isVoter, othersVoted := false, false
	for i, row := range rows {
		voter := Voter{
			UserID: ids.UserIDFrom(row.VoterID), Voted: row.Choice.Valid,
			Choice: domain.Choice(row.Choice.String), CastAt: row.CastAt.Time,
		}
		switch voter.Choice {
		case domain.ChoiceYes:
			tally.Yes++
		case domain.ChoiceNo:
			tally.No++
		}
		isVoter = isVoter || voter.UserID == caller
		othersVoted = othersVoted || (voter.Voted && voter.UserID != caller)
		if !reveal && voter.UserID != caller {
			voter.Choice, voter.CastAt = "", time.Time{}
		}
		voters[i] = voter
	}
	return voters, tally, isVoter, othersVoted
}
