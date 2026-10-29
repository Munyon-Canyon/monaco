package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ReplyCounts interface {
	ReplyCounts(ctx context.Context, proposalIDs []ids.ProposalID) (map[ids.ProposalID]int, error)
}

type AssetCards interface {
	ByMints(ctx context.Context, mints []string) ([]market.Asset, error)
}

type PendingVotePorts struct {
	Assets  AssetCards
	Replies ReplyCounts
}

type PendingProposer struct {
	ID          ids.UserID
	Handle      string
	DisplayName string
	PhotoURL    string
}

type PendingVote struct {
	ProposalID  ids.ProposalID
	CabalID     ids.CabalID
	CabalName   string
	Kind        domain.Kind
	Symbol      string
	USDC        money.Micros
	TokenAmount money.BaseUnits
	AssetName   string
	AssetLogo   string
	Proposer    PendingProposer
	Tally       Tally
	VotedCount  int
	ReplyCount  int
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type PendingVoteReads struct {
	reads *ProposalReads
	ports PendingVotePorts
}

func NewPendingVoteReads(reads *ProposalReads, ports PendingVotePorts) *PendingVoteReads {
	return &PendingVoteReads{reads: reads, ports: ports}
}

func (r *PendingVoteReads) PendingVotes(ctx context.Context, voter ids.UserID) ([]PendingVote, error) {
	rows, err := r.reads.q.PendingVotes(ctx, voter.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "governance.PendingVotes")
	}
	if len(rows) == 0 {
		return []PendingVote{}, nil
	}
	extra, err := r.pendingExtras(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]PendingVote, len(rows))
	for i, row := range rows {
		kind, err := domain.ParseKind(row.Kind)
		if err != nil {
			return nil, err
		}
		id, cabal, proposer := ids.ProposalIDFrom(row.ID), ids.CabalIDFrom(row.CabalID), ids.UserIDFrom(row.ProposerID)
		asset := extra.assets[row.Mint]
		voters := int(row.VoterCount)
		out[i] = PendingVote{
			ProposalID: id, CabalID: cabal, CabalName: extra.names[cabal], Kind: kind, Symbol: row.Symbol,
			USDC:        money.MicrosFromUint64(uint64(max(row.UsdcMicros.Int64, 0))),
			TokenAmount: money.NewBaseUnits(uint64(max(row.TokenAmount.Int64, 0)), asset.Decimals),
			AssetName:   nameOr(asset.DisplayName, row.Symbol), AssetLogo: asset.LogoURL,
			Proposer: extra.proposers[proposer],
			Tally: Tally{
				Yes: int(row.YesCount), No: int(row.NoCount), Voters: voters,
				Needed: domain.ThresholdRule(row.Threshold).Needed(voters),
			},
			VotedCount: int(row.VotedCount), ReplyCount: extra.replies[id],
			CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
		}
		out[i].Proposer.ID = proposer
	}
	return out, nil
}

type cardExtras struct {
	names     map[ids.CabalID]string
	assets    map[string]market.Asset
	proposers map[ids.UserID]PendingProposer
}

type pendingExtras struct {
	cardExtras
	replies map[ids.ProposalID]int
}

func (r *PendingVoteReads) pendingExtras(ctx context.Context, rows []sqlc.PendingVotesRow) (pendingExtras, error) {
	const op = "governance.PendingVotes"
	cabalIDs := make([]ids.CabalID, len(rows))
	mints := make([]string, len(rows))
	proposerIDs := make([]ids.UserID, len(rows))
	proposalIDs := make([]ids.ProposalID, len(rows))
	for i, row := range rows {
		cabalIDs[i], mints[i] = ids.CabalIDFrom(row.CabalID), row.Mint
		proposerIDs[i], proposalIDs[i] = ids.UserIDFrom(row.ProposerID), ids.ProposalIDFrom(row.ID)
	}
	var out pendingExtras
	var err error
	if out.cardExtras, err = r.cardExtras(ctx, op, cabalIDs, mints, proposerIDs); err != nil {
		return pendingExtras{}, err
	}
	if out.replies, err = r.ports.Replies.ReplyCounts(ctx, proposalIDs); err != nil {
		return pendingExtras{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return out, nil
}

func (r *PendingVoteReads) cardExtras(
	ctx context.Context, op string, cabalIDs []ids.CabalID, mints []string, proposerIDs []ids.UserID,
) (cardExtras, error) {
	var out cardExtras
	var err error
	if out.names, err = r.reads.cabalNames(ctx, cabalIDs); err != nil {
		return cardExtras{}, err
	}
	assets, err := r.ports.Assets.ByMints(ctx, mints)
	if err != nil {
		return cardExtras{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out.assets = make(map[string]market.Asset, len(assets))
	for _, a := range assets {
		out.assets[a.Mint.String()] = a
	}
	cards, err := r.reads.users.UsersByID(ctx, proposerIDs)
	if err != nil {
		return cardExtras{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out.proposers = make(map[ids.UserID]PendingProposer, len(cards))
	for id, card := range cards {
		if !card.Deleted {
			out.proposers[id] = PendingProposer{
				ID: id, Handle: card.Handle, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL,
			}
		}
	}
	return out, nil
}

func nameOr(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}
