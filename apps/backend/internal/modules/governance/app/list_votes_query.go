package app

import (
	"context"
	"encoding/base64"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	listVotesOp = "governance.ListVotes"

	ListVotesDefault = 50
	listVotesMax     = 200
)

type ListVotes struct {
	ID     ids.ProposalID
	Caller ids.UserID
	Limit  int
	Cursor string
}

type VoterEntry struct {
	Voter
	Voted bool
	Card  identityport.UserCard
}

type VotePage struct {
	Items      []VoterEntry
	NextCursor string
}

func (r *ProposalReads) Votes(ctx context.Context, req ListVotes) (VotePage, error) {
	limit := req.Limit
	if limit < 1 || limit > listVotesMax {
		return VotePage{}, errs.New(errs.CodeInvalidInput, listVotesOp, slog.String("field", "limit"))
	}
	after, hasCursor, err := decodeVotesCursor(req.Cursor)
	if err != nil {
		return VotePage{}, err
	}
	proposal, err := r.q.FindProposalID(ctx, req.ID.UUID())
	if err != nil {
		return VotePage{}, lookupFailed(err, listVotesOp)
	}
	reveal := sidesVisible(domain.Status(proposal.Status), proposal.SidesAfterClose)
	rows, err := r.q.ProposalVotersPage(ctx, sqlc.ProposalVotersPageParams{
		ProposalID: req.ID.UUID(), HasCursor: hasCursor, CursorVoterID: after, RowLimit: int32(limit) + 1,
	})
	if err != nil {
		return VotePage{}, errs.Wrap(err, errs.CodeInternal, listVotesOp)
	}
	page := VotePage{Items: []VoterEntry{}}
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextCursor = encodeVotesCursor(rows[len(rows)-1].VoterID)
	}
	if len(rows) == 0 {
		return page, nil
	}
	voterIDs := make([]ids.UserID, len(rows))
	for i, row := range rows {
		voterIDs[i] = ids.UserIDFrom(row.VoterID)
	}
	cards, err := r.users.UsersByID(ctx, voterIDs)
	if err != nil {
		return VotePage{}, errs.Wrap(err, errs.CodeOf(err), listVotesOp)
	}
	for i, row := range rows {
		page.Items = append(page.Items, voterEntry(row, voterIDs[i], cards[voterIDs[i]], req.Caller, reveal))
	}
	return page, nil
}

func voterEntry(
	row sqlc.ProposalVotersPageRow,
	id ids.UserID,
	card identityport.UserCard,
	caller ids.UserID,
	reveal bool,
) VoterEntry {
	entry := VoterEntry{
		Voter: Voter{UserID: id, Choice: domain.Choice(row.Choice.String), CastAt: row.CastAt.Time},
		Voted: row.Choice.Valid,
		Card:  card,
	}
	if !reveal && id != caller {
		entry.Choice, entry.CastAt = "", time.Time{}
	}
	return entry
}

func encodeVotesCursor(voterID uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(voterID.String()))
}

func decodeVotesCursor(raw string) (uuid.UUID, bool, error) {
	if raw == "" {
		return uuid.UUID{}, false, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return uuid.UUID{}, false, errs.Wrap(err, errs.CodeInvalidInput, listVotesOp, slog.String("field", "cursor"))
	}
	voterID, err := uuid.ParseBytes(body)
	if err != nil {
		return uuid.UUID{}, false, errs.Wrap(err, errs.CodeInvalidInput, listVotesOp, slog.String("field", "cursor"))
	}
	return voterID, true, nil
}
