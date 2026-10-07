package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func pricesRun(environ []string, history *marketfake.PriceHistoryFake, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := backfillPrices(environ, func(config.Config) app.PriceHistory { return history }, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestBackfillPrices_mintDrainsOneMintAndPrintsTheCounts(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	environ := opsEnv(pool.Config().ConnString())
	aapl := marketfake.AAPLx().Mint
	at := clock.Real{}.Now().UTC().Truncate(24 * time.Hour).Add(-48 * time.Hour)
	history := &marketfake.PriceHistoryFake{}
	history.Put(aapl, 1, app.Sample{At: at.Add(7 * time.Minute), Price: money.MicrosFromUint64(105_000_000)})
	history.Put(aapl, 365, app.Sample{At: at.Add(time.Hour), Price: money.MicrosFromUint64(120_000_000)})

	code, stdout, stderr := pricesRun(environ, history, "--mint", aapl.String())
	if want := "backfill prices: 1 mints, 3 calls, 2 rows inserted\n"; code != 0 || stdout != want || stderr != "" {
		t.Fatalf("backfill prices --mint = %d, stdout %q, stderr %q, want %q", code, stdout, stderr, want)
	}
	var rows, done int
	err := pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM price_points WHERE mint = $1 AND source = 'coingecko'),
		(SELECT count(*) FROM price_backfills WHERE mint = $1 AND done_at IS NOT NULL)`, aapl.String()).Scan(&rows, &done)
	if err != nil || rows != 2 || done != 1 {
		t.Fatalf("rows = %d, done = %d, %v, want 2 coingecko points and the mint done", rows, done, err)
	}
	if code, stdout, _ = pricesRun(environ, history, "--mint", aapl.String()); code != 0 ||
		stdout != "backfill prices: 1 mints, 3 calls, 0 rows inserted\n" {
		t.Fatalf("second run = %d, %q, want the same calls and no new rows", code, stdout)
	}
}

func TestBackfillPrices_allDrainsEveryListedMint(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ('01920000-0000-7000-8000-000000000001', 'AAPLx', 'XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp', 8,
		'xstocks', 'equity', 'Apple xStock', true, 'apple', now(), now(), now()),
		('01920000-0000-7000-8000-000000000002', 'TSLAx', 'XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB', 8,
		'xstocks', 'equity', 'Tesla xStock', true, 'tesla', now(), now(), now()),
		('01920000-0000-7000-8000-000000000003', 'JPSTx', 'XsCAXu7xTaZMG9b9KJhNWYapuvNjxPuE4SysZq8uvMq', 8,
		'xstocks', 'equity', 'JPMorgan xStock', false, 'jpm', now(), now(), now())`)
	if err != nil {
		t.Fatal(err)
	}
	history := &marketfake.PriceHistoryFake{}
	code, stdout, stderr := pricesRun(opsEnv(pool.Config().ConnString()), history, "--all")
	if want := "backfill prices: 2 mints, 6 calls, 0 rows inserted\n"; code != 0 || stdout != want || stderr != "" {
		t.Fatalf("backfill prices --all = %d, stdout %q, stderr %q, want %q", code, stdout, stderr, want)
	}
	if got := len(history.Calls()); got != 6 {
		t.Fatalf("%d calls, want 3 per listed mint, none for the unlisted one", got)
	}
}

func TestBackfillPrices_aFailedDrainPrintsTheCountsAndExitsOne(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	history := &marketfake.PriceHistoryFake{}
	history.Fail("MarketChart", errs.New(errs.CodeCoinGeckoRateLimited, "test"))
	code, stdout, stderr := pricesRun(opsEnv(pool.Config().ConnString()), history,
		"--mint", marketfake.AAPLx().Mint.String())
	if code != 1 || stdout != "backfill prices: 1 mints, 1 calls, 0 rows inserted\n" ||
		!strings.Contains(stderr, "coin_gecko_rate_limited") {
		t.Fatalf("backfill prices = %d, stdout %q, stderr %q, want 1, the counts and the code", code, stdout, stderr)
	}
}

func TestBackfillPrices_refusesBadArgumentsAndAMissingKeyBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	environ := opsEnv(unreachable)
	aapl := marketfake.AAPLx().Mint.String()
	for _, args := range [][]string{
		{}, {"--all", "--mint", aapl}, {"--mint", aapl, "extra"}, {"--bogus"}, {"--all", "extra"},
	} {
		code, stdout, stderr := pricesRun(environ, &marketfake.PriceHistoryFake{}, args...)
		if code != 2 || stdout != "" || stderr != backfillPricesUsage+"\n" {
			t.Fatalf("backfill prices %v = %d, %q, %q, want the usage and 2", args, code, stdout, stderr)
		}
	}
}

func TestBackfillPrices_refusesAMissingKeyABadMintAndABadConfig(t *testing.T) {
	t.Parallel()
	environ := opsEnv(unreachable)
	keyless := &marketfake.PriceHistoryFake{}
	keyless.WithoutKey()
	code, _, stderr := pricesRun(environ, keyless, "--all")
	if code != 1 || stderr != "monacoctl: COINGECKO_API_KEY is not set\n" || len(keyless.Calls()) != 0 {
		t.Fatalf("keyless = %d, stderr %q, want 1 and the missing key", code, stderr)
	}
	if code, _, stderr = pricesRun(environ, &marketfake.PriceHistoryFake{}, "--mint", "nope"); code != 1 ||
		!strings.Contains(stderr, "invalid_address") {
		t.Fatalf("bad mint = %d, stderr %q, want 1 and invalid_address", code, stderr)
	}
	if code, _, stderr = pricesRun([]string{"PATH=/usr/bin"}, &marketfake.PriceHistoryFake{}, "--all"); code != 1 ||
		!strings.Contains(stderr, "missing") {
		t.Fatalf("no config = %d, stderr %q, want 1 and the missing keys", code, stderr)
	}
	if code, _, stderr = pricesRun(environ, &marketfake.PriceHistoryFake{}, "--all"); code != 1 || stderr == "" {
		t.Fatalf("unreachable database = %d, stderr %q, want 1", code, stderr)
	}
}

func TestBackfillPrices_theRealToolRoutesPricesAndNeverPollsWithoutAKey(t *testing.T) {
	t.Parallel()
	environ := opsEnv(unreachable)
	code, _, stderr := runOps(environ, "backfill", "prices")
	if code != 2 || stderr != backfillPricesUsage+"\n" {
		t.Fatalf("backfill prices with no flag = %d, %q, want the prices usage and 2", code, stderr)
	}
	code, stdout, stderr := runOps(environ, "backfill", "prices", "--all")
	if code != 1 || stdout != "" || stderr != "monacoctl: COINGECKO_API_KEY is not set\n" {
		t.Fatalf("backfill prices --all with no key = %d, %q, %q, want 1 and the missing key", code, stdout, stderr)
	}
}
