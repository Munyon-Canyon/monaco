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
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

	class  string
	dbCmds func(db testDB) [][]string
	prep   [][]string

	lockWaited string
}

const (
	xcodeKind     = "xcode"
	lockHoldVar   = "MONACO_LOCK_HOLD="
	xcodeLockWait = 90 * time.Minute
	lockWaitedDir = "xcode-lock"
)

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
	patchID, generated := env.patchID(ctx, parent)
	if !fresh {
		if carried, err := env.carry(patchID, tree, head, base, parent, generated, stdout); carried || err != nil {
			return err
		}
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return env.runStage0(ctx, base, parent, head, tree, patchID, stdout)
}

func (env *Env) runStage0(
	ctx context.Context, base, parent, head, tree, patchID string, stdout io.Writer,
) error {
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

func (env *Env) carry(patchID, tree, head, base, parent string, generated []string, stdout io.Writer) (bool, error) {
	old, ok := env.carriedTree(patchID)
	if !ok {
		return false, nil
	}
	record := fmt.Appendf(nil, "head %s\nbase %s\ncarried from %s\n", head, base, old)
	if _, err := env.writeState("checks", tree, record); err != nil {
		return false, err
	}
	_, _ = fmt.Fprintf(stdout, "stage 0 carried from tree %s (same diff against %s)\n", old[:min(12, len(old))], parent)
	if len(generated) > 0 {
		_, _ = fmt.Fprintf(
			stdout, "carry key ignored %d generated files: %s\n", len(generated), strings.Join(generated, ", "),
		)
	}
	return true, nil
}

func (env *Env) patchID(ctx context.Context, parent string) (string, []string) {
	globs := env.generatedGlobs()
	spec := make([]string, 0, 2+len(globs))
	spec = append(spec, "--", ".")
	for _, g := range globs {
		spec = append(spec, ":(exclude,glob)"+g)
	}
	diff, err := env.Run(ctx, env.Work, "", "git", append([]string{"diff", parent, "HEAD"}, spec...)...)
	if err != nil || len(bytes.TrimSpace(diff)) == 0 {
		return "", nil
	}
	out, err := env.Run(ctx, env.Work, string(diff), "git", "patch-id", "--verbatim")
	id, _, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	if err != nil {
		return "", nil
	}
	var generated []string
	if len(globs) > 0 {
		only := []string{"diff", "--name-only", parent, "HEAD", "--"}
		for _, g := range globs {
			only = append(only, ":(glob)"+g)
		}
		if names, err := env.Run(ctx, env.Work, "", "git", only...); err == nil {
			generated = strings.Fields(string(names))
		}
	}
	return id, generated
}

func (env *Env) generatedGlobs() []string {
	raw, err := os.ReadFile(filepath.Join(env.Work, ".gitattributes"))
	if err != nil {
		return nil
	}
	var globs []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) == 1 || slices.Contains(fields[1:], "linguist-generated") {
			globs = append(globs, fields[0])
		}
	}
	return globs
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
		parent = base
	}
	mb, err := env.Run(ctx, env.Work, "", "git", "merge-base", "HEAD", parent)
	if sha := strings.TrimSpace(string(mb)); err == nil && sha != "" {
		return sha
	}
	return parent
}

func (env *Env) diffNames(ctx context.Context, base, filter string) ([]string, error) {
	out, err := env.Run(ctx, env.Work, "", "git", "diff", "--name-only", "--diff-filter="+filter, base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", base, err)
	}
	return strings.Fields(string(out)), nil
}

func (env *Env) stage0(ctx context.Context, base, parent, head string) ([]checkRow, error) {
	changed, err := env.diffNames(ctx, base, "d")
	if err != nil {
		return nil, err
	}
	removed, err := env.diffNames(ctx, base, "DR")
	if err != nil {
		return nil, err
	}
	rows := env.prRows(parent, head)
	backend := slices.ContainsFunc(changed, func(f string) bool { return strings.HasPrefix(f, "apps/backend/") })
	if backend {
		goRows, err := env.goRows(ctx, base, head, changed)
		if err != nil {
			return nil, err
		}
		rows = append(rows, goRows...)
	}
	rows = append(rows, env.shellRows(changed)...)
	rows = append(rows, env.testFileRows(changed, len(removed) > 0)...)
	swift := swiftChanged(changed)
	if swift {
		rows = append(rows, env.swiftRow(parent))
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
	if journeyChanged(changed) {
		rows = append(rows, checkRow{
			label: "journeys", kind: "journeys", dir: env.Work,
			cmds: [][]string{
				{"python3", "scripts/qa/journey.py", "check"},
				{"python3", "scripts/qa/test_journey.py"},
				{"python3", "scripts/qa/test_skill_eval.py"},
			},
		})
	}
	return env.pathRows(ctx, rows, changed, parent, head)
}

func journeyChanged(changed []string) bool {
	return slices.ContainsFunc(changed, func(file string) bool {
		return underAny(file, []string{"apps/mobile/", "docs/journeys/", "apps/mobile/qa/", "scripts/qa/"})
	})
}

func swiftChanged(changed []string) bool {
	return slices.ContainsFunc(changed, func(f string) bool {
		return strings.HasPrefix(f, "packages/mobile-core/") || strings.HasPrefix(f, "packages/flows/") ||
			strings.HasPrefix(f, "apps/mobile/") || f == openAPISpec || f == ".swift-format" || f == ".swiftlint.yml"
	})
}

func (env *Env) swiftRow(parent string) checkRow {
	waited := env.statePath(lockWaitedDir, strconv.Itoa(os.Getpid())+".swift.waited")
	return checkRow{
		label: "swift test", kind: "swift", class: "cpu", dir: filepath.Join(env.Work, "packages", "mobile-core"),
		cmds: [][]string{
			{"swift", "format", "lint", "--strict", "--recursive", "--parallel", "../../apps/mobile", "."},
			{"../../scripts/swiftlint-ratchet.sh", "--base", parent},
			lockWaitedCmd(waited, "../../scripts/mobile-core-test.sh"),
		},
		lockWaited: waited,
	}
}

func lockWaitedCmd(waited string, cmd ...string) []string {
	return append([]string{"env", "MONACO_LOCK_WAITED=" + waited}, cmd...)
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
		if !slices.Contains(ids, f.ID) {
			continue
		}
		for _, module := range flowModules(f) {
			if pkg := "./internal/modules/" + module + "/..."; !slices.Contains(pkgs, pkg) {
				pkgs = append(pkgs, pkg)
			}
		}
	}
	results, err := env.writeState("flows", head[:12]+".json", nil)
	if err != nil {
		return checkRow{}, err
	}
	alternatives := strings.Join(ids, "|")
	row.class, row.cmds = "db", nil
	row.dbCmds = func(db testDB) [][]string {
		cmds := [][]string{
			slices.Concat(db.testEnv(), []string{
				"bash", "-c", `go test -tags faultpoints -json -run "$1" "${@:3}" > "$2" || true`, "flows",
				"^TestFlow(" + alternatives + ")_", results,
			}, pkgs),
			append(check, "--from", results),
		}
		if swift {
			cmds = append(cmds, lockWaitedCmd(
				row.lockWaited,
				filepath.Join(env.Work, "scripts", "mobile-core-test.sh"),
				"--filter",
				"(F|Flow)("+alternatives+")[^a-z0-9]",
			))
		}
		return cmds
	}
	if swift {
		row.lockWaited = env.statePath(lockWaitedDir, strconv.Itoa(os.Getpid())+".flows.waited")
	}
	return row, nil
}

func flowModules(f flows.Flow) []string {
	modules := []string{f.Module}
	for _, c := range f.Consumers {
		if module, _, ok := strings.Cut(c, "."); ok && !slices.Contains(modules, module) {
			modules = append(modules, module)
		}
	}
	return modules
}

func (env *Env) xcodeRow(changed []string) (checkRow, bool) {
	if env.GOOS != "darwin" || !mobileTreeChanged(changed) {
		return checkRow{}, false
	}
	if _, err := env.lookPath("xcodebuild"); err != nil {
		return checkRow{}, false
	}
	waited := env.statePath(lockWaitedDir, strconv.Itoa(os.Getpid())+".waited")
	locked := []string{
		"env",
		"MONACO_LOCK_WAITED=" + waited,
		lockHoldVar + seconds(env.Config.Budget[xcodeKind]),
		"MONACO_XCODE_LOCK_TIMEOUT=" + seconds(xcodeLockWait),
		"bash", "-c",
	}
	cmds := env.installXcsift()
	cmds = append(cmds,
		append(slices.Clone(locked), xcodeScript("build-for-testing")),
		append(slices.Clone(locked), xcodeScript("-only-testing:MonacoTests test-without-building")),
	)
	return checkRow{label: "xcode", kind: xcodeKind, dir: env.Work, cmds: cmds, lockWaited: waited}, true
}

func seconds(d time.Duration) string {
	return strconv.Itoa(int(d.Seconds()))
}

func lockWaited(file string) time.Duration {
	if file == "" {
		return 0
	}
	raw, _ := os.ReadFile(file)
	var total time.Duration
	for _, f := range strings.Fields(string(raw)) {
		if n, err := strconv.Atoi(f); err == nil {
			total += time.Duration(n) * time.Second
		}
	}
	return total
}

func mobileTreeChanged(changed []string) bool {
	return slices.ContainsFunc(changed, func(f string) bool {
		return strings.HasPrefix(f, "apps/mobile/") ||
			strings.HasPrefix(f, "packages/mobile-core/") && !strings.HasPrefix(f, "packages/mobile-core/Tests/")
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
		`cache_args=(); while IFS= read -r a; do cache_args+=("$a"); done < <(scripts/xcode-cache-args.sh)` + "\n" +
		`scripts/qa/xcode-lock.sh xcodebuild -project apps/mobile/Monaco.xcodeproj -scheme Monaco -configuration Debug ` +
		`-destination "platform=iOS Simulator,id=$(scripts/resolve-ios-sim.sh)" ` +
		`-derivedDataPath "$(git rev-parse --show-toplevel)/.build/DerivedData" ` +
		`-skipMacroValidation -skipPackagePluginValidation "${cache_args[@]}" CODE_SIGNING_ALLOWED=NO ` +
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
		{label: "legacy growth", kind: "pr", dir: env.Work, cmds: [][]string{
			append(slices.Clone(vars), "scripts/check-legacy-growth.py"),
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
			[]string{"apps/backend/", "packages/flows/", "scripts/ci/ready.sh", "scripts/install-atlas.sh", "scripts/install-sqlc.sh"},
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
	return checkRow{label: "ready", kind: "ready", dir: env.Work, cmds: [][]string{{"scripts/ci/ready.sh"}}}, nil
}

func (env *Env) migrateRow() (checkRow, error) {
	return checkRow{
		label: "migrate lint", kind: "migrate", dir: filepath.Join(env.Work, "apps", "backend"),
		cmds: [][]string{{"go", "run", "./cmd/monacoctl", "migrate", "lint"}},
	}, nil
}

func (env *Env) installXcsift() [][]string {
	if isFile(filepath.Join(env.Work, ".bin", "xcsift")) {
		return nil
	}
	return [][]string{{filepath.Join(env.Work, "scripts", "install-xcsift.sh")}}
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
	tags := []string{"-tags", "faultpoints"}
	lint, err := env.lintRow(ctx, backend, pkgs)
	if err != nil {
		return nil, err
	}
	profile, err := env.writeState("coverage", filepath.Base(env.coverProfile(head)), nil)
	if err != nil {
		return nil, err
	}
	covered := coverageFiles(backend, changed)
	return []checkRow{
		{
			label: "go build", kind: "go", dir: backend, class: "cpu",
			cmds: [][]string{slices.Concat([]string{"go", "build", "-o", os.DevNull}, tags, buildable(backend, pkgs))},
		},
		{
			label: "go vet",
			kind:  "go",
			dir:   backend,
			class: "cpu",
			cmds:  [][]string{slices.Concat([]string{"go", "vet"}, tags, pkgs)},
		},
		lint,
		{
			label: "go test -short", kind: packageKind, dir: backend, class: "db",
			dbCmds: func(db testDB) [][]string {
				p := strconv.Itoa(testParallelism(runtime.NumCPU(), db.busy))
				return [][]string{slices.Concat(db.testEnv(), []string{"go", "test"}, tags, []string{
					"-short", "-count=1", "-timeout", env.Config.Budget[packageKind].String(), "-p", p, "-json",
				}, coverFlags(backend, pkgs, covered, profile), pkgs)}
			},
		},
		coverageRow(backend, self, profile, covered),
	}, nil
}

func coverageFiles(backend string, changed []string) map[string]bool {
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
	return only
}

func coverFlags(backend string, pkgs []string, covered map[string]bool, profile string) []string {
	tested := buildable(backend, pkgs)
	if len(covered) == 0 || len(tested) == 0 {
		return nil
	}
	return []string{"-coverpkg=" + strings.Join(tested, ","), "-coverprofile=" + profile}
}

func coverageRow(backend, self, profile string, only map[string]bool) checkRow {
	row := checkRow{label: "coverage", kind: "go", dir: backend}
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
	bin := env.golangciLint(ctx, backend)
	out, err := env.Run(ctx, backend, "", bin, "version", "--short")
	have := "v" + strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	if err != nil {
		have = "none"
	}
	if have != want {
		install := "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@" + want
		return checkRow{}, detailErr(errs.CodeInvalidInput, "monacoctl.agents.check",
			fmt.Sprintf("golangci-lint on PATH is %s and CI pins %s; run: %s", have, want, install))
	}
	return checkRow{label: "go lint", kind: "lint", dir: backend, class: "cpu", cmds: [][]string{
		slices.Concat([]string{bin, "run", "--allow-parallel-runners"}, pkgs),
		slices.Concat([]string{"go", "run", "./internal/platform/lint/nogo/cmd/nogo"}, pkgs),
		{"go", "run", "./cmd/monacoctl", "lint", "comments"},
	}}, nil
}

func (env *Env) golangciLint(ctx context.Context, backend string) string {
	if _, err := env.lookPath("golangci-lint"); err == nil {
		return "golangci-lint"
	}
	gopath, err := env.Run(ctx, backend, "", "go", "env", "GOPATH")
	if err != nil || strings.TrimSpace(string(gopath)) == "" {
		return "golangci-lint"
	}
	bin := filepath.Join(strings.TrimSpace(string(gopath)), "bin", "golangci-lint")
	if _, err := os.Stat(bin); err != nil {
		return "golangci-lint"
	}
	return bin
}

func testParallelism(cpus, busy int) int {
	return min(maxTestP, max(2, cpus/max(1, busy)))
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

func (env *Env) testFileRows(changed []string, removed bool) []checkRow {
	goTests, pyTests := env.affectedTests(changed)
	if removed {
		goTests["."] = append(
			goTests["."],
			"TestSkillPaths_everyBacktickedPathInABackendSkillExists",
			"TestBackendAgentsMD_staysUnder60LinesAndCitesOnlyRealPaths",
		)
	}
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
			return f == "scripts/"+p || bytes.Contains(body, []byte(`"`+path.Base(f)+`"`)) ||
				bytes.Contains(body, []byte(`"`+f+`"`)) || scannedGlobHit(p, f)
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
	if err := r.tools(stdout); err != nil {
		return err
	}
	var paced []checkRow
	for _, row := range rows {
		if row.kind == xcodeKind {
			paced = append(paced, row)
			continue
		}
		if err := r.row(ctx, row, stdout); err != nil {
			return err
		}
	}
	for _, row := range paced {
		if err := r.row(ctx, row, stdout); err != nil {
			return err
		}
	}
	return nil
}

func (r *checkRun) tools(stdout io.Writer) error {
	for _, tool := range pinnedTools() {
		if isFile(filepath.Join(r.env.Work, ".bin", tool)) {
			continue
		}
		missing := ".bin/" + tool + " missing; run scripts/install-" + tool + ".sh"
		_, _ = fmt.Fprintf(stdout, "  %-15s fail  %s\n", "tools", missing)
		_, _ = fmt.Fprintf(&r.log, "fail tools: %s\n", missing)
		return detailErr(errs.CodeInvalidInput, "monacoctl.agents.check", "tools failed; see the log")
	}
	return nil
}

func (r *checkRun) row(ctx context.Context, row checkRow, stdout io.Writer) error {
	if row.skip != "" {
		_, _ = fmt.Fprintf(stdout, "  %-15s skip  %s\n", row.label, row.skip)
		_, _ = fmt.Fprintf(&r.log, "skip %s: %s\n", row.label, row.skip)
		return nil
	}
	release, tokenWait, err := r.admit(ctx, &row, stdout)
	if err != nil {
		return err
	}
	defer release()
	if err := r.prepare(ctx, row, stdout); err != nil {
		return err
	}
	return r.runRow(ctx, row, stdout, tokenWait)
}

func (r *checkRun) runRow(ctx context.Context, row checkRow, stdout io.Writer, tokenWait string) error {
	ctx, cancel, sb := r.rowBudget(ctx, row)
	defer cancel()
	watch := r.env.watchLoad(ctx, sb)
	defer watch.stop()
	if row.lockWaited != "" {
		if _, err := r.env.writeState(lockWaitedDir, filepath.Base(row.lockWaited), nil); err != nil {
			return err
		}
	}
	rowStart, first := r.env.Now(), len(r.timings)
	warnings := 0
	var waited time.Duration
	for _, cmd := range row.cmds {
		if sb.capped() {
			cmd = withTimeout(cmd, sb.ceiling)
		}
		text, err := r.exec(ctx, row, cmd)
		sb = watch.sample(ctx)
		warnings += strings.Count(text, "::warning ")
		waited = lockWaited(row.lockWaited)
		rescued, budgetErr := r.checkBudget(ctx, stdout, row, cmd, sb, rowStart, waited, first)
		if budgetErr != nil {
			return budgetErr
		}
		if err != nil && !rescued {
			return failRow(stdout, row, cmd, text, err, sb)
		}
	}
	note := tokenWait
	if waited > 0 {
		note += fmt.Sprintf("  waited %s for %s lock", waited, row.label)
	}
	note += budgetNote(sb, warnings)
	took := r.env.Now().Sub(rowStart) - waited
	_, _ = fmt.Fprintf(stdout, "  %-15s ok    %.1fs%s\n", row.label, took.Seconds(), note)
	return nil
}

func budgetNote(sb scaledBudget, warnings int) string {
	note := ""
	if sb.scaled() {
		note += "  budget " + sb.String()
	}
	if warnings > 0 {
		note += fmt.Sprintf("  %d warnings in the log", warnings)
	}
	return note
}

func (r *checkRun) admit(ctx context.Context, row *checkRow, stdout io.Writer) (func(), string, error) {
	if row.class == "" {
		return func() {}, "", nil
	}
	start := r.env.Now()
	release, err := r.env.takeToken(ctx, row.class, stdout)
	if err != nil {
		return nil, "", err
	}
	if row.dbCmds != nil {
		db, releaseDB, err := r.env.takeTestDB()
		if err != nil {
			release()
			return nil, "", err
		}
		_, _ = fmt.Fprintf(&r.log, "test database: port %d; go test -p %d (%d CPUs over %d busy stage 0 slots)\n",
			db.port(), testParallelism(runtime.NumCPU(), db.busy), runtime.NumCPU(), db.busy)
		row.prep, row.cmds = db.row(r.env.Work).cmds, row.dbCmds(db)
		inner := release
		release = func() {
			releaseDB()
			inner()
		}
	}
	waited := r.env.Now().Sub(start)
	line := fmt.Sprintf("waited %s for a %s token", waited, row.class)
	_, _ = fmt.Fprintf(&r.log, "%s: %s\n", row.label, line)
	if waited > 0 {
		return release, "  " + line, nil
	}
	return release, "", nil
}

func (r *checkRun) prepare(ctx context.Context, row checkRow, stdout io.Writer) error {
	prep := checkRow{label: row.label, kind: "go", dir: r.env.Work}
	ctx, cancel, sb := r.rowBudget(ctx, prep)
	defer cancel()
	for _, cmd := range row.prep {
		if text, err := r.exec(ctx, prep, cmd); err != nil {
			return failRow(stdout, prep, cmd, text, err, sb)
		}
	}
	return nil
}

func (r *checkRun) rowBudget(ctx context.Context, row checkRow) (context.Context, context.CancelFunc, scaledBudget) {
	sb := r.env.scaleBudget(ctx, r.env.Config.Budget[row.kind])
	if row.kind == packageKind {
		return ctx, func() {}, sb
	}
	limit := sb.ceiling
	if row.lockWaited != "" {
		limit += xcodeLockWait * time.Duration(len(row.cmds))
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	return ctx, cancel, sb
}

func failRow(stdout io.Writer, row checkRow, cmd []string, text string, err error, sb scaledBudget) error {
	_, _ = fmt.Fprintf(stdout, "  %-15s FAIL  %s%s\n", row.label, strings.Join(cmd, " "), sb.peakNote())
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
	sb scaledBudget,
	rowStart time.Time,
	waited time.Duration,
	first int,
) (bool, error) {
	if row.kind == packageKind {
		retried, err := r.retryAlone(ctx, stdout, row, cmd, sb, r.timings[first:])
		if retried || err != nil {
			return retried, err
		}
	}
	return false, r.overBudget(stdout, row, sb, rowStart, waited, r.timings[first:])
}

type scaledBudget struct {
	base, limit, ceiling time.Duration
	factor               float64
	load                 float64
	cores                int
}

const (
	maxBudgetScale  = 4
	loadSampleEvery = time.Minute
)

func (s scaledBudget) scaled() bool { return s.limit > s.base }

func (s scaledBudget) capped() bool { return s.ceiling > s.base }

func (s scaledBudget) peakNote() string {
	if !s.scaled() {
		return ""
	}
	return fmt.Sprintf("  (highest load1 %.1f)", s.load)
}

func (s scaledBudget) String() string {
	if !s.scaled() {
		return s.limit.String()
	}
	return fmt.Sprintf("%s (base %s, x%.1f for load1 %.1f over %d cores)",
		s.limit, s.base, s.factor, s.load, s.cores)
}

func (s scaledBudget) observe(load float64) scaledBudget {
	if s.cores < 1 || load <= s.load {
		return s
	}
	s.load = load
	s.factor = min(max(1, load/float64(s.cores)), maxBudgetScale)
	s.limit = max(s.base, time.Duration(float64(s.base)*s.factor).Round(100*time.Millisecond))
	return s
}

func (env *Env) scaleBudget(ctx context.Context, base time.Duration) scaledBudget {
	sb := scaledBudget{base: base, limit: base, ceiling: base, factor: 1}
	if env.Actions {
		return sb
	}
	sb.cores = runtime.NumCPU()
	if env.Cores != nil {
		sb.cores = env.Cores()
	}
	if sb.cores < 1 {
		return sb
	}
	sb.ceiling = base * maxBudgetScale
	load, err := env.loadValue(ctx)
	if err != nil {
		return sb
	}
	return sb.observe(load)
}

type loadWatch struct {
	env  *Env
	mu   sync.Mutex
	sb   scaledBudget
	live bool
	done chan struct{}
	wg   sync.WaitGroup
}

func (env *Env) watchLoad(ctx context.Context, sb scaledBudget) *loadWatch {
	w := &loadWatch{env: env, sb: sb, live: sb.capped(), done: make(chan struct{})}
	if !w.live {
		return w
	}
	tick, stop := env.loadTick()
	w.wg.Go(func() {
		defer stop()
		for {
			select {
			case <-w.done:
				return
			case <-tick:
				w.sample(ctx)
			}
		}
	})
	return w
}

func (env *Env) loadTick() (<-chan time.Time, func()) {
	if env.LoadTick != nil {
		return env.LoadTick()
	}
	t := time.NewTicker(loadSampleEvery)
	return t.C, t.Stop
}

func (w *loadWatch) sample(ctx context.Context) scaledBudget {
	if !w.live {
		return w.sb
	}
	load, err := w.env.loadValue(ctx)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err == nil {
		w.sb = w.sb.observe(load)
	}
	return w.sb
}

func (w *loadWatch) stop() {
	close(w.done)
	w.wg.Wait()
}

func withTimeout(cmd []string, d time.Duration) []string {
	out := slices.Clone(cmd)
	if i := slices.Index(out, "-timeout"); i >= 0 && i+1 < len(out) {
		out[i+1] = d.String()
	}
	if i := slices.IndexFunc(out, func(a string) bool { return strings.HasPrefix(a, lockHoldVar) }); i >= 0 {
		out[i] = lockHoldVar + seconds(d)
	}
	return out
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
	ctx context.Context, stdout io.Writer, row checkRow, cmd []string, sb scaledBudget, timings []timing,
) (bool, error) {
	slow := slowPackages(timings, sb.limit)
	if len(slow) == 0 || slices.ContainsFunc(slow, func(t timing) bool { return !strings.HasPrefix(t.name, "./") }) {
		return false, nil
	}
	if slices.ContainsFunc(timings, func(t timing) bool {
		return t.failed && !slices.ContainsFunc(slow, func(s timing) bool { return s.name == t.name })
	}) {
		return false, nil
	}
	first := len(r.timings)
	alone := rerunCmd(withTimeout(cmd, sb.base), slow)
	text, err := r.exec(ctx, row, alone)
	again := r.timings[first:]
	if still := slowPackages(again, sb.base); len(still) > 0 {
		s, orig := slowest(still), slowest(slow)
		_, _ = fmt.Fprintf(stdout, "  %-15s over budget\n", row.label)
		return false, detailErr(errs.CodeUpstreamTimeout, "monacoctl.agents.check", fmt.Sprintf(
			"%s: package %s took %.1fs, over the %s per-package budget; rerun alone: package %s took %.1fs, over the %s base budget",
			row.label,
			orig.name,
			orig.took.Seconds(),
			sb,
			s.name,
			s.took.Seconds(),
			sb.base,
		))
	}
	if err != nil {
		return false, failRow(stdout, row, alone, text, err, sb)
	}
	load := r.env.load1(ctx)
	for _, t := range again {
		_, _ = fmt.Fprintf(stdout, "  %-15s ok    over budget under load (load1 %s), %s passed alone in %.1fs\n",
			row.label, load, t.name, t.took.Seconds())
	}
	return true, nil
}

func (env *Env) load1(ctx context.Context) string {
	v, err := env.loadValue(ctx)
	if err != nil {
		return "unknown"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func (env *Env) loadValue(ctx context.Context) (float64, error) {
	load := env.Load
	if load == nil {
		load = loadAverage
	}
	return load(ctx, env.GOOS)
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
	stdout io.Writer, row checkRow, sb scaledBudget, rowStart time.Time, waited time.Duration, timings []timing,
) error {
	var detail string
	if row.kind == packageKind {
		slow := slowPackages(timings, sb.limit)
		if len(slow) == 0 {
			return nil
		}
		s := slowest(slow)
		detail = fmt.Sprintf("%s: package %s took %.1fs, over the %s per-package budget",
			row.label, s.name, s.took.Seconds(), sb)
	} else {
		took := r.env.Now().Sub(rowStart) - waited
		if took <= sb.limit {
			return nil
		}
		s := slowest(timings)
		detail = fmt.Sprintf("%s row over the %s %s budget after %.0fs; slowest: %s (%.1fs)",
			row.label, sb, row.kind, took.Seconds(), s.name, s.took.Seconds())
	}
	if waited > 0 {
		detail += fmt.Sprintf("; waited %s for %s lock, not counted", waited, row.label)
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
