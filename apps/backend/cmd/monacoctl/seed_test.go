package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const rankedMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func rankedUsers(t *testing.T, pool *pgxpool.Pool) (ids.UserID, ids.UserID) {
	t.Helper()
	return testkit.SeedUser(t, pool, testkit.UserOpts{}).ID, testkit.SeedUser(t, pool, testkit.UserOpts{}).ID
}

func seedPricedAsset(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	now := clock.Real{}.Now().UTC()
	if _, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, 'AAPLx', $2, 8, 'tessera', 'pre_ipo', 'AAPLx', true, 'AAPLx', $3, $3, $3)`,
		ids.Real{}.NewV7(), rankedMint, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
		VALUES ($1, $2, 20000000, 'jupiter')`, rankedMint, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
}

func seedCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func seedRun(t *testing.T, pool *pgxpool.Pool, a, b ids.UserID) (int, string, string) {
	t.Helper()
	return runOps(opsEnv(pool.Config().ConnString()), "seed", "two-cabals-ranked", "--users", a.String()+","+b.String())
}

func TestSeedCommand_OntoExistingUsers(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, b := rankedUsers(t, pool)
	seedPricedAsset(t, pool)
	code, stdout, stderr := seedRun(t, pool, a, b)
	if code != 0 || !strings.HasPrefix(stdout, "seeded two-cabals-ranked: ") || stderr != "" {
		t.Fatalf("seed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, user := range []ids.UserID{a, b} {
		if n := seedCount(t, pool, `SELECT count(*) FROM leaderboard_entries
			WHERE board = 'people' AND range = 'ALL' AND subject_id = $1`, user.UUID()); n != 1 {
			t.Fatalf("user %s has %d rows on the people board, want 1", user, n)
		}
	}
	if n := seedCount(t, pool, `SELECT count(*) FROM users`); n != 2 {
		t.Fatalf("users = %d, want the two that were there", n)
	}
	if n := seedCount(
		t,
		pool,
		`SELECT count(*) FROM leaderboard_entries WHERE board = 'cabals' AND range = 'ALL'`,
	); n != 2 {
		t.Fatalf("cabals board = %d rows, want both cabals", n)
	}
}

func TestSeedCommand_Twice(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, b := rankedUsers(t, pool)
	seedPricedAsset(t, pool)
	if code, _, stderr := seedRun(t, pool, a, b); code != 0 {
		t.Fatalf("first seed: code=%d stderr=%q", code, stderr)
	}
	counts := func() [4]int {
		return [4]int{
			seedCount(t, pool, `SELECT count(*) FROM events`),
			seedCount(t, pool, `SELECT count(*) FROM leaderboard_entries`),
			seedCount(t, pool, `SELECT count(*) FROM leaderboard_runs`),
			seedCount(t, pool, `SELECT count(*) FROM cabal_txns`),
		}
	}
	before := counts()
	code, stdout, stderr := seedRun(t, pool, a, b)
	if code != 0 || stdout != "seeded two-cabals-ranked: 0 events applied\n" || stderr != "" {
		t.Fatalf("second seed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if after := counts(); after != before {
		t.Fatalf("counts (events, entries, runs, cabal txns) = %v after the second seed, want %v", after, before)
	}
}

func TestSeedCommand_UnknownUser(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, _ := rankedUsers(t, pool)
	unknown := ids.UserID(ids.New[struct{}](ids.Real{}))
	code, _, stderr := seedRun(t, pool, a, unknown)
	if code == 0 || !strings.Contains(stderr, "user "+unknown.String()+" not found") {
		t.Fatalf("seed: code=%d stderr=%q, want a failure naming the unknown user", code, stderr)
	}
	if n := seedCount(t, pool, `SELECT count(*) FROM events`); n != 0 {
		t.Fatalf("events = %d, want none", n)
	}
}

func TestSeedCommand_RefusesProduction(t *testing.T) {
	t.Parallel()
	open := func(context.Context, config.DB) (*pgxpool.Pool, error) {
		t.Error("the command opened a connection")
		return nil, context.Canceled
	}
	var stdout, stderr bytes.Buffer
	args := []string{"two-cabals-ranked", "--users", ids.Real{}.NewV7().String() + "," + ids.Real{}.NewV7().String()}
	if code := seedOn(config.Config{Env: config.EnvProduction}, open, args, &stdout, &stderr); code == 0 {
		t.Fatalf("seed in production: code=%d stderr=%q, want a refusal", code, stderr.String())
	}
}

func TestSeedCommand_RefusesBadArguments(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, b := rankedUsers(t, pool)
	env := opsEnv(pool.Config().ConnString())
	users := a.String() + "," + b.String()
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no arguments":     {nil, seedUsage},
		"flag first":       {[]string{"--users", users}, seedUsage},
		"no users":         {[]string{"two-cabals-ranked"}, seedUsage},
		"unknown flag":     {[]string{"two-cabals-ranked", "--who", users}, seedUsage},
		"user not a uuid":  {[]string{"two-cabals-ranked", "--users", "a,b"}, seedUsage},
		"unknown scenario": {[]string{"no-such-scenario", "--users", users}, "no-such-scenario"},
		"one user":         {[]string{"two-cabals-ranked", "--users", a.String()}, "want 2 users"},
		"three users":      {[]string{"two-cabals-ranked", "--users", users + "," + a.String()}, "want 2 users"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runOps(env, append([]string{"seed"}, tc.args...)...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Fatalf(
					"seed %v: code=%d stdout=%q stderr=%q, want exit 2 and %q",
					tc.args,
					code,
					stdout,
					stderr,
					tc.want,
				)
			}
		})
	}
	if n := seedCount(t, pool, `SELECT count(*) FROM events`); n != 0 {
		t.Fatalf("events = %d, want none", n)
	}
}

func TestSeedTool_failsOnAnUnloadableConfig(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := seedTool(nil)([]string{"two-cabals-ranked"}, &stdout, &stderr); code != 1 {
		t.Fatalf("seed without config: code=%d stderr=%q, want exit 1", code, stderr.String())
	}
}

func TestSeedOn_failsWhenTheDatabaseCannotBeUsed(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		drop string
		want string
	}{
		"cabal_members missing":    {"cabal_members", "cabal_members"},
		"leaderboard_runs missing": {"leaderboard_runs", "value the boards"},
		"cabal_pauses missing":     {"cabal_pauses", "value the boards"},
		"users missing":            {"users CASCADE", "read users"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			a, b := rankedUsers(t, pool)
			dropTable(t, pool, tc.drop)
			cfg, err := config.Load(opsEnv(pool.Config().ConnString()))
			if err != nil {
				t.Fatal(err)
			}
			open := func(ctx context.Context, _ config.DB) (*pgxpool.Pool, error) {
				return pgxpool.New(ctx, pool.Config().ConnString())
			}
			var stdout, stderr bytes.Buffer
			args := []string{"two-cabals-ranked", "--users", a.String() + "," + b.String()}
			if code := seedOn(
				cfg,
				open,
				args,
				&stdout,
				&stderr,
			); code != 1 ||
				!strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("seed: code=%d stderr=%q, want exit 1 mentioning %q", code, stderr.String(), tc.want)
			}
		})
	}
}

func TestSeedOn_failsWhenTheConnectionOpens(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(opsEnv(unreachable))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"two-cabals-ranked", "--users", ids.Real{}.NewV7().String() + "," + ids.Real{}.NewV7().String()}
	if code := seedOn(cfg, func(context.Context, config.DB) (*pgxpool.Pool, error) {
		return nil, context.Canceled
	}, args, &stdout, &stderr); code != 1 {
		t.Fatalf("seed: code=%d stderr=%q, want exit 1", code, stderr.String())
	}
}

func dropTable(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `DROP TABLE `+name); err != nil {
		t.Fatal(err)
	}
}
