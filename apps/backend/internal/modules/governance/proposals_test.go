package governance_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	aaplxMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	checkErr  = "23514"
)

type proposalDB struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	ids  *testkit.IDs
	now  time.Time
}

func newProposalDB(t *testing.T) proposalDB {
	t.Helper()
	pool := testkit.DB(t)
	return proposalDB{
		pool: pool,
		q:    sqlc.New(pool),
		ids:  testkit.NewIDs(1),
		now:  time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
}

func (d proposalDB) buy(voters ...uuid.UUID) sqlc.InsertProposalParams {
	return sqlc.InsertProposalParams{
		VoterIds: voters, ID: d.ids.NewV7(), CabalID: d.ids.NewV7(), ProposerID: d.ids.NewV7(),
		Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: 25_000_000, Valid: true},
		Thesis: pgtype.Text{String: "Earnings next week.", Valid: true}, QuoteOutAmount: 105_000_000,
		ExpiresAt: d.now.Add(24 * time.Hour), CreatedAt: d.now,
	}
}

func (d proposalDB) insert(t *testing.T, p sqlc.InsertProposalParams) {
	t.Helper()
	n, err := d.q.InsertProposal(t.Context(), p)
	if err != nil || n != int64(len(p.VoterIds)) {
		t.Fatalf("InsertProposal = %d voters, %v, want %d", n, err, len(p.VoterIds))
	}
}

func (d proposalDB) ballot(t *testing.T, proposal, voter uuid.UUID, choice string) {
	t.Helper()
	err := d.q.UpsertBallot(t.Context(), sqlc.UpsertBallotParams{
		ProposalID: proposal, VoterID: voter, Choice: choice, CastAt: d.now,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (d proposalDB) count(t *testing.T, proposal uuid.UUID) sqlc.CountBallotsRow {
	t.Helper()
	got, err := d.q.CountBallots(t.Context(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (d proposalDB) transition(t *testing.T, id uuid.UUID, from, to, reason string) int64 {
	t.Helper()
	n, err := d.q.Transition(t.Context(), sqlc.TransitionParams{
		ID: id, FromStatus: from, ToStatus: to, Reason: pgtype.Text{String: reason, Valid: reason != ""},
		At: d.now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

type statusRow struct {
	status, statusReason, voidReason pgtype.Text
	updatedAt                        time.Time
}

func (d proposalDB) row(t *testing.T, id uuid.UUID) statusRow {
	t.Helper()
	var r statusRow
	if err := d.pool.QueryRow(t.Context(),
		`SELECT status, status_reason, void_reason, updated_at FROM proposals WHERE id = $1`, id,
	).Scan(&r.status, &r.statusReason, &r.voidReason, &r.updatedAt); err != nil {
		t.Fatal(err)
	}
	return r
}

func isCheckViolation(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == checkErr && pg.ConstraintName == constraint
}

func wantCheckViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	if !isCheckViolation(err, constraint) {
		t.Fatalf("err = %v, want a check violation on %s", err, constraint)
	}
}

func TestInsertProposal_opensTheProposalAndFreezesItsVoters(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	a, b, c := d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()
	p := d.buy(a, b, c)
	d.insert(t, p)
	r := d.row(t, p.ID)
	if r.status.String != "open" || r.statusReason.Valid || r.voidReason.Valid || !r.updatedAt.Equal(d.now) {
		t.Fatalf("row = %+v, want open with no reasons, updated at %s", r, d.now)
	}
	if got := d.count(t, p.ID); got != (sqlc.CountBallotsRow{Voters: 3}) {
		t.Fatalf("CountBallots = %+v, want 3 voters and no ballots", got)
	}
}

func TestInsertProposal_writesTheProposalEvenWithNoVoters(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	p := d.buy()
	d.insert(t, p)
	if got := d.row(t, p.ID).status.String; got != "open" {
		t.Fatalf("status = %s, want open", got)
	}
}

func TestInsertProposal_needsExactlyOnePositiveAmount(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	set := func(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
	for name, amounts := range map[string][2]pgtype.Int8{
		"neither":        {},
		"both":           {set(1), set(1)},
		"zero usdc":      {set(0), {}},
		"negative usdc":  {set(-1), {}},
		"zero tokens":    {{}, set(0)},
		"negative token": {{}, set(-5)},
	} {
		p := d.buy(d.ids.NewV7())
		p.UsdcMicros, p.TokenAmount = amounts[0], amounts[1]
		if _, err := d.q.InsertProposal(t.Context(), p); !isCheckViolation(err, "proposals_one_amount_check") {
			t.Errorf("%s: err = %v, want a check violation on proposals_one_amount_check", name, err)
		}
	}
	sell := d.buy(d.ids.NewV7())
	sell.Kind, sell.UsdcMicros, sell.TokenAmount = "sell", pgtype.Int8{}, set(3_000_000)
	d.insert(t, sell)
}

func TestInsertProposal_capsTheThesisAt280Characters(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	fits := d.buy(d.ids.NewV7())
	fits.Thesis = pgtype.Text{String: strings.Repeat("é", 280), Valid: true}
	d.insert(t, fits)
	long := d.buy(d.ids.NewV7())
	long.Thesis = pgtype.Text{String: strings.Repeat("a", 281), Valid: true}
	_, err := d.q.InsertProposal(t.Context(), long)
	wantCheckViolation(t, err, "proposals_thesis_check")
	none := d.buy(d.ids.NewV7())
	none.Thesis = pgtype.Text{}
	d.insert(t, none)
}

func TestBallots_upsertKeepsOnePerVoterAndCountsOnlyTheFrozenSet(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	a, b, c, outsider := d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()
	p := d.buy(a, b, c)
	d.insert(t, p)
	other := d.buy(a)
	d.insert(t, other)
	d.ballot(t, p.ID, a, "yes")
	d.ballot(t, p.ID, b, "yes")
	d.ballot(t, p.ID, outsider, "yes")
	d.ballot(t, other.ID, a, "no")
	if got := d.count(t, p.ID); got != (sqlc.CountBallotsRow{Voters: 3, Yes: 2}) {
		t.Fatalf("CountBallots = %+v, want 3 voters, 2 yes", got)
	}
	d.ballot(t, p.ID, b, "no")
	if got := d.count(t, p.ID); got != (sqlc.CountBallotsRow{Voters: 3, Yes: 1, No: 1}) {
		t.Fatalf("CountBallots after b changed = %+v, want 3 voters, 1 yes, 1 no", got)
	}
	err := d.q.UpsertBallot(t.Context(), sqlc.UpsertBallotParams{
		ProposalID: p.ID, VoterID: c, Choice: "maybe", CastAt: d.now,
	})
	wantCheckViolation(t, err, "votes_choice_check")
}

func TestTransition_movesOnlyFromTheExpectedStatus(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	if n := d.transition(t, p.ID, "passed", "executed", ""); n != 0 {
		t.Fatalf("Transition from passed on an open row changed %d rows, want 0", n)
	}
	if got := d.row(t, p.ID); got.status.String != "open" || !got.updatedAt.Equal(d.now) {
		t.Fatalf("row after the refused transition = %+v, want untouched", got)
	}
	if n := d.transition(t, p.ID, "open", "passed", ""); n != 1 {
		t.Fatalf("Transition open to passed changed %d rows, want 1", n)
	}
	if n := d.transition(t, p.ID, "open", "failed", ""); n != 0 {
		t.Fatalf("second Transition from open changed %d rows, want 0", n)
	}
	got := d.row(t, p.ID)
	if got.status.String != "passed" || !got.updatedAt.Equal(d.now.Add(time.Minute)) {
		t.Fatalf("row = %+v, want passed, updated a minute later", got)
	}
}

func TestTransition_writesTheReasonToTheColumnItsStatusOwns(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	for _, tc := range []struct {
		to                       string
		statusReason, voidReason string
	}{
		{to: "execution_blocked", statusReason: "slippage_exceeded"},
		{to: "voided", voidReason: "slippage_exceeded"},
		{to: "executed"},
	} {
		p := d.buy(d.ids.NewV7())
		d.insert(t, p)
		d.transition(t, p.ID, "open", "passed", "")
		if n := d.transition(t, p.ID, "passed", tc.to, "slippage_exceeded"); n != 1 {
			t.Fatalf("Transition passed to %s changed %d rows, want 1", tc.to, n)
		}
		got := d.row(t, p.ID)
		if got.status.String != tc.to || got.statusReason.String != tc.statusReason ||
			got.voidReason.String != tc.voidReason {
			t.Errorf("%s row = %+v, want status_reason %q, void_reason %q", tc.to, got, tc.statusReason, tc.voidReason)
		}
	}
}
