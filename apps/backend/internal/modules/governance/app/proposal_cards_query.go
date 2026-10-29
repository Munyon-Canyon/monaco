package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const proposalCardsOp = "governance.ProposalCards"

func (r *PendingVoteReads) ProposalCards(
	ctx context.Context, viewer ids.UserID, proposals []ids.ProposalID,
) (map[ids.ProposalID]port.CatchUpProposal, error) {
	if len(proposals) == 0 {
		return map[ids.ProposalID]port.CatchUpProposal{}, nil
	}
	wanted := make([]uuid.UUID, len(proposals))
	for i, id := range proposals {
		wanted[i] = id.UUID()
	}
	rows, err := r.reads.q.ProposalCards(ctx, sqlc.ProposalCardsParams{ViewerID: viewer.UUID(), ProposalIds: wanted})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, proposalCardsOp)
	}
	out := make(map[ids.ProposalID]port.CatchUpProposal, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	cabalIDs := make([]ids.CabalID, len(rows))
	mints := make([]string, len(rows))
	proposerIDs := make([]ids.UserID, len(rows))
	for i, row := range rows {
		cabalIDs[i], mints[i], proposerIDs[i] = ids.CabalIDFrom(row.CabalID), row.Mint, ids.UserIDFrom(row.ProposerID)
	}
	extra, err := r.cardExtras(ctx, proposalCardsOp, cabalIDs, mints, proposerIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		card, err := catchUpCard(row, extra)
		if err != nil {
			return nil, err
		}
		out[card.ID] = card
	}
	return out, nil
}

func catchUpCard(row sqlc.ProposalCardsRow, extra cardExtras) (port.CatchUpProposal, error) {
	kind, err := domain.ParseKind(row.Kind)
	if err != nil {
		return port.CatchUpProposal{}, err
	}
	cabal, proposer := ids.CabalIDFrom(row.CabalID), ids.UserIDFrom(row.ProposerID)
	asset := extra.assets[row.Mint]
	who := extra.proposers[proposer]
	usdc := positiveAmount(row.UsdcMicros.Valid, row.UsdcMicros.Int64)
	tokens := positiveAmount(row.TokenAmount.Valid, row.TokenAmount.Int64)
	card := port.CatchUpProposal{
		ID: ids.ProposalIDFrom(row.ID), CabalID: cabal, CabalName: extra.names[cabal], Kind: string(kind),
		Symbol: row.Symbol, Mint: row.Mint, AssetName: nameOr(asset.DisplayName, row.Symbol), AssetLogo: asset.LogoURL,
		USDCMicros: usdc, TokenAmount: tokens,
		Proposer:  port.CatchUpPerson{ID: proposer, Name: nameOr(who.DisplayName, who.Handle), PhotoURL: who.PhotoURL},
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, Status: domain.Status(row.Status),
		Yes: int(row.YesCount), No: int(row.NoCount), Voted: int(row.VotedCount), Eligible: int(row.VoterCount),
		YourChoice: row.YourChoice.String,
	}
	if row.DecidedBy.Valid {
		decider := ids.UserIDFrom(row.DecidedBy.Bytes)
		card.DecidedBy = &decider
	}
	return card, nil
}

func positiveAmount(valid bool, amount int64) *uint64 {
	if !valid || amount <= 0 {
		return nil
	}
	n := uint64(amount)
	return &n
}
