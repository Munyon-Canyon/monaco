package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PendingInvite struct {
	ID              uuid.UUID
	CabalID         ids.CabalID
	CabalName       string
	PictureURL      string
	MemberCount     int
	InviterID       ids.UserID
	InviterName     string
	InviterPhotoURL string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

type InviteReader interface {
	PendingInvites(ctx context.Context, user ids.UserID, now time.Time) ([]PendingInvite, error)
}
