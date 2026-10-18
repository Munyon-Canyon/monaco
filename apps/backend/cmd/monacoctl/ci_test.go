package main

import (
	"bytes"
	"context"
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

func TestCIAffectedRejectsBadArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"unknown"}, {"affected"}, {"affected", "--base", "x", "extra"}, {"affected", "--nope"}} {
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
			if got, _ := reverseDeps(byDir, tc.changed); !slices.Equal(got, tc.want) {
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

func TestCIAffectedSeesABackendFlowFileOutsideTheModule(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	module := filepath.Join(repo, "apps", "backend")
	writeCIFile(t, module, "go.mod", "module example.com/m\n\ngo 1.25.0\n")
	writeCIFile(t, module, "leaf/leaf.go", "package leaf\n")
	for _, dir := range flowsReaders() {
		writeCIFile(t, module, dir+"/x.go", "package "+path.Base(dir)+"\n")
	}
	writeCIFile(t, repo, "packages/flows/backend/03.tsv", "old\n")
	git(t, repo, "init", "-q")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "base")
	git(t, repo, "tag", "base")
	writeCIFile(t, repo, "packages/flows/backend/03.tsv", "new\n")
	git(t, repo, "commit", "-q", "-am", "change")
	env := ciEnv{moduleDir: module, goBin: "go", gitBin: "git", exec: runCommand}
	code, stdout, stderr := runCIAffected(env, "affected", "--base", "base")
	want := "./" + strings.Join(slices.Sorted(slices.Values(flowsReaders())), "\n./") + "\n"
	if code != 0 || stdout != want {
		t.Fatalf("code=%d stdout=%q want %q stderr=%s", code, stdout, want, stderr)
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
			"a direct module import selects only the tooling that imports it",
			[]string{"internal/modules/cabal/app.go"},
			[]string{"./cmd/monacoctl", "./internal/modules/cabal"},
			nil,
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
			selected, excluded := reverseDeps(byDir, tc.changed)
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
			with(obs, cabal, []string{"./internal/modules/trading/app"}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, _ := reverseDeps(byDir, tc.changed); !slices.Equal(got, tc.want) {
				t.Fatalf("reverseDeps(%v) = %v, want %v", tc.changed, got, tc.want)
			}
		})
	}
}
