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
	Attachment     *domain.Attachment
	Stock          *StockCard
	Poll           *PollView
	CreatedAt      time.Time
	Deleted        bool
	RemovedByAdmin bool
	ReplyTo        *CommentQuote
	Reactions      []Reaction
	ReactionCount  int32
}

type CommentQuote struct {
	CommentID     uuid.UUID
	RootID        uuid.UUID
	AuthorID      ids.UserID
	Excerpt       *string
	Deleted       bool
	Folded        bool
	HasAttachment bool
}

func quotedText(body string, attached bool) *domain.CommentBody {
	text := domain.CommentBody(body)
	if text == "" && attached {
		text = domain.GifDisplay
	}
	return &text
}

func quoteOf(id, author, parent uuid.UUID, body string, attached, deleted, blocked bool) *CommentQuote {
	quote := &CommentQuote{
		CommentID: id, RootID: id, AuthorID: ids.UserIDFrom(author), Deleted: deleted, Folded: parent != uuid.Nil,
		HasAttachment: attached && !deleted,
	}
	if parent != uuid.Nil {
		quote.RootID = parent
	}
	if !blocked {
		quote.Excerpt = domain.QuoteExcerpt(quotedText(body, attached), deleted)
	}
	return quote
}

type CreateComment struct {
	FeedObjectID uuid.UUID
	Author       ids.UserID
	Input        domain.CommentInput
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
	Stocks  StockCards
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
	stock, err := h.resolveAttachment(ctx, thread, cmd.Input.Attachment)
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
	if stock.ID != uuid.Nil {
		created.Stock = &stock
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
	body := ""
	if cmd.Input.Body != nil {
		body = cmd.Input.Body.String()
	}
	row, err := q.InsertComment(ctx, sqlc.InsertCommentParams{
		ID: h.d.IDs.NewV7(), FeedObjectID: cmd.FeedObjectID, AuthorID: cmd.Author.UUID(),
		ParentCommentID: place.ParentID, ReplyToUserID: place.ReplyToUserID, ReplyToCommentID: place.ReplyToCommentID,
		Body: body, CreatedAt: h.d.Clock.Now().UTC(),
	})
	if err != nil {
		return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if _, err := q.BumpCommentCount(ctx, sqlc.BumpCommentCountParams{Delta: 1, ID: cmd.FeedObjectID}); err != nil {
		return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if err := reply.insert(ctx, q, h.d.IDs.NewV7(), row, cmd.Input.Text().String()); err != nil {
		return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	created := commentOf(row)
	if att := cmd.Input.Attachment; att != nil {
		err := q.InsertCommentAttachment(ctx, sqlc.InsertCommentAttachmentParams{
			CommentID:    row.ID,
			FeedObjectID: row.FeedObjectID,
			Kind:         string(att.Kind),
			Payload:      att.Payload(row.CreatedAt),
			CreatedAt:    row.CreatedAt,
		})
		if err != nil {
			return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
		}
		created.Attachment, created.Poll, err = storePoll(ctx, q, row, att)
		if err != nil {
			return Comment{}, err
		}
	}
	tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
	if target.ID != uuid.Nil {
		created.ReplyTo = quoteOf(
			target.ID, target.AuthorID, target.ParentCommentID.Bytes, target.Body.String, target.HasAttachment,
			target.Deleted, target.Blocked,
		)
	}
	return created, tx.Events.Append(ctx, createdEvent(at, row, target, cmd.Input))
}

type replyDraft struct {
	feedObjectID uuid.UUID
	payload      feed.Payload
	title        string
}

func (h *CreateCommentHandler) resolveAttachment(
	ctx context.Context, thread sqlc.GetCommentItemRow, att *domain.Attachment,
) (StockCard, error) {
	const op = "social.CreateComment"
	switch {
	case att == nil:
		return StockCard{}, nil
	case att.Kind == domain.AttachmentThesisQuote:
		proposal, _ := feed.ParsePayload(thread.Payload)
		text, ok := att.Range.Cut(thread.Body.String)
		reason := "invalid_range"
		if feed.Kind(thread.Kind) != feed.KindProposal || thread.Body.String == "" {
			ok, reason = false, "no_thesis"
		}
		if !ok {
			return StockCard{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", reason))
		}
		att.Thesis = &domain.ThesisQuote{ProposalID: thread.RefID, Text: text, ProposerName: proposal.ActorName}
	case att.Kind == domain.AttachmentStock:
		cards, err := h.d.Stocks.StockCards(ctx, []uuid.UUID{att.Stock.AssetID})
		if err != nil {
			return StockCard{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
		card, found := cards[att.Stock.AssetID]
		if !found {
			return StockCard{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "asset_not_found"))
		}
		att.Stock = &domain.StockSnapshot{
			AssetID: card.ID, Symbol: card.Symbol, Name: card.Name, LogoURL: card.LogoURL, IsPreIPO: card.IsPreIPO,
		}
		return card, nil
	}
	return StockCard{}, nil
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

func (d replyDraft) insert(
	ctx context.Context, q *sqlc.Queries, id uuid.UUID, comment sqlc.FeedComment, body string,
) error {
	if d.title == "" {
		return nil
	}
	return q.InsertFeedReply(ctx, sqlc.InsertFeedReplyParams{
		ID: id, FeedObjectID: d.feedObjectID, CommentID: comment.ID, ActorID: comment.AuthorID, Title: d.title,
		Body: body, Payload: d.payload.JSON(), At: comment.CreatedAt,
	})
}

func createdEvent(
	at postedOn, row sqlc.FeedComment, target sqlc.GetCommentTargetRow, input domain.CommentInput,
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
		ItemActorID: parentOf(thread.ActorID.Bytes), Excerpt: input.Text().Excerpt(),
		ItemKind: at.item.Kind, ThreadFeedObjectID: thread.ID, ItemActorName: threadPayload.ActorName,
		ItemLabel: feed.RenderLabel(itemKind, itemPayload), ItemNoun: feed.RenderNoun(itemKind, itemPayload),
		ReplyRule: string(at.rule),
	}
	if input.Attachment != nil {
		e.AttachmentKind = string(input.Attachment.Kind)
		if input.Attachment.Poll != nil {
			e.PollQuestion = input.Attachment.Poll.Question
		}
	}
	if target.ID != uuid.Nil {
		answered := quotedText(target.Body.String, target.HasAttachment)
		root := domain.CommentTarget{ID: target.ID, ParentID: target.ParentCommentID.Bytes}.RootID()
		e.ReplyToCommentID, e.ReplyToAuthorID, e.ReplyToRootID = &target.ID, &target.AuthorID, &root
		e.ReplyToDeleted, e.ReplyToExcerpt = target.Deleted, domain.QuoteExcerpt(answered, target.Deleted)
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
		Body: row.Body.String, CreatedAt: row.CreatedAt.UTC(), Deleted: row.DeletedAt.Valid,
		RemovedByAdmin: row.DeletedByKind == CommentDeletedByAdmin,
	}
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
		sqlc.GetCommentTargetParams{ID: cmd.ParentID, FeedObjectID: cmd.FeedObjectID, Viewer: cmd.Author.UUID()},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return target, domain.CommentPlacement{},
			errs.New(errs.CodeCommentParentMismatch, op, slog.String("parent_comment_id", cmd.ParentID.String()))
	case err != nil:
		return target, domain.CommentPlacement{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return target, domain.PlaceReply(domain.CommentTarget{
		ID: target.ID, AuthorID: target.AuthorID, ParentID: target.ParentCommentID.Bytes,
	}, cmd.Author.UUID(), threadAuthor), nil
}
