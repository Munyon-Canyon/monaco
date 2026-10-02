package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestMarketTradable_setsAndClearsTheOverride(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now())
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ('01920000-0000-7000-8000-000000000001', 'AAPLx', 'XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp', 8,
		'xstocks', 'equity', 'Apple xStock', true, 'apple', now(), now(), now()),
		('01920000-0000-7000-8000-000000000002', 'TSLAx', 'XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB', 8,
		'xstocks', 'equity', 'Tesla xStock', true, 'tesla', now(), now(), NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	market := marketTool(environ, clk)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{
			[]string{"tradable", "AAPLx", "off"},
			"AAPLx\ttradable=false\toverride=off\tissuer_tradable=true\tchain_checked=true\n",
		},
		{
			[]string{"tradable", "AAPLx", "on"},
			"AAPLx\ttradable=true\toverride=on\tissuer_tradable=true\tchain_checked=true\n",
		},
		{
			[]string{"tradable", "AAPLx", "auto"},
			"AAPLx\ttradable=true\toverride=auto\tissuer_tradable=true\tchain_checked=true\n",
		},
		{
			[]string{"tradable", "TSLAx", "on"},
			"TSLAx\ttradable=false\toverride=on\tissuer_tradable=true\tchain_checked=false\n",
		},
	} {
		var stdout, stderr bytes.Buffer
		if code := market(tc.args, &stdout, &stderr); code != 0 || stdout.String() != tc.want {
			t.Fatalf("market %v = %d, stdout %q, stderr %q, want %q", tc.args, code, stdout.String(), stderr.String(),
				tc.want)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := market([]string{"tradable", "NOPEx", "off"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "asset_not_found") {
		t.Fatalf("unknown symbol = %d, stderr %q, want 1 and asset_not_found", code, stderr.String())
	}
}

func TestMarketTradable_refusesBadArgumentsAndAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1",
		"NATS_URL=nats://unused",
	}
	market := marketTool(environ, clock.Real{})
	for _, args := range [][]string{{"tradable"}, {"tradable", "AAPLx"}, {"tradable", "AAPLx", "maybe"}} {
		var stdout, stderr bytes.Buffer
		if code := market(args, &stdout, &stderr); code != 2 || stderr.String() != marketUsage+"\n" {
			t.Fatalf("market %v = %d, stderr %q, want the usage and 2", args, code, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := market([]string{"tradable", "AAPLx", "off"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "db_unavailable") {
		t.Fatalf("unreachable database = %d, stderr %q, want 1 and db_unavailable", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := market([]string{"thin-prices"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "db_unavailable") {
		t.Fatalf("thin-prices unreachable database = %d, stderr %q, want 1 and db_unavailable", code, stderr.String())
	}
	if toolMarket(toolEnv{}) == nil {
		t.Fatal("toolMarket returned no tool")
	}
}

func TestMarketThinPrices_deletesOldDuplicates(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Hour)
	aapl := marketfake.AAPLx().Mint.String()
	old := now.Add(-10 * 24 * time.Hour)
	_, err := pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source) VALUES
		($1, $2, 1, 'jupiter'), ($1, $3, 2, 'coingecko'), ($1, $4, 3, 'jupiter')`,
		aapl, old, old.Add(time.Minute), now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	market := marketTool(environ, testkit.NewClock(now))
	var stdout, stderr bytes.Buffer
	if code := market([]string{"thin-prices"}, &stdout, &stderr); code != 0 || stdout.String() != "deleted=1\n" {
		t.Fatalf("thin-prices = %d, stdout %q, stderr %q, want deleted=1", code, stdout.String(), stderr.String())
	}
	var left int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM price_points`).Scan(&left); err != nil || left != 2 {
		t.Fatalf("rows left = %d (%v), want the earliest old row and the recent row", left, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := market([]string{"thin-prices", "now"}, &stdout, &stderr); code != 2 ||
		stderr.String() != marketUsage+"\n" {
		t.Fatalf("thin-prices now = %d, stderr %q, want the usage and 2", code, stderr.String())
	}
	readOnly := pool.Config().ConnString()
	if strings.Contains(readOnly, "?") {
		readOnly += "&default_transaction_read_only=on"
	} else {
		readOnly += "?default_transaction_read_only=on"
	}
	market = marketTool([]string{"MONACO_ENV=test", "DATABASE_URL=" + readOnly, "NATS_URL=nats://unused"}, clock.Real{})
	stdout.Reset()
	stderr.Reset()
	if code := market([]string{"thin-prices"}, &stdout, &stderr); code != 1 || stderr.Len() == 0 {
		t.Fatalf("read-only thin-prices = %d, stderr %q, want 1", code, stderr.String())
	}
}

func TestMarketThinPrices_skipsWhenTheWorkerHoldsTheLock(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Hour)
	aapl := marketfake.AAPLx().Mint.String()
	old := now.Add(-10 * 24 * time.Hour)
	_, err := pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source) VALUES
		($1, $2, 1, 'jupiter'), ($1, $3, 2, 'jupiter')`, aapl, old, old.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	lock := db.NewLock(pool, "poller:market.retention")
	held, err := lock.Hold(t.Context())
	if err != nil || !held {
		t.Fatalf("Hold = %v, %v, want the worker's lock", held, err)
	}
	t.Cleanup(func() { _ = lock.Release(context.Background()) })
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	var stdout, stderr bytes.Buffer
	code := marketTool(environ, testkit.NewClock(now))([]string{"thin-prices"}, &stdout, &stderr)
	if code != 0 || stdout.String() != "skipped: the worker holds the lock\n" || stderr.Len() != 0 {
		t.Fatalf("thin-prices = %d, stdout %q, stderr %q, want the skip line", code, stdout.String(), stderr.String())
	}
	var left int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM price_points`).Scan(&left); err != nil || left != 2 {
		t.Fatalf("rows left = %d (%v), want both samples", left, err)
	}
}

func TestMarketThinPrices_reportsALockError(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	market := marketToolWithLock(environ, clock.Real{}, func(context.Context, *pgxpool.Pool) (bool, func(), error) {
		return false, nil, errors.New("lock failed")
	})
	var stdout, stderr bytes.Buffer
	code := market([]string{"thin-prices"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "lock failed") {
		t.Fatalf("thin-prices = %d, stdout %q, stderr %q, want the lock error", code, stdout.String(), stderr.String())
	}
}

func TestHoldRetention_reportsAClosedPool(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	closed, err := pgxpool.NewWithConfig(t.Context(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	held, release, err := holdRetention(t.Context(), closed)
	if held || release != nil || err == nil {
		t.Fatalf("holdRetention held=%v release=%t err=%v, want an error and no lock", held, release != nil, err)
	}
}
