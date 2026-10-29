package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CallBoards struct{ db dbsqlc.DBTX }

var _ port.CallBoards = CallBoards{}

func NewCallBoards(pool dbsqlc.DBTX) CallBoards { return CallBoards{db: pool} }

func (c CallBoards) CallStanding(
	ctx context.Context, userID ids.UserID, period domain.PeriodKind,
) (port.Standing, error) {
	row, err := sqlc.New(c.db).CallStandingOf(ctx, sqlc.CallStandingOfParams{
		Period: string(period), UserID: userID.UUID(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return port.Standing{}, nil
	}
	if err != nil {
		return port.Standing{}, err
	}
	return port.Standing{
		Rank: intOrNil(
			row.Rank.Int32,
			row.Rank.Valid,
		),
		Of:          int(row.Ranked),
		RateBps:     intOrNil(row.RateBps.Int32, row.RateBps.Valid),
		Right:       int(row.RightCount),
		Resolved:    int(row.Resolved),
		PeriodStart: row.PeriodStart.Time,
	}, nil
}

func (c CallBoards) CallLeader(
	ctx context.Context, board domain.Board, period domain.PeriodKind,
) (port.Leader, error) {
	row, err := sqlc.New(c.db).CallLeaderOf(ctx, sqlc.CallLeaderOfParams{Board: string(board), Period: string(period)})
	if errors.Is(err, sql.ErrNoRows) {
		return port.Leader{}, errs.New(errs.CodeNotFound, "ranking.CallBoards.CallLeader")
	}
	if err != nil {
		return port.Leader{}, err
	}
	return port.Leader{
		SubjectID: row.SubjectID, Name: row.SubjectName, Handle: row.SubjectHandle.String,
		PictureURL: row.SubjectPictureUrl.String, RateBps: int(row.RateBps.Int32), Right: int(row.RightCount),
		Resolved: int(row.Resolved), PeriodStart: row.PeriodStart.Time,
	}, nil
}

func (c CallBoards) ClosedTops(
	ctx context.Context, subjectID uuid.UUID, board domain.Board, period domain.PeriodKind,
) ([]port.ClosedTop, error) {
	rows, err := sqlc.New(c.db).ClosedTopsOf(ctx, sqlc.ClosedTopsOfParams{
		SubjectID: subjectID, Board: string(board), Period: string(period),
	})
	if err != nil {
		return nil, err
	}
	out := make([]port.ClosedTop, len(rows))
	for i, row := range rows {
		out[i] = port.ClosedTop{
			PeriodStart: row.PeriodStart.Time,
			Right:       int(row.RightCount),
			Resolved:    int(row.Resolved),
		}
	}
	return out, nil
}

func intOrNil(n int32, valid bool) *int {
	if !valid {
		return nil
	}
	v := int(n)
	return &v
}
