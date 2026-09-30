package gen_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func requireOneWalletsBlock(t *testing.T, got string) {
	t.Helper()
	for _, want := range []string{
		"  # BEGIN GENERATED modules\n  - engine: postgresql\n    schema: migrations\n    queries: queries/wallets\n",
		"      out: internal/modules/wallets/sqlc\n",
		"          go_type: time.Time\n  # END GENERATED modules\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sqlc.yaml lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "queries: queries/wallets\n"); n != 1 {
		t.Errorf("sqlc.yaml has %d wallets blocks, want 1:\n%s", n, got)
	}
}

func TestSqlc_writesTheBlockOfAModuleWhoseFirstQueryWasWrittenByHand(t *testing.T) {
	t.Parallel()
	const mergedBlock = "  - engine: postgresql\n    queries: queries/wallets\n    queries: queries/wallets\n"
	for name, start := range map[string]string{
		"no block yet":        sqlcYAML,
		"a hand-merged block": strings.Replace(sqlcYAML, "  # END GENERATED", mergedBlock+"  # END GENERATED", 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := stubBackend(t, map[string]string{
				"apps/backend/sqlc.yaml":                          start,
				"apps/backend/internal/modules/wallets/module.go": "package wallets\n",
				"apps/backend/queries/wallets/get_wallet.sql":     "-- name: GetWallet :one\nSELECT 1;\n",
			})
			if _, err := run(t, root, "sqlc"); err != nil {
				t.Fatal(err)
			}
			got := read(t, root, "sqlc.yaml")
			requireOneWalletsBlock(t, got)
			if _, err := run(t, root, "sqlc"); err != nil {
				t.Fatal(err)
			}
			if again := read(t, root, "sqlc.yaml"); again != got {
				t.Fatalf("a second run changed sqlc.yaml:\n%s", again)
			}
		})
	}
}

func TestSqlc_takesNoArgumentsAndNamesAConfigWithoutMarkers(t *testing.T) {
	t.Parallel()
	g, _ := gen.Find("sqlc")
	if got := g.Usage(); got != "gen sqlc" {
		t.Fatalf("Usage() = %q, want %q", got, "gen sqlc")
	}
	_, err := run(t, stubBackend(t, nil), "sqlc", "wallets")
	if err == nil || !strings.Contains(err.Error(), "usage: gen sqlc") {
		t.Fatalf("err = %v, want the usage line", err)
	}
	unmarked := stubBackend(t, map[string]string{"apps/backend/sqlc.yaml": "version: \"2\"\n"})
	_, err = run(t, unmarked, "sqlc")
	if err == nil || !strings.Contains(err.Error(), "# BEGIN GENERATED modules") {
		t.Fatalf("err = %v, want the missing marker named", err)
	}
}
