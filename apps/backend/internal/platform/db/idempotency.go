package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const (
	idempotencyInFlight  int16 = 1
	idempotencyCompleted int16 = 2
	claimAttempts              = 3
	inFlightTakeover           = 5 * time.Minute
)

type IdempotencyStore struct {
	q     *sqlc.Queries
	clock clock.Clock
}

func NewIdempotencyStore(q sqlc.DBTX, c clock.Clock) *IdempotencyStore {
	return &IdempotencyStore{q: sqlc.New(q), clock: c}
}

type ClaimOutcome uint8

const (
	ClaimOwned ClaimOutcome = iota + 1
	ClaimInFlight
	ClaimMismatch
	ClaimCompleted
)

type Claim struct {
	Outcome  ClaimOutcome
	Response StoredResponse
}

type StoredResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

func (s *IdempotencyStore) Begin(ctx context.Context, actorKey, key string, requestHash []byte) (Claim, error) {
	const op = "db.IdempotencyStore.Begin"
	for range claimAttempts {
		now := s.clock.Now()
		n, err := s.q.BeginIdempotencyKey(ctx, sqlc.BeginIdempotencyKeyParams{
			ActorKey: actorKey, Key: key, RequestHash: requestHash, CreatedAt: now,
		})
		if err != nil {
			return Claim{}, classify(err, op)
		}
		if n == 1 {
			return Claim{Outcome: ClaimOwned}, nil
		}
		row, err := s.q.GetIdempotencyKey(ctx, sqlc.GetIdempotencyKeyParams{ActorKey: actorKey, Key: key})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return Claim{}, classify(err, op)
		}
		if !abandoned(row, requestHash, now) {
			return claimFrom(row, requestHash)
		}
		taken, err := s.takeOver(ctx, actorKey, key, now)
		if err != nil {
			return Claim{}, err
		}
		if taken {
			return Claim{Outcome: ClaimOwned}, nil
		}
	}
	return Claim{}, errs.New(errs.CodeInternal, op, slog.Int("attempts", claimAttempts))
}

func (s *IdempotencyStore) takeOver(ctx context.Context, actorKey, key string, now time.Time) (bool, error) {
	taken, err := s.q.TakeOverIdempotencyKey(ctx, sqlc.TakeOverIdempotencyKeyParams{
		ActorKey: actorKey, Key: key, CreatedAt: now, CreatedAt_2: now.Add(-inFlightTakeover),
	})
	if err != nil {
		return false, classify(err, "db.IdempotencyStore.Begin")
	}
	return taken == 1, nil
}

func abandoned(row sqlc.IdempotencyKey, requestHash []byte, now time.Time) bool {
	return row.Status == idempotencyInFlight && bytes.Equal(row.RequestHash, requestHash) &&
		row.CreatedAt.Before(now.Add(-inFlightTakeover))
}

func claimFrom(row sqlc.IdempotencyKey, requestHash []byte) (Claim, error) {
	const op = "db.IdempotencyStore.Begin"
	if !bytes.Equal(row.RequestHash, requestHash) {
		return Claim{Outcome: ClaimMismatch}, nil
	}
	if row.Status == idempotencyInFlight {
		return Claim{Outcome: ClaimInFlight}, nil
	}
	var header http.Header
	if err := json.Unmarshal(row.ResponseHeaders, &header); err != nil {
		return Claim{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return Claim{Outcome: ClaimCompleted, Response: StoredResponse{
		Status: int(row.ResponseStatus.Int32), Header: header, Body: row.ResponseBody,
	}}, nil
}

func (s *IdempotencyStore) Complete(ctx context.Context, actorKey, key string, resp StoredResponse) error {
	const op = "db.IdempotencyStore.Complete"
	header, _ := json.Marshal(resp.Header)
	n, err := s.q.CompleteIdempotencyKey(ctx, sqlc.CompleteIdempotencyKeyParams{
		ActorKey:        actorKey,
		Key:             key,
		ResponseStatus:  pgtype.Int4{Int32: int32(resp.Status), Valid: true},
		ResponseBody:    resp.Body,
		ResponseHeaders: header,
		CompletedAt:     pgtype.Timestamptz{Time: s.clock.Now(), Valid: true},
	})
	if err != nil {
		return classify(err, op)
	}
	if n != 1 {
		return errs.New(errs.CodeInternal, op, slog.Int64("rows", n))
	}
	return nil
}

func (s *IdempotencyStore) Release(ctx context.Context, actorKey, key string) error {
	const op = "db.IdempotencyStore.Release"
	if _, err := s.q.ReleaseIdempotencyKey(ctx, sqlc.ReleaseIdempotencyKeyParams{
		ActorKey: actorKey, Key: key,
	}); err != nil {
		return classify(err, op)
	}
	return nil
}

type IntentKey struct {
	ActorKey    string
	Key         string
	Operation   string
	RequestHash []byte
}

func (k IntentKey) Bound() bool { return k.Key != "" }

func (k IntentKey) Bind(
	ctx context.Context, db sqlc.DBTX, resourceID uuid.UUID, now time.Time,
) (uuid.UUID, bool, error) {
	const op = "db.IntentKey.Bind"
	if !k.Bound() {
		return resourceID, false, nil
	}
	q := sqlc.New(db)
	for range claimAttempts {
		n, err := q.BindMoneyIntent(ctx, sqlc.BindMoneyIntentParams{
			ActorKey: k.ActorKey, Key: k.Key, Operation: k.Operation, RequestHash: k.RequestHash,
			ResourceID: resourceID, CreatedAt: now,
		})
		if err != nil {
			return uuid.Nil, false, classify(err, op)
		}
		if n == 1 {
			return resourceID, false, nil
		}
		row, err := q.GetMoneyIntent(ctx, sqlc.GetMoneyIntentParams{ActorKey: k.ActorKey, Key: k.Key})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return uuid.Nil, false, classify(err, op)
		}
		if row.Operation != k.Operation || !bytes.Equal(row.RequestHash, k.RequestHash) {
			return uuid.Nil, false, errs.New(errs.CodeIdempotencyMismatch, op,
				slog.String("idempotency_key", k.Key), slog.String("operation", k.Operation))
		}
		return row.ResourceID, true, nil
	}
	return uuid.Nil, false, errs.New(errs.CodeInternal, op, slog.Int("attempts", claimAttempts))
}

func UnbindIntent(ctx context.Context, q sqlc.DBTX, operation string, resourceID uuid.UUID) error {
	if _, err := sqlc.New(q).UnbindMoneyIntent(ctx, sqlc.UnbindMoneyIntentParams{
		Operation: operation, ResourceID: resourceID,
	}); err != nil {
		return classify(err, "db.UnbindIntent")
	}
	return nil
}
