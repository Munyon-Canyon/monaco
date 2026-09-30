package gen_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func TestModule_leavesOneBlankLineBeforeTheNextHeadingWhenTheAddedListWasEmpty(t *testing.T) {
	t.Parallel()
	const head = "# Changelog\n\n## [Unreleased]\n\n### Added\n\n"
	for name, next := range map[string]string{
		"a release heading":  "## [0.1.0]\n\n### Added\n\n- Released.\n",
		"another subsection": "### Changed\n\n- Reworded.\n\n## [0.1.0]\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := tree(t, map[string]string{"go.mod": "module example.com/app\n", "CHANGELOG.md": head + next})
			for _, step := range []struct{ module, entries string }{
				{"wallets", "- The `wallets` module.\n\n"},
				{"ledger", "- The `ledger` module.\n- The `wallets` module.\n\n"},
			} {
				if _, err := gen.Apply(root, "module", step.module); err != nil {
					t.Fatal(err)
				}
				if got, want := read(t, root, "CHANGELOG.md"), head+step.entries+next; got != want {
					t.Fatalf("CHANGELOG.md after %s =\n%s\nwant\n%s", step.module, got, want)
				}
			}
		})
	}
}
