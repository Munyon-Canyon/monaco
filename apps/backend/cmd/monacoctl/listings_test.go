package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type listingsFake struct {
	failing  map[string]bool
	decimals uint8
}

func (f *listingsFake) Verify(_ context.Context, l app.ListingToVerify) (uint8, error) {
	if f.failing[l.TokenAddress] {
		return 0, errs.Wrap(fmt.Errorf("%s symbol() differs", l.TokenAddress), errs.CodeInvalidAddress, "test.Verify")
	}
	return f.decimals, nil
}

func forgetVerification(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `UPDATE asset_listings SET verified_at = NULL, decimals = NULL, enabled = false
		WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func TestListings_verifiesAndOverridesListings(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	g := testkit.NewIDs(4969)
	good, bad := g.NewV7(), g.NewV7()
	if _, err := pool.Exec(t.Context(), `DELETE FROM asset_listings`); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(
		t.Context(),
		`INSERT INTO asset_listings (id, issuer, symbol, network, chain_id, token_address, shares_per_token, venue,
		enabled, created_at) VALUES
		($1, 'coinbase', 'AAPLc', 'base', 8453, '0xb200000000000000000000c2e324d24d7eecd1fb', 1, 'uniswap', false, now()),
		($2, 'coinbase', 'TSLAc', 'base', 8453, '0xb2000000000000000000001e800a7f5189430cd0', 1, 'uniswap', false, now())`,
		good,
		bad,
	)
	if err != nil {
		t.Fatal(err)
	}
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	verifier := &listingsFake{failing: map[string]bool{"0xb2000000000000000000001e800a7f5189430cd0": true}, decimals: 8}
	listings := listingsTool(environ, testkit.NewClock(clock.Real{}.Now()), func(config.Config) app.ListingVerifier {
		return verifier
	})
	failed := fmt.Sprintf("failed\t%s\tcoinbase\tTSLAc\tbase\ttest.Verify: invalid_address: "+
		"0xb2000000000000000000001e800a7f5189430cd0 symbol() differs\n", bad)
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{
			[]string{"verify"},
			1,
			fmt.Sprintf("verified\t%s\tcoinbase\tAAPLc\tbase\tdecimals=8\n", good) + failed + "verified=1 failed=1\n",
		},
		{
			[]string{"override", good.String(), "off"},
			0,
			fmt.Sprintf("%s\tcoinbase\tAAPLc\tbase\toverride=off\tenabled=false\n", good),
		},
		{
			[]string{"override", good.String(), "auto"},
			0,
			fmt.Sprintf("%s\tcoinbase\tAAPLc\tbase\toverride=auto\tenabled=true\n", good),
		},
		{
			[]string{"override", bad.String(), "on"},
			0,
			fmt.Sprintf("%s\tcoinbase\tTSLAc\tbase\toverride=on\tenabled=false\n", bad),
		},
	} {
		var stdout, stderr bytes.Buffer
		if code := listings(tc.args, &stdout, &stderr); code != tc.code || stdout.String() != tc.want {
			t.Fatalf("listings %v = %d, stdout %q, stderr %q, want %d and %q", tc.args, code, stdout.String(),
				stderr.String(), tc.code, tc.want)
		}
	}
	delete(verifier.failing, "0xb2000000000000000000001e800a7f5189430cd0")
	var stdout, stderr bytes.Buffer
	if code := listings([]string{"verify", "--all"}, &stdout, &stderr); code != 0 ||
		!strings.HasSuffix(stdout.String(), "verified=2 failed=0\n") {
		t.Fatalf("verify --all = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	verifier.decimals = 40
	forgetVerification(t, pool, good)
	stderr.Reset()
	if code := listings([]string{"verify"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "market.ListingSync.Verify") {
		t.Fatalf("verify storing 40 decimals = %d, stderr %q, want the check constraint refused", code,
			stderr.String())
	}
	stderr.Reset()
	if code := listings([]string{"override", g.NewV7().String(), "on"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "not_found") {
		t.Fatalf("unknown listing = %d, stderr %q, want 1 and not_found", code, stderr.String())
	}
}

func TestListings_refusesBadArgumentsAndAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1",
		"NATS_URL=nats://unused",
	}
	listings := listingsTool(environ, clock.Real{}, func(config.Config) app.ListingVerifier { return &listingsFake{} })
	id := testkit.NewIDs(1).NewV7().String()
	for _, args := range [][]string{
		{"verify", "--some"},
		{"verify", "--all", "--all"},
		{"override", id},
		{"override", "not-a-uuid", "on"},
		{"override", id, "maybe"},
	} {
		var stdout, stderr bytes.Buffer
		if code := listings(args, &stdout, &stderr); code != 2 || stderr.String() != listingsUsage+"\n" {
			t.Fatalf("listings %v = %d, stderr %q, want the usage and 2", args, code, stderr.String())
		}
	}
	for _, args := range [][]string{{"verify"}, {"override", id, "on"}} {
		var stdout, stderr bytes.Buffer
		if code := listings(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "db_unavailable") {
			t.Fatalf("listings %v = %d, stderr %q, want 1 and db_unavailable", args, code, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := toolListings(toolEnv{environ: environ})([]string{"verify"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "db_unavailable") {
		t.Fatalf("live verify = %d, stderr %q, want the live verifier built and 1 with db_unavailable", code,
			stderr.String())
	}
}
