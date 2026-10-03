package flows

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	ModelRoot = "packages/mobile-core/Sources"
	AppRoot   = "packages/flows"
	AppDir    = AppRoot + "/app"
	AppHeader = "id\tscreen\tstatus\tdoc"
)

type AppStatus string

const (
	AppPlanned  AppStatus = "planned"
	AppBuilt    AppStatus = "built"
	AppVerified AppStatus = "verified"
	AppNone     AppStatus = "none"
)

func (s AppStatus) valid() bool {
	return s == AppPlanned || s == AppBuilt || s == AppVerified || s == AppNone
}

func (s AppStatus) AtLeastBuilt() bool { return s == AppBuilt || s == AppVerified }

type AppRow struct {
	Line   int
	File   string
	ID     string
	Screen string
	Status AppStatus
	Doc    string
}

func ReadApp(repo fs.FS) ([]AppRow, []Problem) {
	files, _ := fs.Glob(repo, AppDir+"/*.tsv")
	var (
		rows     []AppRow
		problems []Problem
	)
	for _, name := range files {
		body, err := fs.ReadFile(repo, name)
		if err != nil {
			problems = append(problems, Problem{File: name, Msg: "read: " + err.Error()})
			continue
		}
		row, ok, fileProblems := parseApp(name, string(body))
		problems = append(problems, fileProblems...)
		if ok {
			rows = append(rows, row)
		}
	}
	return rows, problems
}

func parseApp(name, body string) (AppRow, bool, []Problem) {
	var problems []Problem
	bad := func(line int, format string, args ...any) {
		problems = append(problems, Problem{File: name, Line: line, Msg: fmt.Sprintf(format, args...)})
	}
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	if lines[0] != AppHeader {
		bad(1, "header must be %q", AppHeader)
	}
	if len(lines) < 2 {
		bad(1, "has no data row; add one row after the header")
		return AppRow{}, false, problems
	}
	for i, extra := range lines[2:] {
		bad(i+3, "extra data row %q; keep exactly one row per file", extra)
	}
	cells := strings.Split(lines[1], "\t")
	if want := strings.Count(AppHeader, "\t") + 1; len(cells) != want {
		bad(2, "has %d columns, want %d", len(cells), want)
		return AppRow{}, false, problems
	}
	row := AppRow{Line: 2, File: name, ID: cells[0], Screen: cells[1], Status: AppStatus(cells[2]), Doc: cells[3]}
	return row, true, problems
}

func CheckApp(app []AppRow, backend []Flow, env Env) []Problem {
	ids := flowIDs(backend)
	backendFile := path.Join(env.BackendDir, File)
	docs := docAnchors{repo: env.Repo, cache: map[string]map[string]bool{}}
	var problems []Problem
	for _, r := range app {
		for _, msg := range appRowProblems(r, ids, backendFile, docs) {
			problems = append(problems, Problem{File: r.File, Line: r.Line, Msg: msg})
		}
	}
	return problems
}

func appRowProblems(r AppRow, ids map[string]bool, backendFile string, docs docAnchors) []string {
	var msgs []string
	if stem := strings.TrimSuffix(path.Base(r.File), ".tsv"); r.ID != stem {
		msgs = append(msgs, fmt.Sprintf(
			"id %s differs from the file name; rename the file to %s.tsv or set the id to %s", r.ID, r.ID, stem))
	} else if !ids[r.ID] {
		msgs = append(msgs, fmt.Sprintf(
			"id %s is not in %s; add the backend row first or delete this file", r.ID, backendFile))
	}
	switch {
	case !r.Status.valid():
		msgs = append(msgs, fmt.Sprintf("status %q is not planned, built, verified or none", r.Status))
	case r.Status == AppNone && r.Screen != "-":
		msgs = append(msgs, fmt.Sprintf("screen %q must be - on a none flow, which has no app surface", r.Screen))
	case r.Status != AppNone && (r.Screen == "" || r.Screen == "-"):
		msgs = append(msgs, fmt.Sprintf("screen is empty on a %s flow; name the screen or set status none", r.Status))
	}
	if r.Doc == "" {
		msgs = append(msgs, "doc is empty; point it at path#anchor")
	} else if msg := docs.check(r.Doc); msg != "" {
		msgs = append(msgs, msg)
	}
	return msgs
}

func CheckAppModels(app []AppRow, backend []Flow, env Env) []Problem {
	modules := map[string]string{}
	for _, f := range backend {
		modules[f.ID] = f.Module
	}
	var problems []Problem
	for _, r := range app {
		module, ok := modules[r.ID]
		if !r.Status.AtLeastBuilt() || !ok || module == "" {
			continue
		}
		dir := path.Join(ModelRoot, moduleTarget(module))
		if models, _ := fs.Glob(env.Repo, path.Join(dir, "Flow"+r.ID+"*.swift")); len(models) > 0 {
			continue
		}
		problems = append(problems, Problem{File: r.File, Line: r.Line, Msg: fmt.Sprintf(
			"status %s but no model Flow%s*.swift in %s", r.Status, r.ID, dir)})
	}
	return problems
}

func moduleTarget(module string) string {
	return "Monaco" + strings.ToUpper(module[:1]) + module[1:]
}

func flowIDs(backend []Flow) map[string]bool {
	ids := map[string]bool{}
	for _, f := range backend {
		ids[f.ID] = true
	}
	return ids
}

var flowIDToken = regexp.MustCompile(`\b[0-9]+[a-z]?\b`)

func CheckNoAggregate(repo fs.FS, backend []Flow) []Problem {
	ids := flowIDs(backend)
	var problems []Problem
	_ = fs.WalkDir(repo, AppRoot, func(name string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == ".build":
			return fs.SkipDir
		case d.IsDir() || name == AppRoot+"/README.md":
			return nil
		}
		if p, ok := aggregate(repo, name, ids); ok {
			problems = append(problems, p)
		}
		return nil
	})
	return problems
}

func aggregate(repo fs.FS, name string, ids map[string]bool) (Problem, bool) {
	body, err := fs.ReadFile(repo, name)
	if err != nil {
		return Problem{File: name, Msg: "read: " + err.Error()}, true
	}
	var named []string
	for _, token := range flowIDToken.FindAllString(string(body), -1) {
		if ids[token] && !slices.Contains(named, token) {
			named = append(named, token)
		}
	}
	if len(named) < 2 {
		return Problem{}, false
	}
	return Problem{
		File: name,
		Msg:  fmt.Sprintf("names flows %s and %s; keep one flow per file", named[0], named[1]),
	}, true
}
