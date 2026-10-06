package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type ChatSeenCleanup struct{}

func (ChatSeenCleanup) Handle(ctx context.Context, tx db.Tx, e events.CabalMemberLeft, _ time.Time) error {
	err := sqlc.New(tx.Queries()).DeleteChatSeen(ctx, sqlc.DeleteChatSeenParams{CabalID: e.CabalID, UserID: e.UserID})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "social.ChatSeenCleanup")
	}
	return nil
}
