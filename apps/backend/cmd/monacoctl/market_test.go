package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMarketTradable_setsAndClearsTheOverride(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now())
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ('01920000-0000-7000-8000-000000000001', 'AAPLx', 'XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp', 8,
		'xstocks', 'equity', 'Apple xStock', true, 'apple', now(), now())`)
	if err != nil {
		t.Fatal(err)
	}
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	market := marketTool(environ, clk)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"tradable", "AAPLx", "off"}, "AAPLx\ttradable=false\toverride=off\tissuer_tradable=true\n"},
		{[]string{"tradable", "AAPLx", "on"}, "AAPLx\ttradable=true\toverride=on\tissuer_tradable=true\n"},
		{[]string{"tradable", "AAPLx", "auto"}, "AAPLx\ttradable=true\toverride=auto\tissuer_tradable=true\n"},
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
	if toolMarket(toolEnv{}) == nil {
		t.Fatal("toolMarket returned no tool")
	}
}
