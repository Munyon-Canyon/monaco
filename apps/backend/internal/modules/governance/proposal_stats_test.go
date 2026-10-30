package governance_test

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func (d readsDB) readsWith(replies app.ReplyCounts) *app.ProposalReads {
	return app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, &cardBook{}, &returnBook{}, replies)
}

func (d readsDB) setPot(t *testing.T, id uuid.UUID, pot int64) {
	t.Helper()
	const set = `UPDATE proposals SET pot_value_micros = $2 WHERE id = $1`
	if _, err := d.pool.Exec(t.Context(), set, id, pot); err != nil {
		t.Fatal(err)
	}
}

type closing struct {
	closedAt  pgtype.Timestamptz
	decidedBy pgtype.UUID
}

func (d proposalDB) closing(t *testing.T, id uuid.UUID) closing {
	t.Helper()
	var c closing
	err := d.pool.QueryRow(t.Context(), `SELECT closed_at, decided_by FROM proposals WHERE id = $1`, id).
		Scan(&c.closedAt, &c.decidedBy)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func uuidOf(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func TestGetProposal_openStats(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	d.setPot(t, p, 1_000_000_000)
	got := d.get(t, p, d.caller)
	share := int64(250)
	if got.Stats.TreasuryShareBps == nil || *got.Stats.TreasuryShareBps != share ||
		!got.Stats.ClosesAt.Equal(got.Proposal.ExpiresAt) || got.Stats.ClosedAt != nil || got.MyVote != nil {
		t.Fatalf("open stats = %+v, my vote %+v, want 250 bps, closing at expiry, unclosed, no vote",
			got.Stats, got.MyVote)
	}
}

func TestGetProposal_sellTreasuryShare(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.buy(d.caller)
	p.CabalID, p.Kind, p.UsdcMicros = d.cabal, "sell", pgtype.Int8{}
	p.TokenAmount, p.QuoteOutAmount = pgtype.Int8{Int64: 5_000_000, Valid: true}, 40_000_000
	d.insert(t, p)
	d.setPot(t, p.ID, 1_000_000_000)
	got := d.get(t, p.ID, d.caller).Stats.TreasuryShareBps
	if got == nil || *got != 400 {
		t.Fatalf("sell share = %v, want 400 bps from quote_out_amount over the pot", got)
	}
}

func TestGetProposal_legacyRowHasNoTreasuryShare(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	d.setStatus(t, p, "passed", "")
	got := d.get(t, p, d.caller)
	if got.Stats.TreasuryShareBps != nil || got.Stats.ClosedAt != nil {
		t.Fatalf("legacy stats = %+v, want no share and no close time, not updated_at", got.Stats)
	}
}

func TestGetProposal_replyCountFromSocial(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	id := ids.ProposalIDFrom(p)
	replies := &replyBook{counts: map[ids.ProposalID]int{id: 6}}
	got, err := d.readsWith(replies).Get(t.Context(), app.GetProposal{ID: id, Caller: ids.UserIDFrom(d.caller)})
	if err != nil || got.ReplyCount != 6 {
		t.Fatalf("Get = reply count %d, %v, want 6", got.ReplyCount, err)
	}
	if len(replies.calls) != 1 || !slices.Equal(replies.calls[0], []ids.ProposalID{id}) {
		t.Fatalf("reply reads = %v, want one read for this proposal", replies.calls)
	}
}

func TestGetProposal_refusesWhenAFactCannotBeRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.seed(t, d.now)
	d.ballot(t, p, d.caller, "yes")
	req := app.GetProposal{ID: ids.ProposalIDFrom(p), Caller: ids.UserIDFrom(d.caller)}
	down := &replyBook{err: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := d.readsWith(down).Get(t.Context(), req); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Get with a failing reply read = %v, want db_unavailable", err)
	}
	const rename = `ALTER TABLE votes RENAME COLUMN first_cast_at TO first_cast_at_gone`
	if _, err := d.pool.Exec(t.Context(), rename); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reads().Get(t.Context(), req); err == nil {
		t.Error("Get with a failing ballot order read succeeded")
	}
}

func TestGetProposal_refusesAShareThatOverflows(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.buy(d.caller)
	p.CabalID, p.Kind, p.UsdcMicros = d.cabal, "sell", pgtype.Int8{}
	p.TokenAmount, p.QuoteOutAmount = pgtype.Int8{Int64: 5_000_000, Valid: true}, math.MaxInt64
	d.insert(t, p)
	d.setPot(t, p.ID, 1)
	req := app.GetProposal{ID: ids.ProposalIDFrom(p.ID), Caller: ids.UserIDFrom(d.caller)}
	_, err := d.reads().Get(t.Context(), req)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Get with a share past uint64 = %v, want invalid_input", err)
	}
}

func TestOpenProposal_aShareThatOverflowsOpensNothing(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	h.w.treasury.SetPotValue(h.w.cabal, money.MicrosFromUint64(1))
	aapl := marketfake.AAPLx()
	h.w.routes.Ok(aapl.ID, market.RouteCheck{OutAmount: money.NewBaseUnits(math.MaxInt64, aapl.Decimals)})
	_, err := h.propose(h.d.ids, sellAAPL(heldUnits))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("propose sell with a share past uint64 = %v, want invalid_input", err)
	}
	var n int
	if err := h.d.pool.QueryRow(t.Context(), `SELECT count(*) FROM proposals`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("proposals = %d, %v, want none", n, err)
	}
}

func TestOpenProposal_storesPot(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	buy, err := h.propose(h.d.ids, buyAAPLFor(25_000_000))
	if err != nil {
		t.Fatal(err)
	}
	sell, err := h.propose(h.d.ids, sellAAPL(heldUnits))
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]ids.ProposalID{"buy": buy, "sell": sell} {
		var pot int64
		row := h.d.pool.QueryRow(t.Context(), `SELECT pot_value_micros FROM proposals WHERE id = $1`, id.UUID())
		if err := row.Scan(&pot); err != nil || pot != potMicros {
			t.Errorf("%s pot_value_micros = %d, %v, want %d", name, pot, err, potMicros)
		}
	}
}

func TestOpenProposal_aSellOpensWithoutAPotWhenPricesAreDown(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	h.w.treasury.Fail("PotValue", errs.New(errs.CodePriceUnavailable, "test"))
	sell, err := h.propose(h.d.ids, sellAAPL(heldUnits))
	if err != nil {
		t.Fatalf("propose sell = %v, want it to open without the pot", err)
	}
	var pot pgtype.Int8
	row := h.d.pool.QueryRow(t.Context(), `SELECT pot_value_micros FROM proposals WHERE id = $1`, sell.UUID())
	if err := row.Scan(&pot); err != nil || pot.Valid {
		t.Fatalf("pot_value_micros = %+v, %v, want null", pot, err)
	}
}

func TestTransition_setsClosedAtOnlyLeavingOpen(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	opened, decider := d.buy(d.ids.NewV7()), d.ids.NewV7()
	d.insert(t, opened)
	closedAt := d.now.Add(time.Hour)
	moved := func(from, to string, at time.Time, by string) {
		t.Helper()
		n, err := d.q.Transition(t.Context(), sqlc.TransitionParams{
			ID: opened.ID, FromStatus: from, ToStatus: to, At: at, DecidedBy: by,
		})
		if err != nil || n != 1 {
			t.Fatalf("Transition %s to %s = %d, %v", from, to, n, err)
		}
	}
	moved("open", "passed", closedAt, decider.String())
	moved("passed", "executed", closedAt.Add(time.Hour), "")
	got := d.closing(t, opened.ID)
	if !got.closedAt.Valid || !got.closedAt.Time.Equal(closedAt) || got.decidedBy != uuidOf(decider) {
		t.Fatalf("after executed: %+v, want closed at %s by %s", got, closedAt, decider)
	}
	withdrawn := d.buy(d.ids.NewV7())
	d.insert(t, withdrawn)
	n, err := d.q.Transition(t.Context(), sqlc.TransitionParams{
		ID: withdrawn.ID, FromStatus: "open", ToStatus: "withdrawn", At: closedAt,
	})
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if got := d.closing(t, withdrawn.ID); !got.closedAt.Valid || got.decidedBy.Valid {
		t.Fatalf("withdrawn: %+v, want a close time and no decider", got)
	}
}

func TestVoid_setsClosedAtOnlyFromOpen(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	at := d.now.Add(time.Hour)
	for from, wantClosed := range map[string]bool{"open": true, "passed": false} {
		p := d.buy(d.ids.NewV7())
		d.insert(t, p)
		if from != "open" {
			const set = `UPDATE proposals SET status = $2 WHERE id = $1`
			if _, err := d.pool.Exec(t.Context(), set, p.ID, from); err != nil {
				t.Fatal(err)
			}
		}
		_, err := d.q.Void(t.Context(), sqlc.VoidParams{
			ID: p.ID, FromStatuses: []string{from}, ToStatus: "voided", Reason: "admin", At: at,
		})
		if err != nil {
			t.Fatalf("Void from %s = %v", from, err)
		}
		got := d.closing(t, p.ID)
		if got.closedAt.Valid != wantClosed || got.decidedBy.Valid {
			t.Errorf("Void from %s: %+v, want closed_at set = %v and no decider", from, got, wantClosed)
		}
	}
}

func (d proposalDB) seedClosed(t *testing.T, status, event string, closed, updated time.Time) uuid.UUID {
	t.Helper()
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	const set = `UPDATE proposals SET status = $2, updated_at = $3 WHERE id = $1`
	if _, err := d.pool.Exec(t.Context(), set, p.ID, status, updated); err != nil {
		t.Fatal(err)
	}
	if event == "" {
		return p.ID
	}
	_, err := d.pool.Exec(t.Context(), `
		INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
		VALUES ($1, 'proposal', $2, $3, '{"v":1}', 'system', 'test', $4)`,
		d.ids.NewV7(), p.ID, event, closed)
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func runBackfill(t *testing.T, d proposalDB) {
	t.Helper()
	files, err := filepath.Glob("../../../migrations/*_governance_proposal_stats.sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("migration files = %v, %v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	_, backfill, ok := strings.Cut(string(raw), "UPDATE proposals")
	if !ok {
		t.Fatal("migration has no backfill")
	}
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET closed_at = NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(t.Context(), "UPDATE proposals"+backfill); err != nil {
		t.Fatal(err)
	}
}

func TestMigration_backfillsClosedAtFromCloseEvents(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	closed := d.now.Add(time.Hour)
	later := closed.Add(48 * time.Hour)
	want := map[string]struct {
		id    uuid.UUID
		valid bool
	}{
		"close event":    {d.seedClosed(t, "failed", "proposal.failed", closed, later), true},
		"no close event": {d.seedClosed(t, "passed", "", closed, later), false},
		"executed":       {d.seedClosed(t, "executed", "proposal.passed", closed, later), true},
		"open":           {d.seedClosed(t, "open", "", closed, later), false},
	}
	runBackfill(t, d)
	for name, w := range want {
		got := d.closing(t, w.id).closedAt
		if got.Valid != w.valid || w.valid && !got.Time.Equal(closed) {
			t.Errorf("%s: closed_at = %+v, want valid = %v at %s, not %s", name, got, w.valid, closed, later)
		}
	}
}

type settled struct {
	voteDB
	proposal sqlc.InsertProposalParams
	voters   []uuid.UUID
	results  []app.CastVoteResult
}

func settleFiveYesOfEight(t *testing.T) settled {
	t.Helper()
	d := newVoteDB(t)
	p, voters := d.open(t, 8)
	h := d.handler()
	s := settled{voteDB: d, proposal: p, voters: voters}
	for i, choice := range []domain.Choice{
		domain.ChoiceYes, domain.ChoiceNo, domain.ChoiceYes, domain.ChoiceYes, domain.ChoiceYes,
	} {
		d.clk.Advance(time.Minute)
		s.results = append(s.results, d.mustCast(t, h, p.ID, voters[i], choice))
	}
	return s
}

func (s settled) reads() *app.ProposalReads {
	return app.NewProposalReads(
		s.pool, fakes.NewTrading(), &namedCabals{}, marketfake.NewCatalog(marketfake.Fixtures()...),
		&cardBook{}, &returnBook{}, &replyBook{},
	)
}

func (s settled) get(t *testing.T, caller uuid.UUID) app.ProposalDetail {
	t.Helper()
	got, err := s.reads().Get(t.Context(), app.GetProposal{
		ID: ids.ProposalIDFrom(s.proposal.ID), Caller: ids.UserIDFrom(caller),
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCastVote_returnsMyVote(t *testing.T) {
	t.Parallel()
	s := settleFiveYesOfEight(t)
	want := []app.MyVote{
		{Order: 1, Of: 8, YesOrder: 1},
		{Order: 2, Of: 8, NoOrder: 1},
		{Order: 3, Of: 8, YesOrder: 2},
		{Order: 4, Of: 8, YesOrder: 3},
	}
	for i, w := range want {
		if got := s.results[i].MyVote; got != w {
			t.Errorf("vote %d my vote = %+v, want %+v", i+1, got, w)
		}
	}
	h := s.handler()
	s.clk.Advance(time.Minute)
	s.mustCast(t, h, s.proposal.ID, s.voters[5], domain.ChoiceNo)
	s.clk.Advance(time.Minute)
	got := s.mustCast(t, h, s.proposal.ID, s.voters[5], domain.ChoiceYes)
	if got.Status != domain.StatusPassed || got.MyVote != (app.MyVote{Order: 6, Of: 8, YesOrder: 5, Decided: true}) {
		t.Fatalf("settling vote = %s, %+v, want passed as the 6th voter and 5th yes, decided", got.Status, got.MyVote)
	}
}

func TestGetProposal_settledByCallerMyVote(t *testing.T) {
	t.Parallel()
	s := settleFiveYesOfEight(t)
	decider := s.voters[5]
	s.clk.Advance(time.Minute)
	closedAt := s.clk.Now()
	s.mustCast(t, s.handler(), s.proposal.ID, decider, domain.ChoiceYes)
	got := s.get(t, decider)
	want := app.MyVote{Order: 6, Of: 8, YesOrder: 5, Decided: true}
	if got.MyVote == nil || *got.MyVote != want {
		t.Fatalf("decider my vote = %+v, want %+v", got.MyVote, want)
	}
	if got.Stats.ClosedAt == nil || !got.Stats.ClosedAt.Equal(closedAt) {
		t.Fatalf("closed at = %v, want the settling vote's time %s", got.Stats.ClosedAt, closedAt)
	}
	other := s.get(t, s.voters[0])
	if other.MyVote == nil || other.MyVote.Decided || other.MyVote.Order != 1 {
		t.Fatalf("other voter my vote = %+v, want first and not decided", other.MyVote)
	}
	if bystander := s.get(t, s.voters[7]); bystander.MyVote != nil {
		t.Fatalf("voter without a ballot my vote = %+v, want nil", bystander.MyVote)
	}
	if stranger := s.get(t, s.ids.NewV7()); stranger.MyVote != nil {
		t.Fatalf("non-member my vote = %+v, want nil", stranger.MyVote)
	}
}

func TestTransition_executionKeepsDeciderAndCloseTime(t *testing.T) {
	t.Parallel()
	s := settleFiveYesOfEight(t)
	s.clk.Advance(time.Minute)
	closedAt := s.clk.Now()
	s.mustCast(t, s.handler(), s.proposal.ID, s.voters[5], domain.ChoiceYes)
	before := s.closing(t, s.proposal.ID)
	if !before.closedAt.Time.Equal(closedAt) || before.decidedBy != uuidOf(s.voters[5]) {
		t.Fatalf("after the vote: %+v, want closed at %s by the settling voter", before, closedAt)
	}
	later := closedAt.Add(24 * time.Hour)
	for _, step := range [][2]string{{"passed", "execution_blocked"}, {"execution_blocked", "passed"}, {"passed", "executed"}} {
		from, to := step[0], step[1]
		n, err := s.q.Transition(t.Context(), sqlc.TransitionParams{
			ID: s.proposal.ID, FromStatus: from, ToStatus: to, At: later, DecidedBy: s.ids.NewV7().String(),
			Reason: pgtype.Text{String: "swap_failed", Valid: to == "execution_blocked"},
		})
		if err != nil || n != 1 {
			t.Fatalf("Transition %s to %s = %d, %v", from, to, n, err)
		}
		if got := s.closing(t, s.proposal.ID); got != before {
			t.Fatalf("after %s: %+v, want %+v unchanged", to, got, before)
		}
	}
}

func TestProposals_List_carriesReplyCountPerProposalInOneRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	first := d.seed(t, d.now)
	second := d.seed(t, d.now.Add(time.Second))
	replies := &replyBook{counts: map[ids.ProposalID]int{ids.ProposalIDFrom(first): 6}}
	page, err := d.readsWith(replies).List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(d.cabal), Caller: ids.UserIDFrom(d.caller),
	})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("List = %+v, %v, want two items", page, err)
	}
	got := map[uuid.UUID]int{}
	for _, v := range page.Items {
		got[v.ID.UUID()] = v.ReplyCount
	}
	if got[first] != 6 || got[second] != 0 {
		t.Errorf("reply counts = %v, want 6 for the first and 0 for the second", got)
	}
	if len(replies.calls) != 1 || len(replies.calls[0]) != 2 {
		t.Errorf("reply reads = %v, want one batched read of both proposals", replies.calls)
	}
}

func TestProposals_List_refusesWhenReplyCountsCannotBeRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	d.seed(t, d.now)
	down := &replyBook{err: errs.New(errs.CodeDBUnavailable, "test")}
	_, err := d.readsWith(down).List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(d.cabal), Caller: ids.UserIDFrom(d.caller),
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("List with a failing reply read = %v, want db_unavailable", err)
	}
}
