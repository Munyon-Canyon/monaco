package main

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestReplay_verifyPassesOnASeededTreasuryLedgerAndFailsOnPlantedDrift(t *testing.T) {
	t.Parallel()
	const mint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	g := testkit.NewIDs(11)
	user, err := ids.ParseUserID(g.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	cabal, err := ids.ParseCabalID(g.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		plant  string
		code   int
		stdout string
	}{
		"seeded": {code: 0, stdout: "replayed 0 events: 0 applied, 0 duplicate\nverify: 0 diffs\n"},
		"drifted": {
			plant: `UPDATE cabal_positions SET units = units + 1 WHERE asset = '` + mint + `'`,
			code:  1,
			stdout: "replayed 0 events: 0 applied, 0 duplicate\n" +
				"ledger treasury: cabal_positions " + cabal.String() + " " + mint + ": entries 4, position 5\n" +
				"verify: 1 diffs\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := testkit.DB(t)
			testkit.NewLedger(t, source).
				WithFundedMember(user, cabal, money.MicrosFromUint64(25_000_000)).
				WithHolding(cabal, mint, money.NewBaseUnits(4, 8))
			if tc.plant != "" {
				if _, err := source.Exec(t.Context(), tc.plant); err != nil {
					t.Fatal(err)
				}
			}
			t.Run("target", func(t *testing.T) {
				t.Parallel()
				env, into := opsEnv(source.Config().ConnString()), testkit.DB(t).Config().ConnString()
				code, stdout, stderr := runOps(env, "replay", "--into", into, "--verify")
				if code != tc.code || stdout != tc.stdout || stderr != "" {
					t.Fatalf("code=%d stdout=%q stderr=%q, want %d and %q", code, stdout, stderr, tc.code, tc.stdout)
				}
				t.Logf("monacoctl replay --verify on the %s ledger:\n%s", name, stdout)
			})
		})
	}
}
