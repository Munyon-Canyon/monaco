package governance_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (d proposalDB) proposed(t *testing.T, proposer uuid.UUID, at time.Time, voters ...uuid.UUID) uuid.UUID {
	t.Helper()
	p := d.buy(voters...)
	p.ProposerID, p.CreatedAt = proposer, at
	d.insert(t, p)
	return p.ID
}

func (d proposalDB) voteAt(t *testing.T, proposal, voter uuid.UUID, choice string, at time.Time) {
	t.Helper()
	err := d.q.UpsertBallot(t.Context(), sqlc.UpsertBallotParams{
		ProposalID: proposal, VoterID: voter, Choice: choice, CastAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newestFirst(ids []uuid.UUID, at map[uuid.UUID]time.Time) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int {
		if c := at[b].Compare(at[a]); c != 0 {
			return c
		}
		return strings.Compare(b.String(), a.String())
	})
	return out
}

func TestMemberActivity_VotesBy_ordersNewestFirstAndPagesPastEqualTimestamps(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	me, other := d.ids.NewV7(), d.ids.NewV7()
	castAt := map[uuid.UUID]time.Time{}
	mine := make([]uuid.UUID, 0, 7)
	for i := range 7 {
		id := d.proposed(t, other, d.now, me, other)
		at := d.now.Add(time.Duration(i/3) * time.Minute)
		d.voteAt(t, id, me, "yes", at)
		d.voteAt(t, id, other, "no", at.Add(time.Hour))
		castAt[id] = at
		mine = append(mine, id)
	}
	queries := app.NewQueries(d.pool)
	want := newestFirst(mine, castAt)
	var got []uuid.UUID
	var after *port.Cursor
	for range 10 {
		page, err := queries.VotesBy(t.Context(), ids.UserIDFrom(me), after, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range page {
			got = append(got, v.ProposalID.UUID())
			if v.Choice != "yes" || v.Symbol != "AAPLx" || v.USDCMicros == nil || *v.USDCMicros != 25_000_000 {
				t.Errorf("vote = %+v, want the caller's own yes on a $25 AAPLx proposal", v)
			}
		}
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		after = &port.Cursor{At: last.CastAt, ID: last.ProposalID.UUID()}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("votes = %v, want %v newest first with none lost or repeated across equal timestamps", got, want)
	}
}

func TestMemberActivity_ProposalsBy_ordersNewestFirstWithStatusAndCounts(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	me, a, b := d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()
	createdAt := map[uuid.UUID]time.Time{}
	mine := make([]uuid.UUID, 0, 5)
	for i := range 5 {
		at := d.now.Add(time.Duration(i/2) * time.Minute)
		id := d.proposed(t, me, at, a, b)
		d.proposed(t, a, at, me)
		createdAt[id] = at
		mine = append(mine, id)
	}
	decided := mine[0]
	d.voteAt(t, decided, a, "yes", d.now)
	d.voteAt(t, decided, b, "no", d.now)
	if n := d.transition(t, decided, string(domain.StatusOpen), string(domain.StatusFailed), ""); n != 1 {
		t.Fatalf("transition rows = %d, want 1", n)
	}
	queries := app.NewQueries(d.pool)
	want := newestFirst(mine, createdAt)
	var got []uuid.UUID
	var after *port.Cursor
	for range 10 {
		page, err := queries.ProposalsBy(t.Context(), ids.UserIDFrom(me), after, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range page {
			got = append(got, p.ProposalID.UUID())
			if p.ProposalID.UUID() == decided &&
				(p.Status != domain.StatusFailed || p.Yes != 1 || p.No != 1) {
				t.Errorf("decided proposal = %+v, want failed 1-1", p)
			}
		}
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		after = &port.Cursor{At: last.CreatedAt, ID: last.ProposalID.UUID()}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("proposals = %v, want %v newest first with none lost or repeated", got, want)
	}
}

func TestMemberActivity_ProposalSubjects_namesTheCabalKindSymbolAndAmount(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	buy := d.buy(d.ids.NewV7())
	d.insert(t, buy)
	sell := d.buy(d.ids.NewV7())
	sell.Kind, sell.UsdcMicros, sell.TokenAmount = "sell", pgtype.Int8{}, pgtype.Int8{Int64: 3, Valid: true}
	d.insert(t, sell)
	got, err := app.NewQueries(d.pool).ProposalSubjects(t.Context(), []ids.ProposalID{
		ids.ProposalIDFrom(buy.ID), ids.ProposalIDFrom(sell.ID), ids.ProposalIDFrom(d.ids.NewV7()),
	})
	if err != nil {
		t.Fatal(err)
	}
	bought, sold := got[ids.ProposalIDFrom(buy.ID)], got[ids.ProposalIDFrom(sell.ID)]
	if len(got) != 2 || bought.CabalID.UUID() != buy.CabalID || bought.Kind != "buy" || bought.Symbol != "AAPLx" ||
		bought.USDCMicros == nil || *bought.USDCMicros != 25_000_000 {
		t.Errorf("buy subject = %+v in %v, want the cabal, buy AAPLx and $25", bought, got)
	}
	if sold.Kind != "sell" || sold.USDCMicros != nil {
		t.Errorf("sell subject = %+v, want no USDC amount", sold)
	}
}

func TestMemberActivity_refusesALimitOutsideTheRange(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	queries := app.NewQueries(d.pool)
	for _, limit := range []int{0, -1, port.MemberActivityMaxLimit + 1} {
		if _, err := queries.VotesBy(
			t.Context(),
			ids.NewUserID(d.ids),
			nil,
			limit,
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Errorf("VotesBy limit %d err = %v, want invalid_input", limit, err)
		}
		if _, err := queries.ProposalsBy(
			t.Context(),
			ids.NewUserID(d.ids),
			nil,
			limit,
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Errorf("ProposalsBy limit %d err = %v, want invalid_input", limit, err)
		}
	}
}

func TestMemberActivity_aDeadDatabaseFailsEveryRead(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	queries := app.NewQueries(d.pool)
	dead, cancel := context.WithCancel(t.Context())
	cancel()
	user := ids.NewUserID(d.ids)
	if _, err := queries.VotesBy(dead, user, nil, 5); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("VotesBy err = %v, want internal", err)
	}
	if _, err := queries.ProposalsBy(dead, user, nil, 5); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("ProposalsBy err = %v, want internal", err)
	}
	if _, err := queries.ProposalSubjects(dead, nil); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("ProposalSubjects err = %v, want internal", err)
	}
}

func TestMemberActivity_aStoredStatusNobodyWroteFailsTheProposalRead(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	me := d.ids.NewV7()
	id := d.proposed(t, me, d.now)
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET status = 'mystery' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewQueries(d.pool).
		ProposalsBy(t.Context(), ids.UserIDFrom(me), nil, 5); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Errorf("err = %v, want decode_failed", err)
	}
}
