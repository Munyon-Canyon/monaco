package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Availability struct {
	Handle    string
	Available bool
	Reason    string
}

func HandleAvailability(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, raw string, now time.Time,
) (Availability, error) {
	folded := strings.ToLower(raw)
	h, valid := availabilityHandle(raw)
	if !valid {
		return Availability{Handle: folded, Available: false, Reason: "invalid"}, nil
	}
	row, err := LoadHandleClaimFacts(ctx, q, id, h.String())
	if err != nil {
		return Availability{}, err
	}
	if row.Handle.Valid && row.Handle.String == h.String() {
		return Availability{Handle: h.String(), Available: true}, nil
	}
	if err := (domain.HandlePolicy{}).Check(h, claimFacts(row, now)); err != nil {
		return Availability{
			Handle: h.String(), Available: false, Reason: availabilityReason(string(errs.CodeOf(err))),
		}, nil
	}
	if row.HandleTaken {
		return Availability{Handle: h.String(), Available: false, Reason: "taken"}, nil
	}
	return Availability{Handle: h.String(), Available: true}, nil
}

func availabilityHandle(raw string) (domain.Handle, bool) {
	h, err := domain.ParseHandle(raw)
	return h, err == nil
}

func LoadHandleClaimFacts(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, handle string,
) (sqlc.HandleClaimFactsRow, error) {
	const op = "identity.HandleAvailability"
	row, err := sqlc.New(q).HandleClaimFacts(ctx, sqlc.HandleClaimFactsParams{Handle: handle, ID: id.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return sqlc.HandleClaimFactsRow{}, errs.New(errs.CodeUserNotFound, op)
	case err != nil:
		return sqlc.HandleClaimFactsRow{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return row, nil
}

func LoadLockedHandleClaimFacts(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, handle string,
) (sqlc.HandleClaimFactsRow, error) {
	const op = "identity.SetHandle"
	row, err := sqlc.New(q).LockedHandleClaimFacts(ctx, sqlc.LockedHandleClaimFactsParams{
		Handle: handle, ID: id.UUID(),
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return sqlc.HandleClaimFactsRow{}, errs.New(errs.CodeUserNotFound, op)
	case err != nil:
		return sqlc.HandleClaimFactsRow{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return sqlc.HandleClaimFactsRow(row), nil
}

func claimFacts(row sqlc.HandleClaimFactsRow, now time.Time) domain.HandleFacts {
	facts := domain.HandleFacts{
		OwnXUsername: row.XUsername.String, OtherUserXUsernameMatch: row.OtherXMatch, Now: now,
	}
	if row.Handle.Valid {
		facts.CurrentHandle = row.Handle.String
	}
	if row.HandleChangedAt.Valid {
		facts.HandleChangedAt = row.HandleChangedAt.Time
	}
	return facts
}

func availabilityReason(code string) string {
	switch code {
	case string(errs.CodeHandleTaken):
		return "taken"
	case string(errs.CodeHandleReserved):
		return "reserved"
	case string(errs.CodeHandleTooSoon):
		return "too_soon"
	default:
		return "invalid"
	}
}
