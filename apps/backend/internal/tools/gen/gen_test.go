package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

const changelog = "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- Old line.\n\n## [0.1.0]\n\n### Added\n\n- Released.\n"

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestModule_writesTheSkeletonAndAnUnreleasedChangelogLine(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{"go.mod": "module example.com/app\n\ngo 1.25\n", "CHANGELOG.md": changelog})
	touched, err := gen.Apply(root, "module", "wallets")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/modules/wallets/adapters/adapters.go",
		"internal/modules/wallets/app/app.go",
		"internal/modules/wallets/domain/domain.go",
		"internal/modules/wallets/main_test.go",
		"internal/modules/wallets/module.go",
		"queries/wallets/.gitkeep",
		"CHANGELOG.md",
	}
	if strings.Join(touched, "\n") != strings.Join(want, "\n") {
		t.Fatalf("touched\n%s\nwant\n%s", strings.Join(touched, "\n"), strings.Join(want, "\n"))
	}
	if got := read(
		t,
		root,
		"CHANGELOG.md",
	); got != strings.Replace(
		changelog,
		"- Old line.",
		"- The `wallets` module.\n- Old line.",
		1,
	) {
		t.Fatalf("CHANGELOG.md =\n%s", got)
	}
	mod := read(t, root, "internal/modules/wallets/module.go")
	for _, want := range []string{"package wallets\n", `"example.com/app/internal/platform/module"`, `return "wallets"`} {
		if !strings.Contains(mod, want) {
			t.Errorf("module.go lacks %q:\n%s", want, mod)
		}
	}
	if got := read(t, root, "internal/modules/wallets/main_test.go"); !strings.Contains(got, "testkit.Main(m)") {
		t.Errorf("main_test.go does not run testkit.Main:\n%s", got)
	}
}

func TestModule_refusesToOverwriteAndLeavesTheTreeUntouched(t *testing.T) {
	t.Parallel()
	root := tree(t, map[string]string{
		"go.mod":                             "module example.com/app\n",
		"CHANGELOG.md":                       changelog,
		"internal/modules/wallets/module.go": "package wallets\n",
	})
	if _, err := gen.Apply(root, "module", "wallets"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already exists", err)
	}
	if got := read(t, root, "CHANGELOG.md"); got != changelog {
		t.Fatalf("a refused generate edited CHANGELOG.md:\n%s", got)
	}
}

func TestModule_rejectsBadInput(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		files map[string]string
		args  []string
		want  string
	}{
		"upper case name": {map[string]string{"go.mod": "module m\n", "CHANGELOG.md": changelog}, []string{"Wallets"}, "must match"},
		"underscore":      {map[string]string{"go.mod": "module m\n", "CHANGELOG.md": changelog}, []string{"my_wallets"}, "must match"},
		"no module line":  {map[string]string{"go.mod": "go 1.25\n", "CHANGELOG.md": changelog}, []string{"wallets"}, "no module line"},
		"no unreleased":   {map[string]string{"go.mod": "module m\n", "CHANGELOG.md": "# Changelog\n"}, []string{"wallets"}, "Unreleased"},
		"no added list": {
			map[string]string{"go.mod": "module m\n", "CHANGELOG.md": "## [Unreleased]\n\n## [0.1.0]\n\n### Added\n\n"},
			[]string{"wallets"},
			"no ### Added",
		},
		"wrong arity": {map[string]string{"go.mod": "module m\n"}, nil, "usage: gen module <name>"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := tree(t, tc.files)
			if _, err := gen.Apply(root, "module", tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if _, err := os.Stat(filepath.Join(root, "internal")); !os.IsNotExist(err) {
				t.Fatalf("a rejected generate wrote files: %v", err)
			}
		})
	}
}
