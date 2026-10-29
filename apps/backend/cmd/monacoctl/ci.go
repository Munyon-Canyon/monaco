package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const (
	ciUsage      = "usage: monacoctl ci affected --base <ref> [--excluded | --scoped]"
	allPackages  = "./..."
	scanSentinel = "scansModuleSource"
)

type scanRule struct {
	roots   []string
	added   *regexp.Regexp
	scanner string
}

func scanRules() []scanRule {
	return []scanRule{
		{
			roots:   []string{"internal/modules", "internal/platform"},
			added:   regexp.MustCompile(`^\+.*\bobservability\.(Info|Warn|Error|Debug)\(`),
			scanner: "internal/platform/observability",
		},
	}
}

type ciEnv struct {
	moduleDir, goBin, gitBin string
	exec                     execFunc
}

type listedPackage struct {
	ImportPath   string   `json:"ImportPath"`
	Imports      []string `json:"Imports"`
	TestImports  []string `json:"TestImports"`
	XTestImports []string `json:"XTestImports"`
	Module       *struct {
		Path string `json:"Path"`
		Main bool   `json:"Main"`
	} `json:"Module"`
}

func toolCi(_ toolEnv) tool {
	return ciTool(ciEnv{moduleDir: ".", goBin: "go", gitBin: "git", exec: runCommand})
}

func ciTool(env ciEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		if len(args) == 0 || args[0] != "affected" {
			_, _ = fmt.Fprintln(stderr, ciUsage)
			return 2
		}
		fs := flag.NewFlagSet("ci affected", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		base := fs.String("base", "", "")
		excluded := fs.Bool("excluded", false, "")
		scopedOnly := fs.Bool("scoped", false, "")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *base == "" || *excluded && *scopedOnly {
			_, _ = fmt.Fprintln(stderr, ciUsage)
			return 2
		}
		pkgs, skipped, scoped, err := env.affected(context.Background(), *base)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl ci affected: %v\n", err)
			return 1
		}
		if *scopedOnly {
			_, _ = fmt.Fprintln(stdout, scoped)
			return 0
		}
		if *excluded {
			pkgs = skipped
		}
		for _, p := range pkgs {
			_, _ = fmt.Fprintln(stdout, p)
		}
		return 0
	}
}

func (env ciEnv) affected(ctx context.Context, base string) (selected, excluded []string, scoped bool, err error) {
	const op = "monacoctl.ciAffected"
	diff, err := env.exec(ctx, env.moduleDir, nil, env.gitBin,
		"diff", "--name-only", "--relative", base+"...HEAD", "--", ".")
	if err != nil {
		return nil, nil, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	rows, err := env.exec(ctx, env.moduleDir, nil, env.gitBin,
		"diff", "--name-only", base+"...HEAD", "--", ":(top)"+flows.Dir, ":(top)"+flows.AppDir)
	if err != nil {
		return nil, nil, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	changed := slices.Concat(strings.Fields(string(diff)), strings.Fields(string(rows)))
	if len(changed) == 0 {
		return nil, nil, false, nil
	}
	listed, err := env.exec(ctx, env.moduleDir, nil, env.goBin,
		"list", "-deps", "-json=ImportPath,Imports,TestImports,XTestImports,Module", allPackages)
	if err != nil {
		return nil, nil, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	pkgs, err := decodePackages(listed)
	if err != nil {
		return nil, nil, false, err
	}
	scanners, err := env.scanners(ctx, base, changed)
	if err != nil {
		return nil, nil, false, err
	}
	parsed := env.changedFlows(ctx, changed)
	_, _, _, scoped, _ = seedPackages(pkgs, changed, parsed)
	selected, excluded = reverseDeps(pkgs, changed, parsed, scanners...)
	return selected, excluded, scoped, nil
}

func (env ciEnv) changedFlows(ctx context.Context, changed []string) []flows.Flow {
	if !slices.ContainsFunc(changed, flowFile) {
		return nil
	}
	top, err := env.exec(ctx, env.moduleDir, nil, env.gitBin, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	parsed, _, err := flows.ReadAll(os.DirFS(strings.TrimSpace(string(top))))
	if err != nil {
		return nil
	}
	return parsed
}

func flowFile(f string) bool {
	return flowRow(f) ||
		strings.HasPrefix(f, "internal/testkit/flows/") ||
		strings.HasPrefix(f, "internal/testkit/fakes/testdata/")
}

func (env ciEnv) scanners(ctx context.Context, base string, changed []string) ([]string, error) {
	const op = "monacoctl.ciScanners"
	var dirs []string
	for _, rule := range scanRules() {
		specs := make([]string, len(rule.roots))
		for i, root := range rule.roots {
			specs[i] = ":(glob)" + root + "/**/*.go"
		}
		diff, err := env.exec(ctx, env.moduleDir, nil, env.gitBin,
			slices.Concat([]string{"diff", "-U0", "--relative", base + "...HEAD", "--"}, specs)...)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		if slices.ContainsFunc(strings.Split(string(diff), "\n"), rule.added.MatchString) {
			dirs = append(dirs, rule.scanner)
		}
	}
	if !slices.ContainsFunc(changed, func(f string) bool { return strings.HasSuffix(f, ".go") }) {
		return dirs, nil
	}
	files, err := env.exec(ctx, env.moduleDir, nil, env.gitBin, "grep", "-l", scanSentinel, "--", "*_test.go")
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, f := range strings.Fields(string(files)) {
		dirs = append(dirs, path.Dir(f))
	}
	return dirs, nil
}

func decodePackages(data []byte) (map[string]listedPackage, error) {
	byDir := map[string]listedPackage{}
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var p listedPackage
		err := dec.Decode(&p)
		if errors.Is(err, io.EOF) {
			return byDir, nil
		}
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, "monacoctl.decodePackages")
		}
		if p.Module == nil || !p.Module.Main {
			continue
		}
		byDir[path.Clean("."+strings.TrimPrefix(p.ImportPath, p.Module.Path))] = p
	}
}

func flowRow(f string) bool {
	return strings.HasPrefix(f, flows.Dir+"/") || strings.HasPrefix(f, flows.AppDir+"/")
}

func flowsReaders() []string {
	return []string{
		"cmd/gen",
		"cmd/monacoctl",
		"cmd/monacoctl/agents",
		"cmd/monacoctl/verify",
		"internal/testkit/flows",
		"internal/tools/flows",
		"internal/tools/gen",
	}
}

func harnessPackages() []string {
	return []string{
		"internal/testkit",
		"internal/testkit/scenario",
		"internal/testkit/flows",
		"internal/platform/httpx/api/apiall",
	}
}

func agentTooling() []string {
	return []string{"cmd/monacoctl", "cmd/monacoctl/agents", "cmd/monacoctl/verify"}
}

func sharedTooling(dir string) bool {
	return slices.ContainsFunc([]string{"internal/platform", "internal/events"}, func(root string) bool {
		return dir == root || strings.HasPrefix(dir, root+"/")
	})
}

func reverseDeps(
	byDir map[string]listedPackage,
	changed []string,
	parsed []flows.Flow,
	scanners ...string,
) (selected, excluded []string) {
	hit, local, bare, scoped, ok := seedPackages(byDir, changed, parsed)
	if !ok {
		return []string{allPackages}, nil
	}
	seedScanners(byDir, bare, scanners)
	seeds := maps.Clone(hit)
	if scoped {
		growScoped(byDir, hit, bare)
	} else {
		growToImporters(byDir, hit, nil)
	}
	dirOf := map[string]string{}
	for dir, p := range byDir {
		dirOf[p.ImportPath] = dir
	}
	testHit := testPropagation(byDir, dirOf, hit, seeds, local)
	for dir, p := range byDir {
		tests := slices.Concat(p.TestImports, p.XTestImports)
		testsHit := slices.ContainsFunc(tests, func(i string) bool { return testHit[i] })
		switch {
		case !hit[p.ImportPath] && !local[p.ImportPath] && !bare[p.ImportPath] && !testsHit:
		case slices.Contains(agentTooling(), dir) && !inputsChanged(p, dirOf, seeds, local, scoped):
			excluded = append(excluded, "./"+dir)
		default:
			selected = append(selected, "./"+dir)
		}
	}
	slices.Sort(selected)
	slices.Sort(excluded)
	return selected, excluded
}

func testPropagation(
	byDir map[string]listedPackage,
	dirOf map[string]string,
	hit, seeds, local map[string]bool,
) map[string]bool {
	harness := map[string]bool{}
	for _, dir := range harnessPackages() {
		if p, found := byDir[dir]; found {
			harness[p.ImportPath] = true
		}
	}
	api := byDir["api"].ImportPath
	shared := hit[api]
	for i := range seeds {
		shared = shared || sharedSeed(dirOf[i])
	}
	for i := range harness {
		if seeds[i] || local[i] {
			return hit
		}
	}
	if shared {
		return hit
	}
	rest := maps.Clone(hit)
	maps.DeleteFunc(rest, func(i string, _ bool) bool { return harness[i] })
	return rest
}

func sharedSeed(dir string) bool {
	moduleAPI := strings.HasPrefix(dir, "internal/platform/httpx/api/") && strings.HasSuffix(dir, "api")
	return dir == "internal/errs" || (sharedTooling(dir) && !moduleAPI)
}

func seedScanners(byDir map[string]listedPackage, bare map[string]bool, scanners []string) {
	for _, dir := range scanners {
		if p, found := byDir[dir]; found {
			bare[p.ImportPath] = true
		}
	}
}

func growthStops(byDir map[string]listedPackage) map[string]bool {
	stop := map[string]bool{}
	for dir, p := range byDir {
		harness := slices.Contains(harnessPackages(), dir) || dir == "internal/testkit" ||
			strings.HasPrefix(dir, "internal/testkit/")
		command := strings.HasPrefix(dir, "cmd/") && !slices.Contains(agentTooling(), dir)
		if harness || command {
			stop[p.ImportPath] = true
		}
	}
	return stop
}

func growScoped(byDir map[string]listedPackage, hit, bare map[string]bool) {
	stop := growthStops(byDir)
	reach := maps.Clone(hit)
	growToImporters(byDir, reach, nil)
	growToImporters(byDir, hit, stop)
	for i := range stop {
		if reach[i] && !hit[i] {
			bare[i] = true
		}
	}
}

func growToImporters(byDir map[string]listedPackage, hit, stop map[string]bool) {
	for grew := true; grew; {
		grew = false
		for _, p := range byDir {
			if !hit[p.ImportPath] && !stop[p.ImportPath] &&
				slices.ContainsFunc(p.Imports, func(i string) bool { return hit[i] }) {
				hit[p.ImportPath], grew = true, true
			}
		}
	}
}

func inputsChanged(p listedPackage, dirOf map[string]string, seeds, local map[string]bool, scoped bool) bool {
	return seeds[p.ImportPath] || local[p.ImportPath] ||
		slices.ContainsFunc(p.Imports, func(i string) bool {
			return seeds[i] && !sharedTooling(dirOf[i]) &&
				(!scoped || !strings.HasPrefix(dirOf[i], "internal/modules/"))
		})
}

func seedPackages(
	byDir map[string]listedPackage,
	changed []string,
	parsed []flows.Flow,
) (hit, local, bare map[string]bool, scoped, ok bool) {
	hit, local, bare, scoped = map[string]bool{}, map[string]bool{}, map[string]bool{}, true
	for _, f := range changed {
		if specSeed(byDir, changed, f, hit, bare) || flowSeed(byDir, parsed, f, bare) || scopedSeed(byDir, f, bare) {
			continue
		}
		if module, found := moduleOf(f, changed); found && !testOnly(f) {
			if !moduleSeed(byDir, f, module, hit, bare) {
				return nil, nil, nil, false, false
			}
			continue
		}
		into := hit
		if testOnly(f) {
			into = local
		} else {
			scoped = false
		}
		if !plainSeed(byDir, f, into) {
			return nil, nil, nil, false, false
		}
	}
	return hit, local, bare, scoped, true
}

func plainSeed(byDir map[string]listedPackage, f string, into map[string]bool) bool {
	dirs := seedDirs(byDir, f)
	if len(dirs) == 0 {
		return false
	}
	for _, d := range dirs {
		p, found := byDir[d]
		if !found {
			return false
		}
		into[p.ImportPath] = true
	}
	return true
}

func moduleOf(f string, changed []string) (string, bool) {
	const apiRoot = "internal/platform/httpx/api/"
	if rest, found := strings.CutPrefix(f, "internal/modules/"); found {
		return nestedModule(rest)
	}
	if rest, found := strings.CutPrefix(f, "queries/"); found {
		return nestedModule(rest)
	}
	if rest, found := strings.CutPrefix(f, apiRoot); found {
		dir, _, nested := strings.Cut(rest, "/")
		module, suffixed := strings.CutSuffix(dir, "api")
		if !nested || !suffixed || module == "" {
			return "", false
		}
		return module, true
	}
	if strings.HasPrefix(f, "migrations/") && (strings.HasSuffix(f, ".sql") || f == "migrations/atlas.sum") {
		return migrationModule(changed)
	}
	return "", false
}

func nestedModule(rest string) (string, bool) {
	module, _, nested := strings.Cut(rest, "/")
	if !nested || module == "" {
		return "", false
	}
	return module, true
}

func migrationModule(changed []string) (string, bool) {
	var modules []string
	for _, c := range changed {
		module, found := "", false
		if rest, queried := strings.CutPrefix(c, "queries/"); queried {
			module, found = nestedModule(rest)
		} else if rest, owned := strings.CutPrefix(c, "internal/modules/"); owned {
			var inner string
			module, inner, found = strings.Cut(rest, "/")
			found = found && strings.HasPrefix(inner, "sqlc/")
		}
		if found && !slices.Contains(modules, module) {
			modules = append(modules, module)
		}
	}
	if len(modules) != 1 {
		return "", false
	}
	return modules[0], true
}

func inModule(dir, module string) bool {
	root := "internal/modules/" + module
	return dir == root || strings.HasPrefix(dir, root+"/")
}

func moduleSeed(byDir map[string]listedPackage, f, module string, hit, bare map[string]bool) bool {
	if !strings.HasPrefix(f, "migrations/") {
		for _, d := range seedDirs(byDir, f) {
			p, found := byDir[d]
			if !found {
				return false
			}
			hit[p.ImportPath] = true
		}
	}
	owned := false
	for dir, p := range byDir {
		owns := inModule(dir, module) || dir == "internal/platform/httpx/api/"+module+"api"
		owned = owned || owns
		if owns || importsModule(dir, p, module) {
			bare[p.ImportPath] = true
		}
	}
	return owned
}

func importsModule(dir string, p listedPackage, module string) bool {
	if path.Dir(dir) != "internal/modules" {
		return false
	}
	root := strings.TrimSuffix(p.ImportPath, dir) + "internal/modules/" + module
	return slices.ContainsFunc(p.Imports, func(i string) bool { return i == root || strings.HasPrefix(i, root+"/") })
}

func flowSeed(byDir map[string]listedPackage, parsed []flows.Flow, f string, bare map[string]bool) bool {
	for _, fl := range parsed {
		id := regexp.QuoteMeta(fl.ID)
		var owners []string
		switch {
		case f == flows.Dir+"/"+fl.ID+".tsv" || f == flows.AppDir+"/"+fl.ID+".tsv":
			owners = flowsReaders()
		case regexp.MustCompile(`^internal/testkit/flows/f` + id + `(\.gen)?\.go$`).MatchString(f):
			owners = []string{"internal/testkit/flows"}
		case strings.HasPrefix(f, "internal/testkit/fakes/testdata/") &&
			regexp.MustCompile(`(^|[^a-z0-9])f`+id+`([^a-z0-9]|$)`).MatchString(path.Base(f)):
			owners = []string{"internal/testkit/fakes"}
		default:
			continue
		}
		for _, dir := range owners {
			bare[byDir[dir].ImportPath] = true
		}
		for _, module := range flowModules(fl) {
			for _, dir := range moduleDirs(byDir, module) {
				bare[byDir[dir].ImportPath] = true
			}
		}
		return true
	}
	return false
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

func scopedSeed(byDir map[string]listedPackage, f string, bare map[string]bool) bool {
	shared, modules, scoped := moduleScoped(byDir, f)
	if !scoped {
		return false
	}
	for _, dir := range append(modules, shared) {
		bare[byDir[dir].ImportPath] = true
	}
	return true
}

func specSeed(byDir map[string]listedPackage, changed []string, f string, hit, bare map[string]bool) bool {
	api, found := byDir["api"]
	if !found {
		return false
	}
	module, isSpec := strings.CutSuffix(strings.TrimPrefix(f, "api/spec/"), ".yaml")
	switch {
	case f == "api/openapi.yaml":
		if !slices.ContainsFunc(changed, func(c string) bool {
			return strings.HasPrefix(c, "api/spec/") && strings.HasSuffix(c, ".yaml")
		}) {
			return false
		}
		bare[api.ImportPath] = true
		return true
	case isSpec && strings.HasPrefix(f, "api/spec/") && module != "base":
		moduleAPI, found := byDir["internal/platform/httpx/api/"+module+"api"]
		if !found {
			return false
		}
		hit[moduleAPI.ImportPath], bare[api.ImportPath] = true, true
		return true
	}
	return false
}

func moduleScoped(byDir map[string]listedPackage, f string) (shared string, modules []string, scoped bool) {
	shared, area := moduleArea(f)
	if _, found := byDir[shared]; !found || area == "" {
		return "", nil, false
	}
	if dirs := moduleDirs(byDir, area); len(dirs) > 0 {
		return shared, dirs, true
	}
	if shared == "internal/errs" {
		return shared, moduleDirs(byDir, ""), true
	}
	return "", nil, false
}

func moduleArea(f string) (shared, area string) {
	dir, name := path.Split(f)
	shared = strings.TrimSuffix(path.Clean(dir), "/testdata")
	if strings.HasSuffix(name, "_test.go") {
		return shared, ""
	}
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".go"), ".gen")
	switch {
	case shared == "internal/platform/observability" && strings.HasSuffix(name, ".go"):
		area = strings.TrimPrefix(stem, "msgs_")
	case shared == "internal/events" && strings.HasSuffix(name, ".go"):
		area = strings.TrimSuffix(stem, "_registrations")
	case shared == "internal/errs" && strings.HasSuffix(name, ".go"):
		area = strings.TrimPrefix(stem, "codes_")
	case shared == "internal/errs" && strings.HasSuffix(name, ".golden"):
		stem = strings.TrimSuffix(name, ".golden")
		area = strings.TrimPrefix(stem, "codes_")
	}
	if area == stem {
		return shared, ""
	}
	return shared, area
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func moduleDirs(byDir map[string]listedPackage, area string) []string {
	var dirs []string
	for dir := range byDir {
		module, found := strings.CutPrefix(dir, "internal/modules/")
		if !found {
			continue
		}
		module, _, _ = strings.Cut(module, "/")
		if area == "" || module == area {
			dirs = append(dirs, dir)
		}
	}
	if trimmed, _, found := cutLast(area, "_"); len(dirs) == 0 && found {
		return moduleDirs(byDir, trimmed)
	}
	return dirs
}

func testOnly(f string) bool {
	return strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "internal/events/testdata/")
}

func seedDirs(byDir map[string]listedPackage, f string) []string {
	query, isQuery := strings.CutPrefix(f, "queries/")
	module, _, nested := strings.Cut(query, "/")
	switch {
	case f == "sqlc.yaml":
		return sqlcDirs(byDir)
	case flowRow(f):
		return flowsReaders()
	case isQuery && nested:
		return []string{"internal/modules/" + module + "/sqlc"}
	case runsEverything(f):
		return nil
	case strings.HasSuffix(f, ".go"):
		return []string{path.Dir(f)}
	}
	dir := path.Dir(f)
	for byDir[dir].ImportPath == "" && dir != "." {
		dir = path.Dir(dir)
	}
	return []string{dir}
}

func runsEverything(f string) bool {
	return f == "go.mod" || f == "go.sum" || f == ".golangci.yml"
}

func sqlcDirs(byDir map[string]listedPackage) []string {
	var dirs []string
	for dir := range byDir {
		if path.Base(dir) == "sqlc" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}
