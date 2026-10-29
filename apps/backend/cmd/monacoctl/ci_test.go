package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func ciModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                          "module example.com/m\n\ngo 1.25.0\n",
		"README.md":                       "m\n",
		"leaf/leaf.go":                    "package leaf\n",
		"shared/shared.go":                "package shared\n\nfunc S() int { return 1 }\n",
		"mid/mid.go":                      "package mid\n\nimport \"example.com/m/shared\"\n\nfunc M() int { return shared.S() }\n",
		"top/top.go":                      "package top\n\nimport \"example.com/m/mid\"\n\nfunc T() int { return mid.M() }\n",
		"user/user.go":                    "package user\n\nimport _ \"errors\"\n",
		"user/user_test.go":               "package user_test\n\nimport \"example.com/m/shared\"\n\nvar _ = shared.S\n",
		"internal/testkit/testkit.go":     "package testkit\n",
		"internal/testkit/flows/flows.go": "package flows\n\nfunc F() int { return 1 }\n",
		"flowuser/flowuser.go":            "package flowuser\n\nimport \"example.com/m/internal/testkit/flows\"\n\nvar _ = flows.F\n",
	}
	for name, body := range files {
		writeCIFile(t, dir, name, body)
	}
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "tag", "base")
	return dir
}

func writeCIFile(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runCIAffected(env ciEnv, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := ciTool(env)(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestCIAffectedPrintsChangedPackagesAndTheirImporters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change map[string]string
		want   string
	}{
		{"leaf change", map[string]string{"leaf/leaf.go": "package leaf\n\nfunc L() {}\n"}, "./leaf\n"},
		{
			"shared-package change reaches importers and packages whose tests import it",
			map[string]string{"shared/shared.go": "package shared\n\nfunc S() int { return 2 }\n"},
			"./mid\n./shared\n./top\n./user\n",
		},
		{"go.mod change", map[string]string{"go.mod": "module example.com/m\n\ngo 1.25.1\n"}, "./...\n"},
		{"go.sum change", map[string]string{"go.sum": ""}, "./...\n"},
		{
			"a testkit flows change reaches its importers, not every package",
			map[string]string{"internal/testkit/flows/flows.go": "package flows\n\nfunc F() int { return 2 }\n"},
			"./flowuser\n./internal/testkit/flows\n",
		},
		{
			"a testkit change with no importers selects only itself",
			map[string]string{"internal/testkit/testkit.go": "package testkit\n\nvar X = 1\n"},
			"./internal/testkit\n",
		},
		{"non-Go change", map[string]string{"README.md": "changed\n"}, "./...\n"},
		{"Go file outside any package", map[string]string{"leaf/testdata/x.go": "package x\n"}, "./...\n"},
		{"no change", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := ciModule(t)
			for name, body := range tc.change {
				writeCIFile(t, dir, name, body)
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")
			env := ciEnv{moduleDir: dir, goBin: "go", gitBin: "git", exec: runCommand}
			code, stdout, stderr := runCIAffected(env, "affected", "--base", "base")
			if code != 0 || stdout != tc.want {
				t.Fatalf("code=%d stdout=%q want %q stderr=%s", code, stdout, tc.want, stderr)
			}
		})
	}
}

func TestCIAffectedScopedPrintsWhetherTheSelectionIsScoped(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change map[string]string
		want   string
	}{
		{"a module-scoped diff", map[string]string{"internal/modules/ranking/app/a.go": "package app\n"}, "true\n"},
		{"a shared seed", map[string]string{"internal/platform/db/uow.go": "package db\n"}, "false\n"},
		{"no change", nil, "false\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := ciModule(t)
			writeCIFile(t, dir, "internal/modules/ranking/app/b.go", "package app\n")
			writeCIFile(t, dir, "internal/platform/db/db.go", "package db\n")
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-q", "-m", "more")
			git(t, dir, "tag", "-f", "base")
			for name, body := range tc.change {
				writeCIFile(t, dir, name, body)
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")
			env := ciEnv{moduleDir: dir, goBin: "go", gitBin: "git", exec: runCommand}
			code, stdout, stderr := runCIAffected(env, "affected", "--base", "base", "--scoped")
			if code != 0 || stdout != tc.want {
				t.Fatalf("code=%d stdout=%q want %q stderr=%s", code, stdout, tc.want, stderr)
			}
		})
	}
}

func TestCIAffectedRejectsBadArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"unknown"}, {"affected"}, {"affected", "--base", "x", "extra"}, {"affected", "--nope"}, {"affected", "--base", "x", "--scoped", "--excluded"}} {
		code, stdout, stderr := runCIAffected(ciEnv{}, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, ciUsage) {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestCIAffectedFailsWhenGitOrGoListFails(t *testing.T) {
	t.Parallel()
	dir := ciModule(t)
	writeCIFile(t, dir, "leaf/leaf.go", "package leaf\n\nfunc L() {}\n")
	git(t, dir, "commit", "-q", "-am", "change")
	fakeGo := func(out string, err error) execFunc {
		return func(ctx context.Context, d string, env []string, name string, args ...string) ([]byte, error) {
			if name == "go" {
				return []byte(out), err
			}
			return runCommand(ctx, d, env, name, args...)
		}
	}
	cases := []struct {
		name, base, want string
		exec             execFunc
	}{
		{"unknown base", "nope", "bad revision", runCommand},
		{"go list fails", "base", "go list broke", fakeGo("", errs.New(errs.CodeInternal, "go list broke"))},
		{"go list prints bad JSON", "base", "decode_failed", fakeGo("{", nil)},
		{
			"the scanner diff fails", "base", "scan diff broke",
			func(ctx context.Context, d string, env []string, name string, args ...string) ([]byte, error) {
				if slices.Contains(args, "-U0") {
					return nil, errs.New(errs.CodeInternal, "scan diff broke")
				}
				return runCommand(ctx, d, env, name, args...)
			},
		},
		{
			"the sentinel search fails", "base", "sentinel broke",
			func(ctx context.Context, d string, env []string, name string, args ...string) ([]byte, error) {
				if slices.Contains(args, "-l") {
					return nil, errs.New(errs.CodeInternal, "sentinel broke")
				}
				return runCommand(ctx, d, env, name, args...)
			},
		},
		{
			"the flow file diff fails", "base", "flow diff broke",
			func(ctx context.Context, d string, env []string, name string, args ...string) ([]byte, error) {
				if slices.Contains(args, ":(top)"+flows.Dir) {
					return nil, errs.New(errs.CodeInternal, "flow diff broke")
				}
				return runCommand(ctx, d, env, name, args...)
			},
		},
	}
	for _, tc := range cases {
		env := ciEnv{moduleDir: dir, goBin: "go", gitBin: "git", exec: tc.exec}
		code, stdout, stderr := runCIAffected(env, "affected", "--base", tc.base)
		if code != 1 || stdout != "" || !strings.Contains(stderr, tc.want) {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.name, code, stdout, stderr)
		}
	}
}

func TestToolCiIsRegistered(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	if code := run(nil, tools(nil), nil, []string{"ci"}, &bytes.Buffer{}, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), ciUsage) {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestReverseDepsSeedsNonGoFilesFromTheirPackage(t *testing.T) {
	t.Parallel()
	const mod = "example.com/m/"
	pkg := func(dir string, imports ...string) listedPackage {
		p := listedPackage{ImportPath: mod + dir}
		for _, i := range imports {
			p.Imports = append(p.Imports, mod+i)
		}
		return p
	}
	byDir := map[string]listedPackage{
		"migrations":                  pkg("migrations"),
		"api":                         pkg("api"),
		"app":                         pkg("app", "migrations", "api"),
		"leaf":                        pkg("leaf"),
		"internal/modules/cabal/sqlc": pkg("internal/modules/cabal/sqlc"),
		"internal/modules/cabal":      pkg("internal/modules/cabal", "internal/modules/cabal/sqlc"),
		"internal/platform/db/sqlc":   pkg("internal/platform/db/sqlc"),
		"internal/tools/flows":        pkg("internal/tools/flows"),
		"internal/testkit/flows":      pkg("internal/testkit/flows"),
		"top":                         pkg("top", "internal/testkit/flows"),
		"cmd/gen":                     pkg("cmd/gen"),
		"cmd/monacoctl":               pkg("cmd/monacoctl"),
		"cmd/monacoctl/agents":        pkg("cmd/monacoctl/agents"),
		"cmd/monacoctl/verify":        pkg("cmd/monacoctl/verify"),
		"internal/tools/gen":          pkg("internal/tools/gen"),
	}
	all := []string{allPackages}
	cases := []struct {
		name    string
		changed []string
		want    []string
	}{
		{
			"migration sql yields its package and importers",
			[]string{"migrations/20260101_x.sql"},
			[]string{"./app", "./migrations"},
		},
		{"openapi yields its package and importers", []string{"api/openapi.yaml"}, []string{"./api", "./app"}},
		{"testdata uses the nearest package ancestor", []string{"leaf/testdata/deep/golden.txt"}, []string{"./leaf"}},
		{
			"module query yields the module sqlc package and importers",
			[]string{"queries/cabal/cabal.sql"},
			[]string{"./internal/modules/cabal", "./internal/modules/cabal/sqlc"},
		},
		{
			"sqlc.yaml yields every sqlc package",
			[]string{"sqlc.yaml"},
			[]string{"./internal/modules/cabal", "./internal/modules/cabal/sqlc", "./internal/platform/db/sqlc"},
		},
		{
			"a backend flow file yields the flow readers",
			[]string{"packages/flows/backend/03.tsv"},
			[]string{
				"./cmd/gen", "./cmd/monacoctl", "./cmd/monacoctl/agents", "./cmd/monacoctl/verify",
				"./internal/testkit/flows", "./internal/tools/flows", "./internal/tools/gen", "./top",
			},
		},
		{"mixed seeds union", []string{"leaf/x.go", "api/openapi.yaml"}, []string{"./api", "./app", "./leaf"}},
		{"query for a module without a sqlc package", []string{"queries/ghost/q.sql"}, all},
		{"query file directly under queries", []string{"queries/q.sql"}, all},
		{"non-Go file outside any package", []string{"scripts/run.sh"}, all},
		{"root file", []string{"README.md"}, all},
		{"go.mod", []string{"go.mod"}, all},
		{"go.sum", []string{"go.sum"}, all},
		{".golangci.yml", []string{".golangci.yml"}, all},
		{
			"a testkit fixture yields its package and importers",
			[]string{"internal/testkit/flows/testdata/x.json"},
			[]string{"./internal/testkit/flows", "./top"},
		},
		{"one fallback among seeds", []string{"leaf/x.go", "go.mod"}, all},
		{"a test file yields only its own package", []string{"migrations/m_test.go"}, []string{"./migrations"}},
		{
			"a test file beside a source file still yields the importers",
			[]string{"migrations/m_test.go", "api/x.go"},
			[]string{"./api", "./app", "./migrations"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, _ := reverseDeps(byDir, tc.changed, nil); !slices.Equal(got, tc.want) {
				t.Fatalf("reverseDeps(%v) = %v, want %v", tc.changed, got, tc.want)
			}
		})
	}
}

func TestFlowsReadersAreEveryPackageReadingTheFlowFiles(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	var found []string
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || path.Base(p) == "ci_test.go" {
			return err
		}
		body, err := fs.ReadFile(root.FS(), p)
		if err == nil && flowReader.Match(body) {
			found = append(found, path.Dir(p))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(found)
	found = slices.Compact(found)
	if want := slices.Sorted(slices.Values(flowsReaders())); !slices.Equal(found, want) {
		t.Fatalf("flowsReaders() = %v, packages calling ReadAll = %v", want, found)
	}
}

var flowReader = regexp.MustCompile(`\bfunc ReadAll\(|\b(?:flows|toolflows|tools)\.ReadAll\(`)

func TestCIAffectedSeesAFlowFileOutsideTheModule(t *testing.T) {
	t.Parallel()
	for _, row := range []string{"packages/flows/backend/03.tsv", "packages/flows/app/03.tsv"} {
		repo := t.TempDir()
		module := filepath.Join(repo, "apps", "backend")
		writeCIFile(t, module, "go.mod", "module example.com/m\n\ngo 1.25.0\n")
		writeCIFile(t, module, "leaf/leaf.go", "package leaf\n")
		for _, dir := range flowsReaders() {
			writeCIFile(t, module, dir+"/x.go", "package "+path.Base(dir)+"\n")
		}
		writeCIFile(t, repo, row, "old\n")
		git(t, repo, "init", "-q")
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-q", "-m", "base")
		git(t, repo, "tag", "base")
		writeCIFile(t, repo, row, "new\n")
		git(t, repo, "commit", "-q", "-am", "change")
		env := ciEnv{moduleDir: module, goBin: "go", gitBin: "git", exec: runCommand}
		code, stdout, stderr := runCIAffected(env, "affected", "--base", "base")
		want := "./" + strings.Join(slices.Sorted(slices.Values(flowsReaders())), "\n./") + "\n"
		if code != 0 || stdout != want {
			t.Fatalf("%s: code=%d stdout=%q want %q stderr=%s", row, code, stdout, want, stderr)
		}
	}
}

func TestAffectedLeavesTheAgentToolingOutUnlessItsOwnInputsChanged(t *testing.T) {
	t.Parallel()
	const mod = "example.com/m/"
	pkg := func(dir string, imports ...string) listedPackage {
		p := listedPackage{ImportPath: mod + dir}
		for _, i := range imports {
			p.Imports = append(p.Imports, mod+i)
		}
		return p
	}
	byDir := map[string]listedPackage{
		"internal/events":         pkg("internal/events"),
		"internal/errs":           pkg("internal/errs"),
		"internal/platform/chain": pkg("internal/platform/chain"),
		"internal/modules/cabal":  pkg("internal/modules/cabal", "internal/events"),
		"internal/tools/flows":    pkg("internal/tools/flows"),
		"cmd/monacoctl": pkg(
			"cmd/monacoctl",
			"internal/events",
			"internal/errs",
			"internal/modules/cabal",
			"internal/platform/chain",
		),
		"cmd/monacoctl/agents": pkg("cmd/monacoctl/agents", "internal/events", "internal/errs"),
		"cmd/monacoctl/verify": pkg(
			"cmd/monacoctl/verify",
			"internal/events",
			"internal/platform/chain",
			"internal/tools/flows",
		),
		"cmd/gen":                pkg("cmd/gen"),
		"internal/testkit/flows": pkg("internal/testkit/flows"),
		"internal/tools/gen":     pkg("internal/tools/gen"),
		"cmd/monacoctl/agents/x": pkg("cmd/monacoctl/agents/x", "internal/errs"),
	}
	tooling := []string{"./cmd/monacoctl", "./cmd/monacoctl/agents", "./cmd/monacoctl/verify"}
	cases := []struct {
		name     string
		changed  []string
		selected []string
		excluded []string
	}{
		{
			"an events change no longer selects the tooling",
			[]string{"internal/events/proposal.go"},
			[]string{"./internal/events", "./internal/modules/cabal"},
			tooling,
		},
		{
			"a platform change no longer selects the tooling",
			[]string{"internal/platform/chain/network.go"},
			[]string{"./internal/platform/chain"},
			[]string{"./cmd/monacoctl", "./cmd/monacoctl/verify"},
		},
		{
			"an events golden selects internal/events only",
			[]string{"internal/events/testdata/golden/vote.cast.v1.json"},
			[]string{"./internal/events"},
			nil,
		},
		{
			"a change in the verify package still selects it and its importers",
			[]string{"cmd/monacoctl/verify/run.go"},
			[]string{"./cmd/monacoctl/verify"},
			nil,
		},
		{
			"a change in agents still selects agents",
			[]string{"cmd/monacoctl/agents/check.go"},
			[]string{"./cmd/monacoctl/agents"},
			nil,
		},
		{
			"a direct import outside platform and events still selects the tooling",
			[]string{"internal/errs/errs.go"},
			[]string{"./cmd/monacoctl", "./cmd/monacoctl/agents", "./cmd/monacoctl/agents/x", "./internal/errs"},
			nil,
		},
		{
			"a module diff leaves the tooling that imports the module excluded",
			[]string{"internal/modules/cabal/app.go"},
			[]string{"./internal/modules/cabal"},
			[]string{"./cmd/monacoctl"},
		},
		{
			"a flow file still selects the flow readers",
			[]string{"packages/flows/backend/03.tsv"},
			[]string{
				"./cmd/gen", "./cmd/monacoctl", "./cmd/monacoctl/agents", "./cmd/monacoctl/verify",
				"./internal/testkit/flows", "./internal/tools/flows", "./internal/tools/gen",
			},
			nil,
		},
		{"go.mod still selects everything", []string{"go.mod"}, []string{allPackages}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			selected, excluded := reverseDeps(byDir, tc.changed, nil)
			if !slices.Equal(selected, tc.selected) || !slices.Equal(excluded, tc.excluded) {
				t.Fatalf("reverseDeps(%v) = %v excluding %v, want %v excluding %v",
					tc.changed, selected, excluded, tc.selected, tc.excluded)
			}
		})
	}
}

func TestAffectedExcludedFlagPrintsWhatTheClosureDropped(t *testing.T) {
	t.Parallel()
	dir := ciModule(t)
	writeCIFile(t, dir, "internal/events/events.go", "package events\n")
	writeCIFile(
		t,
		dir,
		"cmd/monacoctl/main.go",
		"package main\n\nimport _ \"example.com/m/internal/events\"\n\nfunc main() {}\n",
	)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "tooling")
	git(t, dir, "tag", "tooling")
	writeCIFile(t, dir, "internal/events/events.go", "package events\n\nvar E = 1\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "change")
	env := ciEnv{moduleDir: dir, goBin: "go", gitBin: "git", exec: runCommand}
	code, stdout, stderr := runCIAffected(env, "affected", "--base", "tooling", "--excluded")
	if code != 0 || stdout != "./cmd/monacoctl\n" {
		t.Fatalf("code=%d stdout=%q stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr = runCIAffected(env, "affected", "--base", "tooling")
	if code != 0 || strings.Contains(stdout, "cmd/monacoctl") || !strings.Contains(stdout, "./internal/events") {
		t.Fatalf("code=%d stdout=%q stderr=%s", code, stdout, stderr)
	}
}

func TestReverseDepsSeedsPerModuleSharedFilesFromTheirModule(t *testing.T) {
	t.Parallel()
	const mod = "example.com/m/"
	pkg := func(dir string, imports ...string) listedPackage {
		p := listedPackage{ImportPath: mod + dir}
		for _, i := range imports {
			p.Imports = append(p.Imports, mod+i)
		}
		return p
	}
	const obs, errsPkg, events = "internal/platform/observability", "internal/errs", "internal/events"
	byDir := map[string]listedPackage{
		obs:                              pkg(obs),
		errsPkg:                          pkg(errsPkg),
		events:                           pkg(events),
		"internal/modules/trading":       pkg("internal/modules/trading", obs, errsPkg, events),
		"internal/modules/trading/app":   pkg("internal/modules/trading/app", obs),
		"internal/modules/cabal":         pkg("internal/modules/cabal", "internal/modules/trading", obs),
		"internal/modules/cabal/adapter": pkg("internal/modules/cabal/adapter", obs),
		"cmd/api":                        pkg("cmd/api", "internal/modules/cabal", obs, errsPkg, events),
	}
	trading := []string{"./internal/modules/trading", "./internal/modules/trading/app"}
	cabal := []string{"./internal/modules/cabal", "./internal/modules/cabal/adapter"}
	with := func(shared string, groups ...[]string) []string {
		out := []string{"./" + shared}
		for _, g := range groups {
			out = append(out, g...)
		}
		slices.Sort(out)
		return out
	}
	fullObs := []string{
		"./cmd/api", "./internal/modules/cabal", "./internal/modules/cabal/adapter",
		"./internal/modules/trading", "./internal/modules/trading/app", "./" + obs,
	}
	fullErrs := []string{"./cmd/api", "./" + errsPkg, "./internal/modules/cabal", "./internal/modules/trading"}
	fullEvents := []string{"./cmd/api", "./" + events, "./internal/modules/cabal", "./internal/modules/trading"}
	cases := []struct {
		name    string
		changed []string
		want    []string
	}{
		{"a module message file", []string{obs + "/msgs_trading.go"}, with(obs, trading)},
		{"a module generated message file", []string{obs + "/msgs_trading.gen.go"}, with(obs, trading)},
		{"a message file named for a module area", []string{obs + "/msgs_trading_stranded.go"}, with(obs, trading)},
		{
			"two modules' message files",
			[]string{obs + "/msgs_trading.go", obs + "/msgs_cabal.go"},
			with(obs, trading, cabal),
		},
		{"a message file without a module", []string{obs + "/msgs_bus.go"}, fullObs},
		{"another file in observability", []string{obs + "/observability.go"}, fullObs},
		{"a message test file", []string{obs + "/msgs_trading_test.go"}, []string{"./" + obs}},
		{"a module error code file", []string{errsPkg + "/codes_trading.go"}, with(errsPkg, trading)},
		{"a module error code golden", []string{errsPkg + "/testdata/codes_trading.golden"}, with(errsPkg, trading)},
		{
			"an error code file named for a module area",
			[]string{errsPkg + "/codes_cabal_blocks.go"},
			with(errsPkg, cabal),
		},
		{"an error code file with no module", []string{errsPkg + "/codes_platform.go"}, with(errsPkg, trading, cabal)},
		{
			"an error code golden with no module",
			[]string{errsPkg + "/testdata/codes_platform.golden"},
			with(errsPkg, trading, cabal),
		},
		{"another file in errs", []string{errsPkg + "/errs.go"}, fullErrs},
		{"a module event registrations file", []string{events + "/trading_registrations.go"}, with(events, trading)},
		{"an event registrations file with no module", []string{events + "/ghost_registrations.go"}, fullEvents},
		{"another file in events", []string{events + "/fund.go"}, fullEvents},
		{
			"a module file beside a shared module file",
			[]string{obs + "/msgs_cabal.go", "internal/modules/trading/app/x.go"},
			with(obs, cabal, trading),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, _ := reverseDeps(byDir, tc.changed, nil); !slices.Equal(got, tc.want) {
				t.Fatalf("reverseDeps(%v) = %v, want %v", tc.changed, got, tc.want)
			}
		})
	}
}

func TestCIAffectedSelectsSourceScanningTests(t *testing.T) {
	t.Parallel()
	const call = "package a\n\nimport \"example.com/m/internal/platform/observability\"\n\nfunc A() { observability.Info(\"x\", 1) }\n"
	cases := []struct {
		name   string
		change map[string]string
		want   string
	}{
		{
			"a changed observability call selects the registry package and the sentinel package",
			map[string]string{"internal/modules/a/a.go": strings.Replace(call, "1)", "2)", 1)},
			"./internal/modules/a\n./internal/platform/observability\n./scanner\n",
		},
		{
			"a module change with no observability call selects only the sentinel package",
			map[string]string{"internal/modules/b/b.go": "package b\n\nfunc B() int { return 2 }\n"},
			"./internal/modules/b\n./scanner\n",
		},
		{
			"a non-Go change in a package does not select the sentinel package",
			map[string]string{"internal/modules/b/data.json": "{}\n"},
			"./internal/modules/b\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := ciModule(t)
			for name, body := range map[string]string{
				"internal/platform/observability/observability.go": "package observability\n\nfunc Info(string, int) {}\n",
				"internal/modules/a/a.go":                          call,
				"internal/modules/b/b.go":                          "package b\n\nfunc B() int { return 1 }\n",
				"scanner/scanner.go":                               "package scanner\n",
				"scanner/scanner_test.go":                          "package scanner\n\nconst scansModuleSource = true\n\nvar _ = scansModuleSource\n",
			} {
				writeCIFile(t, dir, name, body)
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-q", "-m", "scanners")
			git(t, dir, "tag", "scanners")
			for name, body := range tc.change {
				writeCIFile(t, dir, name, body)
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-q", "-m", "change")
			env := ciEnv{moduleDir: dir, goBin: "go", gitBin: "git", exec: runCommand}
			code, stdout, stderr := runCIAffected(env, "affected", "--base", "scanners")
			if code != 0 || stdout != tc.want {
				t.Fatalf("code=%d stdout=%q want %q stderr=%s", code, stdout, tc.want, stderr)
			}
		})
	}
}

func specGraph() map[string]listedPackage {
	const mod = "example.com/m/"
	pkg := func(dir string, imports, tests []string) listedPackage {
		p := listedPackage{ImportPath: mod + dir}
		for _, i := range imports {
			p.Imports = append(p.Imports, mod+i)
		}
		for _, i := range tests {
			p.TestImports = append(p.TestImports, mod+i)
		}
		return p
	}
	const (
		rankingAPI = "internal/platform/httpx/api/rankingapi"
		fundingAPI = "internal/platform/httpx/api/fundingapi"
		apiAll     = "internal/platform/httpx/api/apiall"
		scenario   = "internal/testkit/scenario"
		httpx      = "internal/platform/httpx"
	)
	return map[string]listedPackage{
		"api":                          pkg("api", nil, nil),
		httpx:                          pkg(httpx, nil, nil),
		rankingAPI:                     pkg(rankingAPI, []string{"api"}, nil),
		fundingAPI:                     pkg(fundingAPI, []string{"api"}, nil),
		apiAll:                         pkg(apiAll, []string{rankingAPI, fundingAPI, httpx}, nil),
		"internal/testkit":             pkg("internal/testkit", []string{"api", apiAll}, nil),
		scenario:                       pkg(scenario, []string{"internal/testkit", "api", apiAll}, nil),
		"internal/modules/ranking":     pkg("internal/modules/ranking", []string{rankingAPI}, []string{scenario}),
		"internal/modules/ranking/app": pkg("internal/modules/ranking/app", nil, nil),
		"internal/modules/social": pkg(
			"internal/modules/social", []string{"internal/modules/ranking"}, []string{scenario},
		),
		"internal/modules/funding":  pkg("internal/modules/funding", []string{fundingAPI}, []string{scenario}),
		"internal/modules/treasury": pkg("internal/modules/treasury", nil, []string{scenario}),
		"cmd/monacoctl":             pkg("cmd/monacoctl", []string{"api", apiAll}, nil),
		"cmd/monacoctl/agents":      pkg("cmd/monacoctl/agents", []string{"api", apiAll}, nil),
		"cmd/monacoctl/verify":      pkg("cmd/monacoctl/verify", []string{"api", apiAll}, nil),
	}
}

func TestReverseDepsSelectsASpecChangeFromItsModule(t *testing.T) {
	t.Parallel()
	selected, excluded := reverseDeps(specGraph(), []string{
		"api/spec/ranking.yaml", "api/openapi.yaml",
		"internal/platform/httpx/api/rankingapi/api.gen.go",
	}, nil)
	want := []string{
		"./api", "./internal/modules/ranking", "./internal/modules/ranking/app", "./internal/modules/social",
		"./internal/platform/httpx/api/apiall", "./internal/platform/httpx/api/rankingapi",
		"./internal/testkit", "./internal/testkit/scenario",
	}
	if !slices.Equal(selected, want) {
		t.Fatalf("selected %v, want %v", selected, want)
	}
	if len(excluded) != 0 {
		t.Fatalf("excluded %v", excluded)
	}
}

func TestReverseDepsSpecChangeWithoutAModulePackageKeepsTheBundleSeed(t *testing.T) {
	t.Parallel()
	byDir := specGraph()
	selected, _ := reverseDeps(byDir, []string{"api/spec/ghost.yaml"}, nil)
	if !slices.Contains(selected, "./internal/modules/funding") || !slices.Contains(selected, "./api") {
		t.Fatalf("selected %v", selected)
	}
	delete(byDir, "api")
	if got, _ := reverseDeps(byDir, []string{"api/spec/ranking.yaml"}, nil); !slices.Equal(got, []string{allPackages}) {
		t.Fatalf("selected %v", got)
	}
}

func TestReverseDepsSelectsEveryImporterOfAHandEditedBundle(t *testing.T) {
	t.Parallel()
	selected, _ := reverseDeps(specGraph(), []string{"api/openapi.yaml"}, nil)
	for _, dir := range []string{"./internal/modules/funding", "./internal/modules/treasury", "./cmd/monacoctl"} {
		if !slices.Contains(selected, dir) {
			t.Fatalf("selected %v without %s", selected, dir)
		}
	}
}

func TestReverseDepsSelectsEveryModuleForTheBaseSpec(t *testing.T) {
	t.Parallel()
	selected, _ := reverseDeps(specGraph(), []string{"api/spec/base.yaml"}, nil)
	for _, dir := range []string{"./internal/modules/funding", "./internal/modules/treasury", "./internal/modules/ranking"} {
		if !slices.Contains(selected, dir) {
			t.Fatalf("selected %v without %s", selected, dir)
		}
	}
}

func TestReverseDepsSelectsEveryModuleForAHarnessOrSharedChange(t *testing.T) {
	t.Parallel()
	for _, changed := range []string{"internal/testkit/scenario/app.go", "internal/platform/httpx/problem.go"} {
		selected, _ := reverseDeps(specGraph(), []string{changed}, nil)
		for _, dir := range []string{"./internal/modules/funding", "./internal/modules/treasury", "./internal/modules/ranking"} {
			if !slices.Contains(selected, dir) {
				t.Fatalf("%s selected %v without %s", changed, selected, dir)
			}
		}
	}
}

func flowSeedGraph() map[string]listedPackage {
	const mod = "example.com/m/"
	pkg := func(dir string, imports ...string) listedPackage {
		p := listedPackage{ImportPath: mod + dir}
		for _, i := range imports {
			p.Imports = append(p.Imports, mod+i)
		}
		return p
	}
	byDir := map[string]listedPackage{
		"internal/testkit/fakes":    pkg("internal/testkit/fakes"),
		"internal/testkit/scenario": pkg("internal/testkit/scenario"),
		"internal/testkit":          pkg("internal/testkit", "internal/testkit/fakes", "internal/testkit/scenario"),
		"internal/testkit/flows":    pkg("internal/testkit/flows", "internal/testkit"),
		"internal/tools/flows":      pkg("internal/tools/flows"),
		"cmd/gen":                   pkg("cmd/gen", "internal/tools/flows"),
		"cmd/monacoctl":             pkg("cmd/monacoctl", "internal/tools/flows", "internal/testkit/flows"),
		"cmd/monacoctl/agents":      pkg("cmd/monacoctl/agents", "internal/tools/flows"),
		"cmd/monacoctl/verify":      pkg("cmd/monacoctl/verify", "internal/tools/flows"),
		"internal/tools/gen":        pkg("internal/tools/gen", "internal/tools/flows"),
	}
	for _, m := range []string{"cabal", "notify", "funding"} {
		byDir["internal/modules/"+m] = pkg("internal/modules/"+m, "internal/testkit/flows")
		byDir["internal/modules/"+m+"/app"] = pkg("internal/modules/"+m+"/app", "internal/testkit/flows")
	}
	return byDir
}

func TestReverseDepsScopesAFlowsFilesToItsModules(t *testing.T) {
	t.Parallel()
	byDir := flowSeedGraph()
	parsed := []flows.Flow{{ID: "34", Module: "cabal", Consumers: []string{"notify.dispatch", "notify.other", "bare"}}}
	cabalNotify := []string{
		"./internal/modules/cabal", "./internal/modules/cabal/app",
		"./internal/modules/notify", "./internal/modules/notify/app",
	}
	everyModule := slices.Concat(cabalNotify, []string{"./internal/modules/funding", "./internal/modules/funding/app"})
	sorted := func(groups ...[]string) []string {
		out := slices.Concat(groups...)
		slices.Sort(out)
		return out
	}
	tooling := []string{"./cmd/monacoctl"}
	cases := []struct {
		name         string
		changed      []string
		parsed       []flows.Flow
		want, wantEx []string
	}{
		{
			"a script, its generated file and its tsv row",
			[]string{
				"internal/testkit/flows/f34.go", "internal/testkit/flows/f34.gen.go", flows.Dir + "/34.tsv",
			},
			parsed,
			sorted(cabalNotify, []string{
				"./cmd/gen", "./internal/testkit/flows", "./internal/tools/flows", "./internal/tools/gen",
			}),
			[]string{"./cmd/monacoctl", "./cmd/monacoctl/agents", "./cmd/monacoctl/verify"},
		},
		{
			"a named fixture",
			[]string{"internal/testkit/fakes/testdata/fakes/privy/v1/users/qa-f34-ok.json"},
			parsed, sorted(cabalNotify, []string{"./internal/testkit/fakes"}), nil,
		},
		{
			"the scenario harness",
			[]string{"internal/testkit/scenario/app.go"},
			parsed,
			sorted(everyModule, []string{
				"./internal/testkit", "./internal/testkit/flows", "./internal/testkit/scenario",
			}),
			tooling,
		},
		{
			"a fixture with no flow id",
			[]string{"internal/testkit/fakes/testdata/fakes/privy/v1/users/other.json"},
			parsed,
			sorted(everyModule, []string{"./internal/testkit", "./internal/testkit/fakes", "./internal/testkit/flows"}),
			tooling,
		},
		{
			"a fixture whose flow id is only a prefix of another",
			[]string{"internal/testkit/fakes/testdata/fakes/privy/v1/users/qa-f340-ok.json"},
			parsed,
			sorted(everyModule, []string{"./internal/testkit", "./internal/testkit/fakes", "./internal/testkit/flows"}),
			tooling,
		},
		{
			"a script of an unknown flow",
			[]string{"internal/testkit/flows/f99.go"},
			parsed,
			sorted(everyModule, []string{"./cmd/monacoctl", "./internal/testkit/flows"}),
			nil,
		},
		{
			"a script without parsed flows",
			[]string{"internal/testkit/flows/f34.go"},
			nil,
			sorted(everyModule, []string{"./cmd/monacoctl", "./internal/testkit/flows"}),
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, excluded := reverseDeps(byDir, tc.changed, tc.parsed)
			if !slices.Equal(got, tc.want) || !slices.Equal(excluded, tc.wantEx) {
				t.Fatalf("reverseDeps(%v) = %v excluding %v, want %v excluding %v",
					tc.changed, got, excluded, tc.want, tc.wantEx)
			}
		})
	}
}

func TestCIChangedFlowsReadsTheFlowsOfTheRepo(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeCIFile(t, repo, flows.Dir+"/01a.tsv", flows.Header+"\n"+
		"01a\tSet handle\tidentity\tPUT /v1/me/handle\tSetHandle\tuser.profile_updated\t"+
		"social.feed.profile_updated\tok;HandleInvalid\tverified\tdocs/architecture/auth.md#handle\n")
	empty := t.TempDir()
	top := func(dir string) execFunc {
		return func(context.Context, string, []string, string, ...string) ([]byte, error) {
			return []byte(dir + "\n"), nil
		}
	}
	failing := func(context.Context, string, []string, string, ...string) ([]byte, error) {
		return nil, errors.New("no git")
	}
	changed := []string{flows.Dir + "/01a.tsv"}
	if got := (ciEnv{exec: top(repo)}).changedFlows(
		context.Background(),
		changed,
	); len(got) != 1 ||
		got[0].Module != "identity" {
		t.Fatalf("changedFlows = %v, want the identity flow", got)
	}
	if got := (ciEnv{exec: top(empty)}).changedFlows(context.Background(), changed); got != nil {
		t.Fatalf("changedFlows with no flow files = %v, want nil", got)
	}
	if got := (ciEnv{exec: failing}).changedFlows(context.Background(), changed); got != nil {
		t.Fatalf("changedFlows with a failing git = %v, want nil", got)
	}
	if got := (ciEnv{exec: failing}).changedFlows(context.Background(), []string{"leaf/leaf.go"}); got != nil {
		t.Fatalf("changedFlows with no flow file changed = %v, want nil", got)
	}
}

func TestModuleOf(t *testing.T) {
	t.Parallel()
	cabalQuery := []string{"queries/cabal/members.sql"}
	cases := []struct {
		name    string
		f       string
		changed []string
		want    string
		ok      bool
	}{
		{"a module package file", "internal/modules/cabal/app/x.go", nil, "cabal", true},
		{"a module root file", "internal/modules/cabal/module.go", nil, "cabal", true},
		{"a file directly under modules", "internal/modules/doc.go", nil, "", false},
		{"a module query", "queries/cabal/members.sql", nil, "cabal", true},
		{"a query directly under queries", "queries/q.sql", nil, "", false},
		{"a module api file", "internal/platform/httpx/api/cabalapi/api.gen.go", nil, "cabal", true},
		{"the aggregate api package", "internal/platform/httpx/api/apiall/all.go", nil, "", false},
		{"a file named like a module api", "internal/platform/httpx/api/cabalapi", nil, "", false},
		{"an api package with no module", "internal/platform/httpx/api/api/x.go", nil, "", false},
		{"a migration beside a module query", "migrations/1_x.sql", cabalQuery, "cabal", true},
		{"atlas.sum beside a module query", "migrations/atlas.sum", cabalQuery, "cabal", true},
		{
			"a migration beside a module sqlc file", "migrations/1_x.sql",
			[]string{"internal/modules/cabal/sqlc/a.go"},
			"cabal", true,
		},
		{
			"a migration beside a module app file", "migrations/1_x.sql",
			[]string{"internal/modules/cabal/app/a.go"},
			"", false,
		},
		{
			"a migration beside a module file without a subdirectory", "migrations/1_x.sql",
			[]string{"internal/modules/cabal"},
			"", false,
		},
		{
			"a migration beside two modules", "migrations/1_x.sql",
			[]string{"queries/cabal/a.sql", "queries/funding/b.sql"},
			"", false,
		},
		{
			"a migration beside one module twice", "migrations/1_x.sql",
			[]string{"queries/cabal/a.sql", "internal/modules/cabal/sqlc/a.go"},
			"cabal", true,
		},
		{"a migration with no module", "migrations/1_x.sql", []string{"migrations/1_x.sql"}, "", false},
		{"a go file in migrations", "migrations/m.go", cabalQuery, "", false},
		{"a shared file", "internal/platform/db/uow.go", cabalQuery, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, ok := moduleOf(tc.f, tc.changed); got != tc.want || ok != tc.ok {
				t.Fatalf("moduleOf(%q, %v) = %q, %v; want %q, %v", tc.f, tc.changed, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func moduleScopedGraph() map[string]listedPackage {
	const mod = "example.com/m/"
	pkg := func(dir string, imports ...string) listedPackage {
		p := listedPackage{ImportPath: mod + dir}
		if dir == "." {
			p.ImportPath = strings.TrimSuffix(mod, "/")
		}
		for _, i := range imports {
			if i == "." {
				p.Imports = append(p.Imports, strings.TrimSuffix(mod, "/"))
				continue
			}
			p.Imports = append(p.Imports, mod+i)
		}
		return p
	}
	const (
		apiAll   = "internal/platform/httpx/api/apiall"
		api      = "internal/platform/httpx/api/"
		db       = "internal/platform/db"
		testkit  = "internal/testkit"
		scenario = "internal/testkit/scenario"
		kitFlows = "internal/testkit/flows"
		modules  = "internal/modules/"
	)
	byDir := map[string]listedPackage{
		".":                pkg("."),
		db:                 pkg(db, "."),
		"api":              pkg("api"),
		api + "cabalapi":   pkg(api+"cabalapi", "api"),
		api + "rankingapi": pkg(api+"rankingapi", "api"),
		api + "fundingapi": pkg(api+"fundingapi", "api"),
		apiAll:             pkg(apiAll, api+"cabalapi", api+"rankingapi", api+"fundingapi"),
		testkit: pkg(
			testkit,
			modules+"cabal/sqlc",
			modules+"ranking",
			modules+"funding/sqlc",
			apiAll,
		),
		scenario:                   pkg(scenario, testkit, apiAll),
		kitFlows:                   pkg(kitFlows, testkit),
		"internal/testkit/fakes":   pkg("internal/testkit/fakes"),
		"cmd/api":                  pkg("cmd/api", apiAll, modules+"cabal", modules+"ranking", modules+"funding"),
		"cmd/worker":               pkg("cmd/worker", modules+"cabal", modules+"ranking", modules+"funding"),
		"cmd/monacoctl":            pkg("cmd/monacoctl", modules+"cabal", kitFlows),
		"cmd/monacoctl/agents":     pkg("cmd/monacoctl/agents", modules+"cabal"),
		"cmd/monacoctl/verify":     pkg("cmd/monacoctl/verify", modules+"cabal"),
		"cmd/gen":                  pkg("cmd/gen"),
		"internal/tools/flows":     pkg("internal/tools/flows"),
		"internal/tools/gen":       pkg("internal/tools/gen"),
		modules + "cabal/sqlc":     pkg(modules + "cabal/sqlc"),
		modules + "cabal/app":      pkg(modules+"cabal/app", modules+"cabal/sqlc"),
		modules + "cabal/domain":   pkg(modules + "cabal/domain"),
		modules + "cabal/adapters": pkg(modules+"cabal/adapters", modules+"cabal/app"),
		modules + "cabal":          pkg(modules+"cabal", modules+"cabal/app", modules+"cabal/sqlc", api+"cabalapi", db),
		modules + "ranking/domain": pkg(modules + "ranking/domain"),
		modules + "ranking/app":    pkg(modules+"ranking/app", modules+"ranking/domain"),
		modules + "ranking":        pkg(modules+"ranking", modules+"ranking/app", api+"rankingapi", db),
		modules + "funding/sqlc":   pkg(modules + "funding/sqlc"),
		modules + "funding": pkg(
			modules+"funding",
			modules+"funding/sqlc",
			api+"fundingapi",
			db,
			modules+"cabal",
		),
		modules + "treasury": pkg(modules+"treasury", db, modules+"cabal/domain"),
		modules + "admin":    pkg(modules+"admin", modules+"ranking", modules+"cabal", db),
		modules + "social":   pkg(modules+"social", modules+"ranking", db),
		modules + "notify":   pkg(modules+"notify", modules+"ranking/app", db),
	}
	return byDir
}

func TestReverseDepsScopesACabalMigrationAndQueriesToCabal(t *testing.T) {
	t.Parallel()
	selected, excluded := reverseDeps(moduleScopedGraph(), []string{
		"migrations/20261029045644_cabal_member_sort.sql", "migrations/atlas.sum",
		"queries/cabal/members.sql", "internal/modules/cabal/sqlc/members.sql.gen.go",
	}, nil)
	want := []string{
		"./cmd/api", "./cmd/worker",
		"./internal/modules/admin", "./internal/modules/cabal", "./internal/modules/cabal/adapters",
		"./internal/modules/cabal/app", "./internal/modules/cabal/domain", "./internal/modules/cabal/sqlc",
		"./internal/modules/funding", "./internal/modules/treasury",
		"./internal/platform/httpx/api/cabalapi",
		"./internal/testkit", "./internal/testkit/flows", "./internal/testkit/scenario",
	}
	if !slices.Equal(selected, want) {
		t.Fatalf("selected %v, want %v", selected, want)
	}
	wantExcluded := []string{"./cmd/monacoctl", "./cmd/monacoctl/agents", "./cmd/monacoctl/verify"}
	if !slices.Equal(excluded, wantExcluded) {
		t.Fatalf("excluded %v, want %v", excluded, wantExcluded)
	}
}

func TestReverseDepsScopesAModuleAppFileToItsImportingModules(t *testing.T) {
	t.Parallel()
	selected, _ := reverseDeps(moduleScopedGraph(), []string{"internal/modules/ranking/app/portfolio.go"}, nil)
	want := []string{
		"./cmd/api", "./cmd/worker",
		"./internal/modules/admin", "./internal/modules/notify", "./internal/modules/ranking",
		"./internal/modules/ranking/app", "./internal/modules/ranking/domain", "./internal/modules/social",
		"./internal/platform/httpx/api/rankingapi",
		"./internal/testkit", "./internal/testkit/flows", "./internal/testkit/scenario",
	}
	if !slices.Equal(selected, want) {
		t.Fatalf("selected %v, want %v", selected, want)
	}
}

func TestReverseDepsKeepsEverythingForAnUnresolvedMigrationOrASharedFile(t *testing.T) {
	t.Parallel()
	for _, changed := range [][]string{
		{"migrations/20261029045644_x.sql", "migrations/atlas.sum"},
		{"migrations/20261029045644_x.sql", "queries/cabal/a.sql", "queries/funding/b.sql"},
		{"internal/platform/db/uow.go"},
		{"internal/modules/cabal/app/x.go", "internal/platform/db/uow.go"},
	} {
		selected, _ := reverseDeps(moduleScopedGraph(), changed, nil)
		for _, dir := range []string{
			"./internal/modules/funding", "./internal/modules/treasury", "./internal/modules/ranking",
			"./internal/testkit/scenario", "./cmd/api",
		} {
			if !slices.Contains(selected, dir) {
				t.Fatalf("%v selected %v without %s", changed, selected, dir)
			}
		}
	}
}

func TestReverseDepsFallsBackToEverythingWhenAModuleFileHasNoPackage(t *testing.T) {
	t.Parallel()
	for _, changed := range [][]string{
		{"internal/modules/cabal/ghost/x.go"},
		{"migrations/1_x.sql", "queries/ghost/q.sql"},
	} {
		if got, _ := reverseDeps(moduleScopedGraph(), changed, nil); !slices.Equal(got, []string{allPackages}) {
			t.Fatalf("%v selected %v", changed, got)
		}
	}
}

func TestReverseDepsSelectsACabalRouteTicketWithItsImportingModules(t *testing.T) {
	t.Parallel()
	changed := []string{
		"api/openapi.yaml", "api/spec/cabal.yaml", "api/spec/ranking.yaml",
		"internal/modules/cabal/adapters/http.go", "internal/modules/cabal/app/reorder_cabals.go",
		"internal/modules/cabal/domain/cabal_order.go", "internal/modules/cabal/domain/cabal_order_test.go",
		"internal/modules/cabal/flow34_test.go", "internal/modules/cabal/http_test.go",
		"internal/modules/cabal/module.go", "internal/modules/cabal/reorder_cabals_test.go",
		"internal/modules/cabal/sqlc/cabals.sql.gen.go", "internal/modules/cabal/sqlc/members.sql.gen.go",
		"internal/modules/cabal/sqlc/models.gen.go",
		"internal/modules/ranking/app/portfolio_order_test.go", "internal/modules/ranking/domain/portfolio.go",
		"internal/modules/ranking/domain/portfolio_test.go",
		"internal/platform/httpx/api/cabalapi/api.gen.go",
		"internal/testkit/fakes/testdata/fakes/privy/v1/users/qa-f34-invalid.json",
		"internal/testkit/fakes/testdata/fakes/privy/v1/users/qa-f34-ok.json",
		"internal/testkit/flows/f34.gen.go", "internal/testkit/flows/f34.go",
		"migrations/20261029045644_cabal_member_sort.sql", "migrations/atlas.sum",
		"queries/cabal/cabals.sql", "queries/cabal/members.sql",
		flows.Dir + "/34.tsv",
	}
	parsed := []flows.Flow{{ID: "34", Module: "cabal"}}
	selected, excluded := reverseDeps(moduleScopedGraph(), changed, parsed)
	if len(selected) > 85 {
		t.Fatalf("selected %d packages: %v", len(selected), selected)
	}
	for _, dir := range []string{
		"./internal/modules/cabal", "./internal/modules/cabal/sqlc", "./internal/modules/ranking/domain",
		"./internal/platform/httpx/api/cabalapi", "./internal/modules/admin",
		"./internal/testkit/flows", "./internal/testkit/scenario",
	} {
		if !slices.Contains(selected, dir) {
			t.Fatalf("selected %v without %s", selected, dir)
		}
	}
	for _, dir := range []string{"./internal/modules/funding", "./internal/modules/treasury"} {
		if !slices.Contains(selected, dir) {
			t.Fatalf("selected %v without %s", selected, dir)
		}
	}
	wantExcluded := []string{"./cmd/monacoctl", "./cmd/monacoctl/agents", "./cmd/monacoctl/verify"}
	if !slices.Equal(excluded, wantExcluded) {
		t.Fatalf("excluded %v, want %v", excluded, wantExcluded)
	}
}
