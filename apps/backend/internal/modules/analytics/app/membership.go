package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type MembershipReader interface {
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
}
