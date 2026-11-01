package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	fundingapp "github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededBounce(t *testing.T) (tool, string, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	deposit := ids.Real{}.NewV7()
	if _, err := pool.Exec(t.Context(), `INSERT INTO external_deposits (id, signature, cabal_id, sender, mint, amount,
		source, status, detected_at) VALUES ($1, 'stray', $2, 'sender', 'mint', 25000000, 'reconcile', 'detected', now())`,
		deposit, cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO cabal_pauses (id, cabal_id, reason, created_at, external_deposit_id)
		VALUES ($1, $2, 'external_deposit', now(), $3)`,
		ids.Real{}.NewV7(),
		cabal.ID.UUID(),
		deposit,
	); err != nil {
		t.Fatal(err)
	}
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused", "USER=ops-person",
	}
	return fundingTool(environ, testkit.NewClock(clock.Real{}.Now().UTC())), deposit.String(), pool
}

func TestFundingBounce_printsTheUsageForHelpAndMalformedArguments(t *testing.T) {
	t.Parallel()
	bounce, id, _ := seededBounce(t)
	if code, stdout, _ := runOpsTool(t, bounce, "bounce", "hold", "--help"); code != 0 ||
		!strings.Contains(stdout, fundingUsage) {
		t.Fatalf("hold --help = %d, stdout %q, want 0 and the usage", code, stdout)
	}
	for _, args := range [][]string{
		{"bounce"},
		{"bounce", "hold"},
		{"bounce", "hold", "not-a-uuid"},
		{"bounce", "set-return-address", id},
		{"bounce", "bogus", id},
		{"bounce", "retry", id, "extra"},
	} {
		if code, _, stderr := runOpsTool(t, bounce, args...); code != 2 || !strings.Contains(stderr, fundingUsage) {
			t.Errorf("funding %v = %d, stderr %q, want 2 and the usage", args, code, stderr)
		}
	}
}

func TestFundingBounce_holdsADetectedDepositAndEndsItsPause(t *testing.T) {
	t.Parallel()
	bounce, id, pool := seededBounce(t)
	if code, stdout, stderr := runOpsTool(t, bounce, "bounce", "hold", id); code != 0 || stdout != "hold "+id+"\n" {
		t.Fatalf("hold = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	var status string
	var open int
	if err := pool.QueryRow(t.Context(), `SELECT status, (SELECT count(*) FROM cabal_pauses WHERE resolved_at IS NULL)
		FROM external_deposits`).Scan(&status, &open); err != nil || status != "held" || open != 0 {
		t.Fatalf("status = %s with %d open pauses, %v, want held and none", status, open, err)
	}
	if code, _, stderr := runOpsTool(t, bounce, "bounce", "hold", id); code != 1 ||
		!strings.Contains(stderr, "version_conflict") {
		t.Fatalf("second hold = %d, stderr %q, want 1 and version_conflict", code, stderr)
	}
}

func TestFundingBounce_setsAReturnAddress(t *testing.T) {
	t.Parallel()
	bounce, id, pool := seededBounce(t)
	to := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	if code, _, stderr := runOpsTool(t, bounce, "bounce", "set-return-address", id, to); code != 0 {
		t.Fatalf("set-return-address = %d, stderr %q", code, stderr)
	}
	var got string
	if err := pool.QueryRow(t.Context(), `SELECT return_address FROM external_deposits`).Scan(&got); err != nil ||
		got != to {
		t.Fatalf("return_address = %q, %v, want %s", got, err, to)
	}
	if code, _, _ := runOpsTool(t, bounce, "bounce", "retry", id); code != 1 {
		t.Fatalf("retry of a detected deposit = %d, want 1", code)
	}
}

func TestFundingBounce_namesTheOperator(t *testing.T) {
	t.Parallel()
	if got := operatorOf([]string{"HOME=/x", "USER=ops-person"}); got != "ops-person" {
		t.Fatalf("operator = %q, want ops-person", got)
	}
	if got := operatorOf([]string{"USER="}); got != "unknown" {
		t.Fatalf("operator = %q, want unknown", got)
	}
}

func TestFundingBounce_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}
	var stdout, stderr bytes.Buffer
	args := []string{"bounce", "hold", ids.Real{}.NewV7().String()}
	if code := toolFunding(toolEnv{environ: environ})(args, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("hold with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}

func TestFundingBounceList_printsTheOpenDepositsByDefault(t *testing.T) {
	t.Parallel()
	bounce, id, _ := seededBounce(t)
	code, stdout, stderr := runOpsTool(t, bounce, "bounce", "list")
	fields := strings.Split(strings.TrimSuffix(stdout, "\n"), "\t")
	if code != 0 || len(fields) != 8 || fields[0] != id || fields[2] != "detected" || fields[3] != "25000000" ||
		fields[4] != "mint" || fields[5] != "-" || fields[6] != "0" {
		t.Fatalf(
			"list = %d, stdout %q, stderr %q, want the detected deposit on one tab-separated line",
			code,
			stdout,
			stderr,
		)
	}
}

func TestFundingBounceList_filtersByStatusAndRefusesAnUnknownOne(t *testing.T) {
	t.Parallel()
	bounce, _, pool := seededBounce(t)
	if _, err := pool.Exec(t.Context(),
		`UPDATE external_deposits SET status = 'bounce_failed', return_address = 'Return1'`); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := runOpsTool(t, bounce, "bounce", "list", "bounce_failed", "held"); code != 0 ||
		!strings.Contains(stdout, "\tbounce_failed\t") || !strings.Contains(stdout, "\tReturn1\t") {
		t.Fatalf("list bounce_failed = %d, stdout %q, want the failed deposit with its return address", code, stdout)
	}
	if code, stdout, _ := runOpsTool(t, bounce, "bounce", "list", "held"); code != 0 || stdout != "" {
		t.Fatalf("list held = %d, stdout %q, want nothing held", code, stdout)
	}
	if code, _, stderr := runOpsTool(t, bounce, "bounce", "list", "bogus"); code != 2 ||
		!strings.Contains(stderr, fundingUsage) {
		t.Fatalf("list bogus = %d, stderr %q, want 2 and the usage", code, stderr)
	}
}

func seededWatchWallet(t *testing.T, pool *pgxpool.Pool) testkit.SeededUser {
	t.Helper()
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := pool.Exec(t.Context(), `INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot,
		first_seen_at) VALUES ($1, $2, 0, now())`, user.Address, user.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO deposit_watch_accounts (token_account, wallet_address, canonical,
		state, high_signature, recovery_due_at) VALUES ('ata', $1, true, 'open', 'baseline', now())`, user.Address); err != nil {
		t.Fatal(err)
	}
	return user
}

func watchAccount(t *testing.T, pool *pgxpool.Pool) (high string, dirty, clean int64) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), `SELECT coalesce(high_signature, ''), dirty_gen, clean_gen
		FROM deposit_watch_accounts WHERE token_account = 'ata'`).Scan(&high, &dirty, &clean); err != nil {
		t.Fatal(err)
	}
	return high, dirty, clean
}

func TestFundingDepositRescan_marksTheWalletForAScan(t *testing.T) {
	t.Parallel()
	tl, _, pool := seededBounce(t)
	user := seededWatchWallet(t, pool)
	if code, stdout, stderr := runOpsTool(
		t,
		tl,
		"deposit",
		"rescan",
		string(user.Address),
		"--from",
		"sigX",
	); code != 0 ||
		stdout != "rescan "+string(user.Address)+" 1\n" {
		t.Fatalf("rescan = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if high, dirty, clean := watchAccount(t, pool); high != "sigX" || dirty <= clean {
		t.Fatalf("account = %q dirty %d clean %d, want the cursor at sigX and the account dirty", high, dirty, clean)
	}
	if code, _, stderr := runOpsTool(t, tl, "deposit", "rescan", string(user.Address)); code != 0 {
		t.Fatalf("rescan without --from = %d, stderr %q", code, stderr)
	}
	if high, _, _ := watchAccount(t, pool); high != "" {
		t.Fatalf("high_signature = %q, want it cleared", high)
	}
}

func TestFundingDepositRescan_refusesAnUnwatchedWalletAndBadArguments(t *testing.T) {
	t.Parallel()
	tl, _, pool := seededBounce(t)
	user := seededWatchWallet(t, pool)
	stranger := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if code, _, stderr := runOpsTool(t, tl, "deposit", "rescan", string(stranger.Address)); code != 1 ||
		!strings.Contains(stderr, "not_found") {
		t.Fatalf("rescan of an unwatched wallet = %d, stderr %q, want 1 and not_found", code, stderr)
	}
	if code, _, stderr := runOpsTool(t, tl, "deposit", "rescan", "not-an-address"); code != 1 || stderr == "" {
		t.Fatalf("rescan of a bad address = %d, stderr %q, want 1", code, stderr)
	}
	for _, args := range [][]string{
		{"deposit"},
		{"deposit", "rescan"},
		{"deposit", "bogus", "x"},
		{"deposit", "rescan", string(user.Address), "--since", "sig"},
		{"deposit", "rescan", string(user.Address), "--from"},
	} {
		if code, _, stderr := runOpsTool(t, tl, args...); code != 2 || !strings.Contains(stderr, fundingUsage) {
			t.Errorf("funding %v = %d, stderr %q, want 2 and the usage", args, code, stderr)
		}
	}
}

func TestFunding_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}
	tl := fundingTool(environ, testkit.NewClock(clock.Real{}.Now().UTC()))
	for _, args := range [][]string{
		{"bounce", "list"},
		{"deposit", "rescan", string(testkit.SeedUser(t, testkit.DB(t), testkit.UserOpts{WithWallet: true}).Address)},
	} {
		if code, _, stderr := runOpsTool(
			t,
			tl,
			args...); code != 1 ||
			!strings.Contains(stderr, "monacoctl: db.Open: ") {
			t.Errorf("funding %v with no database = %d, stderr %q, want 1 and db.Open", args, code, stderr)
		}
	}
}

func TestFundingBounceList_reportsAFailedRead(t *testing.T) {
	t.Parallel()
	bounce, _, pool := seededBounce(t)
	if _, err := pool.Exec(t.Context(), `ALTER TABLE external_deposits RENAME TO external_deposits_gone`); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runOpsTool(t, bounce, "bounce", "list"); code != 1 || stderr == "" {
		t.Fatalf("list with the table gone = %d, stderr %q, want 1", code, stderr)
	}
}

type fakeWithdrawalResolver struct {
	got fundingapp.ResolveEVMWithdrawal
	err error
}

func (f *fakeWithdrawalResolver) Handle(
	_ context.Context, cmd fundingapp.ResolveEVMWithdrawal,
) (fundingapp.EVMWithdrawalResolution, error) {
	f.got = cmd
	return fundingapp.EVMWithdrawalResolution{
		ID: cmd.ID, Status: cmd.To, Facts: fundingapp.EVMWithdrawalFacts{SolanaChain: "failed"},
	}, f.err
}

func withdrawalTool(t *testing.T, resolver *fakeWithdrawalResolver) tool {
	t.Helper()
	pool := testkit.DB(t)
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused", "USER=ops-person",
	}
	return fundingToolWith(environ, testkit.NewClock(clock.Real{}.Now().UTC()),
		func(module.Deps) withdrawalResolver { return resolver })
}

func TestFundingWithdrawalResolve_passesTheOperatorReasonAndOverride(t *testing.T) {
	t.Parallel()
	resolver := &fakeWithdrawalResolver{}
	id := uuid.New()
	code, stdout, stderr := runOpsTool(t, withdrawalTool(t, resolver), "withdrawal", "resolve", id.String(),
		"--to", "failed", "--reason", "support refunded the member", "--override")
	want := fundingapp.ResolveEVMWithdrawal{
		ID: id, To: domain.WithdrawalFailed, Reason: "support refunded the member", Actor: "operator:ops-person",
		Override: true,
	}
	if code != 0 || resolver.got != want || !strings.HasPrefix(stdout, "resolve "+id.String()+" failed ") ||
		!strings.Contains(stdout, `"solana_chain_state":"failed"`) {
		t.Fatalf(
			"resolve = %d, stdout %q, stderr %q, handler got %+v, want %+v",
			code,
			stdout,
			stderr,
			resolver.got,
			want,
		)
	}
}

func TestFundingWithdrawalResolve_reportsAHandlerRefusal(t *testing.T) {
	t.Parallel()
	resolver := &fakeWithdrawalResolver{err: errs.New(errs.CodeInvalidInput, "test")}
	code, _, stderr := runOpsTool(t, withdrawalTool(t, resolver), "withdrawal", "resolve", uuid.NewString(),
		"--to", "confirmed", "--reason", "landed")
	if code != 1 || !strings.Contains(stderr, "invalid_input") {
		t.Fatalf("resolve = %d, stderr %q, want 1 and invalid_input", code, stderr)
	}
}

func TestFundingWithdrawalResolve_printsTheUsageForMalformedArguments(t *testing.T) {
	t.Parallel()
	tl := withdrawalTool(t, &fakeWithdrawalResolver{})
	id := uuid.NewString()
	for _, args := range [][]string{
		{"withdrawal"},
		{"withdrawal", "resolve"},
		{"withdrawal", "resolve", "not-a-uuid", "--to", "failed", "--reason", "x"},
		{"withdrawal", "resolve", id, "--reason", "x"},
		{"withdrawal", "resolve", id, "--to", "failed"},
		{"withdrawal", "resolve", id, "--to", "submitted", "--reason", "x"},
		{"withdrawal", "resolve", id, "--to", "failed", "--reason", "x", "extra"},
		{"withdrawal", "retry", id},
	} {
		if code, _, stderr := runOpsTool(t, tl, args...); code != 2 || !strings.Contains(stderr, fundingUsage) {
			t.Errorf("funding %v = %d, stderr %q, want 2 and the usage", args, code, stderr)
		}
	}
}

func TestFundingWithdrawalResolve_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1",
		"NATS_URL=nats://unused", "USER=ops-person",
	}
	tl := fundingToolWith(environ, testkit.NewClock(clock.Real{}.Now().UTC()),
		func(module.Deps) withdrawalResolver { return &fakeWithdrawalResolver{} })
	if code, _, stderr := runOpsTool(t, tl, "withdrawal", "resolve", uuid.NewString(), "--to", "failed",
		"--reason", "x"); code != 1 || stderr == "" {
		t.Fatalf("resolve = %d, stderr %q, want 1 and a message", code, stderr)
	}
}

func TestFundingWithdrawalResolve_theLiveResolverReportsAnUnknownWithdrawal(t *testing.T) {
	t.Parallel()
	bounce, _, _ := seededBounce(t)
	code, _, stderr := runOpsTool(
		t,
		bounce,
		"withdrawal",
		"resolve",
		uuid.NewString(),
		"--to",
		"failed",
		"--reason",
		"x",
	)
	if code != 1 || !strings.Contains(stderr, "not_found") {
		t.Fatalf("resolve = %d, stderr %q, want 1 and not_found", code, stderr)
	}
}
