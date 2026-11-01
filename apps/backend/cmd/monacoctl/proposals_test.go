package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededProposals(t *testing.T) (tool, uuid.UUID, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC()
	id, voter := ids.Real{}.NewV7(), ids.Real{}.NewV7()
	if _, err := sqlc.New(pool).InsertProposal(t.Context(), sqlc.InsertProposalParams{
		ID: id, CabalID: ids.Real{}.NewV7(), ProposerID: voter, Kind: "buy", Symbol: "AAPLx",
		Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true},
		QuoteOutAmount: 21_000_000, ExpiresAt: now.Add(time.Hour), CreatedAt: now, Threshold: "majority",
		VoterIds: []uuid.UUID{voter},
	}); err != nil {
		t.Fatal(err)
	}
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	return proposalsTool(environ, testkit.NewClock(now)), id, pool
}

func TestProposalsVoid_rejectsMalformedArgumentsWithTheUsage(t *testing.T) {
	t.Parallel()
	proposals, id, _ := seededProposals(t)
	for _, args := range [][]string{
		{"void", "--reason", "spam"},
		{"void", "--proposal", "not-a-uuid", "--reason", "spam"},
		{"void", "--proposal", id.String(), "--reason", "spam", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if code := proposals(args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), proposalsUsage) {
			t.Errorf("proposals %v = %d, stderr %q, want 2 and the usage", args, code, stderr.String())
		}
	}
}

func TestProposalsVoid_voidsAsSystemOnceThenReportsTheProposalClosed(t *testing.T) {
	t.Parallel()
	proposals, id, pool := seededProposals(t)
	var stdout, stderr bytes.Buffer
	blank := []string{"void", "--proposal", id.String(), "--reason", " "}
	if code := proposals(blank, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "invalid_input") {
		t.Fatalf("void with a blank reason = %d, stderr %q, want 1 and invalid_input", code, stderr.String())
	}
	stderr.Reset()
	void := []string{"void", "--proposal", id.String(), "--reason", "spam"}
	if code := proposals(void, &stdout, &stderr); code != 0 || stdout.String() != "voided "+id.String()+"\n" {
		t.Fatalf("first void = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	var actor string
	if err := pool.QueryRow(t.Context(), `SELECT actor_type || ':' || actor_id FROM events
		WHERE type = 'proposal.voided' AND aggregate_id = $1`, id).Scan(&actor); err != nil || actor != "system:monacoctl" {
		t.Fatalf("proposal.voided actor = %q, %v, want system:monacoctl", actor, err)
	}
	stdout.Reset()
	if code := proposals(void, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "proposal_closed") {
		t.Fatalf("second void = %d, stderr %q, want 1 and proposal_closed", code, stderr.String())
	}
}

func TestProposalsVoid_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}
	args := []string{"void", "--proposal", ids.Real{}.NewV7().String(), "--reason", "spam"}
	var stdout, stderr bytes.Buffer
	if code := proposalsTool(environ, clock.Real{})(args, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("void with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}

func blockUnstarted(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `UPDATE proposals SET status = 'execution_blocked',
		status_reason = 'trade_not_started' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func TestProposalsRetryTrade_reopensAProposalBlockedBeforeAnySwap(t *testing.T) {
	t.Parallel()
	proposals, id, pool := seededProposals(t)
	blockUnstarted(t, pool, id)
	var stdout, stderr bytes.Buffer
	args := []string{"retry-trade", "--proposal", id.String(), "--reason", "jupiter was down"}
	want := "retry requested for " + id.String() + "\n"
	if code := proposals(args, &stdout, &stderr); code != 0 || stdout.String() != want {
		t.Fatalf("retry-trade = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	var status string
	var reason *string
	if err := pool.QueryRow(t.Context(), `SELECT status, status_reason FROM proposals WHERE id = $1`, id).
		Scan(&status, &reason); err != nil || status != "passed" || reason != nil {
		t.Fatalf("proposal = %q %v, %v; want it passed again with no reason", status, reason, err)
	}
	var reopened int
	var actor, why string
	if err := pool.QueryRow(t.Context(), `SELECT count(*), max(actor_type || ':' || actor_id), max(payload->>'reason')
		FROM events WHERE type = 'proposal.reopened' AND aggregate_id = $1`, id).
		Scan(&reopened, &actor, &why); err != nil || reopened != 1 || !strings.HasPrefix(actor, "system:operator:") ||
		why != "jupiter was down" {
		t.Fatalf("%d proposal.reopened events by %q for %q, %v; want 1 by an operator with the reason",
			reopened, actor, why, err)
	}
	stderr.Reset()
	if code := proposals(
		args,
		&stdout,
		&stderr,
	); code != 1 ||
		!strings.Contains(stderr.String(), "swap_not_retryable") {
		t.Fatalf("second retry-trade = %d, stderr %q, want 1 and swap_not_retryable", code, stderr.String())
	}
}

func TestProposalsRetryTrade_asksTheEngineToRunTheProposalsOwnBuy(t *testing.T) {
	t.Parallel()
	proposals, id, pool := seededProposals(t)
	blockUnstarted(t, pool, id)
	var stdout, stderr bytes.Buffer
	args := []string{"retry-trade", "--proposal", id.String(), "--reason", "jupiter was down"}
	if code := proposals(args, &stdout, &stderr); code != 0 {
		t.Fatalf("retry-trade = %d, stderr %q", code, stderr.String())
	}
	var action, inAmount, quote, outMint, swap string
	if err := pool.QueryRow(t.Context(), `SELECT payload->>'action', payload->>'in_amount',
		payload->>'quote_out_amount', payload->>'out_mint', payload->>'swap_id' FROM events
		WHERE type = 'trade.retry_requested' AND payload->'source'->>'id' = $1`, id.String()).
		Scan(&action, &inAmount, &quote, &outMint, &swap); err != nil {
		t.Fatal(err)
	}
	if action != "buy" || inAmount != "5000000" || quote != "21000000" ||
		outMint != "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp" || swap != uuid.Nil.String() {
		t.Fatalf("retry event = %s %s %s %s %s, want the proposal's own buy and no swap",
			action, inAmount, quote, outMint, swap)
	}
}

func TestProposalsRetryTrade_refusesWhatItCannotRetry(t *testing.T) {
	t.Parallel()
	proposals, id, pool := seededProposals(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"blank reason", []string{"retry-trade", "--proposal", id.String(), "--reason", " "}, "invalid_input"},
		{"still open", []string{"retry-trade", "--proposal", id.String(), "--reason", "x"}, "swap_not_retryable"},
		{
			"unknown proposal",
			[]string{"retry-trade", "--proposal", ids.Real{}.NewV7().String(), "--reason", "x"},
			"proposal_not_found",
		},
	} {
		var stdout, stderr bytes.Buffer
		if code := proposals(tc.args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%s = %d, stderr %q, want 1 and %s", tc.name, code, stderr.String(), tc.want)
		}
	}
	blockUnstarted(t, pool, id)
	if _, err := pool.Exec(t.Context(), `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address,
		action, symbol, in_mint, out_mint, out_decimals, in_amount, slippage_bps, source_batch_size, status,
		created_at, updated_at)
		VALUES ($1, 'proposal', $2, $3, 'EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v', 'buy', 'AAPLx',
			'EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v', 'XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp', 6, 1, 100, 1,
			'created', now(), now())`,
		uuid.New(), id, uuid.New()); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"retry-trade", "--proposal", id.String(), "--reason", "x"}
	if code := proposals(
		args,
		&stdout,
		&stderr,
	); code != 1 ||
		!strings.Contains(stderr.String(), "swap_not_retryable") {
		t.Errorf("proposal with a swap = %d, stderr %q, want 1 and swap_not_retryable", code, stderr.String())
	}
}

func TestProposalsRetryTrade_asksTheEngineToRunTheProposalsOwnSell(t *testing.T) {
	t.Parallel()
	proposals, _, pool := seededProposals(t)
	now := clock.Real{}.Now().UTC()
	id, voter := ids.Real{}.NewV7(), ids.Real{}.NewV7()
	if _, err := sqlc.New(pool).InsertProposal(t.Context(), sqlc.InsertProposalParams{
		ID: id, CabalID: ids.Real{}.NewV7(), ProposerID: voter, Kind: "sell", Symbol: "AAPLx",
		Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", TokenAmount: pgtype.Int8{Int64: 400, Valid: true},
		QuoteOutAmount: 21_000_000, ExpiresAt: now.Add(time.Hour), CreatedAt: now, Threshold: "majority",
		VoterIds: []uuid.UUID{voter},
	}); err != nil {
		t.Fatal(err)
	}
	blockUnstarted(t, pool, id)
	var stdout, stderr bytes.Buffer
	args := []string{"retry-trade", "--proposal", id.String(), "--reason", "jupiter was down"}
	if code := proposals(args, &stdout, &stderr); code != 0 {
		t.Fatalf("retry-trade = %d, stderr %q", code, stderr.String())
	}
	var action, inAmount, inMint string
	if err := pool.QueryRow(t.Context(), `SELECT payload->>'action', payload->>'in_amount', payload->>'in_mint'
		FROM events WHERE type = 'trade.retry_requested' AND payload->'source'->>'id' = $1`, id.String()).
		Scan(&action, &inAmount, &inMint); err != nil {
		t.Fatal(err)
	}
	if action != "sell" || inAmount != "400" || inMint != "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp" {
		t.Fatalf("retry event = %s %s %s, want the proposal's own sell", action, inAmount, inMint)
	}
}

func refuseOn(table, when string) string {
	name := "refuse_" + table
	return `CREATE FUNCTION ` + name + `() RETURNS trigger LANGUAGE plpgsql AS
		$$BEGIN RAISE EXCEPTION 'refused'; END$$;
		CREATE TRIGGER ` + name + ` BEFORE INSERT OR UPDATE ON ` + table + ` FOR EACH ROW WHEN (` + when +
		`) EXECUTE FUNCTION ` + name + `()`
}

func TestProposalsRetryTrade_reportsStoreFailuresAndChangesNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, setup string }{
		{"swap_lookup", `DROP TABLE swaps CASCADE`},
		{"reopen", refuseOn("proposals", "NEW.status = 'passed'")},
		{"reopened_event", refuseOn("events", "NEW.type = 'proposal.reopened'")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proposals, id, pool := seededProposals(t)
			blockUnstarted(t, pool, id)
			if _, err := pool.Exec(t.Context(), tc.setup); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			args := []string{"retry-trade", "--proposal", id.String(), "--reason", "x"}
			if code := proposals(args, &stdout, &stderr); code != 1 {
				t.Errorf("retry-trade = %d, stderr %q, want 1", code, stderr.String())
			}
			var status string
			if err := pool.QueryRow(t.Context(), `SELECT status FROM proposals WHERE id = $1`, id).
				Scan(&status); err != nil ||
				status != "execution_blocked" {
				t.Errorf("status = %q, %v, want it left blocked", status, err)
			}
		})
	}
}
