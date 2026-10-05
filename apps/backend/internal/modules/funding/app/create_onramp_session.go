package app

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const OnrampTokenTTL = 10 * time.Minute

type CreateOnrampSession struct {
	ID              uuid.UUID
	UserID          ids.UserID
	SuggestedAmount *money.Micros
	CabalID         *uuid.UUID
}

type OnrampSessionCreated struct {
	ID        uuid.UUID
	URL       string
	ExpiresAt time.Time
}

type CreateOnrampSessionHandler struct {
	uow      *db.UnitOfWork
	clock    clock.Clock
	fundPage string
}

func NewCreateOnrampSessionHandler(uow *db.UnitOfWork, c clock.Clock, fundPage string) *CreateOnrampSessionHandler {
	return &CreateOnrampSessionHandler{uow: uow, clock: c, fundPage: fundPage}
}

func (h *CreateOnrampSessionHandler) Handle(
	ctx context.Context, cmd CreateOnrampSession,
) (OnrampSessionCreated, error) {
	var random [domain.OnrampTokenBytes]byte
	_, _ = rand.Read(random[:])
	token := domain.NewOnrampToken(random)
	now := h.clock.Now()
	created := OnrampSessionCreated{
		ID: cmd.ID, URL: h.fundPage + "?s=" + token.Encode(), ExpiresAt: now.Add(OnrampTokenTTL),
	}
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		err := sqlc.New(tx.Queries()).InsertOnrampSession(ctx, sqlc.InsertOnrampSessionParams{
			ID: cmd.ID, UserID: cmd.UserID.UUID(), TokenHash: token.Hash(), CreatedAt: now,
			ExpiresAt: created.ExpiresAt, SuggestedAmountMicros: microsText(cmd.SuggestedAmount),
			CabalID: uuidText(cmd.CabalID),
		})
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, events.OnrampStatusChanged{
			V: 1, SessionID: cmd.ID, UserID: cmd.UserID.UUID(), To: string(domain.OnrampCreated),
			SuggestedAmountMicros: cmd.SuggestedAmount,
		})
	})
	if err != nil {
		return OnrampSessionCreated{}, err
	}
	return created, nil
}

func microsText(m *money.Micros) string {
	if m == nil {
		return ""
	}
	return m.String()
}

func uuidText(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
