package feed

import (
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Item struct {
	Kind    Kind
	RefID   uuid.UUID
	CabalID ids.CabalID
	ActorID ids.UserID
	AssetID uuid.UUID
	Body    string
	Status  string
	Payload Payload
}

type Scope string

const (
	ScopeAll       Scope = "all"
	ScopeFollowing Scope = "following"
	ScopeMine      Scope = "mine"
)

func ParseScope(raw string) (Scope, error) {
	switch Scope(raw) {
	case "", ScopeAll:
		return ScopeAll, nil
	case ScopeFollowing:
		return ScopeFollowing, nil
	case ScopeMine:
		return ScopeMine, nil
	default:
		return "", errs.New(errs.CodeInvalidInput, "feed.ParseScope", slog.String("scope", raw))
	}
}
