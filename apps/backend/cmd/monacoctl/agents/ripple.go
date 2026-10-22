package agents

import (
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	backendDir  = "apps/backend/"
	openAPIPair = backendDir + "api/openapi.yaml\n" + "packages/mobile-core/Sources/MonacoAPI/openapi.yaml"
)

func rippleRules() []rippleRule {
	return []rippleRule{
		{
			regexp.MustCompile(`^apps/backend/internal/errs/(codes_\w+)\.go$`),
			backendDir + "internal/errs/testdata/$1.golden\ndocs/reference/errors.md\n" + openAPIPair +
				"\npackages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift",
		},
		{
			regexp.MustCompile(`^apps/backend/api/spec/\w+\.yaml$`),
			openAPIPair + "\n" + backendDir + "internal/platform/httpx/api/**/*.gen.go",
		},
		{
			regexp.MustCompile(`^apps/backend/internal/platform/observability/(msgs_\w+)\.go$`),
			backendDir + "internal/platform/observability/$1.gen.go\ndocs/reference/logs.md",
		},
		{
			regexp.MustCompile(`^apps/backend/internal/events/\w+_registrations\.go$`),
			"docs/reference/events.md",
		},
		{
			regexp.MustCompile(`^apps/backend/queries/platform/[^/]+\.sql$`),
			backendDir + "internal/platform/db/sqlc/*.gen.go",
		},
		{
			regexp.MustCompile(`^apps/backend/queries/relayer/[^/]+\.sql$`),
			backendDir + "internal/platform/chain/relayer/sqlc/*.gen.go",
		},
		{
			regexp.MustCompile(`^apps/backend/queries/(\w+)/[^/]+\.sql$`),
			backendDir + "internal/modules/$1/sqlc/*.gen.go",
		},
	}
}

type rippleRule struct {
	from  *regexp.Regexp
	feeds string
}

type rippleInput struct {
	touches []string
	files   []string
	read    func(name string) ([]byte, error)
}

func ripple(in rippleInput) []string {
	return slices.Sorted(maps.Keys(rippleReasons(in)))
}

func rippleReasons(in rippleInput) map[string]string {
	reasons := map[string]string{}
	for _, f := range touchedPaths(in.touches, in.files) {
		for _, p := range generated(f, in.files) {
			reasons[p] = cmp.Or(reasons[p], "generated from "+f)
		}
		for _, p := range ownPackage(f, in) {
			reasons[p] = cmp.Or(reasons[p], "test of "+f)
		}
	}
	return reasons
}

func generated(file string, files []string) []string {
	var out []string
	for _, r := range rippleRules() {
		if !r.from.MatchString(file) {
			continue
		}
		for feed := range strings.SplitSeq(r.from.ReplaceAllString(file, r.feeds), "\n") {
			out = append(out, expandGlob(feed, files)...)
		}
	}
	return out
}

func touchedPaths(globs, files []string) []string {
	var out []string
	for _, g := range globs {
		if !strings.ContainsAny(g, "*?") {
			out = append(out, g)
		}
	}
	for _, f := range files {
		if globsMatch(globs, f) && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

func expandGlob(glob string, files []string) []string {
	if !strings.ContainsAny(glob, "*?") {
		return []string{glob}
	}
	re := globRegexp(glob)
	return slices.DeleteFunc(slices.Clone(files), func(f string) bool { return !re.MatchString(f) })
}

func globsMatch(globs []string, file string) bool {
	return slices.ContainsFunc(globs, func(g string) bool { return globRegexp(g).MatchString(file) })
}

func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case glob[i] == '*':
			b.WriteString("[^/]*")
		case glob[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func ownPackage(file string, in rippleInput) []string {
	if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
		return nil
	}
	dir := path.Dir(file)
	stem := strings.TrimSuffix(strings.TrimSuffix(path.Base(file), ".go"), ".gen")
	mention := mentionRE(exportedNames(file, in.read))
	var out []string
	for _, f := range in.files {
		ownTest := path.Dir(f) == dir && strings.HasSuffix(f, "_test.go") &&
			(f == path.Join(dir, stem+"_test.go") || fileMentions(mention, f, in.read))
		ownGolden := path.Dir(f) == path.Join(dir, "testdata") && f == path.Join(dir, "testdata", stem+".golden")
		if ownTest || ownGolden {
			out = append(out, f)
		}
	}
	return out
}

func mentionRE(names []string) *regexp.Regexp {
	if len(names) == 0 {
		return nil
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`\b(?:` + strings.Join(quoted, "|") + `)\b`)
}

func fileMentions(re *regexp.Regexp, file string, read func(string) ([]byte, error)) bool {
	if re == nil {
		return false
	}
	body, err := read(file)
	return err == nil && re.Match(body)
}

func exportedNames(file string, read func(string) ([]byte, error)) []string {
	src, err := read(file)
	if err != nil {
		return nil
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	var names []string
	for _, d := range parsed.Decls {
		names = append(names, declNames(d)...)
	}
	return slices.DeleteFunc(names, func(n string) bool { return !token.IsExported(n) })
}

func declNames(d ast.Decl) []string {
	var names []string
	switch d := d.(type) {
	case *ast.FuncDecl:
		names = append(names, d.Name.Name)
	case *ast.GenDecl:
		for _, s := range d.Specs {
			names = append(names, specNames(s)...)
		}
	}
	return names
}

func specNames(s ast.Spec) []string {
	var names []string
	switch s := s.(type) {
	case *ast.TypeSpec:
		names = append(names, s.Name.Name)
	case *ast.ValueSpec:
		for _, n := range s.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

func (env *Env) rippleInputOf(ctx context.Context, dir string, touches []string) (rippleInput, error) {
	out, err := env.Run(ctx, dir, "", "git", "ls-files")
	if err != nil {
		return rippleInput{}, err
	}
	return rippleInput{
		touches: touches,
		files:   strings.Fields(string(out)),
		read:    func(name string) ([]byte, error) { return readIn(dir, name) },
	}, nil
}

func (env *Env) rippleOf(ctx context.Context, dir string, touches []string) ([]string, error) {
	in, err := env.rippleInputOf(ctx, dir, touches)
	if err != nil {
		return nil, err
	}
	return ripple(in), nil
}

func rippleCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return usageError("ripple <ticket>")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n <= 0 {
		return usageError("ripple <ticket>")
	}
	is, err := env.GitHub.Issue(ctx, n)
	if err != nil {
		return err
	}
	paths, err := env.rippleOf(ctx, env.Work, touchGlobs(is.Body))
	if err != nil {
		return err
	}
	for _, p := range paths {
		_, _ = io.WriteString(stdout, p+"\n")
	}
	return nil
}

func readIn(dir, name string) ([]byte, error) {
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return body, nil
}

func declaredNames() *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[-+](?:func\s+(?:\([^)]*\)\s*)?|type\s+|const\s+|var\s+)([A-Z]\w*)`)
}

func changedIdentifiers(diff string) []string {
	names := map[string]bool{}
	for _, m := range declaredNames().FindAllStringSubmatch(diff, -1) {
		names[m[1]] = true
	}
	return slices.Sorted(maps.Keys(names))
}

func wideTier(files, pkgDirs, idents []string, read func(string) ([]byte, error)) map[string]string {
	reasons := map[string]string{}
	for _, f := range files {
		if !isFixtureIn(f, pkgDirs) {
			continue
		}
		body, err := read(f)
		if err != nil {
			continue
		}
		for _, name := range idents {
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).Match(body) {
				reasons[f] = "fixture naming " + name
				break
			}
		}
	}
	return reasons
}

func isFixtureIn(file string, pkgDirs []string) bool {
	return slices.ContainsFunc(pkgDirs, func(dir string) bool {
		inTests := path.Dir(file) == dir && strings.HasSuffix(file, "_test.go")
		return inTests || strings.HasPrefix(file, dir+"/testdata/")
	})
}
