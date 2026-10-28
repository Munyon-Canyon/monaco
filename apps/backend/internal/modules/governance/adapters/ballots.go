package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Ballots struct{ q *sqlc.Queries }

var _ port.Ballots = Ballots{}

func NewBallots(db sqlc.DBTX) Ballots { return Ballots{q: sqlc.New(db)} }

func (b Ballots) Ballots(ctx context.Context, id ids.ProposalID) ([]port.Ballot, error) {
	rows, err := b.q.ListBallots(ctx, id.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "governance.Ballots")
	}
	out := make([]port.Ballot, len(rows))
	for i, row := range rows {
		out[i] = port.Ballot{VoterID: ids.UserIDFrom(row.VoterID)}
		if row.Choice.Valid {
			out[i].Choice = &row.Choice.String
		}
		if row.CastAt.Valid {
			castAt := row.CastAt.Time.UTC()
			out[i].CastAt = &castAt
		}
	}
	return out, nil
}
