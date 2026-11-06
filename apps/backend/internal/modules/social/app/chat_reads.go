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
	if err := WithProposalReplies(ctx, db, page); err != nil {
		return nil, err
	}
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
			Moment: r.Moment, PinnedAt: r.PinnedAt, PinnedBy: r.PinnedBy, CommentID: r.CommentID,
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
			Moment: r.Moment, PinnedAt: r.PinnedAt, PinnedBy: r.PinnedBy, CommentID: r.CommentID,
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
	parent, err := threadParent(ctx, reads, q, row)
	if err != nil {
		return ChatThread{}, err
	}
	if isProposalCard(parent) {
		return proposalThread(ctx, db, reads, q, parent, limit)
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

func threadParent(
	ctx context.Context, reads *sqlc.Queries, q ChatThreadQuery, row sqlc.CabalMessage,
) (ChatMessage, error) {
	const op = "social.GetChatThread"
	blocked, err := reads.IsBlocked(ctx, sqlc.IsBlockedParams{Blocker: q.Viewer.UUID(), Blocked: row.AuthorID.Bytes})
	if err != nil {
		return ChatMessage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	parent := storedMessage(row)
	if momentHidden(parent, q.Moments) {
		return ChatMessage{}, errs.New(errs.CodeChatMessageNotFound, op, slog.String("kind", string(parent.Kind)))
	}
	if blocked {
		parent.Deleted, parent.Body = true, ""
	}
	return parent, nil
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
		CommentID:     row.CommentID.Bytes,
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

func isProposalCard(m ChatMessage) bool {
	return m.Kind == domain.ChatKindProposal && m.ProposalID != uuid.Nil
}

type ReplyStat struct {
	Count int32
	Last  *time.Time
}

func ProposalReplyStats(
	ctx context.Context, db sqlc.DBTX, proposals []uuid.UUID,
) (map[uuid.UUID]ReplyStat, error) {
	out := make(map[uuid.UUID]ReplyStat, len(proposals))
	if len(proposals) == 0 {
		return out, nil
	}
	rows, err := sqlc.New(db).ProposalChatReplyStats(ctx, proposals)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.ProposalReplyStats")
	}
	for _, row := range rows {
		last := row.LastReplyAt.UTC()
		out[row.ProposalID] = ReplyStat{Count: row.ReplyCount, Last: &last}
	}
	return out, nil
}

func WithProposalReplies(ctx context.Context, db sqlc.DBTX, messages []ChatMessage) error {
	var proposals []uuid.UUID
	for _, m := range messages {
		if isProposalCard(m) {
			proposals = append(proposals, m.ProposalID)
		}
	}
	stats, err := ProposalReplyStats(ctx, db, proposals)
	if err != nil || len(proposals) == 0 {
		return err
	}
	for i, m := range messages {
		if isProposalCard(m) {
			stat := stats[m.ProposalID]
			messages[i].ReplyCount, messages[i].LastReplyAt = stat.Count, stat.Last
		}
	}
	return nil
}

func proposalThread(
	ctx context.Context, db sqlc.DBTX, reads *sqlc.Queries, q ChatThreadQuery, parent ChatMessage, limit int32,
) (ChatThread, error) {
	const op = "social.GetChatThread"
	item, err := reads.GetProposalFeedItem(ctx, parent.ProposalID)
	noItem := errors.Is(err, sql.ErrNoRows)
	if err != nil && !noItem {
		return ChatThread{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if !noItem {
		cards := []ChatMessage{parent}
		if err := WithProposalReplies(ctx, db, cards); err != nil {
			return ChatThread{}, err
		}
		parent = cards[0]
	}
	if parent.Deleted && parent.ReplyCount == 0 {
		return ChatThread{}, errs.New(errs.CodeChatMessageNotFound, op, slog.Bool("deleted", true))
	}
	if noItem {
		return ChatThread{Parent: parent}, nil
	}
	params := sqlc.ListProposalChatRepliesParams{FeedObjectID: item, Viewer: q.Viewer.UUID(), RowLimit: limit}
	if q.Before != nil {
		at, err := reads.GetProposalCommentCursor(ctx, sqlc.GetProposalCommentCursorParams{
			ID: *q.Before, FeedObjectID: item,
		})
		if err != nil {
			return ChatThread{}, cursorError(err, *q.Before)
		}
		params.HasBefore, params.BeforeAt, params.BeforeID = true, at.CreatedAt, at.ID
	}
	rows, err := reads.ListProposalChatReplies(ctx, params)
	if err != nil {
		return ChatThread{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	slices.Reverse(rows)
	card := proposalCard{id: parent.ID, cabal: parent.CabalID, proposal: parent.ProposalID}
	replies := make([]ChatMessage, len(rows))
	for i, row := range rows {
		replies[i] = chatMessageOfComment(card, proposalReplyComment(row))
	}
	return ChatThread{Parent: parent, Replies: replies}, nil
}

func proposalReplyComment(row sqlc.ListProposalChatRepliesRow) Comment {
	c := commentOf(sqlc.FeedComment{
		ID: row.ID, FeedObjectID: row.FeedObjectID, AuthorID: row.AuthorID, ParentCommentID: row.ParentCommentID,
		ReplyToUserID: row.ReplyToUserID, ReplyToCommentID: row.ReplyToCommentID, Body: row.Body,
		CreatedAt: row.CreatedAt,
	})
	c.Attachment = domain.DecodeAttachment(row.AttachmentKind.String, row.AttachmentPayload)
	if row.QuoteID.Valid {
		c.ReplyTo = quoteOf(
			row.QuoteID.Bytes, row.QuoteAuthorID.Bytes, row.QuoteParentID.Bytes, row.QuoteBody.String,
			row.QuoteHasAttachment, row.QuoteDeleted, row.QuoteBlocked,
		)
	}
	return c
}

func ChatMessageReactions(
	ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, messages []ChatMessage,
) (map[uuid.UUID]ReactionSummary, error) {
	var chatIDs, commentIDs []uuid.UUID
	for _, m := range messages {
		if m.CommentID == uuid.Nil {
			chatIDs = append(chatIDs, m.ID)
		} else {
			commentIDs = append(commentIDs, m.CommentID)
		}
	}
	out, err := ChatReactions(ctx, reads, viewer, chatIDs)
	if err != nil {
		return nil, err
	}
	found, err := commentReactions(ctx, sqlc.New(reads), viewer, commentIDs)
	if err != nil {
		return nil, err
	}
	for _, m := range messages {
		if summary, ok := found[m.CommentID]; ok && m.CommentID != uuid.Nil {
			out[m.ID] = summary
		}
	}
	return out, nil
}

func LoadCommentViews(
	ctx context.Context, reads sqlc.DBTX, stocks StockCards, viewer ids.UserID, messages []ChatMessage, now time.Time,
) error {
	var held []*Comment
	var at []int
	for i, m := range messages {
		if m.CommentID != uuid.Nil && m.Attachment != nil {
			held, at = append(held, &Comment{ID: m.CommentID, Attachment: m.Attachment}), append(at, i)
		}
	}
	if len(held) == 0 {
		return nil
	}
	if err := pollViews(ctx, reads, viewer, held, now); err != nil {
		return err
	}
	cards, err := commentStocks(ctx, stocks, held)
	if err != nil {
		return err
	}
	for n, i := range at {
		messages[i].Poll = held[n].Poll
		if att := messages[i].Attachment; att.Stock != nil {
			if card, ok := cards[att.Stock.AssetID]; ok {
				messages[i].Stock = &card
			}
		}
	}
	return nil
}

func commentStocks(
	ctx context.Context, stocks StockCards, held []*Comment,
) (map[uuid.UUID]StockCard, error) {
	var assetIDs []uuid.UUID
	for _, c := range held {
		if c.Attachment.Stock != nil {
			assetIDs = append(assetIDs, c.Attachment.Stock.AssetID)
		}
	}
	if len(assetIDs) == 0 {
		return map[uuid.UUID]StockCard{}, nil
	}
	cards, err := stocks.StockCards(ctx, assetIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "social.LoadCommentViews")
	}
	return cards, nil
}
