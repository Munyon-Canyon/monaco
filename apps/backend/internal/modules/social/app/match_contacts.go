package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type MatchContacts struct {
	User   ids.UserID
	Hashes []string
}

type MatchContactsDeps struct {
	UoW   *db.UnitOfWork
	Users Users
	Clock clock.Clock
}

type MatchContactsHandler struct {
	d MatchContactsDeps
}

func NewMatchContactsHandler(d MatchContactsDeps) *MatchContactsHandler {
	return &MatchContactsHandler{d: d}
}

func (h *MatchContactsHandler) Handle(ctx context.Context, cmd MatchContacts) error {
	hashes, err := domain.ParsePhoneHashes(cmd.Hashes)
	if err != nil {
		return err
	}
	found, err := h.d.Users.UsersByPhoneHashes(ctx, hashes)
	if err != nil {
		return err
	}
	matched := make([]uuid.UUID, 0, len(found))
	for _, id := range found {
		if id != cmd.User {
			matched = append(matched, id.UUID())
		}
	}
	if len(matched) > 0 {
		err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			err := sqlc.New(tx.Queries()).InsertContactMatches(ctx, sqlc.InsertContactMatchesParams{
				UserID: cmd.User.UUID(), CreatedAt: h.d.Clock.Now().UTC(), MatchedUserIds: matched,
			})
			if err != nil {
				return errs.Wrap(err, errs.CodeInternal, "social.MatchContacts")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	observability.Info(ctx, observability.ContactsMatched,
		slog.Int("submitted", len(cmd.Hashes)), slog.Int("matched", len(matched)))
	return nil
}
