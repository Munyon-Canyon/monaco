package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type MemberWallets interface {
	MemberWallet(ctx context.Context, id ids.UserID) (identityport.MemberWallet, error)
}

type OnrampExchange struct {
	SessionID       uuid.UUID
	WalletAddress   chain.SolanaAddress
	SuggestedAmount *money.Micros
	USDCMint        string
}

type ExchangeOnrampTokenHandler struct {
	uow      *db.UnitOfWork
	clock    clock.Clock
	wallets  MemberWallets
	usdcMint string
}

func NewExchangeOnrampTokenHandler(
	uow *db.UnitOfWork, c clock.Clock, wallets MemberWallets, usdcMint string,
) *ExchangeOnrampTokenHandler {
	return &ExchangeOnrampTokenHandler{uow: uow, clock: c, wallets: wallets, usdcMint: usdcMint}
}

func (h *ExchangeOnrampTokenHandler) Handle(ctx context.Context, token domain.OnrampToken) (OnrampExchange, error) {
	const op = "funding.ExchangeOnrampToken"
	now := h.clock.Now()
	var out OnrampExchange
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := sqlc.New(tx.Queries()).OpenOnrampSession(ctx,
			sqlc.OpenOnrampSessionParams{TokenHash: token.Hash(), Now: now})
		if errors.Is(err, sql.ErrNoRows) {
			return errs.New(errs.CodeOnrampLinkInvalid, op, slog.String("reason", "unknown"))
		}
		if err != nil {
			return err
		}
		if !row.OpenedNow {
			return domain.RefuseOnrampToken(domain.OnrampStatus(row.Status), row.WasOpened, row.ExpiresAt, now)
		}
		user := ids.UserIDFrom(row.UserID)
		wallet, err := h.wallets.MemberWallet(ctx, user)
		if err != nil {
			return err
		}
		suggested, err := suggestedAmount(row.SuggestedAmountMicros)
		if err != nil {
			return err
		}
		out = OnrampExchange{
			SessionID: row.ID, WalletAddress: wallet.Address, SuggestedAmount: suggested, USDCMint: h.usdcMint,
		}
		ctx = observability.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: user.String()}.Key())
		return tx.Events.Append(ctx, statusChanged(row.ID, row.UserID, domain.OnrampCreated, domain.OnrampOpened,
			suggested, nil))
	})
	if err != nil {
		return OnrampExchange{}, err
	}
	return out, nil
}

func statusChanged(
	session, user uuid.UUID, from, to domain.OnrampStatus, suggested *money.Micros, provider *string,
) events.OnrampStatusChanged {
	prior := string(from)
	return events.OnrampStatusChanged{
		V: 1, SessionID: session, UserID: user, From: &prior, To: string(to),
		SuggestedAmountMicros: suggested, Provider: provider,
	}
}
