package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Unmute struct {
	User       ids.UserID
	TargetType string
	TargetID   string
}

type UnmuteHandler struct{ uow *db.UnitOfWork }

func NewUnmuteHandler(uow *db.UnitOfWork) *UnmuteHandler { return &UnmuteHandler{uow: uow} }

func (h *UnmuteHandler) Handle(ctx context.Context, cmd Unmute) error {
	target, err := parseMuteTarget(cmd.TargetType, cmd.TargetID)
	if err != nil {
		return err
	}
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		err := sqlc.New(tx.Queries()).DeleteFeedMute(ctx, sqlc.DeleteFeedMuteParams{
			UserID: cmd.User.UUID(), TargetType: target.typ, TargetID: target.raw,
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, "social.Unmute")
		}
		return nil
	})
}
