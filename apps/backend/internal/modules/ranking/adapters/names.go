package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const leaderboardsUpdated = "global.leaderboards_updated"

type HintPublisher interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type Names struct {
	Hints HintPublisher
}

type runHint struct {
	RunID      uuid.UUID `json:"run_id"`
	ComputedAt time.Time `json:"computed_at"`
}

func (n Names) Handle(ctx context.Context, tx db.Tx, e events.UserProfileUpdated, _ time.Time) error {
	if !slices.ContainsFunc(e.Fields, showsOnBoards) {
		return nil
	}
	q := sqlc.New(tx.Queries())
	changed, err := q.RenameLeaderboardSubject(ctx, sqlc.RenameLeaderboardSubjectParams{
		Name:       domain.SubjectName(e.DisplayName, e.Handle),
		Handle:     optionalText(e.Handle),
		PictureUrl: optionalText(e.PhotoURL),
		SubjectID:  e.UserID,
	})
	if err != nil || changed == 0 {
		return err
	}
	run, err := q.BumpLatestRunRev(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(runHint{RunID: run.RunID, ComputedAt: run.FinishedAt})
	tx.AfterCommit(func(ctx context.Context) { n.Hints.PublishHint(ctx, leaderboardsUpdated, payload) })
	return nil
}

func showsOnBoards(field string) bool {
	return field == "handle" || field == "display_name" || field == "photo_url"
}

func optionalText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
