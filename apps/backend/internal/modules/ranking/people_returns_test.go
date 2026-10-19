package ranking_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestQueries_peopleReturnsReadsThePeopleBoardForTheRange(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	at := d.clock.Now().Truncate(time.Microsecond)
	run := d.gen.NewV7()
	q := sqlc.New(d.pool)
	if err := q.InsertLeaderboardRun(t.Context(), sqlc.InsertLeaderboardRunParams{
		RunID: run, AsOf: at, PricesAsOf: at, StartedAt: at, FinishedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	ranked, noReturn, otherRange, otherBoard, absent := d.gen.NewV7(), d.gen.NewV7(), d.gen.NewV7(),
		d.gen.NewV7(), d.gen.NewV7()
	bps := int64(820)
	entry := func(board, rng string, rank int, subject uuid.UUID, ret *int64, pnl int64) map[string]any {
		return map[string]any{
			"board": board, "range": rng, "rank": rank, "subject_id": subject.String(), "subject_name": "Subject",
			"subject_created_at": at.Format(time.RFC3339Nano), "value_micros": 1000, "pnl_micros": pnl,
			"return_bps": ret, "prices_as_of": at.Format(time.RFC3339Nano),
			"computed_at": at.Format(time.RFC3339Nano), "flags": []string{},
		}
	}
	raw, err := json.Marshal([]map[string]any{
		entry("people", "1M", 1, ranked, &bps, -250),
		entry("people", "1M", 2, noReturn, nil, 40),
		entry("people", "1W", 1, otherRange, &bps, 1),
		entry("cabals", "1M", 1, otherBoard, &bps, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.InsertLeaderboardEntries(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	queries := ranking.New(module.Deps{Pool: d.pool}).Queries()
	asked := []ids.UserID{
		ids.UserIDFrom(ranked), ids.UserIDFrom(noReturn), ids.UserIDFrom(otherRange),
		ids.UserIDFrom(otherBoard), ids.UserIDFrom(absent),
	}
	got, err := queries.PeopleReturns(t.Context(), domain.Range1M, asked)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("PeopleReturns() = %+v, want only the two people-board 1M rows", got)
	}
	if r := got[ids.UserIDFrom(ranked)]; r.ReturnBps == nil || *r.ReturnBps != 820 ||
		r.PnlMicros != money.SignedMicrosFromInt64(-250) || !r.AsOf.Equal(at) {
		t.Errorf("ranked = %+v, want 820 bps, -250 pnl as of %s", r, at)
	}
	if r := got[ids.UserIDFrom(noReturn)]; r.ReturnBps != nil || r.PnlMicros != money.SignedMicrosFromInt64(40) {
		t.Errorf("noReturn = %+v, want nil bps and 40 pnl", r)
	}
}

func TestQueries_peopleReturnsOfNoUsersIsEmpty(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	got, err := ranking.New(module.Deps{Pool: d.pool}).Queries().PeopleReturns(t.Context(), domain.Range1M, nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("PeopleReturns(nil) = %v, %v, want an empty map", got, err)
	}
}

func TestQueries_peopleReturnsFailsWhenTheReadFails(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := ranking.New(module.Deps{Pool: d.pool}).Queries().PeopleReturns(
		ctx, domain.Range1M, []ids.UserID{ids.UserIDFrom(d.gen.NewV7())},
	)
	if err == nil || errs.CodeOf(err) == "" {
		t.Fatalf("PeopleReturns on a closed pool = %v, want an error", err)
	}
}
