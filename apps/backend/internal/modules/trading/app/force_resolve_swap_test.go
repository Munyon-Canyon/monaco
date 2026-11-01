package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (e *sweeperEnv) force() *ForceResolveSwapHandler {
	if e.read.statuses == nil {
		e.read.statuses = []SigStatus{{State: SigFinalized, Failed: true}}
	}
	return NewForceResolveSwapHandler(e.uow, e.pool, e.clk, e.ids, e.hint, e.read, nil)
}

func (e *sweeperEnv) landed() {
	e.read.statuses = []SigStatus{{State: SigFinalized}}
}

func forceCommand(signature chain.Signature, to domain.Status) ForceResolveSwap {
	return ForceResolveSwap{
		Signature: signature,
		To:        to,
		OutAmount: money.NewBaseUnits(104_000_000, 8),
		Reason:    "chain explorer verified it",
		Actor:     "operator:ops", IdempotencyKey: "force-resolve:" + string(signature) + ":" + string(to),
	}
}

func TestForceResolve_Confirmed(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	e.landed()
	row := e.submit(t)
	cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusConfirmed)
	got, err := e.force().Handle(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusConfirmed {
		t.Fatalf("status = %s", got.Status)
	}
	if got.OutAmount != cmd.OutAmount.Uint64() {
		t.Fatalf("out amount = %d", got.OutAmount)
	}
	event, ok := e.terminalEvent(t, row.ID).(events.TradeConfirmed)
	if !ok {
		t.Fatalf("event = %#v", event)
	}
	if event.OutAmount != cmd.OutAmount.Uint64() || event.OutMint != "aaplx" || event.TxSignature != cmd.Signature {
		t.Fatalf("event = %#v", event)
	}
	if e.terminalEvents(t, row.ID) != 1 {
		t.Fatalf("terminal events = %d", e.terminalEvents(t, row.ID))
	}
	if !e.hint.has("cabal."+row.CabalID.String()+".swap_updated", row.ID) {
		t.Fatalf("hints = %#v", e.hint.keys)
	}
	e.assertForceResolveActor(t, row.ID)
}

func (e *sweeperEnv) assertForceResolveActor(t *testing.T, id uuid.UUID) {
	t.Helper()
	var actorType, actorID string
	if err := e.pool.QueryRow(t.Context(), `SELECT actor_type, actor_id FROM events WHERE aggregate_id = $1`, id).
		Scan(&actorType, &actorID); err != nil {
		t.Fatal(err)
	}
	if actorType != "system" || actorID != "operator:ops" {
		t.Fatalf("event actor = %q:%q", actorType, actorID)
	}
}

func TestForceResolve_Failed(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed)
	got, err := e.force().Handle(t.Context(), cmd)
	if err != nil || got.Status != domain.StatusFailed || got.FailureCode != domain.FailureForceResolved {
		t.Fatalf("Handle = %+v, %v", got, err)
	}
	event, ok := e.terminalEvent(t, row.ID).(events.TradeFailed)
	if !ok || event.FailureCode != string(domain.FailureForceResolved) || e.terminalEvents(t, row.ID) != 1 {
		t.Fatalf("event = %#v, terminal events = %d", event, e.terminalEvents(t, row.ID))
	}
}

func TestForceResolve_AuditRow(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	e.landed()
	row := e.submit(t)
	cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusConfirmed)
	if _, err := e.force().Handle(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	var action, actor, reason, detail string
	const audit = `SELECT action, actor, reason, detail::text FROM swap_operator_actions WHERE swap_id = $1`
	if err := e.pool.QueryRow(t.Context(), audit, row.ID).Scan(&action, &actor, &reason, &detail); err != nil {
		t.Fatal(err)
	}
	wantDetail := []string{
		`"chain_state": "landed"`, `"chain_out_amount": 104000000`, `"override": false`, `"to": "confirmed"`,
	}
	if action != "force_resolve" || actor != "operator:ops" || reason != cmd.Reason ||
		slices.ContainsFunc(wantDetail, func(want string) bool { return !strings.Contains(detail, want) }) {
		t.Fatalf("audit = %q %q %q %s", action, actor, reason, detail)
	}
}

func (e *sweeperEnv) auditRows(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	const q = `SELECT count(*) FROM swap_operator_actions WHERE swap_id = $1`
	if err := e.pool.QueryRow(t.Context(), q, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type chainCase struct {
	to       domain.Status
	statuses []SigStatus
	valid    bool
	amount   uint64
	want     string
}

func (e *sweeperEnv) detail(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var detail string
	const q = `SELECT detail::text FROM swap_operator_actions WHERE swap_id = $1`
	if err := e.pool.QueryRow(t.Context(), q, id).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	return detail
}

func (e *sweeperEnv) assertUntouched(t *testing.T, id uuid.UUID) {
	t.Helper()
	if e.status(t, id) != "submitted" || e.terminalEvents(t, id) != 0 || e.auditRows(t, id) != 0 {
		t.Fatalf("status = %s, terminal events = %d, audit rows = %d",
			e.status(t, id), e.terminalEvents(t, id), e.auditRows(t, id))
	}
}

func TestForceResolve_RefusesWhatTheChainContradicts(t *testing.T) {
	t.Parallel()
	landed, failedOnChain := []SigStatus{{State: SigFinalized}}, []SigStatus{{State: SigFinalized, Failed: true}}
	processing, missing := []SigStatus{{State: SigProcessing}}, []SigStatus{{State: SigNotFound}}
	for name, tc := range map[string]chainCase{
		"failed on a landed swap":      {domain.StatusFailed, landed, false, 0, "landed successfully"},
		"failed while processing":      {domain.StatusFailed, processing, false, 0, "still processing"},
		"failed with a live blockhash": {domain.StatusFailed, missing, true, 0, "may yet land"},
		"confirmed on a failed swap":   {domain.StatusConfirmed, failedOnChain, false, 1, "failed on chain"},
		"confirmed while processing":   {domain.StatusConfirmed, processing, false, 1, "still processing"},
		"confirmed when not found":     {domain.StatusConfirmed, missing, false, 1, "expired"},
		"confirmed with a wrong amount": {
			domain.StatusConfirmed, landed, false, 104_000_001, "received 104000000, not 104000001",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func (tc chainCase) run(t *testing.T) {
	t.Helper()
	e := newSweeperEnv(t)
	e.read.statuses, e.read.valid = tc.statuses, tc.valid
	row := e.submit(t)
	cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), tc.to)
	if tc.amount != 0 {
		cmd.OutAmount = money.NewBaseUnits(tc.amount, 8)
	}
	_, err := e.force().Handle(t.Context(), cmd)
	if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil || !strings.Contains(err.Error(), tc.want) ||
		!strings.Contains(err.Error(), "--override") {
		t.Fatalf("Handle = %v, want invalid_input mentioning %q", err, tc.want)
	}
	e.assertUntouched(t, row.ID)
	cmd.Override = true
	got, err := e.force().Handle(t.Context(), cmd)
	if err != nil || got.Status != tc.to || e.auditRows(t, row.ID) != 1 {
		t.Fatalf("override Handle = %+v, %v, audit rows = %d", got, err, e.auditRows(t, row.ID))
	}
	if detail := e.detail(t, row.ID); !strings.Contains(detail, `"override": true`) ||
		!strings.Contains(detail, `"contradiction"`) {
		t.Fatalf("detail = %s", detail)
	}
}

func TestForceResolve_ChainReadFailures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	landed, missing := []SigStatus{{State: SigFinalized}}, []SigStatus{{State: SigNotFound}}
	for name, set := range map[string]func(r *sweeperReader){
		"statuses":    func(r *sweeperReader) { r.statusErr = boom },
		"wrong count": func(r *sweeperReader) { r.statuses = []SigStatus{{}, {}} },
		"blockhash":   func(r *sweeperReader) { r.statuses, r.validErr = missing, boom },
		"inbound":     func(r *sweeperReader) { r.statuses, r.inboundErr = landed, boom },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			set(e.read)
			row := e.submit(t)
			cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusConfirmed)
			if name == "blockhash" {
				cmd.To = domain.StatusFailed
			}
			if _, err := e.force().Handle(t.Context(), cmd); err == nil {
				t.Fatal("Handle succeeded with an unreadable chain")
			}
			e.assertUntouched(t, row.ID)
			cmd.Override = true
			if _, err := e.force().Handle(t.Context(), cmd); err != nil || e.auditRows(t, row.ID) != 1 {
				t.Fatalf("override Handle = %v, audit rows = %d", err, e.auditRows(t, row.ID))
			}
			if detail := e.detail(t, row.ID); !strings.Contains(detail, `"chain_state": "unknown"`) ||
				!strings.Contains(detail, `"unverified"`) {
				t.Fatalf("detail = %s", detail)
			}
		})
	}
}

func TestForceResolve_SwapVanishesAfterTheChainCheck(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	e.read.onStatus = func() {
		if _, err := e.pool.Exec(t.Context(), `DELETE FROM swaps WHERE id = $1`, row.ID); err != nil {
			t.Error(err)
		}
	}
	_, err := e.force().Handle(t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed))
	if errs.CodeOf(err) != errs.CodeSwapNotFound {
		t.Fatalf("Handle = %v, want swap_not_found", err)
	}
}

func TestForceResolve_OverrideStillChecksTheUnits(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusConfirmed)
	cmd.OutAmount, cmd.Override = money.NewBaseUnits(104_000_000, 6), true
	_, err := e.force().Handle(t.Context(), cmd)
	if errs.CodeOf(err) != errs.CodeInvalidInput || e.status(t, row.ID) != "submitted" || e.auditRows(t, row.ID) != 0 {
		t.Fatalf("Handle = %v, status = %s", err, e.status(t, row.ID))
	}
}

func TestForceResolve_RollsBackWhenTheAuditRowCannotBeWritten(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	const fn = `CREATE FUNCTION audit_blocker() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'audit refused'; END; $$`
	const trg = `CREATE TRIGGER audit_blocker BEFORE INSERT ON swap_operator_actions
FOR EACH ROW EXECUTE FUNCTION audit_blocker()`
	for _, stmt := range []string{fn, trg} {
		if _, err := e.pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	_, err := e.force().Handle(t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed))
	if err == nil || e.status(t, row.ID) != "submitted" || e.terminalEvents(t, row.ID) != 0 {
		t.Fatalf(
			"Handle = %v, status = %s, terminal events = %d",
			err,
			e.status(t, row.ID),
			e.terminalEvents(t, row.ID),
		)
	}
}

func TestForceResolve_RejectsAnActorThatIsNotAnOperator(t *testing.T) {
	t.Parallel()
	for _, actor := range []string{"system:monacoctl", "operator:"} {
		cmd := forceCommand("sig", domain.StatusFailed)
		cmd.Actor = actor
		if errs.CodeOf(cmd.validate()) != errs.CodeInvalidInput {
			t.Fatalf("validate accepted actor %q", actor)
		}
	}
}

func TestForceResolve_NotSubmitted(t *testing.T) {
	t.Parallel()
	for _, status := range []domain.Status{domain.StatusCreated, domain.StatusConfirmed, domain.StatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			row := e.submit(t)
			e.setStatus(t, row.ID, status)
			_, err := e.force().Handle(
				t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed),
			)
			if errs.CodeOf(err) != errs.CodeSwapNotStuck {
				t.Fatalf("Handle = %v, want swap_not_stuck", err)
			}
		})
	}
}

func TestForceResolve_ConfirmedWithoutAmount(t *testing.T) {
	t.Parallel()
	for name, amount := range map[string]money.BaseUnits{
		"missing":        {},
		"wrong decimals": money.NewBaseUnits(104_000_000, 6),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			e.landed()
			row := e.submit(t)
			cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusConfirmed)
			cmd.OutAmount = amount
			_, err := e.force().Handle(t.Context(), cmd)
			if errs.CodeOf(err) != errs.CodeInvalidInput || e.status(t, row.ID) != "submitted" {
				t.Fatalf("Handle = %v, status = %s", err, e.status(t, row.ID))
			}
		})
	}
}

func TestForceResolve_ConfirmedRejectsInvalidStoredDecimals(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	e.landed()
	row := e.submit(t)
	if _, err := e.pool.Exec(t.Context(), `UPDATE swaps SET out_decimals = -1 WHERE id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	_, err := e.force().Handle(
		t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusConfirmed),
	)
	if errs.CodeOf(err) != errs.CodeDecodeFailed || e.status(t, row.ID) != "submitted" {
		t.Fatalf("Handle = %v, status = %s", err, e.status(t, row.ID))
	}
}

func TestForceResolve_RaceWithSweeper(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	e.advance()
	e.read.statuses = []SigStatus{{State: SigFinalized}}
	started, release := make(chan struct{}), make(chan struct{})
	e.read.onStatus = func() {
		close(started)
		<-release
	}
	var group errgroup.Group
	var sweepErr, forceErr error
	group.Go(func() error {
		_, sweepErr = e.poller().Tick(observability.WithActor(t.Context(), "system:poller.trading.swap_sweeper"))
		return nil
	})
	<-started
	group.Go(func() error {
		reader := &sweeperReader{statuses: []SigStatus{{State: SigFinalized, Failed: true}}}
		forced := NewForceResolveSwapHandler(e.uow, e.pool, e.clk, e.ids, e.hint, reader, nil)
		_, forceErr = forced.Handle(
			t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed),
		)
		return nil
	})
	close(release)
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	if (sweepErr != nil && errs.CodeOf(sweepErr) != errs.CodeSwapNotStuck) ||
		(forceErr != nil && errs.CodeOf(forceErr) != errs.CodeSwapNotStuck) || e.terminalEvents(t, row.ID) != 1 {
		t.Fatalf("sweeper = %v, force = %v, terminal events = %d", sweepErr, forceErr, e.terminalEvents(t, row.ID))
	}
}

func TestForceResolve_ValidationAndLookupFailures(t *testing.T) {
	t.Parallel()
	valid := forceCommand("sig", domain.StatusFailed)
	for _, mutate := range []func(*ForceResolveSwap){
		func(c *ForceResolveSwap) { c.Signature = "" },
		func(c *ForceResolveSwap) { c.IdempotencyKey = "" },
		func(c *ForceResolveSwap) { c.Actor = "" },
		func(c *ForceResolveSwap) { c.Reason = "" },
		func(c *ForceResolveSwap) { c.Reason = strings.Repeat("x", 501) },
		func(c *ForceResolveSwap) { c.To = domain.StatusCreated },
		func(c *ForceResolveSwap) { c.To = domain.Status("unknown") },
		func(c *ForceResolveSwap) {
			c.To = domain.StatusConfirmed
			c.OutAmount = money.NewBaseUnits(^uint64(0), 8)
		},
	} {
		cmd := valid
		mutate(&cmd)
		if errs.CodeOf(cmd.validate()) != errs.CodeInvalidInput {
			t.Fatalf("validate(%+v) did not reject invalid input", cmd)
		}
	}
	e := newSweeperEnv(t)
	if _, err := e.force().Handle(t.Context(), valid); errs.CodeOf(err) != errs.CodeSwapNotFound {
		t.Fatalf("missing swap = %v", err)
	}
	if errs.CodeOf(forceResolveFound(errs.New(errs.CodeInternal, "test"))) != errs.CodeInternal {
		t.Fatal("unexpected lookup error was not internal")
	}
}

func TestForceResolve_RejectsAnInvalidStoredSwap(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	if _, err := e.pool.Exec(t.Context(), `ALTER TABLE swaps DROP CONSTRAINT swaps_source_kind_check`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(
		t.Context(),
		`UPDATE swaps SET source_kind = 'invalid' WHERE id = $1`,
		row.ID,
	); err != nil {
		t.Fatal(err)
	}
	_, err := e.force().Handle(
		t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed),
	)
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Handle = %v", err)
	}
}

func TestForceResolve_GuardedUpdateLosesTheRace(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	installForceResolveTrigger(t, e, "RETURN NULL;")
	_, err := e.force().Handle(t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed))
	if errs.CodeOf(err) != errs.CodeSwapNotStuck || e.status(t, row.ID) != "submitted" ||
		e.terminalEvents(t, row.ID) != 0 {
		t.Fatalf(
			"Handle = %v, status = %s, terminal events = %d",
			err,
			e.status(t, row.ID),
			e.terminalEvents(t, row.ID),
		)
	}
}

func TestForceResolve_RollsBackWriteEventAndReadFailures(t *testing.T) {
	t.Parallel()
	for name, prepare := range map[string]func(t *testing.T, e *sweeperEnv){
		"write": func(t *testing.T, e *sweeperEnv) {
			t.Helper()
			installForceResolveTrigger(t, e, "RAISE EXCEPTION 'write refused';")
		},
		"event": func(t *testing.T, e *sweeperEnv) {
			t.Helper()
			const fn = `CREATE FUNCTION events_blocker() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'event refused'; END; $$`
			if _, err := e.pool.Exec(t.Context(), fn); err != nil {
				t.Fatal(err)
			}
			const trg = `CREATE TRIGGER events_blocker BEFORE INSERT ON events
FOR EACH ROW EXECUTE FUNCTION events_blocker()`
			if _, err := e.pool.Exec(t.Context(), trg); err != nil {
				t.Fatal(err)
			}
		},
		"read": func(t *testing.T, e *sweeperEnv) {
			t.Helper()
			if _, err := e.pool.Exec(
				t.Context(),
				`ALTER VIEW swap_views RENAME TO swap_views_gone`,
			); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			row := e.submit(t)
			prepare(t, e)
			cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed)
			_, err := e.force().Handle(t.Context(), cmd)
			if errs.CodeOf(err) != errs.CodeInternal || e.status(t, row.ID) != "submitted" ||
				e.terminalEvents(t, row.ID) != 0 {
				t.Fatalf(
					"Handle = %v, status = %s, terminal events = %d",
					err,
					e.status(t, row.ID),
					e.terminalEvents(t, row.ID),
				)
			}
		})
	}
}

func (e *sweeperEnv) setStatus(t *testing.T, id uuid.UUID, status domain.Status) {
	t.Helper()
	switch status {
	case domain.StatusSubmitted:
		return
	case domain.StatusCreated:
		if _, err := e.pool.Exec(t.Context(), `UPDATE swaps SET status = 'created' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	case domain.StatusConfirmed:
		if n, err := e.q.FinishConfirmed(t.Context(), sqlc.FinishConfirmedParams{
			ID: id, OutAmount: 1, ConfirmedAt: e.clk.Now(),
		}); err != nil || n != 1 {
			t.Fatalf("FinishConfirmed = %d, %v", n, err)
		}
	case domain.StatusFailed:
		if n, err := e.q.FinishFailed(t.Context(), sqlc.FinishFailedParams{
			ID: id, FailureCode: string(domain.FailureForceResolved), FailedAt: e.clk.Now(),
		}); err != nil || n != 1 {
			t.Fatalf("FinishFailed = %d, %v", n, err)
		}
	default:
		t.Fatalf("unsupported status %q", status)
	}
}

func installForceResolveTrigger(t *testing.T, e *sweeperEnv, body string) {
	t.Helper()
	blocker := `CREATE FUNCTION force_resolve_blocker() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  ` + body + `
END;
$$`
	if _, err := e.pool.Exec(t.Context(), blocker); err != nil {
		t.Fatal(err)
	}
	const trigger = `CREATE TRIGGER force_resolve_blocker BEFORE UPDATE ON swaps
FOR EACH ROW EXECUTE FUNCTION force_resolve_blocker()`
	if _, err := e.pool.Exec(t.Context(), trigger); err != nil {
		t.Fatal(err)
	}
}
