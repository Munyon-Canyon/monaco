package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UserCard = port.UserCard

type Users interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]port.UserCard, error)
	UserIDsByHandles(ctx context.Context, handles []string) (map[string]ids.UserID, error)
	UsersByPhoneHashes(ctx context.Context, hashes [][]byte) (map[string]ids.UserID, error)
}

func statusOf(ctx context.Context, users Users, id ids.UserID) (domain.AccountStatus, error) {
	cards, err := users.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return domain.AccountUnknown, err
	}
	card, ok := cards[id]
	switch {
	case !ok:
		return domain.AccountUnknown, nil
	case card.Deleted:
		return domain.AccountDeleted, nil
	default:
		return domain.AccountStatus(card.AccountStatus), nil
	}
}
