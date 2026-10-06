package app

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type sweeperReader struct {
	statuses    []SigStatus
	statusErr   error
	inbound     money.BaseUnits
	inboundErr  error
	valid       bool
	validErr    error
	sigs        []chain.Signature
	batches     [][]chain.Signature
	owner       chain.SolanaAddress
	mint        chain.Mint
	followups   [][]SigStatus
	followupErr error
	onStatus    func()
	calls       int
}

func (r *sweeperReader) SignatureStatuses(
	_ context.Context,
	sigs []chain.Signature,
) ([]SigStatus, error) {
	r.calls++
	r.sigs = append([]chain.Signature(nil), sigs...)
	r.batches = append(r.batches, r.sigs)
	if r.onStatus != nil {
		r.onStatus()
	}
	if r.calls > 1 && len(r.followups) > 0 {
		reply := r.followups[0]
		r.followups = r.followups[1:]
		return reply, r.statusErr
	}
	if r.calls > 1 && r.followupErr != nil {
		return nil, r.followupErr
	}
	return r.statuses, r.statusErr
}

func (r *sweeperReader) BlockhashValid(context.Context, []byte) (bool, error) {
	return r.valid, r.validErr
}

func (r *sweeperReader) InboundAmount(
	_ context.Context, _ chain.Signature, owner chain.SolanaAddress, mint chain.Mint,
) (money.BaseUnits, error) {
	r.owner, r.mint = owner, mint
	return r.inbound, r.inboundErr
}

type sweeperHints struct {
	sent    int
	keys    []string
	payload [][]byte
	cancel  context.CancelFunc
}

func (h *sweeperHints) PublishHint(_ context.Context, key string, payload []byte) {
	h.sent++
	h.keys = append(h.keys, key)
	h.payload = append(h.payload, bytes.Clone(payload))
	if h.cancel != nil {
		h.cancel()
	}
}

func (h *sweeperHints) has(key string, id uuid.UUID) bool {
	want := map[string]string{"swap_id": id.String()}
	for i, got := range h.keys {
		if got != key {
			continue
		}
		var payload map[string]string
		if json.Unmarshal(h.payload[i], &payload) == nil && payload["swap_id"] == want["swap_id"] {
			return true
		}
	}
	return false
}

type sweeperEnv struct {
	pool sqlc.DBTX
	clk  *testkit.Clock
	ids  *testkit.IDs
	uow  *db.UnitOfWork
	q    *sqlc.Queries
	read *sweeperReader
	hint *sweeperHints
}

func newSweeperEnv(t *testing.T) *sweeperEnv {
	t.Helper()
	clk := testkit.NewClock(clock.Real{}.Now().Truncate(time.Second))
	pool, ids := testkit.DB(t), testkit.NewIDs(91)
	return &sweeperEnv{
		pool: pool, clk: clk, ids: ids, uow: db.New(pool, ids, clk), q: sqlc.New(pool),
		read: &sweeperReader{inbound: money.NewBaseUnits(104_000_000, 8)}, hint: &sweeperHints{},
	}
}

func (e *sweeperEnv) poller() *SwapSweeper {
	return NewSwapSweeper(e.uow, e.pool, e.clk, e.read, e.hint, DefaultSweepTiming())
}

func (e *sweeperEnv) insert(t *testing.T) sqlc.InsertCreatedParams {
	t.Helper()
	p := sqlc.InsertCreatedParams{
		ID: e.ids.NewV7(), SourceKind: "proposal", SourceID: e.ids.NewV7(), CabalID: e.ids.NewV7(),
		TreasuryAddress: "treasury", Action: "buy", Symbol: "AAPLx", InMint: "usdc", OutMint: "aaplx",
		OutDecimals: 8, InAmount: 25_000_000,
		SlippageBps: 100, SourceBatchSize: 3, CreatedAt: e.clk.Now(),
	}
	if err := e.q.InsertCreated(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *sweeperEnv) submit(t *testing.T) sqlc.InsertCreatedParams {
	t.Helper()
	p := e.insert(t)
	params := sqlc.MarkSubmittedParams{
		ID: p.ID, ExecuteRequestID: p.ID.String(), SignedTx: []byte{1},
		TxSignature: "sig-" + p.ID.String(), SubmittedAt: e.clk.Now(),
	}
	if n, err := e.q.MarkSubmitted(t.Context(), params); err != nil || n != 1 {
		t.Fatalf("MarkSubmitted = %d, %v", n, err)
	}
	return p
}

func (e *sweeperEnv) advance() { e.clk.Advance(SwapSweepAge + time.Second) }

func (e *sweeperEnv) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := e.pool.QueryRow(t.Context(), `SELECT status FROM swaps WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (e *sweeperEnv) terminalEvents(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	const terminalEvents = `SELECT count(*) FROM events
		WHERE aggregate_id = $1 AND type IN ('trade.confirmed', 'trade.failed')`
	if err := e.pool.QueryRow(t.Context(), terminalEvents, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *sweeperEnv) terminalEvent(t *testing.T, id uuid.UUID) events.Event {
	t.Helper()
	var typ events.Type
	var payload []byte
	const terminalEvent = `SELECT type, payload FROM events
		WHERE aggregate_id = $1 AND type IN ('trade.confirmed', 'trade.failed')`
	if err := e.pool.QueryRow(t.Context(), terminalEvent, id).Scan(&typ, &payload); err != nil {
		t.Fatal(err)
	}
	event, err := events.Decode(typ, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func tickContext(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:poller.trading.swap_sweeper")
}

func TestSwapSweeper_resolvesCreatedAndEveryStaleSignatureState(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	created, finalized, missing, processing := e.insert(t), e.submit(t), e.submit(t), e.submit(t)
	e.read.statuses = []SigStatus{
		{State: SigFinalized},
		{State: SigNotFound},
		{State: SigProcessing, Failed: true},
	}
	e.read.followups = [][]SigStatus{{{State: SigNotFound}}}
	e.advance()
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(
		tickContext(t), observability.NewLogger(config.Config{Env: config.EnvTest}, logs),
	)
	report, err := e.poller().Tick(ctx)
	if err != nil || report.Scanned != 4 || report.Changed != 3 {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
	e.assertStates(t, []sweeperState{
		{created.ID, "failed", 1},
		{finalized.ID, "confirmed", 1},
		{missing.ID, "failed", 1},
		{processing.ID, "submitted", 0},
	})
	wantMint := chain.Mint{Address: "aaplx", Decimals: 8}
	if len(e.read.batches) == 0 || len(e.read.batches[0]) != 3 || e.read.owner != "treasury" ||
		e.read.mint != wantMint ||
		e.hint.sent != 3 {
		t.Fatalf(
			"reader %+v owner %q mint %+v hints %d",
			e.read.batches,
			e.read.owner,
			e.read.mint,
			e.hint.sent,
		)
	}
	e.assertHintsAndLogs(t, logs, created, finalized, missing)
	e.assertRecoveredEvents(t, created.ID, finalized.ID, wantMint.Address)
}

func TestSwapSweeper_waitsForRowsOlderThanSweepAge(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	now := e.clk.Now()
	ages := []time.Time{
		now.Add(-SwapSweepAge + time.Second),
		now.Add(-SwapSweepAge),
		now.Add(-SwapSweepAge - time.Second),
	}
	created, submitted := e.ageCreated(t, ages), e.ageSubmitted(t, ages)
	e.read.statuses = []SigStatus{{State: SigFinalized}}
	report, err := e.poller().Tick(tickContext(t))
	if err != nil || report.Scanned != 2 || report.Changed != 2 || e.read.calls != 1 {
		t.Fatalf("Tick = %+v, %v, status calls = %d", report, err, e.read.calls)
	}
	e.assertStates(t, []sweeperState{
		{created[0].ID, "created", 0},
		{created[1].ID, "created", 0},
		{submitted[0].ID, "submitted", 0},
		{submitted[1].ID, "submitted", 0},
		{created[2].ID, "failed", 1},
		{submitted[2].ID, "confirmed", 1},
	})
}

func TestSwapSweeper_sweepsAtTheConfiguredAgeAndInterval(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	timing := SweepTiming{Interval: time.Second, Age: 2 * time.Second}
	now := e.clk.Now()
	created := e.ageCreated(t, []time.Time{now.Add(-time.Second), now.Add(-3 * time.Second)})
	p := NewSwapSweeper(e.uow, e.pool, e.clk, e.read, e.hint, timing)
	report, err := p.Tick(tickContext(t))
	if err != nil || report.Scanned != 1 || report.Changed != 1 || p.Interval() != time.Second {
		t.Fatalf("Tick = %+v, %v, interval %s; want the 3s old row swept every 1s", report, err, p.Interval())
	}
	e.assertStates(t, []sweeperState{{created[0].ID, "created", 0}, {created[1].ID, "failed", 1}})
}

func TestSwapSweeper_acceptsOnlyValidOutputDecimalBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		decimals int16
		want     uint8
	}{{"zero", 0, 0}, {"max", 255, 255}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			row := e.submit(t)
			if _, err := e.pool.Exec(
				t.Context(), `UPDATE swaps SET out_decimals = $2 WHERE id = $1`, row.ID, tc.decimals,
			); err != nil {
				t.Fatal(err)
			}
			e.read.statuses = []SigStatus{{State: SigFinalized}}
			e.advance()
			if _, err := e.poller().Tick(tickContext(t)); err != nil || e.status(t, row.ID) != "confirmed" ||
				e.read.mint.Decimals != tc.want {
				t.Fatalf("decimal %d recovery = %s, mint %+v, %v", tc.decimals, e.status(t, row.ID), e.read.mint, err)
			}
		})
	}
	e := newSweeperEnv(t)
	row := sqlc.ListStaleSubmittedRow{
		ID: e.ids.NewV7(), CabalID: e.ids.NewV7(), SourceKind: "proposal", SourceID: e.ids.NewV7(),
		Action: "buy", Symbol: "AAPLx", InMint: "usdc", InAmount: 1, SourceBatchSize: 1,
	}
	for _, decimals := range []int16{256, -1} {
		row.OutDecimals = decimals
		_, err := e.poller().resolveSubmitted(tickContext(t), e.clk.Now(), row, SigStatus{State: SigFinalized})
		if errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("decimal %d error = %v", decimals, err)
		}
	}
}

func TestSwapSweeper_readsStaleRowsThroughPartialIndexes(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(t.Context(), `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		index string
		query string
	}{
		{"swaps_stale_created_idx", `SELECT id FROM swaps WHERE status = 'created' AND created_at < $1 ORDER BY updated_at, id LIMIT $2`},
		{"swaps_stale_submitted_idx", `SELECT id FROM swaps WHERE status = 'submitted' AND submitted_at < $1 ORDER BY updated_at, id LIMIT $2`},
	} {
		rows, err := conn.Query(t.Context(), "EXPLAIN "+tc.query, clock.Real{}.Now(), int32(SwapSweepBatch))
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(line)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if plan := plan.String(); !strings.Contains(plan, tc.index) || strings.Contains(plan, "Sort") {
			t.Fatalf("%s plan = %s", tc.index, plan)
		}
	}
}

type sweeperState struct {
	id     uuid.UUID
	status string
	events int
}

func (e *sweeperEnv) assertStates(t *testing.T, want []sweeperState) {
	t.Helper()
	for _, state := range want {
		if got := e.status(t, state.id); got != state.status || e.terminalEvents(t, state.id) != state.events {
			t.Fatalf("swap %s = %s with %d events", state.id, got, e.terminalEvents(t, state.id))
		}
	}
}

func (e *sweeperEnv) ageCreated(t *testing.T, ages []time.Time) []sqlc.InsertCreatedParams {
	t.Helper()
	rows := make([]sqlc.InsertCreatedParams, len(ages))
	for i, at := range ages {
		rows[i] = e.insert(t)
		if _, err := e.pool.Exec(
			t.Context(), `UPDATE swaps SET created_at = $2, updated_at = $2 WHERE id = $1`, rows[i].ID, at,
		); err != nil {
			t.Fatal(err)
		}
	}
	return rows
}

func (e *sweeperEnv) ageSubmitted(t *testing.T, ages []time.Time) []sqlc.InsertCreatedParams {
	t.Helper()
	rows := make([]sqlc.InsertCreatedParams, len(ages))
	for i, at := range ages {
		rows[i] = e.submit(t)
		if _, err := e.pool.Exec(
			t.Context(), `UPDATE swaps SET submitted_at = $2, updated_at = $2 WHERE id = $1`, rows[i].ID, at,
		); err != nil {
			t.Fatal(err)
		}
	}
	return rows
}

func (e *sweeperEnv) assertHintsAndLogs(t *testing.T, logs *testkit.Logs, rows ...sqlc.InsertCreatedParams) {
	t.Helper()
	for _, row := range rows {
		key := "cabal." + row.CabalID.String() + ".swap_updated"
		if !e.hint.has(key, row.ID) {
			t.Fatalf("hint for %s = keys %v payloads %q", row.ID, e.hint.keys, e.hint.payload)
		}
	}
	lines := string(logs.Bytes())
	if strings.Count(lines, `"msg":"trading.swap.finished"`) != len(rows) ||
		!strings.Contains(lines, `"before_status":"created"`) ||
		!strings.Contains(lines, `"before_status":"submitted"`) {
		t.Fatalf("recovery logs = %s", lines)
	}
}

func (e *sweeperEnv) assertRecoveredEvents(t *testing.T, created, finalized uuid.UUID, outMint chain.SolanaAddress) {
	t.Helper()
	createdEvent, ok := e.terminalEvent(t, created).(events.TradeFailed)
	if !ok || createdEvent.SourceBatchSize != 3 {
		t.Fatalf("created recovery event = %+v", createdEvent)
	}
	confirmedEvent, ok := e.terminalEvent(t, finalized).(events.TradeConfirmed)
	if !ok || confirmedEvent.OutMint != outMint {
		t.Fatalf("confirmed recovery event = %+v", confirmedEvent)
	}
}

func TestSwapSweeper_hasTheConfiguredNameAndInterval(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	p := e.poller()
	if p.Name() != "trading.swap_sweeper" || p.Interval() != 30*time.Second {
		t.Fatalf("poller = %s every %s", p.Name(), p.Interval())
	}
	if report, err := p.Tick(tickContext(t)); err != nil || report.Scanned != 0 ||
		report.Changed != 0 {
		t.Fatalf("empty Tick = %+v, %v", report, err)
	}
}

func TestSwapSweeper_reportsAStaleSubmittedQueryFailure(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.poller().staleSubmitted(ctx, e.clk.Now(), nil); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("staleSubmitted = %v", err)
	}
}

func TestSwapSweeper_keepsFailuresAndRacesConvergent(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	failedRow, raced := e.submit(t), e.submit(t)
	req, err := sweepRequest(
		raced.CabalID,
		raced.SourceKind,
		raced.SourceID,
		raced.Action,
		raced.Symbol,
		raced.InMint,
		raced.InAmount,
		raced.SourceBatchSize,
	)
	if err != nil {
		t.Fatal(err)
	}
	p := e.poller()
	e.read.statuses = []SigStatus{{State: SigFinalized, Failed: true}, {State: SigFinalized, Failed: true}}
	e.read.onStatus = func() {
		moved, moveErr := p.move(
			tickContext(t), e.clk.Now(), req, failed(req, raced.ID, domain.FailureJupiterFailed, ""),
		)
		if !moved || moveErr != nil {
			t.Fatalf("racing FinishFailed = %t, %v", moved, moveErr)
		}
	}
	e.advance()
	report, err := p.Tick(tickContext(t))
	if err != nil || report.Scanned != 2 || report.Changed != 1 ||
		e.status(t, failedRow.ID) != "failed" || e.terminalEvents(t, failedRow.ID) != 1 ||
		e.status(t, raced.ID) != "failed" || e.terminalEvents(t, raced.ID) != 1 || e.hint.sent != 2 {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
}

func TestSwapSweeper_createdRaceLeavesOneTerminalEventAndNoSweepHint(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.insert(t)
	e.advance()
	rows, err := sqlc.New(e.pool).ListStaleCreated(tickContext(t), sqlc.ListStaleCreatedParams{
		OlderThan: e.clk.Now().Add(-SwapSweepAge), MaxRows: SwapSweepBatch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != row.ID {
		t.Fatalf("stale created = %+v", rows)
	}
	p := e.poller()
	req, err := sweepRequest(
		row.CabalID,
		row.SourceKind,
		row.SourceID,
		row.Action,
		row.Symbol,
		row.InMint,
		row.InAmount,
		row.SourceBatchSize,
	)
	if err != nil {
		t.Fatal(err)
	}
	if moved, moveErr := p.move(
		tickContext(t), e.clk.Now(), req, failed(req, row.ID, domain.FailureNeverSubmitted, ""),
	); !moved || moveErr != nil {
		t.Fatalf("racing FinishFailed = %t, %v", moved, moveErr)
	}
	report, resolveErr := p.resolveCreated(
		tickContext(t), e.clk.Now(), poller.Report{Scanned: len(rows)}, nil, rows,
	)
	if resolveErr != nil || report.Changed != 0 || e.status(t, row.ID) != "failed" ||
		e.terminalEvents(t, row.ID) != 1 || e.hint.sent != 1 {
		t.Fatalf("resolveCreated race = %+v, %v", report, resolveErr)
	}
}

func TestSwapSweeper_createdRowsStillFailWhenStatusRPCIsUnavailable(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	created, submitted := e.insert(t), e.submit(t)
	e.read.statusErr = errs.New(errs.CodeRPCUnavailable, "test")
	e.advance()
	report, err := e.poller().Tick(tickContext(t))
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || report.Scanned != 2 || report.Changed != 1 ||
		e.status(t, created.ID) != "failed" || e.terminalEvents(t, created.ID) != 1 ||
		e.status(t, submitted.ID) != "submitted" || e.terminalEvents(t, submitted.ID) != 0 {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
}

func TestSwapSweeper_rotatesCreatedRowsThatCannotBeRecovered(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.insert(t)
	e.advance()
	bad := sqlc.ListStaleCreatedRow{
		ID: row.ID, CabalID: row.CabalID, SourceKind: "bad", SourceID: row.SourceID, Action: row.Action,
		Symbol: row.Symbol, InMint: row.InMint, InAmount: row.InAmount, SourceBatchSize: row.SourceBatchSize,
	}
	if _, err := e.poller().failCreated(tickContext(t), e.clk.Now(), bad); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("failCreated = %v", err)
	}
	if err := e.poller().rotateCreated(tickContext(t), row.ID, e.clk.Now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := e.poller().rotateCreated(ctx, row.ID, e.clk.Now()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("rotateCreated = %v", err)
	}
	var updated time.Time
	const updatedAt = `SELECT updated_at FROM swaps WHERE id = $1`
	if err := e.pool.QueryRow(t.Context(), updatedAt, row.ID).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Equal(e.clk.Now()) {
		t.Fatalf("updated_at = %s; want %s", updated, e.clk.Now())
	}
}

func TestSwapSweeper_reportsReaderFailuresWithoutChangingTheRow(t *testing.T) {
	t.Parallel()
	for name, configure := range map[string]func(*sweeperReader){
		"rpc unavailable": func(r *sweeperReader) { r.statusErr = errs.New(errs.CodeRPCUnavailable, "test") },
		"bad statuses":    func(r *sweeperReader) { r.statuses = nil },
		"inbound unavailable": func(r *sweeperReader) {
			r.statuses, r.inboundErr = []SigStatus{{State: SigFinalized}}, errs.New(errs.CodeRPCUnavailable, "test")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			row := e.submit(t)
			configure(e.read)
			e.advance()
			if _, err := e.poller().Tick(tickContext(t)); errs.CodeOf(err) == "" ||
				e.status(t, row.ID) != "submitted" {
				t.Fatalf("Tick = %v, status %s", err, e.status(t, row.ID))
			}
		})
	}
}

func TestSwapSweeper_keepsANotFoundSignatureWhileItsBlockhashIsValid(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	e.read.statuses, e.read.valid = []SigStatus{{State: SigNotFound}}, true
	e.advance()
	report, err := e.poller().Tick(tickContext(t))
	if err != nil || report.Scanned != 1 || report.Changed != 0 ||
		e.status(t, row.ID) != "submitted" {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
}

func TestSwapSweeper_rechecksAnExpiredBlockhashBeforeFailing(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	e.read.statuses = []SigStatus{{State: SigNotFound}}
	e.read.followups = [][]SigStatus{{{State: SigFinalized}}}
	e.advance()
	if report, err := e.poller().Tick(tickContext(t)); err != nil || report.Changed != 1 ||
		e.status(t, row.ID) != "confirmed" {
		t.Fatalf("Tick = %+v, %v; status = %s", report, err, e.status(t, row.ID))
	}
}

func TestSwapSweeper_reportsExpiryRecheckAndRotationFailures(t *testing.T) {
	t.Parallel()
	for name, configure := range map[string]func(*sweeperEnv, context.CancelFunc){
		"expiry recheck": func(e *sweeperEnv, _ context.CancelFunc) {
			e.read.statuses = []SigStatus{{State: SigNotFound}}
			e.read.followupErr = errs.New(errs.CodeRPCUnavailable, "test")
		},
		"expiry status count": func(e *sweeperEnv, _ context.CancelFunc) {
			e.read.statuses, e.read.followups = []SigStatus{{State: SigNotFound}}, [][]SigStatus{{}}
		},
		"rotation": func(e *sweeperEnv, cancel context.CancelFunc) {
			e.read.statuses, e.read.onStatus = []SigStatus{{State: SigProcessing}}, cancel
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			e.submit(t)
			e.advance()
			ctx, cancel := context.WithCancel(t.Context())
			configure(e, cancel)
			tickCtx := observability.WithActor(ctx, "system:poller.trading.swap_sweeper")
			if _, err := e.poller().Tick(tickCtx); err == nil {
				t.Fatalf("%s Tick succeeded", name)
			}
		})
	}
}

func TestSwapSweeper_rejectsMalformedRecoveryRows(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	if _, err := e.pool.Exec(t.Context(), `UPDATE swaps SET tx_signature = NULL WHERE id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	e.advance()
	if _, err := e.poller().Tick(tickContext(t)); errs.CodeOf(err) != errs.CodeDecodeFailed ||
		e.status(t, row.ID) != "submitted" {
		t.Fatalf("Tick = %v, status %s", err, e.status(t, row.ID))
	}
}

func TestSwapSweeper_helpersRejectInvalidRows(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	for _, tc := range []struct {
		kind, action string
		amount       int64
		batch        int32
	}{{"bad", "buy", 1, 1}, {"proposal", "bad", 1, 1}, {"proposal", "buy", -1, 1}, {"proposal", "buy", 1, 0}} {
		if _, err := sweepRequest(
			e.ids.NewV7(), tc.kind, e.ids.NewV7(), tc.action, "AAPLx", "usdc", tc.amount, tc.batch,
		); errs.CodeOf(err) == "" {
			t.Fatalf("sweepRequest %+v succeeded", tc)
		}
	}
	p := e.poller()
	row := sqlc.ListStaleSubmittedRow{
		ID: e.ids.NewV7(), CabalID: e.ids.NewV7(), SourceKind: "proposal", SourceID: e.ids.NewV7(),
		Action: "buy", Symbol: "AAPLx", InMint: "usdc", InAmount: 1, SourceBatchSize: 1, OutDecimals: 300,
	}
	if _, err := p.resolveSubmitted(
		t.Context(), time.Time{}, row, SigStatus{State: SigFinalized},
	); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("invalid decimals = %v", err)
	}
	if _, err := p.resolveSubmitted(
		t.Context(), time.Time{}, row, SigStatus{State: 99},
	); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("invalid state = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.poller().Tick(ctx); err == nil {
		t.Fatal("Tick on cancelled context succeeded")
	}
}

func TestSwapSweeper_coversRejectedRowsAndTransactionFailures(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	bad := sqlc.ListStaleCreatedRow{
		ID: e.ids.NewV7(), CabalID: e.ids.NewV7(), SourceKind: "bad", SourceID: e.ids.NewV7(),
	}
	if moved, err := e.poller().failCreated(tickContext(t), e.clk.Now(), bad); moved ||
		errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("failCreated = %t, %v", moved, err)
	}
	if _, err := e.poller().resolveSubmitted(tickContext(t), e.clk.Now(), sqlc.ListStaleSubmittedRow{
		CabalID: e.ids.NewV7(), SourceKind: "bad", SourceID: e.ids.NewV7(), SourceBatchSize: 1,
	}, SigStatus{State: SigProcessing}); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("invalid submitted row = %v", err)
	}
	row := e.insert(t)
	req, err := sweepRequest(
		row.CabalID,
		row.SourceKind,
		row.SourceID,
		row.Action,
		row.Symbol,
		row.InMint,
		row.InAmount,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	step := failed(req, e.ids.NewV7(), "never_submitted", "")
	moved, err := e.poller().move(tickContext(t), e.clk.Now(), req, step)
	if moved || err != nil {
		t.Fatalf("zero-row move = %t, %v", moved, err)
	}
	if moved, err := e.poller().failCreated(t.Context(), e.clk.Now(), sqlc.ListStaleCreatedRow{
		ID: row.ID, CabalID: row.CabalID, SourceKind: row.SourceKind, SourceID: row.SourceID, Action: row.Action,
		Symbol: row.Symbol, InMint: row.InMint, SourceBatchSize: row.SourceBatchSize, InAmount: row.InAmount,
	}); moved || err == nil || e.status(t, row.ID) != "created" {
		t.Fatalf("event failure = %t, %v", moved, err)
	}
	testSweepResolverFailures(t, e)
}

func testSweepResolverFailures(t *testing.T, e *sweeperEnv) {
	t.Helper()
	for _, tc := range []struct {
		status SigStatus
		row    sqlc.ListStaleSubmittedRow
	}{
		{SigStatus{State: SigNotFound}, sqlc.ListStaleSubmittedRow{CabalID: e.ids.NewV7(), SourceKind: "proposal", SourceID: e.ids.NewV7(), Action: "buy", InMint: "usdc", InAmount: 1, SourceBatchSize: 1, SignedTx: []byte{1}}},
		{SigStatus{State: SigFinalized}, sqlc.ListStaleSubmittedRow{CabalID: e.ids.NewV7(), SourceKind: "proposal", SourceID: e.ids.NewV7(), Action: "buy", InMint: "usdc", InAmount: 1, SourceBatchSize: 1, OutDecimals: 8, SignedTx: []byte{1}, ID: e.ids.NewV7()}},
	} {
		e.read.validErr, e.read.inboundErr = errs.New(errs.CodeRPCUnavailable, "test"), nil
		if tc.status.State == SigFinalized {
			e.read.validErr, e.read.inboundErr, e.read.inbound = nil, nil, money.NewBaseUnits(
				math.MaxUint64,
				8,
			)
		}
		if _, err := e.poller().resolveSubmitted(tickContext(t), e.clk.Now(), tc.row, tc.status); errs.CodeOf(
			err,
		) == "" {
			t.Fatalf("resolveSubmitted = %v", err)
		}
	}
}
