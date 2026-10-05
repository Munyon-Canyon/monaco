package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Mute struct {
	User       ids.UserID
	TargetType string
	TargetID   string
}

type MuteHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewMuteHandler(uow *db.UnitOfWork, clk clock.Clock) *MuteHandler {
	return &MuteHandler{uow: uow, clock: clk}
}

func (h *MuteHandler) Handle(ctx context.Context, cmd Mute) error {
	target, err := parseMuteTarget(cmd.TargetType, cmd.TargetID)
	if err != nil {
		return err
	}
	if target.typ == "user" && target.id == cmd.User.UUID() {
		return errs.New(errs.CodeInvalidInput, "social.Mute", slog.String("target_id", cmd.TargetID))
	}
	label, err := muteLabel(ctx, h.uow, target)
	if err != nil {
		return err
	}
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		err := sqlc.New(tx.Queries()).InsertFeedMute(ctx, sqlc.InsertFeedMuteParams{
			UserID: cmd.User.UUID(), TargetType: target.typ, TargetID: target.raw,
			Label: label, CreatedAt: h.clock.Now().UTC(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, "social.Mute")
		}
		return nil
	})
}

type muteTarget struct {
	typ string
	raw string
	id  uuid.UUID
}

func parseMuteTarget(typ, raw string) (muteTarget, error) {
	t := muteTarget{typ: typ, raw: raw}
	if typ == "kind" {
		kind, err := feed.ParseKind(raw)
		if err != nil {
			return muteTarget{}, err
		}
		t.raw = string(kind)
		return t, nil
	}
	if typ != "cabal" && typ != "asset" && typ != "user" && typ != "item" {
		return muteTarget{}, errs.New(errs.CodeInvalidInput, "social.parseMuteTarget", slog.String("target_type", typ))
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return muteTarget{}, errs.Wrap(err, errs.CodeInvalidInput, "social.parseMuteTarget")
	}
	t.id, t.raw = id, id.String()
	return t, nil
}

func muteLabel(ctx context.Context, uow *db.UnitOfWork, target muteTarget) (string, error) {
	if target.typ == "kind" {
		return muteKindLabel(feed.Kind(target.raw)), nil
	}
	label, err := sqlc.New(uow.Reads()).FeedMuteLabel(ctx, sqlc.FeedMuteLabelParams{
		TargetType: target.typ, TargetID: target.id,
	})
	if err == nil {
		return label, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "social.Mute", slog.String("target_id", target.raw))
}

func muteKindLabel(kind feed.Kind) string {
	return map[feed.Kind]string{
		feed.KindProposal: "Proposals", feed.KindTrade: "Trades", feed.KindPriceMove: "Price moves",
		feed.KindCabalCreated: "New cabals", feed.KindMemberJoined: "New members",
	}[kind]
}
