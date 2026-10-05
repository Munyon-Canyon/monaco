package funding_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stubStatuses struct {
	statuses []solana.Status
	err      error
	asked    [][]chain.Signature
	before   func()
}

func (s *stubStatuses) SignatureStatuses(_ context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	s.asked = append(s.asked, sigs)
	if s.before != nil {
		s.before()
	}
	return s.statuses, s.err
}

const withdrawalUnsentAge = 2 * time.Minute

func (f *withdrawFixture) poller(statuses *stubStatuses) *app.WithdrawalPoller {
	return app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: f.uow, Reads: f.pool, Clock: f.clock, Chain: statuses,
		Transfers: func() (app.Transfers, error) { return f.transfers, nil }, Hints: f.hints,
		UnsentAge: withdrawalUnsentAge,
	})
}

func (f *withdrawFixture) tick(t *testing.T, p *app.WithdrawalPoller) (int, error) {
	t.Helper()
	report, err := p.Tick(observability.WithActor(t.Context(), "system:poller.funding.withdrawals"))
	return report.Changed, err
}

func (f *withdrawFixture) payloads(t *testing.T, typ events.Type) [][]byte {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1`, typ)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

func (f *withdrawFixture) assertInFlight(t *testing.T, want string) {
	t.Helper()
	inFlight, err := (app.WithdrawalOutflows{Reads: f.pool}).InFlightMicros(t.Context(), f.user.ID)
	if err != nil || inFlight.String() != want {
		t.Fatalf("in flight = %v, %v, want %s", inFlight, err, want)
	}
}

func submitted(t *testing.T) (*withdrawFixture, app.WithdrawResult) {
	t.Helper()
	f := newWithdrawFixture(t, 5_000_000)
	got, err := f.withdraw(t)
	if err != nil {
		t.Fatal(err)
	}
	f.hints.keys = nil
	return f, got
}

func status(state solana.State, failed bool, height uint64) *stubStatuses {
	return &stubStatuses{statuses: []solana.Status{
		{Signature: withdrawSig, State: state, Failed: failed, BlockHeight: height},
	}}
}

func TestWithdrawalPoller_FinalizedConfirmsOnce(t *testing.T) {
	t.Parallel()
	f, got := submitted(t)
	p := f.poller(status(solana.StateFinalized, false, 800))
	if changed, err := f.tick(t, p); err != nil || changed != 1 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	if changed, err := f.tick(t, p); err != nil || changed != 0 {
		t.Fatalf("second Tick = %d, %v", changed, err)
	}
	if rows := f.rows(t); rows[0].status != "confirmed" {
		t.Fatalf("rows = %+v", rows)
	}
	confirmed := f.payloads(t, events.TypeWithdrawalConfirmed)
	if len(confirmed) != 1 {
		t.Fatalf("withdrawal.confirmed = %d, want 1", len(confirmed))
	}
	var e events.WithdrawalConfirmed
	if err := json.Unmarshal(confirmed[0], &e); err != nil {
		t.Fatal(err)
	}
	want := events.WithdrawalConfirmed{
		V: 1, WithdrawalID: got.ID, UserID: f.user.ID.UUID(), AmountMicros: money.MicrosFromUint64(withdrawMicros),
		ToAddress: withdrawTo, TxSignature: withdrawSig,
	}
	if e != want {
		t.Fatalf("event = %+v, want %+v", e, want)
	}
	if len(f.hints.keys) != 1 || f.hints.keys[0] != events.UserBalanceChangedHint(f.user.ID) {
		t.Fatalf("hints = %v", f.hints.keys)
	}
	f.assertInFlight(t, "0")
}

func TestWithdrawalPoller_FailedOrExpiredFailsTheRow(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		statuses *stubStatuses
		code     string
	}{
		"chain error":       {statuses: status(solana.StateFinalized, true, 800), code: "transaction_failed"},
		"blockhash expired": {statuses: status(solana.StateNotFound, false, 901), code: "blockhash_expired"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, got := submitted(t)
			if changed, err := f.tick(t, f.poller(c.statuses)); err != nil || changed != 1 {
				t.Fatalf("Tick = %d, %v", changed, err)
			}
			f.assertFailed(t, got, c.code)
		})
	}
}

func (f *withdrawFixture) assertFailed(t *testing.T, got app.WithdrawResult, code string) {
	t.Helper()
	if rows := f.rows(t); rows[0].status != "failed" || rows[0].failCode != code {
		t.Fatalf("rows = %+v", rows)
	}
	failed := f.payloads(t, events.TypeWithdrawalFailed)
	var e events.WithdrawalFailed
	if len(failed) != 1 || json.Unmarshal(failed[0], &e) != nil {
		t.Fatalf("withdrawal.failed = %d", len(failed))
	}
	want := events.WithdrawalFailed{
		V: 1, WithdrawalID: got.ID, UserID: f.user.ID.UUID(),
		AmountMicros: money.MicrosFromUint64(withdrawMicros), Code: code,
	}
	if e != want || len(f.payloads(t, events.TypeWithdrawalConfirmed)) != 0 {
		t.Fatalf("event = %+v, want %+v", e, want)
	}
	f.assertInFlight(t, "0")
}

func TestWithdrawalPoller_NotFoundBeforeExpiryRebroadcasts(t *testing.T) {
	t.Parallel()
	f, _ := submitted(t)
	if changed, err := f.tick(t, f.poller(status(solana.StateNotFound, false, 900))); err != nil || changed != 0 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	if len(f.transfers.sent) != 2 || f.transfers.sent[1].Signature != withdrawSig ||
		string(f.transfers.sent[1].Bytes) != "\x01\x02\x03" || f.transfers.sent[1].LastValidBlockHeight != 900 {
		t.Fatalf("sent = %+v", f.transfers.sent)
	}
	f.transfers.sendErr = errs.New(errs.CodeRPCUnavailable, "test")
	if _, err := f.tick(t, f.poller(status(solana.StateNotFound, false, 900))); err != nil {
		t.Fatalf("Tick with a failed rebroadcast = %v, want nil", err)
	}
	if rows := f.rows(t); rows[0].status != "submitted" || len(f.hints.keys) != 0 {
		t.Fatalf("rows = %+v hints = %v", rows, f.hints.keys)
	}
	f.assertInFlight(t, "2000000")
}

func TestWithdrawalPoller_RebroadcastNeedsTransfers(t *testing.T) {
	t.Parallel()
	f, _ := submitted(t)
	boom := errs.New(errs.CodePrivyUnavailable, "test")
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: f.uow, Reads: f.pool, Clock: f.clock, Chain: status(solana.StateNotFound, false, 900),
		Transfers: func() (app.Transfers, error) { return nil, boom }, Hints: f.hints, UnsentAge: withdrawalUnsentAge,
	})
	if _, err := f.tick(t, p); !errors.Is(err, boom) {
		t.Fatalf("Tick = %v, want %v", err, boom)
	}
}

func TestWithdrawalPoller_ProcessingWaits(t *testing.T) {
	t.Parallel()
	f, _ := submitted(t)
	if changed, err := f.tick(t, f.poller(status(solana.StateProcessing, false, 950))); err != nil || changed != 0 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	if rows := f.rows(t); rows[0].status != "submitted" || len(f.transfers.sent) != 1 {
		t.Fatalf("rows = %+v sent = %d", rows, len(f.transfers.sent))
	}
}

func TestWithdrawalPoller_StaleCreatedFails(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	f.transfers.buildErr = errs.New(errs.CodeInternal, "test")
	f.transfers.beforeSign = func() {
		if _, err := f.pool.Exec(context.Background(), `UPDATE withdrawals SET status = 'created'`); err != nil {
			t.Error(err)
		}
	}
	if _, err := f.withdraw(t); err == nil {
		t.Fatal("withdraw err = nil")
	}
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE withdrawals SET status = 'created', fail_code = NULL, completed_at = NULL`); err != nil {
		t.Fatal(err)
	}
	statuses := &stubStatuses{}
	p := f.poller(statuses)
	f.clock.Advance(withdrawalUnsentAge - time.Second)
	if changed, err := f.tick(t, p); err != nil || changed != 0 {
		t.Fatalf("Tick before 2 min = %d, %v", changed, err)
	}
	f.clock.Advance(2 * time.Second)
	f.hints.keys = nil
	if changed, err := f.tick(t, p); err != nil || changed != 1 {
		t.Fatalf("Tick after 2 min = %d, %v", changed, err)
	}
	if len(statuses.asked) != 0 || len(f.hints.keys) != 1 {
		t.Fatalf("asked = %v hints = %v", statuses.asked, f.hints.keys)
	}
	f.assertNotSent(t)
}

func (f *withdrawFixture) assertNotSent(t *testing.T) {
	t.Helper()
	if rows := f.rows(t); rows[0].status != "failed" || rows[0].failCode != "withdrawal_not_sent" {
		t.Fatalf("rows = %+v", rows)
	}
	if len(f.payloads(t, events.TypeWithdrawalFailed)) != 0 || len(f.payloads(t, events.TypeWithdrawalSubmitted)) != 0 {
		t.Fatal("an unsent withdrawal appended withdrawal.failed or withdrawal.submitted")
	}
	f.assertInFlight(t, "0")
}

func TestWithdrawalPoller_ChainReadErrors(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeRPCUnavailable, "test")
	for name, c := range map[string]struct {
		statuses *stubStatuses
		code     errs.Code
	}{
		"rpc down":    {statuses: &stubStatuses{err: boom}, code: errs.CodeRPCUnavailable},
		"short reply": {statuses: &stubStatuses{}, code: errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, _ := submitted(t)
			if _, err := f.tick(t, f.poller(c.statuses)); errs.CodeOf(err) != c.code {
				t.Fatalf("Tick = %v, want %s", err, c.code)
			}
			if rows := f.rows(t); rows[0].status != "submitted" {
				t.Fatalf("rows = %+v", rows)
			}
		})
	}
}

func TestWithdrawalPoller_BadRowsReportDecodeFailed(t *testing.T) {
	t.Parallel()
	for name, sql := range map[string]string{
		"submitted amount": `UPDATE withdrawals SET amount_micros = 99999999999999999999`,
		"submitted height": `UPDATE withdrawals SET last_valid_block_height = -1`,
		"created amount": `UPDATE withdrawals SET amount_micros = 99999999999999999999, status = 'created',
			signed_tx = NULL, tx_signature = NULL, last_valid_block_height = NULL, submitted_at = NULL,
			created_at = created_at - interval '1 hour'`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, _ := submitted(t)
			if _, err := f.pool.Exec(t.Context(), sql); err != nil {
				t.Fatal(err)
			}
			if _, err := f.tick(t, f.poller(status(solana.StateFinalized, false, 800))); errs.CodeOf(err) !=
				errs.CodeDecodeFailed {
				t.Fatalf("Tick = %v, want decode_failed", err)
			}
		})
	}
}

func closedPool(t *testing.T, open *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	closed, err := pgxpool.NewWithConfig(t.Context(), open.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	return closed
}

func TestWithdrawalPoller_ListErrors(t *testing.T) {
	t.Parallel()
	f, _ := submitted(t)
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: f.uow, Reads: closedPool(t, f.pool), Clock: f.clock,
		Chain: status(solana.StateFinalized, false, 800), Hints: f.hints, UnsentAge: withdrawalUnsentAge,
	})
	if _, err := f.tick(t, p); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Tick = %v, want internal", err)
	}
}

func TestWithdrawalPoller_CommitErrorMovesNothing(t *testing.T) {
	t.Parallel()
	f, _ := submitted(t)
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: db.New(closedPool(t, f.pool), testkit.NewIDs(6), f.clock), Reads: f.pool, Clock: f.clock,
		Chain: status(solana.StateFinalized, false, 800), Hints: f.hints, UnsentAge: withdrawalUnsentAge,
	})
	if changed, err := f.tick(t, p); err == nil || changed != 0 {
		t.Fatalf("Tick = %d, %v, want an error", changed, err)
	}
	if rows := f.rows(t); rows[0].status != "submitted" || len(f.hints.keys) != 0 {
		t.Fatalf("rows = %+v hints = %v", rows, f.hints.keys)
	}
}

func TestWithdrawalPoller_Identity(t *testing.T) {
	t.Parallel()
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{})
	if p.Name() != "funding.withdrawals" || p.Interval() != 5*time.Second {
		t.Fatalf("poller = %s every %s", p.Name(), p.Interval())
	}
}

func TestWithdrawalPoller_RowMovedElsewhereIsLeftAlone(t *testing.T) {
	t.Parallel()
	f, _ := submitted(t)
	statuses := status(solana.StateFinalized, false, 800)
	statuses.before = func() {
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE withdrawals SET status = 'failed', fail_code = 'blockhash_expired', completed_at = now()`,
		); err != nil {
			t.Error(err)
		}
	}
	if changed, err := f.tick(t, f.poller(statuses)); err != nil || changed != 0 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	if len(f.payloads(t, events.TypeWithdrawalConfirmed)) != 0 || len(f.hints.keys) != 0 {
		t.Fatalf("hints = %v", f.hints.keys)
	}
}
