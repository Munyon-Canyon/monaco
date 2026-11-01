package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	showSidesChange = `{"v":1,"rule":"show_sides","from":false,"to":true}`
	uniqueViolation = "23505"
	checkViolation  = "23514"
)

func (d proposalDB) ruleSQL(ctx context.Context, t *testing.T, q interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}, cabal uuid.UUID, change string, voters ...uuid.UUID,
) (uuid.UUID, error) {
	t.Helper()
	id := d.ids.NewV7()
	_, err := q.Exec(ctx, `INSERT INTO proposals
		(id, cabal_id, proposer_id, kind, rule_change, threshold, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'rule', $4::jsonb, 'majority', 'open', $5, $6, $6)`,
		id, cabal, d.ids.NewV7(), change, d.now.Add(24*time.Hour), d.now)
	if err != nil {
		return id, err
	}
	for _, voter := range voters {
		if _, err := q.Exec(ctx,
			`INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, id, voter); err != nil {
			t.Fatal(err)
		}
	}
	return id, nil
}

func (d proposalDB) mustRule(t *testing.T, cabal uuid.UUID, change string, voters ...uuid.UUID) uuid.UUID {
	t.Helper()
	id, err := d.ruleSQL(t.Context(), t, d.pool, cabal, change, voters...)
	if err != nil {
		t.Fatalf("insert rule proposal %s = %v", change, err)
	}
	return id
}

func wantViolation(t *testing.T, err error, code string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != code {
		t.Fatalf("err = %v, want SQLSTATE %s", err, code)
	}
}

func TestRuleProposals_twoConcurrentOpenInsertsForOneCabalWideRuleLeaveOne(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	cabal := d.ids.NewV7()
	start := make(chan struct{})
	var results [2]error
	var g errgroup.Group
	for i := range results {
		g.Go(func() error {
			<-start
			results[i] = d.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
				_, err := d.ruleSQL(ctx, t, tx.Queries(), cabal, showSidesChange)
				return err
			})
			return nil
		})
	}
	close(start)
	_ = g.Wait()
	first, second := results[0], results[1]
	if (first == nil) == (second == nil) {
		t.Fatalf("results = %v and %v, want exactly one insert to commit", first, second)
	}
	wantViolation(t, errors.Join(first, second), uniqueViolation)
	var open int
	if err := d.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM proposals WHERE cabal_id = $1 AND kind = 'rule' AND status = 'open'`, cabal,
	).Scan(&open); err != nil || open != 1 {
		t.Fatalf("open rule proposals = %d, %v, want 1", open, err)
	}
	d.mustRule(t, d.ids.NewV7(), showSidesChange)
	d.mustRule(t, cabal, `{"v":1,"rule":"join_mode","from":"open","to":"request"}`)
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE proposals SET status = 'failed' WHERE cabal_id = $1`,
		cabal,
	); err != nil {
		t.Fatal(err)
	}
	d.mustRule(t, cabal, showSidesChange)
}

func TestRuleProposals_voterChangesAreUniquePerMember(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	cabal, mia, kai := d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()
	voter := func(user uuid.UUID, rule string) string {
		return `{"v":1,"rule":"` + rule + `","from":false,"to":true,"user_id":"` + user.String() + `"}`
	}
	d.mustRule(t, cabal, voter(mia, "voter"))
	d.mustRule(t, cabal, voter(kai, "voter"))
	d.mustRule(t, cabal, voter(mia, "admin"))
	_, err := d.ruleSQL(t.Context(), t, d.pool, cabal, voter(mia, "voter"))
	wantViolation(t, err, uniqueViolation)
	_, err = d.ruleSQL(t.Context(), t, d.pool, cabal, voter(mia, "admin"))
	wantViolation(t, err, uniqueViolation)
}

func TestRuleProposals_theShapeCheckKeepsTradesAndRulesApart(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	exec := func(kind, symbol, mint, usdc, quote, change string) error {
		_, err := d.pool.Exec(t.Context(), `INSERT INTO proposals
			(id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, rule_change, threshold, status,
			 expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, '')::bigint, NULLIF($8, '')::bigint,
			 NULLIF($9, '')::jsonb, 'majority', 'open', $10, $11, $11)`,
			d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7(), kind, symbol, mint, usdc, quote, change,
			d.now.Add(time.Hour), d.now)
		return err
	}
	if err := exec("rule", "", "", "", "", showSidesChange); err != nil {
		t.Fatalf("rule row with null trade fields = %v", err)
	}
	for name, err := range map[string]error{
		"trade with a null symbol":       exec("buy", "", "mint", "5", "5", ""),
		"trade with a null mint":         exec("buy", "AAPLx", "", "5", "5", ""),
		"trade with a null quote":        exec("buy", "AAPLx", "mint", "5", "", ""),
		"trade carrying a rule change":   exec("buy", "AAPLx", "mint", "5", "5", showSidesChange),
		"rule without a change":          exec("rule", "", "", "", "", ""),
		"rule carrying a symbol":         exec("rule", "AAPLx", "", "", "", showSidesChange),
		"rule carrying an amount":        exec("rule", "", "", "5", "", showSidesChange),
		"rule carrying a mint and quote": exec("rule", "", "mint", "", "5", showSidesChange),
	} {
		if err == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		wantViolation(t, err, checkViolation)
	}
}

type ruleFixture struct {
	readsDB
	rule  uuid.UUID
	user  uuid.UUID
	cards map[ids.UserID]identityport.UserCard
}

func newRuleFixture(t *testing.T) ruleFixture {
	t.Helper()
	d := newReadsDB(t)
	user := d.ids.NewV7()
	change := `{"v":1,"rule":"voter","from":false,"to":true,"user_id":"` + user.String() + `"}`
	return ruleFixture{
		readsDB: d, user: user, rule: d.mustRule(t, d.cabal, change, d.caller),
		cards: map[ids.UserID]identityport.UserCard{
			ids.UserIDFrom(user): {Handle: "kai", DisplayName: "Kai", PhotoURL: "https://cdn.example.com/kai.png"},
		},
	}
}

func (f ruleFixture) readsWithCards() *app.ProposalReads {
	return app.NewProposalReads(
		f.pool,
		f.swaps,
		f.names,
		f.assets,
		&cardBook{cards: f.cards},
		&returnBook{},
		&replyBook{},
	)
}

func (f ruleFixture) actor() context.Context {
	return auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorUser, ID: f.caller.String()})
}

type ruleRead struct {
	kind       domain.Kind
	symbol     string
	amounts    [3]int64
	rule       domain.Rule
	userID     string
	userHandle string
}

func ruleReadOf(kind domain.Kind, symbol string, amounts [3]int64, rule *app.RuleView) ruleRead {
	out := ruleRead{kind: kind, symbol: symbol, amounts: amounts}
	if rule == nil {
		return out
	}
	out.rule = rule.Change.Rule()
	if rule.User != nil {
		out.userID, out.userHandle = rule.User.ID.String(), rule.User.Handle
	}
	return out
}

func TestRuleProposals_getListAndPendingVotesReadARuleRow(t *testing.T) {
	t.Parallel()
	f := newRuleFixture(t)
	reads := f.readsWithCards()
	want := ruleRead{
		kind:       domain.KindRule,
		rule:       domain.RuleVoter,
		userID:     ids.UserIDFrom(f.user).String(),
		userHandle: "kai",
	}

	detail, err := reads.Get(
		t.Context(),
		app.GetProposal{ID: ids.ProposalIDFrom(f.rule), Caller: ids.UserIDFrom(f.caller)},
	)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	view := detail.Proposal
	got := ruleReadOf(view.Kind, view.Symbol, [3]int64{view.USDCMicros, view.TokenAmount, view.QuoteOut}, view.Rule)
	if got != want || !detail.CanVote || detail.Swap != nil {
		t.Fatalf("Get = %+v, want %+v, a voter who can vote and no swap", got, want)
	}

	page, err := reads.List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(f.cabal), Caller: ids.UserIDFrom(f.caller), Filter: app.FilterAll,
	})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("List = %+v, %v, want the one rule proposal", page, err)
	}
	item := page.Items[0]
	got = ruleReadOf(item.Kind, item.Symbol, [3]int64{item.USDCMicros, item.TokenAmount, item.QuoteOut}, item.Rule)
	if got != want {
		t.Fatalf("List item = %+v, want %+v", got, want)
	}

	pending := app.NewPendingVoteReads(
		reads,
		app.PendingVotePorts{Assets: f.assets, Replies: &replyBook{}, Cabals: f.roster()},
	)
	votes, err := pending.PendingVotes(t.Context(), ids.UserIDFrom(f.caller))
	if err != nil || len(votes) != 1 || votes[0].AssetName != "" {
		t.Fatalf("PendingVotes = %+v, %v, want the one rule proposal without an asset", votes, err)
	}
	got = ruleReadOf(votes[0].Kind, votes[0].Symbol, [3]int64{}, votes[0].Rule)
	if got != want {
		t.Fatalf("PendingVotes item = %+v, want %+v", got, want)
	}
}

func TestRuleProposals_theCabalWideRuleReadsWithoutAMember(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.mustRule(t, d.cabal, showSidesChange, d.caller)
	detail, err := d.reads().
		Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(id), Caller: ids.UserIDFrom(d.caller)})
	if err != nil || detail.Proposal.Rule == nil || detail.Proposal.Rule.User != nil ||
		detail.Proposal.Rule.Change.Rule() != domain.RuleShowSides {
		t.Fatalf("Get = %+v, %v, want a cabal-wide show_sides change with no member", detail.Proposal, err)
	}
}

func TestRuleProposals_aCorruptRuleChangeFailsEveryRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.mustRule(t, d.cabal, showSidesChange, d.caller)
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE proposals SET rule_change = '{"v":9,"rule":"show_sides","from":false,"to":true}' WHERE id = $1`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reads().
		Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(id), Caller: ids.UserIDFrom(d.caller)}); err == nil {
		t.Error("Get accepted a rule change at an unknown version")
	}
	if _, err := d.votes().PendingVotes(t.Context(), ids.UserIDFrom(d.caller)); err == nil {
		t.Error("PendingVotes accepted a rule change at an unknown version")
	}
}

func TestRuleProposals_aCardLookupFailureFailsTheRead(t *testing.T) {
	t.Parallel()
	f := newRuleFixture(t)
	reads := app.NewProposalReads(
		f.pool,
		f.swaps,
		f.names,
		f.assets,
		&cardBook{err: errs.New(errs.CodeInternal, "test.cards")},
		&returnBook{},
		&replyBook{},
	)
	if _, err := reads.Get(
		t.Context(),
		app.GetProposal{ID: ids.ProposalIDFrom(f.rule), Caller: ids.UserIDFrom(f.caller)},
	); err == nil {
		t.Error("Get ignored a failed card lookup")
	}
}

type ruleWireRead struct {
	kind       string
	tradeNulls bool
	rule       string
	from, to   any
	userID     uuid.UUID
	hasUser    bool
}

func (f ruleFixture) wireReads(t *testing.T, reads *app.ProposalReads) (ruleWireRead, ruleWireRead, ruleWireRead) {
	t.Helper()
	pendingReads := app.NewPendingVoteReads(
		reads,
		app.PendingVotePorts{Assets: f.assets, Replies: &replyBook{}, Cabals: f.roster()},
	)
	h := adapters.HTTP{Reads: reads, Pending: pendingReads}
	return f.detailWire(t, h), f.listWire(t, h), f.pendingWire(t, h)
}

func (f ruleFixture) detailWire(t *testing.T, h adapters.HTTP) ruleWireRead {
	t.Helper()
	res, err := h.GetProposal(f.actor(), api.GetProposalRequestObject{Id: f.rule})
	d, ok := res.(api.GetProposal200JSONResponse)
	if err != nil || !ok || d.RuleChange == nil {
		t.Fatalf("GetProposal = %#v, %v, want a rule proposal", res, err)
	}
	out := ruleWireRead{
		kind:       string(d.Kind),
		tradeNulls: d.Symbol == nil && d.UsdcMicros == nil && d.TokenAmount == nil && d.QuoteOutAmount == nil,
		rule:       string(d.RuleChange.Rule), from: d.RuleChange.From, to: d.RuleChange.To,
		hasUser: d.RuleChange.User != nil,
	}
	if out.hasUser {
		out.userID = d.RuleChange.User.UserId
	}
	return out
}

func (f ruleFixture) listWire(t *testing.T, h adapters.HTTP) ruleWireRead {
	t.Helper()
	res, err := h.GetCabalProposals(f.actor(), api.GetCabalProposalsRequestObject{Id: f.cabal})
	page, ok := res.(api.GetCabalProposals200JSONResponse)
	if err != nil || !ok || len(page.Proposals) != 1 || page.Proposals[0].RuleChange == nil {
		t.Fatalf("GetCabalProposals = %#v, %v, want one rule proposal", res, err)
	}
	p := page.Proposals[0]
	out := ruleWireRead{
		kind:       string(p.Kind),
		tradeNulls: p.Symbol == nil && p.UsdcMicros == nil && p.TokenAmount == nil && p.QuoteOutAmount == nil,
		rule:       string(p.RuleChange.Rule), from: p.RuleChange.From, to: p.RuleChange.To,
		hasUser: p.RuleChange.User != nil,
	}
	if out.hasUser {
		out.userID = p.RuleChange.User.UserId
	}
	return out
}

func (f ruleFixture) pendingWire(t *testing.T, h adapters.HTTP) ruleWireRead {
	t.Helper()
	res, err := h.GetMyPendingVotes(f.actor(), api.GetMyPendingVotesRequestObject{})
	got, ok := res.(api.GetMyPendingVotes200JSONResponse)
	if err != nil || !ok || len(got) != 1 || got[0].RuleChange == nil {
		t.Fatalf("GetMyPendingVotes = %#v, %v, want one rule proposal", res, err)
	}
	v := got[0]
	out := ruleWireRead{
		kind:       string(v.Kind),
		tradeNulls: v.Symbol == nil && v.UsdcMicros == nil && v.TokenAmount == nil && v.AssetName == nil,
		rule:       string(v.RuleChange.Rule), from: v.RuleChange.From, to: v.RuleChange.To,
		hasUser: v.RuleChange.User != nil,
	}
	if out.hasUser {
		out.userID = v.RuleChange.User.UserId
	}
	return out
}

func TestRuleProposals_theHTTPWireCarriesTheRuleChangeAndNullTradeFields(t *testing.T) {
	t.Parallel()
	f := newRuleFixture(t)
	want := ruleWireRead{
		kind:       "rule",
		tradeNulls: true,
		rule:       "voter",
		from:       false,
		to:         true,
		userID:     f.user,
		hasUser:    true,
	}
	detail, list, pending := f.wireReads(t, f.readsWithCards())
	for name, got := range map[string]ruleWireRead{"detail": detail, "list": list, "pending": pending} {
		if got != want {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
}

func TestRuleProposals_aCabalWideChangeHasNoUserOnTheWire(t *testing.T) {
	t.Parallel()
	f := newRuleFixture(t)
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE proposals SET rule_change = $2::jsonb WHERE id = $1`,
		f.rule,
		showSidesChange,
	); err != nil {
		t.Fatal(err)
	}
	want := ruleWireRead{kind: "rule", tradeNulls: true, rule: "show_sides", from: false, to: true}
	detail, list, pending := f.wireReads(t, f.readsWithCards())
	for name, got := range map[string]ruleWireRead{"detail": detail, "list": list, "pending": pending} {
		if got != want {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
}

func TestRuleProposals_aPassingVoteMovesItToPassedWithoutATradeEvent(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	voter := d.ids.NewV7()
	id := d.mustRule(t, d.ids.NewV7(), showSidesChange, voter)
	got := d.mustCast(t, d.handler(), id, voter, domain.ChoiceYes)
	if got.Status != domain.StatusPassed || d.status(t, id) != "passed" {
		t.Fatalf("status = %s / %s, want passed", got.Status, d.status(t, id))
	}
	if n := d.terminal(t, id); n != 0 {
		t.Fatalf("terminal events = %d, want none until the rule pass event exists", n)
	}
}

func TestRuleProposals_aFailingVoteAndAnExpiryCarryTheRuleKind(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	voter := d.ids.NewV7()
	rejected := d.mustRule(t, d.ids.NewV7(), showSidesChange, voter)
	d.mustCast(t, d.handler(), rejected, voter, domain.ChoiceNo)
	failed := d.payloads(t, rejected, events.TypeProposalFailed)
	if len(failed) != 1 || !jsonHasKind(failed[0], "rule") {
		t.Fatalf("proposal.failed payloads = %s, want one with kind rule", failed)
	}

	late := d.mustRule(t, d.ids.NewV7(), showSidesChange, d.ids.NewV7())
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE proposals SET expires_at = $2 WHERE id = $1`,
		late,
		d.clk.Now().Add(-time.Minute),
	); err != nil {
		t.Fatal(err)
	}
	d.mustTick(t, 1, 1)
	expired := d.payloads(t, late, events.TypeProposalExpired)
	if len(expired) != 1 || !jsonHasKind(expired[0], "rule") {
		t.Fatalf("proposal.expired payloads = %s, want one with kind rule", expired)
	}
}

func jsonHasKind(raw []byte, kind string) bool {
	var got struct {
		Kind string `json:"kind"`
	}
	return json.Unmarshal(raw, &got) == nil && got.Kind == kind
}

func TestRuleProposals_aListMixesTradesAndRuleChanges(t *testing.T) {
	t.Parallel()
	f := newRuleFixture(t)
	f.seed(t, f.now.Add(time.Hour))
	page, err := f.readsWithCards().List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(f.cabal), Caller: ids.UserIDFrom(f.caller), Filter: app.FilterAll,
	})
	if err != nil || len(page.Items) != 2 || page.Items[0].Rule != nil || page.Items[0].Symbol != "AAPLx" ||
		page.Items[1].Rule == nil || page.Items[1].Rule.User == nil {
		t.Fatalf("List = %+v, %v, want the trade without a rule and the rule with its member", page, err)
	}
}
