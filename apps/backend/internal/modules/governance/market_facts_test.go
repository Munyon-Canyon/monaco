package governance_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type factCabals struct {
	mu       sync.Mutex
	mine     []ids.CabalID
	views    map[ids.CabalID]cabalport.CabalView
	ofCalls  int
	viewCall int
	ofErr    error
	viewErr  error
}

func (f *factCabals) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ofCalls++
	return f.mine, f.ofErr
}

func (f *factCabals) Cabals(_ context.Context, want []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.viewCall++
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range want {
		if v, ok := f.views[id]; ok {
			out[id] = v
		}
	}
	return out, f.viewErr
}

type factHeld struct {
	mu    sync.Mutex
	rows  []treasuryport.HeldRow
	calls int
	err   error
}

func (f *factHeld) HeldBy(context.Context, ids.UserID) (treasuryport.HeldList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return treasuryport.HeldList{Holdings: f.rows}, f.err
}

type statementCounter struct{ n atomic.Int64 }

func (c *statementCounter) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	if strings.Contains(data.SQL, "unnest(") {
		c.n.Add(1)
	}
	return ctx
}

func (*statementCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type factsWorld struct {
	readsDB
	clock      *testkit.Clock
	cabals     *factCabals
	held       *factHeld
	cards      *cardBook
	statements *statementCounter
	counted    *pgxpool.Pool
	acCapital  uuid.UUID
	degens     uuid.UUID
}

func newFactsWorld(t *testing.T) factsWorld {
	t.Helper()
	d := newReadsDB(t)
	w := factsWorld{
		readsDB:    d,
		clock:      testkit.NewClock(d.now),
		cabals:     &factCabals{views: map[ids.CabalID]cabalport.CabalView{}},
		held:       &factHeld{},
		cards:      &cardBook{cards: map[ids.UserID]identityport.UserCard{}},
		statements: &statementCounter{},
		acCapital:  d.ids.NewV7(),
		degens:     d.ids.NewV7(),
	}
	cfg := d.pool.Config().Copy()
	cfg.ConnConfig.Tracer = w.statements
	counted, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(counted.Close)
	w.counted = counted
	w.join(w.acCapital, "AC Capital", cabalport.StatusActive)
	w.join(w.degens, "Degens", cabalport.StatusActive)
	return w
}

func (w factsWorld) join(cabal uuid.UUID, name string, status cabalport.Status) {
	id := ids.CabalIDFrom(cabal)
	w.cabals.mine = append(w.cabals.mine, id)
	w.cabals.views[id] = cabalport.CabalView{ID: id, Name: name, Status: status}
}

func (w factsWorld) facts() *app.MarketFacts {
	return app.NewMarketFacts(w.counted, w.cabals, w.held, w.cards, w.clock)
}

func (w factsWorld) proposal(
	t *testing.T, cabal uuid.UUID, symbol string, voters ...uuid.UUID,
) uuid.UUID {
	t.Helper()
	p := w.buy(voters...)
	p.CabalID, p.Symbol = cabal, symbol
	w.insert(t, p)
	return p.ID
}

func (w factsWorld) pass(t *testing.T, id uuid.UUID, status string, ago time.Duration) {
	t.Helper()
	_, err := w.pool.Exec(t.Context(), `UPDATE proposals SET status = $2, closed_at = $3 WHERE id = $1`,
		id, status, pgtype.Timestamptz{Time: w.now.Add(-ago), Valid: true})
	if err != nil {
		t.Fatal(err)
	}
}

func (w factsWorld) holding(cabal uuid.UUID, name, symbol, units string) treasuryport.HeldRow {
	return treasuryport.HeldRow{
		Cabal:   treasuryport.HeldCabal{ID: ids.CabalIDFrom(cabal), Name: name},
		Holding: treasuryport.Holding{Symbol: symbol, Units: units},
	}
}

func (w factsWorld) name(user uuid.UUID, display, handle string) {
	w.cards.cards[ids.UserIDFrom(user)] = identityport.UserCard{DisplayName: display, Handle: handle}
}

func (w factsWorld) lookup(t *testing.T, symbols ...string) map[string]*app.MarketFact {
	t.Helper()
	rows, err := w.facts().For(t.Context(), ids.UserIDFrom(w.caller), symbols)
	if err != nil {
		t.Fatalf("For(%v) = %v", symbols, err)
	}
	if len(rows) != len(symbols) {
		t.Fatalf("For(%v) = %d rows, want one per symbol", symbols, len(rows))
	}
	out, order := map[string]*app.MarketFact{}, make([]string, len(rows))
	for i, row := range rows {
		out[row.Symbol], order[i] = row.Fact, row.Symbol
	}
	if !slices.Equal(order, symbols) {
		t.Fatalf("For(%v) symbols = %v, want request order", symbols, order)
	}
	return out
}

func wantFact(t *testing.T, symbol string, got *app.MarketFact, want app.MarketFact) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s fact = %+v, want %+v", symbol, got, want)
	}
}

func wantNoFact(t *testing.T, symbol string, got *app.MarketFact) {
	t.Helper()
	if got != nil {
		t.Errorf("%s fact = %+v, want none", symbol, got)
	}
}

func TestMarketFacts_RanksVoteOpenHeldCalledNone(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	other := w.ids.NewV7()
	w.proposal(t, w.acCapital, "NVDAx", w.caller, other)
	meta := w.proposal(t, w.degens, "METAx", other)
	w.pass(t, meta, string(domain.StatusPassed), 24*time.Hour)
	if _, err := w.pool.Exec(
		t.Context(),
		`UPDATE proposals SET proposer_id = $2 WHERE id = $1`,
		meta,
		other,
	); err != nil {
		t.Fatal(err)
	}
	w.name(other, "Christelle", "chris")
	w.held.rows = []treasuryport.HeldRow{
		w.holding(w.acCapital, "AC Capital", "AAPLx", "24.0000"),
		w.holding(w.acCapital, "AC Capital", "NVDAx", "38.0000"),
	}
	got := w.lookup(t, "NVDAx", "AAPLx", "METAx", "MSFTx")
	wantFact(t, "NVDAx", got["NVDAx"], app.MarketFact{Kind: app.MarketFactVoteOpen, CabalName: "AC Capital", Count: 1})
	wantFact(t, "AAPLx", got["AAPLx"], app.MarketFact{Kind: app.MarketFactHeld, Units: "24.0000", CabalCount: 1})
	wantFact(t, "METAx", got["METAx"], app.MarketFact{Kind: app.MarketFactCalled, ProposerName: "Christelle"})
	wantNoFact(t, "MSFTx", got["MSFTx"])
}

func TestMarketFacts_HeldBeatsCalledAndVoteOpenBeatsBoth(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	called := w.proposal(t, w.acCapital, "TSLAx", w.caller)
	w.pass(t, called, string(domain.StatusExecuted), time.Hour)
	w.held.rows = []treasuryport.HeldRow{w.holding(w.acCapital, "AC Capital", "TSLAx", "2.2000")}
	wantFact(t, "TSLAx", w.lookup(t, "TSLAx")["TSLAx"],
		app.MarketFact{Kind: app.MarketFactHeld, Units: "2.2000", CabalCount: 1})
}

func TestMarketFacts_VoteOpenCountsTheCallersOpenVotesAndNamesTheSoonestClosing(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	later := w.buy(w.caller)
	later.CabalID, later.Symbol, later.ExpiresAt = w.acCapital, "NVDAx", w.now.Add(48*time.Hour)
	w.insert(t, later)
	sooner := w.buy(w.caller)
	sooner.CabalID, sooner.Symbol, sooner.ExpiresAt = w.degens, "NVDAx", w.now.Add(2*time.Hour)
	w.insert(t, sooner)
	w.held.rows = []treasuryport.HeldRow{w.holding(w.acCapital, "AC Capital", "NVDAx", "1.0000")}
	wantFact(t, "NVDAx", w.lookup(t, "NVDAx")["NVDAx"],
		app.MarketFact{Kind: app.MarketFactVoteOpen, CabalName: "Degens", Count: 2})
}

func TestMarketFacts_CalledLastsSevenDaysByClosedAtAndSurvivesExecution(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	proposer := w.ids.NewV7()
	w.name(proposer, "", "mia")
	for symbol, c := range map[string]struct {
		status string
		ago    time.Duration
	}{
		"OLDx":  {string(domain.StatusPassed), 7*24*time.Hour + time.Minute},
		"NEWx":  {string(domain.StatusPassed), 6*24*time.Hour + 23*time.Hour},
		"DONEx": {string(domain.StatusExecuted), 2 * 24 * time.Hour},
		"BLOKx": {string(domain.StatusExecutionBlocked), time.Hour},
	} {
		id := w.proposal(t, w.acCapital, symbol, w.caller)
		w.pass(t, id, c.status, c.ago)
		if _, err := w.pool.Exec(t.Context(), `UPDATE proposals SET proposer_id = $2, updated_at = $3 WHERE id = $1`,
			id, proposer, w.now); err != nil {
			t.Fatal(err)
		}
	}
	got := w.lookup(t, "OLDx", "NEWx", "DONEx", "BLOKx")
	wantNoFact(t, "OLDx", got["OLDx"])
	wantFact(t, "NEWx", got["NEWx"], app.MarketFact{Kind: app.MarketFactCalled, ProposerName: "mia"})
	wantFact(t, "DONEx", got["DONEx"], app.MarketFact{Kind: app.MarketFactCalled, ProposerName: "mia"})
	wantNoFact(t, "BLOKx", got["BLOKx"])
}

func TestMarketFacts_CalledNamesTheNewestPassByClosedAt(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	older, newer := w.ids.NewV7(), w.ids.NewV7()
	w.name(older, "Older", "")
	w.name(newer, "Newer", "")
	for proposer, ago := range map[uuid.UUID]time.Duration{older: 3 * 24 * time.Hour, newer: time.Hour} {
		id := w.proposal(t, w.acCapital, "METAx", w.caller)
		w.pass(t, id, string(domain.StatusPassed), ago)
		if _, err := w.pool.Exec(
			t.Context(),
			`UPDATE proposals SET proposer_id = $2 WHERE id = $1`,
			id,
			proposer,
		); err != nil {
			t.Fatal(err)
		}
	}
	wantFact(t, "METAx", w.lookup(t, "METAx")["METAx"],
		app.MarketFact{Kind: app.MarketFactCalled, ProposerName: "Newer"})
}

func TestMarketFacts_AMemberAdmittedAfterTheVoteOpenedGetsNoVoteOpenButKeepsHeldAndCalled(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	earlier := w.ids.NewV7()
	w.proposal(t, w.acCapital, "NVDAx", earlier)
	passed := w.proposal(t, w.acCapital, "METAx", earlier)
	w.pass(t, passed, string(domain.StatusPassed), time.Hour)
	w.held.rows = []treasuryport.HeldRow{w.holding(w.acCapital, "AC Capital", "NVDAx", "3.0000")}
	got := w.lookup(t, "NVDAx", "METAx")
	wantFact(t, "NVDAx", got["NVDAx"], app.MarketFact{Kind: app.MarketFactHeld, Units: "3.0000", CabalCount: 1})
	if got["METAx"] == nil || got["METAx"].Kind != app.MarketFactCalled {
		t.Errorf("METAx fact = %+v, want called", got["METAx"])
	}
}

func TestMarketFacts_ACabalTheCallerLeftOrWasBannedFromYieldsNoFact(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	left, banned := w.ids.NewV7(), w.ids.NewV7()
	w.cabals.views[ids.CabalIDFrom(left)] = cabalport.CabalView{
		ID:     ids.CabalIDFrom(left),
		Name:   "Left",
		Status: cabalport.StatusActive,
	}
	w.join(banned, "Banned", cabalport.StatusBanned)
	w.proposal(t, left, "NVDAx", w.caller)
	w.proposal(t, banned, "AAPLx", w.caller)
	passed := w.proposal(t, left, "METAx", w.caller)
	w.pass(t, passed, string(domain.StatusPassed), time.Hour)
	got := w.lookup(t, "NVDAx", "AAPLx", "METAx")
	for symbol, fact := range got {
		wantNoFact(t, symbol, fact)
	}
}

func TestMarketFacts_ACallerInNoCabalGetsNoFactAndNoReads(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	w.cabals.mine = nil
	got := w.lookup(t, "NVDAx", "AAPLx")
	wantNoFact(t, "NVDAx", got["NVDAx"])
	wantNoFact(t, "AAPLx", got["AAPLx"])
	if w.statements.n.Load() != 0 || w.held.calls != 0 {
		t.Errorf("reads = %d statements, %d held calls, want none", w.statements.n.Load(), w.held.calls)
	}
}

func TestMarketFacts_HeldSumsUnitsAcrossCabalsAndSkipsAnEmptyHolding(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	w.held.rows = []treasuryport.HeldRow{
		w.holding(w.acCapital, "AC Capital", "AAPLx", "24.1000"),
		w.holding(w.degens, "Degens", "AAPLx", "0.2500"),
		w.holding(w.degens, "Degens", "ZEROx", "0.0000"),
	}
	got := w.lookup(t, "AAPLx", "ZEROx", "TSLAx")
	wantFact(t, "AAPLx", got["AAPLx"], app.MarketFact{Kind: app.MarketFactHeld, Units: "24.3500", CabalCount: 2})
	wantNoFact(t, "ZEROx", got["ZEROx"])
	wantNoFact(t, "TSLAx", got["TSLAx"])
}

func TestMarketFacts_ReadsAFixedNumberOfTimesForAnySymbolCount(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	called := w.proposal(t, w.acCapital, "METAx", w.caller)
	w.pass(t, called, string(domain.StatusPassed), time.Hour)
	symbols := make([]string, 50)
	for i := range symbols {
		symbols[i] = "SYM" + string(rune('A'+i/26)) + string(rune('A'+i%26))
	}
	symbols[0], symbols[1] = "METAx", "METAx"
	w.lookup(t, symbols...)
	if w.cabals.ofCalls != 1 || w.cabals.viewCall != 1 || w.held.calls != 1 || len(w.cards.calls) != 1 ||
		w.statements.n.Load() != 2 {
		t.Errorf("calls = CabalsOf %d, Cabals %d, Held %d, UsersByID %d, SQL %d, want 1, 1, 1, 1, 2",
			w.cabals.ofCalls, w.cabals.viewCall, w.held.calls, len(w.cards.calls), w.statements.n.Load())
	}
}

func TestMarketFacts_ParseSymbols(t *testing.T) {
	t.Parallel()
	fifty := strings.Repeat("A,", 49) + "A"
	for name, c := range map[string]struct {
		raw  string
		want int
		code errs.Code
	}{
		"one":              {"NVDAx", 1, ""},
		"trims":            {" NVDAx , AAPLx", 2, ""},
		"fifty":            {fifty, 50, ""},
		"fifty one":        {fifty + ",A", 0, errs.CodeInvalidInput},
		"empty":            {"", 0, errs.CodeInvalidInput},
		"blank entry":      {"NVDAx,,AAPLx", 0, errs.CodeInvalidInput},
		"too long":         {strings.Repeat("A", 65), 0, errs.CodeInvalidInput},
		"longest accepted": {strings.Repeat("A", 64), 1, ""},
	} {
		got, err := app.ParseMarketFactSymbols(c.raw)
		if (c.code == "" && err != nil) || (c.code != "" && errs.CodeOf(err) != c.code) {
			t.Errorf("%s: err = %v, want %q", name, err, c.code)
		}
		if len(got) != c.want {
			t.Errorf("%s: %d symbols, want %d", name, len(got), c.want)
		}
	}
}

func TestMarketFacts_ReportsEachFailingRead(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeUpstreamUnavailable, "test.boom")
	for name, c := range map[string]struct {
		breakIt func(t *testing.T, w factsWorld)
		code    errs.Code
	}{
		"cabals of":   {func(_ *testing.T, w factsWorld) { w.cabals.ofErr = boom }, errs.CodeUpstreamUnavailable},
		"cabal views": {func(_ *testing.T, w factsWorld) { w.cabals.viewErr = boom }, errs.CodeUpstreamUnavailable},
		"held":        {func(_ *testing.T, w factsWorld) { w.held.err = boom }, errs.CodeUpstreamUnavailable},
		"bad units": {func(_ *testing.T, w factsWorld) {
			w.held.rows = []treasuryport.HeldRow{w.holding(w.acCapital, "AC Capital", "AAPLx", "lots")}
		}, errs.CodeInternal},
		"proposer cards": {func(t *testing.T, w factsWorld) {
			t.Helper()
			id := w.proposal(t, w.acCapital, "METAx", w.caller)
			w.pass(t, id, string(domain.StatusPassed), time.Hour)
			w.cards.err = boom
		}, errs.CodeUpstreamUnavailable},
		"cancelled context": {func(*testing.T, factsWorld) {}, errs.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newFactsWorld(t)
			c.breakIt(t, w)
			ctx := t.Context()
			if name == "cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := w.facts().
				For(ctx, ids.UserIDFrom(w.caller), []string{"METAx", "AAPLx"}); errs.CodeOf(
				err,
			) != c.code {
				t.Errorf("err = %v, want %s", err, c.code)
			}
		})
	}
}

func TestMarketFacts_ReportsAFailingCalledStatement(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	if _, err := w.pool.Exec(t.Context(), `ALTER TABLE proposals RENAME COLUMN closed_at TO closed_when`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.facts().
		For(t.Context(), ids.UserIDFrom(w.caller), []string{"METAx"}); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Errorf("For with a failing CalledFacts err = %v, want internal", err)
	}
}

func (w factsWorld) httpFacts(t *testing.T, symbols string) (api.GetMyMarketFactsResponseObject, error) {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: w.caller.String()})
	return adapters.HTTP{Facts: w.facts()}.GetMyMarketFacts(ctx, api.GetMyMarketFactsRequestObject{
		Params: api.GetMyMarketFactsParams{Symbols: symbols},
	})
}

func TestHTTP_GetMyMarketFacts_wiresEachKind(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	proposer := w.ids.NewV7()
	w.name(proposer, "Christelle", "")
	w.proposal(t, w.acCapital, "NVDAx", w.caller)
	called := w.proposal(t, w.degens, "METAx", w.caller)
	w.pass(t, called, string(domain.StatusPassed), time.Hour)
	if _, err := w.pool.Exec(
		t.Context(),
		`UPDATE proposals SET proposer_id = $2 WHERE id = $1`,
		called,
		proposer,
	); err != nil {
		t.Fatal(err)
	}
	w.held.rows = []treasuryport.HeldRow{w.holding(w.acCapital, "AC Capital", "AAPLx", "24.0000")}
	res, err := w.httpFacts(t, "NVDAx,AAPLx,METAx,MSFTx")
	got, ok := res.(api.GetMyMarketFacts200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetMyMarketFacts = %#v, %v, want facts", res, err)
	}
	vote, held, calledFact := api.MarketFact{
		Kind: api.VoteOpen,
	}, api.MarketFact{
		Kind: api.Held,
	}, api.MarketFact{
		Kind: api.Called,
	}
	name, count, units, cabals, by := "AC Capital", 1, "24.0000", 1, "Christelle"
	vote.CabalName, vote.Count = &name, &count
	held.Units, held.CabalCount = &units, &cabals
	calledFact.ProposerName = &by
	want := []api.MarketFactRow{
		{Symbol: "NVDAx", Fact: &vote},
		{Symbol: "AAPLx", Fact: &held},
		{Symbol: "METAx", Fact: &calledFact},
		{Symbol: "MSFTx", Fact: nil},
	}
	if !reflect.DeepEqual(got.Facts, want) {
		t.Errorf("facts = %+v, want %+v", got.Facts, want)
	}
}

func TestHTTP_GetMyMarketFacts_refuses(t *testing.T) {
	t.Parallel()
	w := newFactsWorld(t)
	if _, err := (adapters.HTTP{Facts: w.facts()}).GetMyMarketFacts(
		t.Context(), api.GetMyMarketFactsRequestObject{},
	); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Errorf("GetMyMarketFacts with no actor err = %v, want unauthorized", err)
	}
	if _, err := w.httpFacts(t, strings.Repeat("A,", 50)+"A"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("GetMyMarketFacts with 51 symbols err = %v, want invalid_input", err)
	}
	w.cabals.ofErr = errs.New(errs.CodeUpstreamUnavailable, "test.boom")
	if _, err := w.httpFacts(t, "NVDAx"); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Errorf("GetMyMarketFacts with a failing read err = %v, want upstream_unavailable", err)
	}
}
