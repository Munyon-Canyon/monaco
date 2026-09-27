package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const (
	idempotencyInFlight  int16 = 1
	idempotencyCompleted int16 = 2
	claimAttempts              = 3
)

type IdempotencyStore struct {
	q     *sqlc.Queries
	clock clock.Clock
}

func NewIdempotencyStore(pool *pgxpool.Pool, c clock.Clock) *IdempotencyStore {
	return &IdempotencyStore{q: sqlc.New(pool), clock: c}
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
		n, err := s.q.BeginIdempotencyKey(ctx, sqlc.BeginIdempotencyKeyParams{
			ActorKey: actorKey, Key: key, RequestHash: requestHash, CreatedAt: s.clock.Now(),
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
		return claimFrom(row, requestHash)
	}
	return Claim{}, errs.New(errs.CodeInternal, op, slog.Int("attempts", claimAttempts))
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
	header, err := json.Marshal(resp.Header)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
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
