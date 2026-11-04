package flows

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
)

const (
	privySubjectPrefix = "did:privy:"
	scriptsDir         = "internal/testkit/flows"
)

type templatePart struct {
	literal string
	param   int
	isParam bool
}

type subjectTemplate []templatePart

func (t subjectTemplate) key() string {
	var b strings.Builder
	for _, p := range t {
		if p.isParam {
			b.WriteString("\x00" + strconv.Itoa(p.param) + "\x00")
		} else {
			b.WriteString(p.literal)
		}
	}
	return b.String()
}

func (t subjectTemplate) expand(args []string) (string, bool) {
	var b strings.Builder
	for _, p := range t {
		if !p.isParam {
			b.WriteString(p.literal)
			continue
		}
		if p.param >= len(args) || args[p.param] == "" {
			return "", false
		}
		b.WriteString(args[p.param])
	}
	return b.String(), true
}

type subjectBuilder struct {
	decl      *ast.FuncDecl
	params    map[string]int
	templates map[string]subjectTemplate
}

type scriptPackage struct {
	consts   map[string]string
	builders map[string]*subjectBuilder
	fileOf   map[string]*ast.File
	flowFns  []flowFunc
	fset     *token.FileSet
}

type flowFunc struct {
	name string
	file string
	decl *ast.FuncDecl
}

type subjectUse struct {
	flowID string
	file   string
	line   int
}

func CheckSharedSubjects(flows []Flow, env Env) []Problem {
	pkg, problems := loadScriptPackage(env)
	pkg.collectBuilders()
	ids := make([]string, 0, len(flows))
	for _, f := range flows {
		ids = append(ids, f.ID)
	}
	return append(problems, sharedSubjectProblems(pkg.subjectUses(ids))...)
}

func loadScriptPackage(env Env) (*scriptPackage, []Problem) {
	pkg := &scriptPackage{
		consts:   map[string]string{},
		builders: map[string]*subjectBuilder{},
		fileOf:   map[string]*ast.File{},
		fset:     token.NewFileSet(),
	}
	dir := path.Join(env.BackendDir, scriptsDir)
	entries, _ := fs.ReadDir(env.Repo, dir)
	var problems []Problem
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".gen.go") {
			continue
		}
		file := path.Join(dir, name)
		body, err := fs.ReadFile(env.Repo, file)
		if err != nil {
			problems = append(problems, Problem{File: file, Msg: "read: " + err.Error()})
			continue
		}
		parsed, err := parser.ParseFile(pkg.fset, file, body, parser.SkipObjectResolution)
		if err != nil {
			problems = append(problems, Problem{File: file, Msg: "parse: " + err.Error()})
			continue
		}
		pkg.addFile(file, parsed)
	}
	return pkg, problems
}

func (pkg *scriptPackage) addFile(file string, parsed *ast.File) {
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			pkg.addConsts(d)
		case *ast.FuncDecl:
			if d.Recv != nil || d.Body == nil {
				continue
			}
			pkg.fileOf[d.Name.Name] = parsed
			pkg.builders[d.Name.Name] = &subjectBuilder{
				decl:      d,
				params:    stringParams(d),
				templates: map[string]subjectTemplate{},
			}
			if strings.HasPrefix(d.Name.Name, "F") {
				pkg.flowFns = append(pkg.flowFns, flowFunc{name: d.Name.Name, file: file, decl: d})
			}
		}
	}
}

func (pkg *scriptPackage) addConsts(d *ast.GenDecl) {
	if d.Tok != token.CONST {
		return
	}
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Names) != len(vs.Values) {
			continue
		}
		for i, name := range vs.Names {
			if value, ok := stringLiteral(vs.Values[i]); ok {
				pkg.consts[name.Name] = value
			}
		}
	}
}

func stringParams(d *ast.FuncDecl) map[string]int {
	params := map[string]int{}
	index := 0
	for _, field := range d.Type.Params.List {
		isString := false
		if ident, ok := field.Type.(*ast.Ident); ok {
			isString = ident.Name == "string"
		}
		if len(field.Names) == 0 {
			index++
		}
		for _, name := range field.Names {
			if isString {
				params[name.Name] = index
			}
			index++
		}
	}
	return params
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func (pkg *scriptPackage) collectBuilders() {
	for changed := true; changed; {
		changed = false
		for _, b := range pkg.builders {
			if b.grow(pkg.builders) {
				changed = true
			}
		}
	}
}

func (b *subjectBuilder) grow(builders map[string]*subjectBuilder) bool {
	grew := false
	ast.Inspect(b.decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if tpl, ok := b.concatTemplate(node); ok && b.add(tpl) {
				grew = true
			}
			return false
		case *ast.CallExpr:
			if b.addCallTemplates(node, builders) {
				grew = true
			}
		}
		return true
	})
	return grew
}

func (b *subjectBuilder) add(tpl subjectTemplate) bool {
	hasParam := slices.ContainsFunc(tpl, func(p templatePart) bool { return p.isParam })
	if !hasParam || !strings.HasPrefix(tpl[0].literal, privySubjectPrefix) {
		return false
	}
	if _, seen := b.templates[tpl.key()]; seen {
		return false
	}
	b.templates[tpl.key()] = tpl
	return true
}

func (b *subjectBuilder) concatTemplate(expr ast.Expr) (subjectTemplate, bool) {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return nil, false
		}
		left, ok := b.concatTemplate(e.X)
		if !ok {
			return nil, false
		}
		right, ok := b.concatTemplate(e.Y)
		if !ok {
			return nil, false
		}
		return append(slices.Clone(left), right...), true
	case *ast.ParenExpr:
		return b.concatTemplate(e.X)
	case *ast.BasicLit:
		value, ok := stringLiteral(e)
		return subjectTemplate{{literal: value}}, ok
	case *ast.Ident:
		index, ok := b.params[e.Name]
		return subjectTemplate{{param: index, isParam: true}}, ok
	}
	return nil, false
}

func (b *subjectBuilder) addCallTemplates(call *ast.CallExpr, builders map[string]*subjectBuilder) bool {
	callee, ok := call.Fun.(*ast.Ident)
	if !ok || builders[callee.Name] == nil || builders[callee.Name] == b {
		return false
	}
	changed := false
	for _, inner := range builders[callee.Name].templates {
		tpl, ok := b.substitute(inner, call.Args)
		if ok && b.add(tpl) {
			changed = true
		}
	}
	return changed
}

func (b *subjectBuilder) substitute(inner subjectTemplate, args []ast.Expr) (subjectTemplate, bool) {
	var out subjectTemplate
	for _, p := range inner {
		if !p.isParam {
			out = append(out, p)
			continue
		}
		if p.param >= len(args) {
			return nil, false
		}
		replacement, ok := b.concatTemplate(args[p.param])
		if !ok {
			return nil, false
		}
		out = append(out, replacement...)
	}
	return out, true
}

func (pkg *scriptPackage) subjectUses(flowIDs []string) map[string][]subjectUse {
	uses := map[string][]subjectUse{}
	for _, fn := range pkg.flowFns {
		id, ok := flowIDOf(fn.name, flowIDs)
		if !ok {
			continue
		}
		for subject, line := range pkg.subjectsIn(fn.decl) {
			uses[subject] = append(uses[subject], subjectUse{flowID: id, file: fn.file, line: line})
		}
	}
	return uses
}

func flowIDOf(funcName string, flowIDs []string) (string, bool) {
	rest := strings.TrimPrefix(funcName, "F")
	best := ""
	for _, id := range flowIDs {
		if strings.HasPrefix(rest, id) && len(id) > len(best) {
			best = id
		}
	}
	return best, best != ""
}

func (pkg *scriptPackage) subjectsIn(fn *ast.FuncDecl) map[string]int {
	found := map[string]int{}
	skipped := map[ast.Node]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if concat, ok := n.(*ast.BinaryExpr); ok && concat.Op == token.ADD {
			ast.Inspect(concat, func(inner ast.Node) bool {
				skipped[inner] = true
				return true
			})
		}
		for _, subject := range pkg.nodeSubjects(n, skipped) {
			if _, seen := found[subject]; !seen {
				found[subject] = pkg.fset.Position(n.Pos()).Line
			}
		}
		return true
	})
	return found
}

func (pkg *scriptPackage) nodeSubjects(n ast.Node, skipped map[ast.Node]bool) []string {
	var value string
	switch node := n.(type) {
	case *ast.BasicLit:
		value, _ = stringLiteral(node)
	case *ast.Ident:
		value = pkg.consts[node.Name]
	case *ast.CallExpr:
		return pkg.callSubjects(node)
	}
	if skipped[n] || !strings.HasPrefix(value, privySubjectPrefix) {
		return nil
	}
	return []string{value}
}

func (pkg *scriptPackage) callSubjects(call *ast.CallExpr) []string {
	callee, ok := call.Fun.(*ast.Ident)
	if !ok || pkg.builders[callee.Name] == nil {
		return nil
	}
	args := make([]string, len(call.Args))
	for i, arg := range call.Args {
		switch a := arg.(type) {
		case *ast.BasicLit:
			args[i], _ = stringLiteral(a)
		case *ast.Ident:
			args[i] = pkg.consts[a.Name]
		}
	}
	var subjects []string
	for _, tpl := range pkg.builders[callee.Name].templates {
		if subject, ok := tpl.expand(args); ok {
			subjects = append(subjects, subject)
		}
	}
	return subjects
}

func sharedSubjectProblems(uses map[string][]subjectUse) []Problem {
	subjects := make([]string, 0, len(uses))
	for subject := range uses {
		subjects = append(subjects, subject)
	}
	slices.Sort(subjects)
	problems := make([]Problem, 0, len(subjects))
	for _, subject := range subjects {
		byFlow := map[string]subjectUse{}
		for _, use := range uses[subject] {
			if first, seen := byFlow[use.flowID]; !seen || use.line < first.line {
				byFlow[use.flowID] = use
			}
		}
		flowIDs := make([]string, 0, len(byFlow))
		for id := range byFlow {
			flowIDs = append(flowIDs, id)
		}
		slices.Sort(flowIDs)
		for _, id := range flowIDs[min(1, len(flowIDs)):] {
			problems = append(problems, Problem{
				File: byFlow[id].file,
				Line: byFlow[id].line,
				Msg: "flow " + id + " signs in " + subject + ", which flow " + flowIDs[0] + " also uses; " +
					"flows share one stack in E2E, so give " + id + " its own label (for example \"gif-ok\") " +
					"and add its fake Privy user under internal/testkit/fakes/testdata/fakes/privy/v1/users",
			})
		}
	}
	return problems
}
