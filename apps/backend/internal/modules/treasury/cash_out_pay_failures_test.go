package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func refuse(table, when string) string {
	name := "refuse_" + table
	return `CREATE FUNCTION ` + name + `() RETURNS trigger LANGUAGE plpgsql AS
		$$BEGIN RAISE EXCEPTION 'refused'; END$$;
		CREATE TRIGGER ` + name + ` BEFORE INSERT OR UPDATE ON ` + table + ` FOR EACH ROW WHEN (` + when +
		`) EXECUTE FUNCTION ` + name + `()`
}

func skip(table, when string) string {
	name := "skip_" + table
	return `CREATE FUNCTION ` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RETURN NULL; END$$;
		CREATE TRIGGER ` + name + ` BEFORE INSERT OR UPDATE ON ` + table + ` FOR EACH ROW WHEN (` + when +
		`) EXECUTE FUNCTION ` + name + `()`
}

const hugePayout = `ALTER TABLE cash_out_jobs DROP CONSTRAINT cash_out_jobs_payout_within_slice;
	UPDATE cash_out_jobs SET payout_micros = 18446744073709551615`

func TestCashOutPayout_aStoreFailureWhileResolvingMovesNoMoney(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		setup   string
		reading domain.PayoutReading
		code    errs.Code
	}{
		{"lapse refused", refuse("cash_out_payouts", "NEW.status = 'expired'"), lapsed(), errs.CodeInternal},
		{"completion refused", refuse("cash_out_jobs", "NEW.status = 'completed'"), landed(), errs.CodeInternal},
		{"completion lost", skip("cash_out_jobs", "NEW.status = 'completed'"), landed(), errs.CodeVersionConflict},
		{"confirmation refused", refuse("cash_out_payouts", "NEW.status = 'confirmed'"), landed(), errs.CodeInternal},
		{"confirmation lost", skip("cash_out_payouts", "NEW.status = 'confirmed'"), landed(), errs.CodeVersionConflict},
		{"treasury post refused", refuse("cabal_txns", "true"), landed(), errs.CodeInternal},
		{"member settle refused", refuse("user_txns", "NEW.status = 'settled'"), landed(), errs.CodeInternal},
		{"completed event refused", refuse("events", "NEW.type = 'cashout.completed'"), landed(), errs.CodeInternal},
		{"zero payout", `ALTER TABLE cash_out_jobs DROP CONSTRAINT cash_out_jobs_payout_micros_check;
			UPDATE cash_out_jobs SET payout_micros = 0`, landed(), errs.CodeInvalidInput},
		{"transfer headers lost", skip("cabal_txns", "NEW.status = 'settled'") + "; " +
			skip("user_txns", "NEW.status = 'settled'"), landed(), errs.CodeInternal},
		{"payout above int64", hugePayout, landed(), errs.CodeInvalidInput},
		{"failure refused", refuse("cash_out_jobs", "NEW.status = 'failed'"), refused(), errs.CodeInternal},
		{"rejection refused", refuse("cash_out_payouts", "NEW.status = 'failed'"), refused(), errs.CodeInternal},
		{"member txn fail refused", refuse("user_txns", "NEW.status = 'failed'"), refused(), errs.CodeInternal},
		{"member txn already gone", skip("user_txns", "NEW.status = 'failed'"), refused(), errs.CodeInternal},
		{"return post refused", refuse("user_txns", "NEW.status = 'settled'"), refused(), errs.CodeInternal},
		{"failed event refused", refuse("events", "NEW.type = 'cashout.failed'"), refused(), errs.CodeInternal},
		{"refund above int64", hugePayout, refused(), errs.CodeInvalidInput},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			r.seed(t, 1, domain.PayoutBroadcast)
			r.chain.set(seededSig(1), c.reading)
			if _, err := r.f.pool.Exec(t.Context(), c.setup); err != nil {
				t.Fatal(err)
			}
			err := r.advance(t, 0)
			if err == nil || (c.code != errs.CodeInternal && errs.CodeOf(err) != c.code) {
				t.Fatalf("Advance = %v, want %s", err, c.code)
			}
			r.wantJob(t, "paying", "")
			if r.events(t, events.TypeCashOutCompleted) != "0" || r.events(t, events.TypeCashOutFailed) != "0" {
				t.Fatal("an event outlived the rollback")
			}
		})
	}
}

func TestCashOutPayout_anUnreadableJobIsADecodeFailure(t *testing.T) {
	t.Parallel()
	attempt := `INSERT INTO cash_out_payouts (job_id, attempt, signature, signed_tx, last_valid_block_height, status,
		created_at) SELECT id, 1, 's', '\x01', 0, 'expired', now() FROM cash_out_jobs; `
	for name, setup := range map[string]string{
		"units": `UPDATE cash_out_jobs SET share_units = 99999999999999999999`,
		"status": `ALTER TABLE cash_out_jobs DROP CONSTRAINT cash_out_jobs_status_check;
			UPDATE cash_out_jobs SET status = 'lost'`,
		"returned": `ALTER TABLE cash_out_jobs DROP CONSTRAINT cash_out_jobs_returned_within_units;
			UPDATE cash_out_jobs SET returned_units = 101`,
		"height": attempt + `ALTER TABLE cash_out_payouts DROP CONSTRAINT
			cash_out_payouts_last_valid_block_height_check; UPDATE cash_out_payouts SET last_valid_block_height = -1`,
		"attempt": attempt + `ALTER TABLE cash_out_payouts DROP CONSTRAINT cash_out_payouts_status_check;
			UPDATE cash_out_payouts SET status = 'lost'`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			if _, err := r.f.pool.Exec(t.Context(), setup); err != nil {
				t.Fatal(err)
			}
			if err := r.advance(t, 0); errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("Advance = %v, want decode_failed", err)
			}
		})
	}
	r := newPayoutRig(t)
	if _, err := r.f.pool.Exec(
		t.Context(),
		`ALTER TABLE cash_out_payouts RENAME TO cash_out_payouts_gone`,
	); err != nil {
		t.Fatal(err)
	}
	if err := r.advance(t, 0); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Advance = %v, want internal", err)
	}
}

func TestCashOutPayout_anOutageOrUnknownJobMovesNothing(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.err = errs.New(errs.CodeRPCUnavailable, "test.rpc")
	if err := r.advance(t, 0); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Advance = %v, want rpc_unavailable", err)
	}
	r.wantJob(t, "paying", "")
	r.job = r.f.ids.NewV7()
	if err := r.advance(t, 0); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("Advance = %v, want not_found", err)
	}
}

func TestCashOutPayout_aJobMovedElsewhereMeanwhileIsLeftAlone(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		reading domain.PayoutReading
		move    string
	}{
		"before the completion commits": {landed(), `UPDATE cash_out_jobs SET status = 'completed' WHERE id = $1`},
		"before the failure commits":    {refused(), `UPDATE cash_out_payouts SET status = 'expired' WHERE job_id = $1`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			r.seed(t, 1, domain.PayoutBroadcast)
			r.chain.set(seededSig(1), c.reading)
			r.chain.onRead = func() {
				if _, err := r.f.pool.Exec(context.Background(), c.move, r.job); err != nil {
					panic(err)
				}
			}
			r.mustAdvance(t)
			if r.events(t, events.TypeCashOutCompleted) != "0" || r.events(t, events.TypeCashOutFailed) != "0" ||
				r.transferStatuses(t) != "cabal= user=pending" {
				t.Fatalf("moved a job another worker had already moved: headers %s", r.transferStatuses(t))
			}
		})
	}
}

func TestCashOutPayout_stopsWaitingWhenTheDeliveryIsCancelled(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	payouts := app.NewCashOutPayouts(app.CashOutPayoutDeps{
		UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: r.f.clock, Chain: r.chain,
		USDC: chain.Mint{Address: usdcMint, Decimals: 6}, Hints: r.hints,
	})
	ctx, cancel := context.WithCancel(observability.WithActor(r.f.ctx(), "system:treasury.cashout_payout"))
	err := payouts.Advance(ctx, r.job, app.CashOutPayoutWait, cancel)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Advance = %v, want upstream_unavailable", err)
	}
	r.wantJob(t, "paying", "")
}

func TestCashOutPayout_aHeldLedgerLockTimesOut(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), landed())
	conn, err := r.f.pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	lock := `SELECT pg_advisory_lock(hashtextextended('cabal-ledger:' || $1::text, 0))`
	if _, err := conn.Exec(t.Context(), lock, r.cabal.UUID()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock_all()`)
	}()
	ctx, cancel := context.WithTimeout(observability.WithActor(r.f.ctx(), "system:test"), 300*time.Millisecond)
	defer cancel()
	if err := r.payouts().Advance(ctx, r.job, 0, nil); err == nil {
		t.Fatal("Advance went past a held ledger lock")
	}
	r.wantJob(t, "paying", "")
}
