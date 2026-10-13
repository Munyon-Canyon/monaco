package main

import (
	"bytes"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type evmWalletsRun struct {
	t     *testing.T
	cfg   config.Config
	clock *testkit.Clock
	mu    sync.Mutex
	waits []time.Duration
}

func newEVMWalletsRun(t *testing.T, pool *pgxpool.Pool) *evmWalletsRun {
	t.Helper()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	cfg := config.Config{Env: config.EnvTest}
	cfg.DB.URL, cfg.DB.MaxConns = pool.Config().ConnString(), 4
	cfg.Privy.BaseURL, cfg.Privy.AppID, cfg.Privy.AppSecret = srv.URL+"/privy", "app", "secret"
	cfg.Privy.AuthorizationKeyID = fakes.PrivyAuthorizationKeyID
	cfg.Privy.AuthorizationPrivateKey = fakes.PrivyAuthorizationKeyConfig()
	cfg.Privy.VerificationKey = fakes.PrivyVerificationKey()
	cfg.Timeouts.Privy = 5 * time.Second
	r := &evmWalletsRun{t: t, cfg: cfg, clock: testkit.NewClock(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))}
	afters := make(chan time.Duration)
	r.clock.NotifyAfters(afters)
	go func() {
		for d := range afters {
			r.mu.Lock()
			r.waits = append(r.waits, d)
			r.mu.Unlock()
			r.clock.Advance(d)
		}
	}()
	t.Cleanup(func() { close(afters) })
	return r
}

func (r *evmWalletsRun) run(args ...string) (int, string, string) {
	r.t.Helper()
	var stdout, stderr bytes.Buffer
	code := evmWalletsCommand(r.clock)(r.cfg, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func (r *evmWalletsRun) paced() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.waits)
}

type evmWalletsSeed struct {
	users, cabals []string
}

func seedEVMWallets(t *testing.T, pool *pgxpool.Pool) evmWalletsSeed {
	t.Helper()
	var s evmWalletsSeed
	for range 3 {
		s.users = append(s.users, testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}).ID.String())
		s.cabals = append(s.cabals, testkit.NewCabal(t, pool).ID.String())
	}
	testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true, AccountStatus: "deleted"})
	if _, err := pool.Exec(t.Context(), `UPDATE user_wallets SET evm_privy_wallet_id = 'held-user',
		evm_address = '0x00000000000000000000000000000000000000aa', evm_created_at = now() WHERE user_id = $1`,
		s.users[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO treasury_evm_wallets (cabal_id, privy_wallet_id, address, created_at)
		VALUES ($1, 'held-cabal', '0x00000000000000000000000000000000000000bb', now())`,
		s.cabals[2],
	); err != nil {
		t.Fatal(err)
	}
	return s
}

func evmWalletCounts(t *testing.T, pool *pgxpool.Pool) (users, cabals int) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM user_wallets WHERE evm_privy_wallet_id IS NOT NULL),
		(SELECT count(*) FROM treasury_evm_wallets)`).Scan(&users, &cabals); err != nil {
		t.Fatal(err)
	}
	return users, cabals
}

func assertCreatedLines(t *testing.T, stdout string, seed evmWalletsSeed) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	want := []string{
		"user\t" + seed.users[0] + "\t0x", "user\t" + seed.users[1] + "\t0x",
		"cabal\t" + seed.cabals[0] + "\t0x", "cabal\t" + seed.cabals[1] + "\t0x",
		"evm-wallets: users created=2 existing=1 failed=0; cabals created=2 existing=1 failed=0",
	}
	if len(lines) != len(want) {
		t.Fatalf("stdout = %q, want %d lines", stdout, len(want))
	}
	for i, prefix := range want {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], prefix)
		}
	}
}

func TestOpsEVMWallets_createsOneWalletPerUserAndCabalAndARerunCreatesNone(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seed := seedEVMWallets(t, pool)
	r := newEVMWalletsRun(t, pool)
	code, stdout, stderr := r.run()
	if code != 0 || stderr != "" {
		t.Fatalf("first run = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	assertCreatedLines(t, stdout, seed)
	if got := r.paced(); !slices.Equal(got, []time.Duration{evmWalletPace, evmWalletPace, evmWalletPace}) {
		t.Errorf("waits between ensures = %v, want three of %v", got, evmWalletPace)
	}
	if users, cabals := evmWalletCounts(t, pool); users != 3 || cabals != 3 {
		t.Fatalf("stored wallets = %d users, %d cabals; want 3 and 3", users, cabals)
	}
	code, stdout, stderr = r.run()
	if code != 0 || stderr != "" ||
		stdout != "evm-wallets: users created=0 existing=3 failed=0; cabals created=0 existing=3 failed=0\n" {
		t.Fatalf("rerun = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if users, cabals := evmWalletCounts(t, pool); users != 3 || cabals != 3 {
		t.Fatalf("stored wallets after rerun = %d users, %d cabals; want 3 and 3", users, cabals)
	}
}

func TestOpsEVMWallets_dryRunCountsAndCreatesNothing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedEVMWallets(t, pool)
	r := newEVMWalletsRun(t, pool)
	code, stdout, stderr := r.run("--dry-run")
	if code != 0 || stderr != "" ||
		stdout != "evm-wallets dry run: users missing=2 existing=1; cabals missing=2 existing=1\n" {
		t.Fatalf("dry run = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if users, cabals := evmWalletCounts(t, pool); users != 1 || cabals != 1 {
		t.Fatalf("stored wallets = %d users, %d cabals; want only the seeded 1 and 1", users, cabals)
	}
}

func TestOpsEVMWallets_aFlagLimitsTheRunToItsKind(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedEVMWallets(t, pool)
	r := newEVMWalletsRun(t, pool)
	if code, stdout, _ := r.run("--dry-run", "--cabals"); code != 0 ||
		stdout != "evm-wallets dry run: cabals missing=2 existing=1\n" {
		t.Fatalf("--cabals = %d, stdout %q", code, stdout)
	}
	code, stdout, _ := r.run("--users")
	if code != 0 || !strings.HasSuffix(stdout, "evm-wallets: users created=2 existing=1 failed=0\n") {
		t.Fatalf("--users = %d, stdout %q", code, stdout)
	}
	if users, cabals := evmWalletCounts(t, pool); users != 3 || cabals != 1 {
		t.Fatalf("stored wallets = %d users, %d cabals; want 3 and 1", users, cabals)
	}
}

func TestOpsEVMWallets_aFailedEnsureIsCountedAndExitsOne(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seed := seedEVMWallets(t, pool)
	if _, err := pool.Exec(t.Context(), `UPDATE user_wallets SET address = 'not-a-solana-address' WHERE user_id = $1`,
		seed.users[0]); err != nil {
		t.Fatal(err)
	}
	r := newEVMWalletsRun(t, pool)
	code, stdout, stderr := r.run()
	if code != 1 || !strings.HasPrefix(stderr, "evm-wallets: user "+seed.users[0]+": ") ||
		!strings.HasSuffix(
			stdout,
			"evm-wallets: users created=1 existing=1 failed=1; cabals created=2 existing=1 failed=0\n",
		) {
		t.Fatalf("run = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestOpsEVMWallets_rejectsUnknownArguments(t *testing.T) {
	t.Parallel()
	r := newEVMWalletsRun(t, testkit.DB(t))
	for _, args := range [][]string{{"--bogus"}, {"extra"}} {
		if code, _, stderr := r.run(args...); code != 2 || stderr != evmWalletsUsage+"\n" {
			t.Errorf("evm-wallets %v = %d, stderr %q", args, code, stderr)
		}
	}
}

func TestOpsEVMWallets_aStoreItCannotReadFailsTheRun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ table, flag string }{{"user_wallets", "--users"}, {"treasury_wallets", "--cabals"}} {
		t.Run(tc.table, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			if _, err := pool.Exec(t.Context(), "ALTER TABLE "+tc.table+" RENAME TO gone"); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := newEVMWalletsRun(t, pool).run(tc.flag)
			if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "monacoctl: ") {
				t.Fatalf("run = %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
	t.Run("unreachable database", func(t *testing.T) {
		t.Parallel()
		r := newEVMWalletsRun(t, testkit.DB(t))
		r.cfg.DB.URL = "postgres://monaco@127.0.0.1:1/monaco?connect_timeout=1"
		if code, _, stderr := r.run(); code != 1 || !strings.HasPrefix(stderr, "monacoctl: ") {
			t.Fatalf("run = %d, stderr %q", code, stderr)
		}
	})
}
