package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Members interface {
	IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error)
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
}

type ChatMembers interface {
	Members
	Members(ctx context.Context, id ids.CabalID) ([]cabalport.MemberView, error)
}

type ChatMessage struct {
	ID            uuid.UUID
	CabalID       ids.CabalID
	AuthorID      ids.UserID
	Body          string
	CreatedAt     time.Time
	ParentID      uuid.UUID
	AlsoInChannel bool
	ReplyCount    int32
	LastReplyAt   *time.Time
	ProposalID    uuid.UUID
	Deleted       bool
	SeenCount     *int
}

func requireMember(ctx context.Context, members Members, cabal ids.CabalID, user ids.UserID) error {
	const op = "social.requireMember"
	ok, err := members.IsMember(ctx, cabal, user)
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), op)
	}
	if !ok {
		return errs.New(errs.CodeNotCabalMember, op)
	}
	return nil
}

func lockChatMessage(ctx context.Context, q *sqlc.Queries, id uuid.UUID) (sqlc.LockChatMessageRow, bool, error) {
	row, err := q.LockChatMessage(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.LockChatMessageRow{}, false, nil
	}
	if err != nil {
		return sqlc.LockChatMessageRow{}, false, errs.Wrap(err, errs.CodeInternal, "social.lockChatMessage")
	}
	return row, true, nil
}

func lockParent(ctx context.Context, q *sqlc.Queries, cabal ids.CabalID, id uuid.UUID) error {
	row, found, err := lockChatMessage(ctx, q, id)
	if err != nil {
		return err
	}
	if !found {
		return errs.New(errs.CodeChatParentNotFound, "social.lockParent")
	}
	return domain.ValidateReply(cabal, domain.ChatParent{
		CabalID: ids.CabalIDFrom(row.CabalID), ParentID: row.ParentID.Bytes, Deleted: row.Deleted,
	})
}
