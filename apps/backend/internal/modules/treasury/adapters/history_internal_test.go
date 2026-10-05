package adapters

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type historyReadStore struct {
	historicalStore
	contributions    []sqlc.CabalContributionHistoryRow
	contributionsErr error
	stakes           []sqlc.UserStakeHistoryRow
	stakesErr        error
}

func (h historyReadStore) CabalContributionHistory(
	context.Context, uuid.UUID,
) ([]sqlc.CabalContributionHistoryRow, error) {
	return h.contributions, h.contributionsErr
}

func (h historyReadStore) UserStakeHistory(context.Context, uuid.UUID) ([]sqlc.UserStakeHistoryRow, error) {
	return h.stakes, h.stakesErr
}

func TestHistoryReadsDecodeFailures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	q := &Queries{}
	q.history = historyReadStore{contributionsErr: boom}
	if _, err := q.CabalContributionHistory(t.Context(), ids.CabalID{}); err == nil {
		t.Fatal("CabalContributionHistory query error = nil")
	}
	q.history = historyReadStore{contributions: []sqlc.CabalContributionHistoryRow{{NetContributedMicros: "bad"}}}
	if _, err := q.CabalContributionHistory(t.Context(), ids.CabalID{}); err == nil {
		t.Fatal("CabalContributionHistory net error = nil")
	}
	q.history = historyReadStore{stakesErr: boom}
	if _, err := q.UserStakeHistory(t.Context(), ids.UserID{}); err == nil {
		t.Fatal("UserStakeHistory query error = nil")
	}
	q.history = historyReadStore{stakes: []sqlc.UserStakeHistoryRow{{ShareUnits: "bad", NetContributedMicros: "1"}}}
	if _, err := q.UserStakeHistory(t.Context(), ids.UserID{}); err == nil {
		t.Fatal("UserStakeHistory shares error = nil")
	}
	q.history = historyReadStore{stakes: []sqlc.UserStakeHistoryRow{{ShareUnits: "1", NetContributedMicros: "bad"}}}
	if _, err := q.UserStakeHistory(t.Context(), ids.UserID{}); err == nil {
		t.Fatal("UserStakeHistory net error = nil")
	}
	q.history = historyReadStore{stakes: []sqlc.UserStakeHistoryRow{{
		CabalID: pgtype.UUID{Valid: true}, ShareUnits: "1", NetContributedMicros: "-2",
	}}}
	got, err := q.UserStakeHistory(t.Context(), ids.UserID{})
	if err != nil || len(got) != 1 || got[0].NetContributed.Int64() != -2 {
		t.Fatalf("UserStakeHistory() = %#v, %v", got, err)
	}
}

func (historicalStore) CabalContributionHistory(
	context.Context, uuid.UUID,
) ([]sqlc.CabalContributionHistoryRow, error) {
	return nil, nil
}

func (historicalStore) UserStakeHistory(context.Context, uuid.UUID) ([]sqlc.UserStakeHistoryRow, error) {
	return nil, nil
}
