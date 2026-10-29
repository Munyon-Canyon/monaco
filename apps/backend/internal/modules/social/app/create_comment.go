package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Comment struct {
	ID             uuid.UUID
	FeedObjectID   uuid.UUID
	AuthorID       ids.UserID
	ParentID       uuid.UUID
	ReplyToUserID  ids.UserID
	Body           string
	CreatedAt      time.Time
	Deleted        bool
	RemovedByAdmin bool
	Quote          *CommentQuote
}

type CommentQuote struct {
	CommentID uuid.UUID
	RootID    uuid.UUID
	AuthorID  ids.UserID
	Excerpt   *string
	Deleted   bool
	Folded    bool
}

type quoteSource struct {
	ID       uuid.UUID
	AuthorID uuid.UUID
	ParentID uuid.UUID
	Body     string
	Deleted  bool
	Blocked  bool
}

func (s quoteSource) quote() *CommentQuote {
	body := domain.CommentBody(s.Body)
	target := domain.CommentTarget{ID: s.ID, AuthorID: s.AuthorID, ParentID: s.ParentID, Deleted: s.Deleted}
	return &CommentQuote{
		CommentID: s.ID, RootID: target.RootID(), AuthorID: ids.UserIDFrom(s.AuthorID),
		Excerpt: domain.QuoteExcerpt(&body, s.Deleted || s.Blocked), Deleted: s.Deleted, Folded: s.ParentID != uuid.Nil,
	}
}

type CreateComment struct {
	FeedObjectID uuid.UUID
	Author       ids.UserID
	Body         domain.CommentBody
	ParentID     uuid.UUID
}

type CommentDeps struct {
	UoW     *db.UnitOfWork
	Reads   sqlc.DBTX
	Members Members
	Users   Users
	IDs     ids.Generator
	Clock   clock.Clock
	Hints   Hints
}

type Hints interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

const FeedHint = "global.feed"

type CreateCommentHandler struct {
	d CommentDeps
}

func NewCreateCommentHandler(d CommentDeps) *CreateCommentHandler {
	return &CreateCommentHandler{d: d}
}

func (h *CreateCommentHandler) Handle(ctx context.Context, cmd CreateComment) (Comment, error) {
	const op = "social.CreateComment"
	item, thread, err := ResolveThread(ctx, sqlc.New(h.d.Reads), cmd.FeedObjectID)
	if err != nil {
		return Comment{}, err
	}
	allowed, rule, err := CanReply(
		ctx, h.d.Members, feed.Kind(thread.Kind), ids.CabalIDFrom(thread.CabalID.Bytes), cmd.Author,
	)
	if err != nil {
		return Comment{}, err
	}
	if !allowed {
		return Comment{}, errs.New(
			errs.CodeCommentMembersOnly,
			op,
			slog.String("feed_object_id", cmd.FeedObjectID.String()),
		)
	}
	cmd.FeedObjectID = thread.ID
	reply, err := h.replyDraft(ctx, thread, cmd)
	if err != nil {
		return Comment{}, err
	}
	var created Comment
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		created, err = h.store(ctx, tx, postedOn{item: item, thread: thread, rule: rule}, cmd, reply)
		return err
	})
	if err != nil {
		return Comment{}, err
	}
	observability.Info(ctx, observability.SocialCommentCreated,
		slog.String("comment_id", created.ID.String()), slog.String("feed_object_id", created.FeedObjectID.String()))
	return created, nil
}

type postedOn struct {
	item   sqlc.GetCommentItemRow
	thread sqlc.GetCommentItemRow
	rule   ReplyRule
}

func (h *CreateCommentHandler) store(
	ctx context.Context, tx db.Tx, at postedOn, cmd CreateComment, reply replyDraft,
) (Comment, error) {
	const op = "social.CreateComment"
	q := sqlc.New(tx.Queries())
	target, place, err := loadParent(ctx, q, cmd, at.thread.ActorID.Bytes)
	if err != nil {
		return Comment{}, err
	}
	row, err := q.InsertComment(ctx, sqlc.InsertCommentParams{
		ID: h.d.IDs.NewV7(), FeedObjectID: cmd.FeedObjectID, AuthorID: cmd.Author.UUID(),
		ParentCommentID: place.ParentID, ReplyToUserID: place.ReplyToUserID, ReplyToCommentID: place.ReplyToID,
		Body: cmd.Body.String(), CreatedAt: h.d.Clock.Now().UTC(),
	})
	if err != nil {
		return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if _, err := q.BumpCommentCount(ctx, sqlc.BumpCommentCountParams{Delta: 1, ID: cmd.FeedObjectID}); err != nil {
		return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if err := reply.insert(ctx, q, h.d.IDs.NewV7(), row); err != nil {
		return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
	created := commentOf(row)
	if row.ReplyToCommentID.Valid {
		created.Quote = quoteOfTarget(target).quote()
	}
	return created, tx.Events.Append(ctx, createdEvent(at, row, target, cmd.Body))
}

type replyDraft struct {
	feedObjectID uuid.UUID
	payload      feed.Payload
	title        string
}

func (h *CreateCommentHandler) replyDraft(
	ctx context.Context, item sqlc.GetCommentItemRow, cmd CreateComment,
) (replyDraft, error) {
	if feed.Kind(item.Kind) != feed.KindProposal {
		return replyDraft{}, nil
	}
	proposal, err := feed.ParsePayload(item.Payload)
	if err != nil {
		return replyDraft{}, err
	}
	cards, err := h.d.Users.UsersByID(ctx, []ids.UserID{cmd.Author})
	if err != nil {
		return replyDraft{}, errs.Wrap(err, errs.CodeOf(err), "social.CreateComment")
	}
	card := cards[cmd.Author]
	name := card.DisplayName
	if name == "" {
		name = card.Handle
	}
	payload := feed.Payload{
		CabalName: proposal.CabalName, ActorName: name, ProposerName: proposal.ActorName, Symbol: proposal.Symbol,
		AssetName: proposal.AssetName, Action: proposal.Action, USDCMicros: proposal.USDCMicros,
		TokenAmount: proposal.TokenAmount, TokenDecimals: proposal.TokenDecimals,
		ShareOfPotBps: proposal.ShareOfPotBps, ProposalID: item.RefID,
	}
	return replyDraft{
		feedObjectID: item.ID, payload: payload, title: feed.RenderTitle(feed.KindReply, payload),
	}, nil
}

func (d replyDraft) insert(ctx context.Context, q *sqlc.Queries, id uuid.UUID, comment sqlc.FeedComment) error {
	if d.title == "" {
		return nil
	}
	return q.InsertFeedReply(ctx, sqlc.InsertFeedReplyParams{
		ID: id, FeedObjectID: d.feedObjectID, CommentID: comment.ID, ActorID: comment.AuthorID, Title: d.title,
		Body: comment.Body, Payload: d.payload.JSON(), At: comment.CreatedAt,
	})
}

func createdEvent(
	at postedOn, row sqlc.FeedComment, target sqlc.GetCommentTargetRow, body domain.CommentBody,
) events.CommentCreated {
	thread := at.thread
	threadPayload, _ := feed.ParsePayload(thread.Payload)
	itemPayload, _ := feed.ParsePayload(at.item.Payload)
	itemKind := feed.Kind(at.item.Kind)
	e := events.CommentCreated{
		V: 1, CommentID: row.ID, FeedObjectID: row.FeedObjectID, FeedKind: thread.Kind, RefType: thread.RefType,
		RefID: thread.RefID, CabalID: parentOf(thread.CabalID.Bytes), AuthorID: row.AuthorID,
		ParentCommentID: parentOf(row.ParentCommentID.Bytes), ParentAuthorID: parentOf(target.AuthorID),
		ParentDeleted: target.Deleted, ReplyToUserID: parentOf(row.ReplyToUserID.Bytes),
		ReplyToCommentID: parentOf(row.ReplyToCommentID.Bytes), ReplyToAuthorID: parentOf(target.AuthorID),
		ReplyToDeleted: target.Deleted, ReplyToRootID: parentOf(targetOf(target).RootID()),
		ReplyToExcerpt: replyToExcerpt(row, target),
		ItemActorID:    parentOf(thread.ActorID.Bytes), Excerpt: body.Excerpt(),
		ItemKind: at.item.Kind, ThreadFeedObjectID: thread.ID, ItemActorName: threadPayload.ActorName,
		ItemLabel: feed.RenderLabel(itemKind, itemPayload), ItemNoun: feed.RenderNoun(itemKind, itemPayload),
		ReplyRule: string(at.rule),
	}
	if threadPayload.CabalName != "" {
		e.CabalName = &threadPayload.CabalName
	}
	if feed.Kind(thread.Kind) == feed.KindProposal {
		e.ProposalID = parentOf(thread.RefID)
	}
	return e
}

func commentOf(row sqlc.FeedComment) Comment {
	return Comment{
		ID: row.ID, FeedObjectID: row.FeedObjectID, AuthorID: ids.UserIDFrom(row.AuthorID),
		ParentID: row.ParentCommentID.Bytes, ReplyToUserID: ids.UserIDFrom(row.ReplyToUserID.Bytes),
		Body: row.Body, CreatedAt: row.CreatedAt.UTC(), Deleted: row.DeletedAt.Valid,
		RemovedByAdmin: row.DeletedByKind == CommentDeletedByAdmin,
	}
}

func targetOf(t sqlc.GetCommentTargetRow) domain.CommentTarget {
	return domain.CommentTarget{ID: t.ID, AuthorID: t.AuthorID, ParentID: t.ParentCommentID.Bytes, Deleted: t.Deleted}
}

func quoteOfTarget(t sqlc.GetCommentTargetRow) quoteSource {
	return quoteSource{
		ID: t.ID, AuthorID: t.AuthorID, ParentID: t.ParentCommentID.Bytes, Body: t.Body, Deleted: t.Deleted,
		Blocked: t.Blocked,
	}
}

func replyToExcerpt(row sqlc.FeedComment, target sqlc.GetCommentTargetRow) *string {
	if !row.ReplyToCommentID.Valid {
		return nil
	}
	body := domain.CommentBody(target.Body)
	return domain.QuoteExcerpt(&body, target.Deleted)
}

func loadParent(
	ctx context.Context, q *sqlc.Queries, cmd CreateComment, threadAuthor uuid.UUID,
) (sqlc.GetCommentTargetRow, domain.CommentPlacement, error) {
	const op = "social.CreateComment"
	if cmd.ParentID == uuid.Nil {
		return sqlc.GetCommentTargetRow{}, domain.CommentPlacement{}, nil
	}
	target, err := q.GetCommentTarget(
		ctx,
		sqlc.GetCommentTargetParams{Viewer: cmd.Author.UUID(), ID: cmd.ParentID, FeedObjectID: cmd.FeedObjectID},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return target, domain.CommentPlacement{},
			errs.New(errs.CodeCommentParentMismatch, op, slog.String("parent_comment_id", cmd.ParentID.String()))
	case err != nil:
		return target, domain.CommentPlacement{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return target, domain.PlaceReply(targetOf(target), cmd.Author.UUID(), threadAuthor), nil
}
