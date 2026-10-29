package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	listMembersOp = "cabal.ListMembers"

	ListMembersDefault = 50
	listMembersMax     = 200
)

type ListMembersQuery struct {
	CabalID ids.CabalID
	Limit   int
	Cursor  string
}

type MemberPage struct {
	Items      []Member
	NextCursor string
}

func ListMembersPage(
	ctx context.Context, q sqlc.DBTX, users UserCards, req ListMembersQuery,
) (MemberPage, error) {
	limit := req.Limit
	if limit < 1 || limit > listMembersMax {
		return MemberPage{}, errs.New(errs.CodeInvalidInput, listMembersOp, slog.String("field", "limit"))
	}
	cursor, err := decodeMembersCursor(req.Cursor)
	if err != nil {
		return MemberPage{}, err
	}
	dbq := sqlc.New(q)
	if _, err := dbq.FindCabal(ctx, req.CabalID.UUID()); errors.Is(err, sql.ErrNoRows) {
		return MemberPage{}, errs.New(errs.CodeCabalNotFound, listMembersOp)
	} else if err != nil {
		return MemberPage{}, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	rows, err := dbq.ListMembersPage(ctx, sqlc.ListMembersPageParams{
		CabalID: req.CabalID.UUID(), HasCursor: cursor.ok, CursorJoinedAt: cursor.joinedAt, CursorUserID: cursor.userID,
		RowLimit: int32(limit) + 1,
	})
	if err != nil {
		return MemberPage{}, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	page := MemberPage{Items: []Member{}}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.NextCursor = encodeMembersCursor(last.JoinedAt, last.UserID)
	}
	if len(rows) == 0 {
		return page, nil
	}
	userIDs := make([]ids.UserID, len(rows))
	for i, row := range rows {
		userIDs[i] = ids.UserIDFrom(row.UserID)
	}
	cards, err := users.UsersByID(ctx, userIDs)
	if err != nil {
		return MemberPage{}, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	for _, row := range rows {
		page.Items = append(page.Items, Member{
			Person: person(row.UserID, cards), Role: row.Role, CanVote: row.CanVote, JoinedAt: row.JoinedAt,
		})
	}
	return page, nil
}

type membersCursor struct {
	ok       bool
	joinedAt time.Time
	userID   uuid.UUID
}

func encodeMembersCursor(joinedAt time.Time, userID uuid.UUID) string {
	raw := strconv.FormatInt(joinedAt.UnixMicro(), 10) + "\x00" + userID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeMembersCursor(raw string) (membersCursor, error) {
	if raw == "" {
		return membersCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return membersCursor{}, errs.Wrap(err, errs.CodeInvalidInput, listMembersOp, slog.String("field", "cursor"))
	}
	micros, id, found := strings.Cut(string(body), "\x00")
	if !found {
		return membersCursor{}, errs.New(errs.CodeInvalidInput, listMembersOp, slog.String("field", "cursor"))
	}
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return membersCursor{}, errs.Wrap(err, errs.CodeInvalidInput, listMembersOp, slog.String("field", "cursor"))
	}
	userID, err := uuid.Parse(id)
	if err != nil {
		return membersCursor{}, errs.Wrap(err, errs.CodeInvalidInput, listMembersOp, slog.String("field", "cursor"))
	}
	return membersCursor{ok: true, joinedAt: time.UnixMicro(at).UTC(), userID: userID}, nil
}
