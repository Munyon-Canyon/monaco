package adapters

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const cashoutSource = "cashout"

type Hints interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type Activity struct {
	Hints Hints
}

func (h Activity) Submitted(ctx context.Context, tx db.Tx, e events.TradeSubmitted, at time.Time) error {
	return h.trade(ctx, tx, e.Source, domain.Trade{
		SwapID: e.SwapID, CabalID: ids.CabalIDFrom(e.CabalID), Action: e.Action,
		In:  domain.Leg{Asset: domain.MintAsset(e.InMint), Amount: e.InAmount},
		Out: domain.Leg{Asset: domain.MintAsset(e.OutMint)}, TxSignature: e.TxSignature,
	}, domain.ActivityPending, at)
}

func (h Activity) Confirmed(ctx context.Context, tx db.Tx, e events.TradeConfirmed, at time.Time) error {
	return h.trade(ctx, tx, e.Source, domain.Trade{
		SwapID: e.SwapID, CabalID: ids.CabalIDFrom(e.CabalID), Action: e.Action,
		In:  domain.Leg{Asset: domain.MintAsset(e.InMint), Amount: e.InAmount},
		Out: domain.Leg{Asset: domain.MintAsset(e.OutMint), Amount: e.OutAmount}, TxSignature: e.TxSignature,
	}, domain.ActivityConfirmed, at)
}

func (h Activity) Failed(ctx context.Context, tx db.Tx, e events.TradeFailed, at time.Time) error {
	return h.trade(ctx, tx, e.Source, domain.Trade{
		SwapID: e.SwapID, CabalID: ids.CabalIDFrom(e.CabalID), Action: e.Action,
		In: domain.Leg{Asset: domain.MintAsset(e.InMint), Amount: e.InAmount},
	}, domain.ActivityFailed, at)
}

func (h Activity) trade(
	ctx context.Context,
	tx db.Tx,
	source events.TradeSource,
	t domain.Trade,
	status domain.ActivityStatus,
	at time.Time,
) error {
	if source.Kind == cashoutSource {
		return nil
	}
	a, err := domain.TradeActivity(t, status)
	if err != nil {
		return err
	}
	return h.record(ctx, tx, a, at)
}

func (h Activity) record(ctx context.Context, tx db.Tx, a domain.Activity, at time.Time) error {
	n, err := sqlc.New(tx.Queries()).UpsertActivity(ctx, sqlc.UpsertActivityParams{
		ID:          a.ID,
		CabalID:     a.CabalID.UUID(),
		Kind:        string(a.Kind),
		Status:      string(a.Status),
		ActorUserID: actor(a.Actor),
		Asset:       text(string(a.Asset)),
		UsdcMicros:  number(a.USDCMicros),
		Units:       number(a.Units),
		TxSignature: text(string(a.TxSignature)),
		At:          at,
	})
	if err != nil || n == 0 {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.Hints.PublishHint(ctx, "cabal."+a.CabalID.String()+".activity_changed", nil)
	})
	return nil
}

func (h Activity) FundSubmitted(ctx context.Context, tx db.Tx, e events.FundSubmitted, at time.Time) error {
	return h.record(ctx, tx, domain.FundActivity(domain.Fund{
		TransferID: e.TransferID, CabalID: ids.CabalIDFrom(e.CabalID), UserID: ids.UserIDFrom(e.UserID),
		USDCMicros: e.AmountMicros.Uint64(), TxSignature: e.TxSignature,
	}, domain.ActivityPending), at)
}

func (h Activity) Funded(ctx context.Context, tx db.Tx, e events.Funded, at time.Time) error {
	return h.record(ctx, tx, domain.FundActivity(domain.Fund{
		TransferID: e.TransferID, CabalID: ids.CabalIDFrom(e.CabalID), UserID: ids.UserIDFrom(e.UserID),
		USDCMicros: e.AmountMicros.Uint64(), ShareUnits: e.ShareUnits.Uint64(), TxSignature: e.TxSignature,
	}, domain.ActivityConfirmed), at)
}

func (h Activity) FundFailed(ctx context.Context, tx db.Tx, e events.FundFailed, at time.Time) error {
	return h.record(ctx, tx, domain.FundActivity(domain.Fund{
		TransferID: e.TransferID, CabalID: ids.CabalIDFrom(e.CabalID), UserID: ids.UserIDFrom(e.UserID),
		USDCMicros: e.AmountMicros.Uint64(),
	}, domain.ActivityFailed), at)
}

func actor(user ids.UserID) pgtype.UUID {
	return pgtype.UUID{Bytes: user.UUID(), Valid: !user.IsZero()}
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func number(v *uint64) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strconv.FormatUint(*v, 10), Valid: true}
}
