package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	ChatPageDefault = 50
	ChatPageMax     = 100
)

type ChatChannelQuery struct {
	CabalID ids.CabalID
	Viewer  ids.UserID
	Before  *uuid.UUID
	After   *uuid.UUID
	Limit   int
	Moments bool
}

type ChatThreadQuery struct {
	CabalID  ids.CabalID
	Viewer   ids.UserID
	ParentID uuid.UUID
	Before   *uuid.UUID
	Limit    int
	Moments  bool
}

type ChatThread struct {
	Parent  ChatMessage
	Replies []ChatMessage
}

func ListChatChannel(ctx context.Context, db sqlc.DBTX, members Members, q ChatChannelQuery) ([]ChatMessage, error) {
	const op = "social.ListChatChannel"
	if q.Before != nil && q.After != nil {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("cursor", "before and after"))
	}
	limit, err := checkChatRead(ctx, members, q.CabalID, q.Viewer, q.Limit)
	if err != nil {
		return nil, err
	}
	reads := sqlc.New(db)
	var rows []channelRow
	if q.After != nil {
		rows, err = listAfter(ctx, reads, q, limit)
	} else {
		rows, err = listBefore(ctx, reads, q, limit)
	}
	if err != nil {
		return nil, err
	}
	page := channelPage(rows)
	if q.Before != nil {
		return page, nil
	}
	return page, withSeenCount(ctx, reads, q.CabalID, page)
}

func listAfter(ctx context.Context, reads *sqlc.Queries, q ChatChannelQuery, limit int32) ([]channelRow, error) {
	at, err := chatCursor(ctx, reads, q.CabalID, *q.After)
	if err != nil {
		return nil, err
	}
	after, err := reads.ListChatChannelAfter(ctx, sqlc.ListChatChannelAfterParams{
		CabalID: q.CabalID.UUID(), Viewer: q.Viewer.UUID(), AfterAt: at.At, AfterID: at.ID, RowLimit: limit,
		Moments: q.Moments,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.ListChatChannel")
	}
	return channelRows(after, func(r sqlc.ListChatChannelAfterRow) channelRow {
		return channelRow{sqlc.CabalMessage{
			ID: r.ID, CabalID: r.CabalID, AuthorID: r.AuthorID, Body: r.Body, CreatedAt: r.CreatedAt,
			ParentID: r.ParentID, AlsoInChannel: r.AlsoInChannel, ReplyCount: r.ReplyCount,
			LastReplyAt: r.LastReplyAt, ProposalID: r.ProposalID, DeletedAt: r.DeletedAt, Kind: r.Kind,
			Moment: r.Moment, PinnedAt: r.PinnedAt, PinnedBy: r.PinnedBy,
		}, r.Blocked}
	}), nil
}

func listBefore(ctx context.Context, reads *sqlc.Queries, q ChatChannelQuery, limit int32) ([]channelRow, error) {
	params := sqlc.ListChatChannelParams{
		CabalID: q.CabalID.UUID(), Viewer: q.Viewer.UUID(), RowLimit: limit, Moments: q.Moments,
	}
	var err error
	if params.HasBefore, params.BeforeAt, params.BeforeID, err = optionalCursor(
		ctx,
		reads,
		q.CabalID,
		q.Before,
	); err != nil {
		return nil, err
	}
	before, err := reads.ListChatChannel(ctx, params)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.ListChatChannel")
	}
	return channelRows(before, func(r sqlc.ListChatChannelRow) channelRow {
		return channelRow{sqlc.CabalMessage{
			ID: r.ID, CabalID: r.CabalID, AuthorID: r.AuthorID, Body: r.Body, CreatedAt: r.CreatedAt,
			ParentID: r.ParentID, AlsoInChannel: r.AlsoInChannel, ReplyCount: r.ReplyCount,
			LastReplyAt: r.LastReplyAt, ProposalID: r.ProposalID, DeletedAt: r.DeletedAt, Kind: r.Kind,
			Moment: r.Moment, PinnedAt: r.PinnedAt, PinnedBy: r.PinnedBy,
		}, r.Blocked}
	}), nil
}

func GetChatThread(ctx context.Context, db sqlc.DBTX, members Members, q ChatThreadQuery) (ChatThread, error) {
	const op = "social.GetChatThread"
	limit, err := checkChatRead(ctx, members, q.CabalID, q.Viewer, q.Limit)
	if err != nil {
		return ChatThread{}, err
	}
	reads := sqlc.New(db)
	row, err := reads.GetChatMessage(ctx, sqlc.GetChatMessageParams{ID: q.ParentID, CabalID: q.CabalID.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ChatThread{}, errs.New(errs.CodeChatMessageNotFound, op)
	case err != nil:
		return ChatThread{}, errs.Wrap(err, errs.CodeInternal, op)
	case row.ParentID.Valid:
		return ChatThread{}, errs.New(errs.CodeChatParentIsReply, op)
	}
	blocked, err := reads.IsBlocked(ctx, sqlc.IsBlockedParams{Blocker: q.Viewer.UUID(), Blocked: row.AuthorID.Bytes})
	if err != nil {
		return ChatThread{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	parent := storedMessage(row)
	if momentHidden(parent, q.Moments) {
		return ChatThread{}, errs.New(errs.CodeChatMessageNotFound, op, slog.String("kind", string(parent.Kind)))
	}
	if blocked {
		parent.Deleted, parent.Body = true, ""
	}
	if parent.Deleted && row.ReplyCount == 0 {
		return ChatThread{}, errs.New(errs.CodeChatMessageNotFound, op, slog.Bool("deleted", true))
	}
	params, err := replyParams(ctx, reads, q, limit)
	if err != nil {
		return ChatThread{}, err
	}
	replies, err := reads.ListChatReplies(ctx, params)
	if err != nil {
		return ChatThread{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	slices.Reverse(replies)
	return ChatThread{Parent: parent, Replies: storedMessages(replies)}, nil
}

func channelPage(rows []channelRow) []ChatMessage {
	page := make([]ChatMessage, 0, len(rows))
	for _, row := range rows {
		m := storedMessage(row.CabalMessage)
		if m.Kind == domain.ChatKindSystem && m.Moment == nil {
			continue
		}
		if row.blocked {
			m.Deleted, m.Body = true, ""
		}
		page = append(page, m)
	}
	return page
}

func replyParams(
	ctx context.Context, reads *sqlc.Queries, q ChatThreadQuery, limit int32,
) (sqlc.ListChatRepliesParams, error) {
	params := sqlc.ListChatRepliesParams{ParentID: q.ParentID, Viewer: q.Viewer.UUID(), RowLimit: limit}
	if q.Before == nil {
		return params, nil
	}
	at, err := threadCursor(ctx, reads, q)
	params.HasBefore, params.BeforeAt, params.BeforeID = true, at.At, at.ID
	return params, err
}

func momentHidden(m ChatMessage, moments bool) bool {
	return m.Kind == domain.ChatKindSystem && (!moments || m.Moment == nil)
}

func checkChatRead(
	ctx context.Context, members Members, cabal ids.CabalID, viewer ids.UserID, limit int,
) (int32, error) {
	if limit < 1 || limit > ChatPageMax {
		return 0, errs.New(errs.CodeInvalidInput, "social.checkChatRead", slog.Int("limit", limit))
	}
	return int32(limit), requireMember(ctx, members, cabal, viewer)
}

func optionalCursor(
	ctx context.Context, reads *sqlc.Queries, cabal ids.CabalID, id *uuid.UUID,
) (bool, time.Time, uuid.UUID, error) {
	if id == nil {
		return false, time.Time{}, uuid.Nil, nil
	}
	at, err := chatCursor(ctx, reads, cabal, *id)
	return err == nil, at.At, at.ID, err
}

func threadCursor(ctx context.Context, reads *sqlc.Queries, q ChatThreadQuery) (domain.Keyset, error) {
	row, err := reads.GetChatMessage(ctx, sqlc.GetChatMessageParams{ID: *q.Before, CabalID: q.CabalID.UUID()})
	if err != nil {
		return domain.Keyset{}, cursorError(err, *q.Before)
	}
	if !row.ParentID.Valid || row.ParentID.Bytes != q.ParentID {
		return domain.Keyset{}, errs.New(
			errs.CodeInvalidInput, "social.threadCursor", slog.String("cursor", q.Before.String()),
		)
	}
	return domain.Keyset{At: row.CreatedAt, ID: row.ID}, nil
}

func chatCursor(ctx context.Context, reads *sqlc.Queries, cabal ids.CabalID, id uuid.UUID) (domain.Keyset, error) {
	row, err := reads.GetChatCursor(ctx, sqlc.GetChatCursorParams{ID: id, CabalID: cabal.UUID()})
	if err != nil {
		return domain.Keyset{}, cursorError(err, id)
	}
	return domain.Keyset{At: row.CreatedAt, ID: row.ID}, nil
}

func cursorError(err error, id uuid.UUID) error {
	const op = "social.chatCursor"
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeChatMessageNotFound, op, slog.String("cursor", id.String()))
	}
	return errs.Wrap(err, errs.CodeInternal, op)
}

type channelRow struct {
	sqlc.CabalMessage
	blocked bool
}

func channelRows[T any](rows []T, to func(T) channelRow) []channelRow {
	out := make([]channelRow, len(rows))
	for i, r := range rows {
		out[i] = to(r)
	}
	return out
}

func storedMessages(rows []sqlc.CabalMessage) []ChatMessage {
	out := make([]ChatMessage, len(rows))
	for i, row := range rows {
		out[i] = storedMessage(row)
	}
	return out
}

func storedMessage(row sqlc.CabalMessage) ChatMessage {
	m := ChatMessage{
		ID:            row.ID,
		Kind:          domain.ChatKind(row.Kind),
		CabalID:       ids.CabalIDFrom(row.CabalID),
		AuthorID:      ids.UserIDFrom(row.AuthorID.Bytes),
		Body:          row.Body.String,
		CreatedAt:     row.CreatedAt.UTC(),
		ParentID:      row.ParentID.Bytes,
		AlsoInChannel: row.AlsoInChannel,
		ReplyCount:    row.ReplyCount,
		ProposalID:    row.ProposalID.Bytes,
		Deleted:       row.DeletedAt.Valid,
	}
	if row.LastReplyAt.Valid {
		at := row.LastReplyAt.Time.UTC()
		m.LastReplyAt = &at
	}
	if row.PinnedAt.Valid {
		at := row.PinnedAt.Time.UTC()
		m.PinnedAt, m.PinnedBy = &at, ids.UserIDFrom(row.PinnedBy.Bytes)
	}
	if m.Deleted {
		m.Body = ""
	}
	if m.Kind == domain.ChatKindSystem {
		if moment, err := domain.ParseChatMoment(row.Moment); err == nil {
			m.Moment = &moment
		}
	}
	return m
}
