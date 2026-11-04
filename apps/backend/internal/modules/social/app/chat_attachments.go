package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
)

type chatAttachment struct {
	att   *domain.Attachment
	stock *StockCard
}

func LoadChatAttachments(ctx context.Context, db sqlc.DBTX, stocks StockCards, messages []ChatMessage) error {
	var pending []uuid.UUID
	for _, m := range messages {
		if !m.Deleted && m.Attachment == nil {
			pending = append(pending, m.ID)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	loaded, err := readChatAttachments(ctx, db, stocks, pending)
	if err != nil {
		return err
	}
	for i, m := range messages {
		if l, ok := loaded[m.ID]; ok {
			messages[i].Attachment, messages[i].Stock = l.att, l.stock
		}
	}
	return nil
}

func readChatAttachments(
	ctx context.Context, db sqlc.DBTX, stocks StockCards, messageIDs []uuid.UUID,
) (map[uuid.UUID]chatAttachment, error) {
	const op = "social.LoadChatAttachments"
	rows, err := sqlc.New(db).ListChatAttachments(ctx, messageIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make(map[uuid.UUID]chatAttachment, len(rows))
	var assetIDs []uuid.UUID
	for _, row := range rows {
		att := domain.DecodeAttachment(row.Kind, row.Payload)
		if att == nil {
			continue
		}
		out[row.MessageID] = chatAttachment{att: att}
		if att.Stock != nil {
			assetIDs = append(assetIDs, att.Stock.AssetID)
		}
	}
	if len(assetIDs) == 0 {
		return out, nil
	}
	cards, err := stocks.StockCards(ctx, assetIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	for id, l := range out {
		if card, ok := cards[stockAsset(l.att)]; ok {
			l.stock = &card
			out[id] = l
		}
	}
	return out, nil
}

func stockAsset(att *domain.Attachment) uuid.UUID {
	if att.Stock == nil {
		return uuid.Nil
	}
	return att.Stock.AssetID
}
