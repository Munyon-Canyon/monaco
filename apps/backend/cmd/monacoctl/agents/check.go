package agents

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const (
	excerptLines = 8
	packageKind  = "package"
	openAPISpec  = "apps/backend/api/openapi.yaml"
	vacuumLint   = "dshanley/vacuum:v0.30.6 lint -b -q -n warn -r /api/.vacuum.yaml /api/openapi.yaml"
)

var (
	testFuncRE = regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	shebangRE  = regexp.MustCompile(`^#!(/usr/bin/env\s+|\S*/)(ba|z)?sh\b`)
)

const toolManifestTestGlobs = "scripts/*.sh\nscripts/*/*.sh\nJustfile\n.github/workflows/*.yml"

type checkRow struct {
	label string
	kind  string
	dir   string
	cmds  [][]string
	skip  string
}

type timing struct {
	name   string
	took   time.Duration
	failed bool
}

type checkRun struct {
	env     *Env
	start   time.Time
	log     bytes.Buffer
	timings []timing
}

func parseCheckArgs(base string, args []string) (string, bool, error) {
	fresh := false
	for len(args) > 0 {
		switch {
		case args[0] == "--fresh":
			fresh, args = true, args[1:]
		case len(args) >= 2 && args[0] == "--base":
			base, args = args[1], args[2:]
		default:
			return "", false, usageError("check [--base <ref>] [--fresh]")
		}
	}
	return base, fresh, nil
}

func checkCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	base, fresh, err := parseCheckArgs("origin/"+env.Config.FeatureBranch, args)
	if err != nil {
		return err
	}
	tree, head, err := env.cleanHead(ctx)
	if err != nil {
		return err
	}
	if _, err := os.Stat(env.statePath("checks", tree)); err == nil && !fresh {
		_, _ = fmt.Fprintf(stdout, "stage 0 already passed on tree %s\n", tree[:12])
		return nil
	}
	parent := env.stackParent(ctx, base)
	patchID := env.patchID(ctx, parent)
	if !fresh {
		if carried, err := env.carry(patchID, tree, head, base, parent, stdout); carried || err != nil {
			return err
		}
		ctx, release, err := env.takeSlot(ctx, stdout)
		if err != nil {
			return err
		}
		defer release()
		return env.runStage0(ctx, base, parent, head, tree, patchID, stdout)
	}
	return env.runStage0(ctx, base, parent, head, tree, patchID, stdout)
}

func (env *Env) runStage0(ctx context.Context, base, parent, head, tree, patchID string, stdout io.Writer) error {
	defer func() { _ = os.Remove(env.coverProfile(head)) }()
	rows, err := env.stage0(ctx, base, parent, head)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "stage 0 on tree %s (base %s, parent %s)\n", tree[:12], base, parent)
	run := &checkRun{env: env, start: env.Now()}
	runErr := run.rows(ctx, rows, stdout)
	logPath, err := env.writeState("logs", "check-"+tree[:12]+".log", run.log.Bytes())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "log: %s\n", logPath)
	if runErr != nil {
		return runErr
	}
	record, err := env.writeState("checks", tree, fmt.Appendf(nil, "head %s\nbase %s\n", head, base))
	if err != nil {
		return err
	}
	if patchID != "" {
		if _, err := env.writeState("checks-diff", patchID, []byte(tree+"\n")); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(stdout, "passed in %.1fs; recorded %s\n", env.Now().Sub(run.start).Seconds(), record)
	return nil
}

func (env *Env) carry(patchID, tree, head, base, parent string, stdout io.Writer) (bool, error) {
	old, ok := env.carriedTree(patchID)
	if !ok {
		return false, nil
	}
	record := fmt.Appendf(nil, "head %s\nbase %s\ncarried from %s\n", head, base, old)
	if _, err := env.writeState("checks", tree, record); err != nil {
		return false, err
	}
	_, _ = fmt.Fprintf(stdout, "stage 0 carried from tree %s (same diff against %s)\n", old[:min(12, len(old))], parent)
	return true, nil
}

func (env *Env) patchID(ctx context.Context, parent string) string {
	diff, err := env.Run(ctx, env.Work, "", "git", "diff", parent, "HEAD")
	if err != nil || len(bytes.TrimSpace(diff)) == 0 {
		return ""
	}
	out, err := env.Run(ctx, env.Work, string(diff), "git", "patch-id", "--verbatim")
	id, _, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	if err != nil {
		return ""
	}
	return id
}

func (env *Env) carriedTree(patchID string) (string, bool) {
	if patchID == "" {
		return "", false
	}
	body, err := os.ReadFile(env.statePath("checks-diff", patchID))
	tree := strings.TrimSpace(string(body))
	return tree, err == nil && tree != ""
}

func (env *Env) cleanHead(ctx context.Context) (tree, head string, err error) {
	out, err := env.Run(ctx, env.Work, "", "git", "rev-parse", "HEAD^{tree}", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("read HEAD: %w", err)
	}
	tree, head, _ = strings.Cut(strings.TrimSpace(string(out)), "\n")
	dirty, err := env.Run(ctx, env.Work, "", "git", "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return "", "", fmt.Errorf("read the working tree: %w", err)
	}
	if len(bytes.TrimSpace(dirty)) > 0 {
		return "", "", detailErr(errs.CodeInvalidInput, "monacoctl.agents.check",
			"the working tree differs from HEAD; commit first, since check records HEAD's tree")
	}
	return tree, head, nil
}

func (env *Env) statePath(sub, name string) string {
	return filepath.Join(env.Common, "pstack", env.Config.Milestone, sub, name)
}

func (env *Env) writeState(sub, name string, body []byte) (string, error) {
	p := env.statePath(sub, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	if err := os.WriteFile(p, body, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	return p, nil
}

func (env *Env) stackParent(ctx context.Context, base string) string {
	out, err := env.Run(ctx, env.Work, "", "gt", "parent", "--no-interactive")
	parent, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if err != nil || parent == "" || parent == env.Config.FeatureBranch {
		return base
	}
	return parent
}

func (env *Env) stage0(ctx context.Context, base, parent, head string) ([]checkRow, error) {
	out, err := env.Run(ctx, env.Work, "", "git", "diff", "--name-only", "--diff-filter=d", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", base, err)
	}
	changed := strings.Fields(string(out))
	rows := env.prRows(parent, head)
	if slices.ContainsFunc(changed, func(f string) bool { return strings.HasPrefix(f, "apps/backend/") }) {
		goRows, err := env.goRows(ctx, base, head, changed)
		if err != nil {
			return nil, err
		}
		rows = append(rows, goRows...)
	}
	rows = append(rows, env.shellRows(changed)...)
	rows = append(rows, env.testFileRows(changed)...)
	swift := swiftChanged(changed)
	if swift {
		rows = append(rows, env.swiftRow())
	}
	if slices.ContainsFunc(changed, env.flowFile) {
		row, err := env.flowsRow(ctx, parent, head, swift)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if row, ok := env.xcodeRow(changed); ok {
		rows = append(rows, row)
	}
	return env.pathRows(ctx, rows, changed, parent, head)
}

func swiftChanged(changed []string) bool {
	return slices.ContainsFunc(changed, func(f string) bool {
		return strings.HasPrefix(f, "packages/mobile-core/") || strings.HasPrefix(f, "packages/flows/") ||
			strings.HasPrefix(f, "apps/mobile/") || f == openAPISpec || f == ".swift-format" || f == ".swiftlint.yml" ||
			f == ".swiftlint-baseline.tsv"
	})
}

func (env *Env) swiftRow() checkRow {
	return checkRow{
		label: "swift test", kind: "swift", dir: filepath.Join(env.Work, "packages", "mobile-core"),
		cmds: [][]string{
			{"swift", "format", "lint", "--strict", "--recursive", "--parallel", "../../apps/mobile", "."},
			{"../../scripts/swiftlint-ratchet.sh"},
			{"../../scripts/mobile-core-test.sh"},
		},
	}
}

func (env *Env) flowFile(file string) bool {
	if underAny(
		file,
		[]string{"packages/flows/", "apps/backend/internal/testkit/flows/", flows.TestRoot + "/"},
	) {
		return true
	}
	target, _, found := strings.Cut(strings.TrimPrefix(file, flows.ModelRoot+"/"), "/")
	if !found || !strings.HasPrefix(file, flows.ModelRoot+"/") {
		return false
	}
	modules, _ := os.ReadDir(filepath.Join(env.Work, "apps", "backend", "internal", "modules"))
	return slices.ContainsFunc(
		modules,
		func(m fs.DirEntry) bool { return m.IsDir() && flows.ModuleTarget(m.Name()) == target },
	)
}

func (env *Env) flowsRow(ctx context.Context, parent, head string, swift bool) (checkRow, error) {
	backend := filepath.Join(env.Work, "apps", "backend")
	self, _ := os.Executable()
	out, err := env.Run(ctx, backend, "", self, "flows", "--affected", "--base", parent)
	if err != nil {
		return checkRow{}, fmt.Errorf("find affected flows: %w", err)
	}
	ids := strings.Fields(string(out))
	check := make([]string, 0, 8)
	check = append(check, self, "flows", "check", "--affected", "--base", parent)
	row := checkRow{label: "flows", kind: "flows", dir: backend, cmds: [][]string{check}}
	if len(ids) == 0 {
		return row, nil
	}
	parsed, _, err := flows.ReadAll(os.DirFS(env.Work))
	if err != nil {
		return checkRow{}, err
	}
	var pkgs []string
	for _, f := range parsed {
		if pkg := "./internal/modules/" + f.Module + "/..."; slices.Contains(ids, f.ID) && !slices.Contains(pkgs, pkg) {
			pkgs = append(pkgs, pkg)
		}
	}
	results, err := env.writeState("flows", head[:12]+".json", nil)
	if err != nil {
		return checkRow{}, err
	}
	alternatives := strings.Join(ids, "|")
	row.cmds = [][]string{
		slices.Concat([]string{
			"bash", "-c", `go test -tags faultpoints -json -run "$1" "${@:3}" > "$2" || true`, "flows",
			"^TestFlow(" + alternatives + ")_", results,
		}, pkgs),
		append(check, "--from", results),
	}
	if swift {
		row.cmds = append(row.cmds, []string{
			filepath.Join(
				env.Work,
				"scripts",
				"mobile-core-test.sh",
			),
			"--filter",
			"(F|Flow)(" + alternatives + ")[^a-z0-9]",
		})
	}
	return row, nil
}

func (env *Env) xcodeRow(changed []string) (checkRow, bool) {
	if env.GOOS != "darwin" || !mobileTreeChanged(changed) {
		return checkRow{}, false
	}
	if _, err := env.lookPath("xcodebuild"); err != nil {
		return checkRow{}, false
	}
	cmds := env.installUnlessPresent("xcsift")
	cmds = append(cmds,
		[]string{"bash", "-c", xcodeScript("build-for-testing")},
		[]string{"bash", "-c", xcodeScript("-only-testing:MonacoTests test-without-building")},
	)
	return checkRow{label: "xcode", kind: "xcode", dir: env.Work, cmds: cmds}, true
}

func mobileTreeChanged(changed []string) bool {
	return slices.ContainsFunc(changed, func(f string) bool {
		return strings.HasPrefix(f, "apps/mobile/") || strings.HasPrefix(f, "packages/mobile-core/")
	})
}

func (env *Env) lookPath(name string) (string, error) {
	if env.LookPath != nil {
		return env.LookPath(name)
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("find %s: %w", name, err)
	}
	return found, nil
}

const xcodePrivyGuard = `set -euo pipefail
has_key() { [[ -f "$1" ]] && grep -q '^DOTENV_PRIVATE_KEY_LOCAL=' "$1"; }
primary=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || echo .git)")
if has_key .env.keys || has_key "$primary/.env.keys" || [[ -n ${DOTENV_PRIVATE_KEY_LOCAL:-} ]] || [[ -n ${DOTENV_PRIVATE_KEY:-} ]]; then
  scripts/ensure-ios-privy-config.sh generate
else
  scripts/ensure-ios-privy-config.sh placeholder
fi
`

func xcodeScript(action string) string {
	return xcodePrivyGuard +
		`scripts/qa/xcode-lock.sh xcodebuild -project apps/mobile/Monaco.xcodeproj -scheme Monaco -configuration Debug ` +
		`-destination "platform=iOS Simulator,id=$(scripts/resolve-ios-sim.sh)" ` +
		`-derivedDataPath "$(git rev-parse --show-toplevel)/.build/DerivedData" ` +
		`-onlyUsePackageVersionsFromResolvedFile -skipMacroValidation -skipPackagePluginValidation ` +
		`CODE_SIGNING_ALLOWED=NO ONLY_ACTIVE_ARCH=YES COMPILER_INDEX_STORE_ENABLE=NO COMPILATION_CACHE_ENABLE_CACHING=YES ` +
		action + ` 2>&1 | .bin/xcsift -f toon --exit-on-failure`
}

func (env *Env) prRows(parent, head string) []checkRow {
	vars := []string{"env", "BASE_SHA=" + parent, "HEAD_SHA=" + head, "PR_LABELS=[]", "python3"}
	return []checkRow{
		{label: "pr size", kind: "pr", dir: env.Work, cmds: [][]string{
			append(slices.Clone(vars), "scripts/check-pr-size.py"),
		}},
		{label: "gate changes", kind: "pr", dir: env.Work, cmds: [][]string{
			append(slices.Clone(vars), "scripts/check-gate-changes.py"),
		}},
	}
}

func (env *Env) pathRows(
	ctx context.Context,
	rows []checkRow,
	changed []string,
	parent, head string,
) ([]checkRow, error) {
	for _, r := range []struct {
		paths []string
		build func() (checkRow, error)
	}{
		{
			[]string{"apps/backend/", "packages/flows/", "scripts/ci/ready.sh", "scripts/gen-docs.sh", "scripts/install-sqlc.sh"},
			env.readyRow,
		},
		{
			[]string{
				"apps/backend/migrations/", "apps/backend/atlas.hcl", "apps/backend/.atlas-version",
				"scripts/install-atlas.sh",
			},
			env.migrateRow,
		},
		{
			[]string{
				openAPISpec, "apps/backend/api/.vacuum.yaml", "scripts/ci/oasdiff-breaking.sh",
				"scripts/ci/oasdiff-breaking-test.sh", "scripts/ci/oasdiff-levels.txt",
			},
			func() (checkRow, error) { return env.openAPIRow(ctx, parent, head) },
		},
		{[]string{"docs/", "mkdocs.yml", "requirements-docs.txt", openAPISpec}, env.docsRow},
	} {
		if !slices.ContainsFunc(changed, func(f string) bool { return underAny(f, r.paths) }) {
			continue
		}
		row, err := r.build()
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func underAny(file string, paths []string) bool {
	return slices.ContainsFunc(paths, func(p string) bool {
		return file == p || strings.HasSuffix(p, "/") && strings.HasPrefix(file, p)
	})
}

func (env *Env) readyRow() (checkRow, error) {
	return checkRow{
		label: "ready", kind: "ready", dir: env.Work,
		cmds: append(env.installUnlessPresent("sqlc"), []string{"scripts/ci/ready.sh"}),
	}, nil
}

func (env *Env) migrateRow() (checkRow, error) {
	return checkRow{
		label: "migrate lint", kind: "migrate", dir: filepath.Join(env.Work, "apps", "backend"),
		cmds: append(env.installUnlessPresent("atlas"), []string{"go", "run", "./cmd/monacoctl", "migrate", "lint"}),
	}, nil
}

func (env *Env) installUnlessPresent(tool string) [][]string {
	if isFile(filepath.Join(env.Work, ".bin", tool)) {
		return nil
	}
	return [][]string{{filepath.Join(env.Work, "scripts", "install-"+tool+".sh")}}
}

func isFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}

func (env *Env) openAPIRow(ctx context.Context, parent, head string) (checkRow, error) {
	api := filepath.Join(env.Work, "apps", "backend", "api")
	row := checkRow{label: "openapi", kind: "openapi", dir: env.Work, cmds: [][]string{
		slices.Concat([]string{"docker", "run", "--rm", "-v", api + ":/api:ro"}, strings.Fields(vacuumLint)),
		{"scripts/ci/oasdiff-breaking-test.sh"},
	}}
	spec := env.specAt(ctx, parent)
	if spec == nil {
		return row, nil
	}
	file, err := env.writeState("openapi", head[:12]+".yaml", spec)
	if err != nil {
		return checkRow{}, err
	}
	row.cmds = append(row.cmds, []string{"scripts/ci/oasdiff-breaking.sh", file, openAPISpec})
	return row, nil
}

func (env *Env) specAt(ctx context.Context, ref string) []byte {
	spec, err := env.Run(ctx, env.Work, "", "git", "show", ref+":"+openAPISpec)
	if err != nil {
		return nil
	}
	return spec
}

func (env *Env) docsRow() (checkRow, error) {
	row := checkRow{label: "mkdocs", kind: "docs", dir: env.Work}
	for _, root := range []string{env.Work, filepath.Dir(env.Common)} {
		if bin := filepath.Join(root, ".venv", "bin", "mkdocs"); isFile(bin) {
			row.cmds = [][]string{{"env", "NO_MKDOCS_2_WARNING=true", bin, "build", "--strict", "--site-dir", "site"}}
			return row, nil
		}
	}
	row.skip = "no .venv/bin/mkdocs here or in the main checkout; README's docs site row installs it"
	return row, nil
}

func (env *Env) coverProfile(head string) string {
	return env.statePath("coverage", head[:12]+".out")
}

func (env *Env) goRows(ctx context.Context, base, head string, changed []string) ([]checkRow, error) {
	backend := filepath.Join(env.Work, "apps", "backend")
	self, _ := os.Executable()
	out, err := env.Run(ctx, backend, "", self, "ci", "affected", "--base", base)
	if err != nil {
		return nil, fmt.Errorf("find affected packages: %w", err)
	}
	pkgs := strings.Fields(string(out))
	if len(pkgs) == 0 {
		return nil, nil
	}
	records, err := env.records()
	if err != nil {
		return nil, err
	}
	running := len(slices.DeleteFunc(records, func(r Record) bool { return r.State == Exited }))
	p := strconv.Itoa(testParallelism(runtime.NumCPU(), running))
	tags := []string{"-tags", "faultpoints"}
	lint, err := env.lintRow(ctx, backend, pkgs)
	if err != nil {
		return nil, err
	}
	profile, err := env.writeState("coverage", filepath.Base(env.coverProfile(head)), nil)
	if err != nil {
		return nil, err
	}
	return []checkRow{
		{
			label: "go build", kind: "go", dir: backend,
			cmds: [][]string{slices.Concat([]string{"go", "build"}, tags, buildable(backend, pkgs))},
		},
		{label: "go vet", kind: "go", dir: backend, cmds: [][]string{slices.Concat([]string{"go", "vet"}, tags, pkgs)}},
		lint,
		{
			label: "go test -short", kind: packageKind, dir: backend,
			cmds: [][]string{slices.Concat([]string{"go", "test"}, tags, []string{
				"-short", "-count=1", "-timeout", env.Config.Budget[packageKind].String(), "-p", p, "-json",
				"-coverpkg=" + strings.Join(buildable(backend, pkgs), ","), "-coverprofile=" + profile,
			}, pkgs)},
		},
		coverageRow(backend, self, profile, changed),
	}, nil
}

func coverageRow(backend, self, profile string, changed []string) checkRow {
	row := checkRow{label: "coverage", kind: "go", dir: backend}
	only := map[string]bool{}
	for _, f := range changed {
		rel, ok := strings.CutPrefix(f, "apps/backend/")
		if !ok || !strings.HasSuffix(rel, ".go") || !isSource(rel) {
			continue
		}
		only[rel] = true
		siblings, _ := filepath.Glob(filepath.Join(backend, path.Dir(rel), "*.go"))
		for _, sibling := range siblings {
			if isSource(sibling) {
				only[path.Join(path.Dir(rel), filepath.Base(sibling))] = true
			}
		}
	}
	cmd := make([]string, 0, 4+2*len(only))
	cmd = append(cmd, self, "coverage", "--profile", profile)
	for _, rel := range slices.Sorted(maps.Keys(only)) {
		cmd = append(cmd, "--only", rel)
	}
	if len(cmd) == 4 {
		row.skip = "no Go file outside tests changed under apps/backend"
		return row
	}
	row.cmds = [][]string{cmd}
	return row
}

func (env *Env) lintRow(ctx context.Context, backend string, pkgs []string) (checkRow, error) {
	pin, err := os.ReadFile(filepath.Join(backend, ".golangci-lint-version"))
	if err != nil {
		return checkRow{}, fmt.Errorf("read the golangci-lint pin: %w", err)
	}
	want := strings.TrimSpace(string(pin))
	out, err := env.Run(ctx, backend, "", "golangci-lint", "version", "--short")
	have := "v" + strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	if err != nil {
		have = "none"
	}
	if have != want {
		install := "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@" + want
		return checkRow{}, detailErr(errs.CodeInvalidInput, "monacoctl.agents.check",
			fmt.Sprintf("golangci-lint on PATH is %s and CI pins %s; run: %s", have, want, install))
	}
	return checkRow{label: "go lint", kind: "lint", dir: backend, cmds: [][]string{
		slices.Concat([]string{"golangci-lint", "run"}, pkgs),
		slices.Concat([]string{"go", "run", "./internal/platform/lint/nogo/cmd/nogo"}, pkgs),
		{"go", "run", "./cmd/monacoctl", "lint", "comments"},
	}}, nil
}

func testParallelism(cpus, running int) int {
	return max(2, cpus/max(1, running))
}

func buildable(backend string, pkgs []string) []string {
	return slices.DeleteFunc(slices.Clone(pkgs), func(pkg string) bool {
		files, _ := filepath.Glob(filepath.Join(backend, pkg, "*.go"))
		return len(files) > 0 && !slices.ContainsFunc(files, isSource)
	})
}

func isSource(file string) bool { return !strings.HasSuffix(file, "_test.go") }

func (env *Env) shellRows(changed []string) []checkRow {
	var scripts []string
	for _, f := range changed {
		if strings.HasSuffix(f, ".sh") || env.hasShellShebang(f) {
			scripts = append(scripts, f)
		}
	}
	if len(scripts) == 0 {
		return nil
	}
	syntax := checkRow{label: "bash -n", kind: "shell", dir: env.Work}
	for _, s := range scripts {
		syntax.cmds = append(syntax.cmds, []string{"bash", "-n", s})
	}
	lint := checkRow{
		label: "shellcheck",
		kind:  "shell",
		dir:   env.Work,
		cmds:  [][]string{append([]string{"shellcheck"}, scripts...)},
	}
	return []checkRow{syntax, lint}
}

func (env *Env) hasShellShebang(file string) bool {
	if path.Ext(file) != "" {
		return false
	}
	f, err := os.Open(filepath.Join(env.Work, file))
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	line, _ := bufio.NewReader(f).ReadString('\n')
	return shebangRE.MatchString(line)
}

func (env *Env) testFileRows(changed []string) []checkRow {
	goTests, pyTests := env.affectedTests(changed)
	var rows []checkRow
	if len(goTests) > 0 {
		row := checkRow{label: "scripts tests", kind: "scripts", dir: filepath.Join(env.Work, "scripts")}
		for _, pkg := range slices.Sorted(maps.Keys(goTests)) {
			run := "^(" + strings.Join(goTests[pkg], "|") + ")$"
			row.cmds = append(
				row.cmds,
				[]string{"go", "test", "-short", "-count=1", "-run", run, "./" + strings.TrimPrefix(pkg, ".")},
			)
		}
		rows = append(rows, row)
	}
	if len(pyTests) > 0 {
		rows = append(rows, checkRow{
			label: "python tests", kind: "python", dir: env.Work,
			cmds: [][]string{append([]string{"python3", "-m", "unittest"}, pyTests...)},
		})
	}
	return rows
}

func (env *Env) affectedTests(changed []string) (goTests map[string][]string, pyTests []string) {
	goTests = map[string][]string{}
	scripts := os.DirFS(filepath.Join(env.Work, "scripts"))
	_ = fs.WalkDir(scripts, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Name() == "testdata" {
			return fs.SkipDir
		}
		isGo := strings.HasSuffix(p, "_test.go")
		isPy := strings.HasPrefix(d.Name(), "test_") && strings.HasSuffix(p, ".py")
		if !isGo && !isPy {
			return nil
		}
		body, _ := fs.ReadFile(scripts, p)
		if !slices.ContainsFunc(changed, func(f string) bool {
			return f == "scripts/"+p || bytes.Contains(body, []byte(`"`+path.Base(f)+`"`)) || scannedGlobHit(p, f)
		}) {
			return nil
		}
		if isPy {
			pyTests = append(pyTests, "scripts/"+p)
			return nil
		}
		for _, m := range testFuncRE.FindAllSubmatch(body, -1) {
			goTests[path.Dir(p)] = append(goTests[path.Dir(p)], string(m[1]))
		}
		return nil
	})
	return goTests, pyTests
}

func scannedGlobHit(testFile, changed string) bool {
	globs := map[string][]string{
		"tool_manifest_test.go": strings.Split(toolManifestTestGlobs, "\n"),
	}
	for _, glob := range globs[path.Base(testFile)] {
		if ok, err := path.Match(glob, changed); err == nil && ok {
			return true
		}
	}
	return false
}

func (r *checkRun) rows(ctx context.Context, rows []checkRow, stdout io.Writer) error {
	for _, row := range rows {
		if err := r.row(ctx, row, stdout); err != nil {
			return err
		}
	}
	return nil
}

func (r *checkRun) row(ctx context.Context, row checkRow, stdout io.Writer) error {
	if row.skip != "" {
		_, _ = fmt.Fprintf(stdout, "  %-15s skip  %s\n", row.label, row.skip)
		_, _ = fmt.Fprintf(&r.log, "skip %s: %s\n", row.label, row.skip)
		return nil
	}
	budget := r.env.Config.Budget[row.kind]
	if row.kind != packageKind {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}
	rowStart, first := r.env.Now(), len(r.timings)
	warnings := 0
	for _, cmd := range row.cmds {
		text, err := r.exec(ctx, row, cmd)
		warnings += strings.Count(text, "::warning ")
		rescued, budgetErr := r.checkBudget(ctx, stdout, row, cmd, budget, rowStart, first)
		if budgetErr != nil {
			return budgetErr
		}
		if err != nil && !rescued {
			return failRow(stdout, row, cmd, text, err)
		}
	}
	note := ""
	if warnings > 0 {
		note = fmt.Sprintf("  %d warnings in the log", warnings)
	}
	_, _ = fmt.Fprintf(stdout, "  %-15s ok    %.1fs%s\n", row.label, r.env.Now().Sub(rowStart).Seconds(), note)
	return nil
}

func failRow(stdout io.Writer, row checkRow, cmd []string, text string, err error) error {
	_, _ = fmt.Fprintf(stdout, "  %-15s FAIL  %s\n", row.label, strings.Join(cmd, " "))
	for _, line := range excerpt(text + "\n" + err.Error()) {
		_, _ = fmt.Fprintf(stdout, "    %s\n", line)
	}
	return detailErr(errs.CodeInvalidInput, "monacoctl.agents.check", row.label+" failed; see the log")
}

func (r *checkRun) checkBudget(
	ctx context.Context,
	stdout io.Writer,
	row checkRow,
	cmd []string,
	budget time.Duration,
	rowStart time.Time,
	first int,
) (bool, error) {
	if row.kind == packageKind {
		retried, err := r.retryAlone(ctx, stdout, row, cmd, budget, r.timings[first:])
		if retried || err != nil {
			return retried, err
		}
	}
	return false, r.overBudget(stdout, row, budget, rowStart, r.timings[first:])
}

func (r *checkRun) exec(ctx context.Context, row checkRow, cmd []string) (string, error) {
	start := r.env.Now()
	_, _ = fmt.Fprintf(&r.log, "$ (cd %s && %s)\n", row.dir, strings.Join(cmd, " "))
	out, err := r.env.Run(ctx, row.dir, "", cmd[0], cmd[1:]...)
	text := string(out)
	timings := []timing{{name: cmdName(row, cmd), took: r.env.Now().Sub(start)}}
	if row.kind == packageKind {
		text, timings = goTestTimings(out, r.env.Now())
	}
	r.timings = append(r.timings, timings...)
	r.log.WriteString(text)
	if err != nil {
		_, _ = fmt.Fprintf(&r.log, "%v\n", err)
	}
	return text, err
}

func slowPackages(timings []timing, budget time.Duration) []timing {
	return slices.DeleteFunc(slices.Clone(timings), func(t timing) bool { return t.took < budget })
}

func rerunCmd(cmd []string, pkgs []timing) []string {
	out := make([]string, 0, len(cmd))
	for i, a := range cmd {
		switch {
		case strings.HasPrefix(a, "-coverpkg=") || strings.HasPrefix(a, "-coverprofile=") || strings.HasPrefix(a, "./"):
		case i > 0 && cmd[i-1] == "-p":
			out = append(out, "1")
		default:
			out = append(out, a)
		}
	}
	for _, p := range pkgs {
		out = append(out, p.name)
	}
	return out
}

func (r *checkRun) retryAlone(
	ctx context.Context, stdout io.Writer, row checkRow, cmd []string, budget time.Duration, timings []timing,
) (bool, error) {
	slow := slowPackages(timings, budget)
	if len(slow) == 0 || slices.ContainsFunc(slow, func(t timing) bool { return !strings.HasPrefix(t.name, "./") }) {
		return false, nil
	}
	if slices.ContainsFunc(timings, func(t timing) bool {
		return t.failed && !slices.ContainsFunc(slow, func(s timing) bool { return s.name == t.name })
	}) {
		return false, nil
	}
	first := len(r.timings)
	text, err := r.exec(ctx, row, rerunCmd(cmd, slow))
	again := r.timings[first:]
	if still := slowPackages(again, budget); len(still) > 0 {
		s, orig := slowest(still), slowest(slow)
		_, _ = fmt.Fprintf(stdout, "  %-15s over budget\n", row.label)
		return false, detailErr(errs.CodeUpstreamTimeout, "monacoctl.agents.check", fmt.Sprintf(
			"%s: package %s took %.1fs, over the %s per-package budget; rerun alone: package %s took %.1fs",
			row.label, orig.name, orig.took.Seconds(), budget, s.name, s.took.Seconds()))
	}
	if err != nil {
		return false, failRow(stdout, row, rerunCmd(cmd, slow), text, err)
	}
	load := r.env.load1(ctx)
	for _, t := range again {
		_, _ = fmt.Fprintf(stdout, "  %-15s ok    over budget under load (load1 %s), %s passed alone in %.1fs\n",
			row.label, load, t.name, t.took.Seconds())
	}
	return true, nil
}

func (env *Env) load1(ctx context.Context) string {
	load := env.Load
	if load == nil {
		load = loadAverage
	}
	v, err := load(ctx, env.GOOS)
	if err != nil {
		return "unknown"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func loadAverage(ctx context.Context, goos string) (float64, error) {
	argv := loadCommand(goos)
	raw, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
	if err != nil {
		return 0, fmt.Errorf("read the load average: %w", err)
	}
	return parseLoad(string(raw))
}

func loadCommand(goos string) []string {
	if goos == "darwin" {
		return []string{"sysctl", "-n", "vm.loadavg"}
	}
	return []string{"cat", "/proc/loadavg"}
}

func parseLoad(raw string) (float64, error) {
	first, _, _ := strings.Cut(strings.TrimSpace(strings.Trim(raw, "{} \n")), " ")
	v, err := strconv.ParseFloat(first, 64)
	if err != nil {
		return 0, fmt.Errorf("read the load average: %w", err)
	}
	return v, nil
}

func (r *checkRun) overBudget(
	stdout io.Writer, row checkRow, budget time.Duration, rowStart time.Time, timings []timing,
) error {
	var detail string
	if row.kind == packageKind {
		slow := slices.DeleteFunc(slices.Clone(timings), func(t timing) bool { return t.took < budget })
		if len(slow) == 0 {
			return nil
		}
		s := slowest(slow)
		detail = fmt.Sprintf("%s: package %s took %.1fs, over the %s per-package budget",
			row.label, s.name, s.took.Seconds(), budget)
	} else {
		took := r.env.Now().Sub(rowStart)
		if took <= budget {
			return nil
		}
		s := slowest(timings)
		detail = fmt.Sprintf("%s row over the %s %s budget after %.0fs; slowest: %s (%.1fs)",
			row.label, budget, row.kind, took.Seconds(), s.name, s.took.Seconds())
	}
	_, _ = fmt.Fprintf(stdout, "  %-15s over budget\n", row.label)
	return detailErr(errs.CodeUpstreamTimeout, "monacoctl.agents.check", detail)
}

func slowest(timings []timing) timing {
	return slices.MaxFunc(timings, func(a, b timing) int { return cmp.Compare(a.took, b.took) })
}

func cmdName(row checkRow, cmd []string) string {
	if len(row.cmds) == 1 {
		return row.label
	}
	return row.label + " " + cmd[len(cmd)-1]
}

type testEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
}

func goTestTimings(out []byte, now time.Time) (string, []timing) {
	var text strings.Builder
	started := map[string]time.Time{}
	var order []string
	var timings []timing
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		var e testEvent
		if json.Unmarshal([]byte(line), &e) != nil {
			text.WriteString(line + "\n")
			continue
		}
		text.WriteString(e.Output)
		if e.Test != "" {
			continue
		}
		switch e.Action {
		case "start":
			started[e.Package] = e.Time
			order = append(order, e.Package)
		case "pass", "fail", "skip":
			delete(started, e.Package)
			took := time.Duration(e.Elapsed * float64(time.Second))
			timings = append(timings, timing{name: pkgName(e.Package), took: took, failed: e.Action == "fail"})
		}
	}
	for _, p := range order {
		if at, running := started[p]; running {
			timings = append(timings, timing{name: pkgName(p), took: now.Sub(at)})
		}
	}
	return text.String(), timings
}

func pkgName(importPath string) string {
	if _, rel, ok := strings.Cut(importPath, "/apps/backend/"); ok {
		return "./" + rel
	}
	return importPath
}

func excerpt(text string) []string {
	var lines, fails []string
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		line = strings.TrimRight(line, " \t")
		lines = append(lines, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- FAIL") || strings.HasPrefix(trimmed, "FAIL") ||
			strings.HasPrefix(trimmed, "panic:") || strings.Contains(trimmed, ".go:") {
			fails = append(fails, line)
		}
	}
	if len(fails) > 0 {
		lines = fails
	}
	if len(lines) > excerptLines {
		lines = lines[len(lines)-excerptLines:]
	}
	return lines
}
