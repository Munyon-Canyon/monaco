package treasury_test

import (
	"testing"
)

func (h *settleHarness) seedWindDown(t *testing.T, status string, attempts int) {
	t.Helper()
	completed := "NULL"
	if status == "completed" {
		completed = "now()"
	}
	if _, err := h.pool.Exec(
		t.Context(),
		`INSERT INTO cabal_winddowns (cabal_id, status, attempts, started_at, completed_at)
		VALUES ($1, $2, $3, now(), `+completed+`)`,
		h.cabal.UUID(),
		status,
		attempts,
	); err != nil {
		t.Fatal(err)
	}
}

func (h *settleHarness) windDown(t *testing.T) (status string, attempts int, completed bool) {
	t.Helper()
	err := h.pool.QueryRow(t.Context(),
		`SELECT status, attempts, completed_at IS NOT NULL FROM cabal_winddowns WHERE cabal_id = $1`,
		h.cabal.UUID()).Scan(&status, &attempts, &completed)
	if err != nil {
		t.Fatal(err)
	}
	return status, attempts, completed
}

func TestFundSettler_aFundLandingAfterTheWindDownCompletedReopensItSoTheMemberIsPaidBack(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	h.seedWindDown(t, "completed", 7)
	id := h.landed(t, "5000000")
	h.tick(t)
	if status, units, _ := h.status(t, id); status != "settled" || units != "5000000" {
		t.Fatalf("late fund = %s/%s, want settled 5000000", status, units)
	}
	if status, attempts, completed := h.windDown(t); status != "running" || attempts != 0 || completed {
		t.Fatalf(
			"wind-down = %s with %d attempts, completed %t, want running, reset and open",
			status,
			attempts,
			completed,
		)
	}
	h.assertMinted(t, id)
}

func TestFundSettler_aFundLandingDuringARunningWindDownLeavesItRunning(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	h.seedWindDown(t, "running", 4)
	id := h.landed(t, "5000000")
	h.tick(t)
	if status, attempts, _ := h.windDown(t); status != "running" || attempts != 4 {
		t.Fatalf("wind-down = %s with %d attempts, want running and untouched", status, attempts)
	}
	h.assertMinted(t, id)
}

func TestFundSettler_aFundIntoACabalNeverBannedLeavesNoWindDownBehind(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id := h.landed(t, "5000000")
	h.tick(t)
	var rows int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_winddowns WHERE cabal_id = $1`, h.cabal.UUID()).
		Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("wind-down rows = %d, %v, want none", rows, err)
	}
	h.assertMinted(t, id)
}

func TestFundSettler_aWindDownThatCannotReopenKeepsTheFundLandedForTheNextTick(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	h.seedWindDown(t, "completed", 7)
	id := h.landed(t, "5000000")
	if _, err := h.pool.Exec(t.Context(), `CREATE FUNCTION refuse_winddown_update() RETURNS trigger LANGUAGE plpgsql
		AS $$ BEGIN RAISE EXCEPTION 'no update'; END $$;
		CREATE TRIGGER refuse_winddown_update BEFORE UPDATE ON cabal_winddowns
		FOR EACH ROW EXECUTE FUNCTION refuse_winddown_update()`); err != nil {
		t.Fatal(err)
	}
	if err := h.tickErr(); err == nil {
		t.Fatal("Tick with the reopen refused = nil error")
	}
	if status, units, _ := h.status(t, id); status != "landed" || units != "0" {
		t.Fatalf("fund = %s/%s, want it landed and unminted so the next tick retries", status, units)
	}
	if status, _, _ := h.windDown(t); status != "completed" {
		t.Fatalf("wind-down = %s, want it left completed", status)
	}
}
