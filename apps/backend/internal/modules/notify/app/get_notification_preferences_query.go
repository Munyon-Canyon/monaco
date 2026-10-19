package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func GetNotificationPreferences(
	ctx context.Context, q sqlc.DBTX, users Users, user ids.UserID,
) (domain.Preferences, error) {
	const op = "notify.GetNotificationPreferences"
	if err := requireLiveUser(ctx, users, user, op); err != nil {
		return domain.Preferences{}, err
	}
	rows, err := sqlc.New(q).GetPreferences(ctx, []uuid.UUID{user.UUID()})
	if err != nil {
		return domain.Preferences{}, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	enabled := make(map[domain.Category]bool, len(rows))
	for _, r := range rows {
		enabled[domain.Category(r.Category)] = r.Enabled
	}
	return domain.PreferencesFrom(enabled), nil
}
