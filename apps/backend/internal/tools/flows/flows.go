package flows

import (
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	Dir    = AppRoot + "/backend"
	Header = "id\tflow\tmodule\ttrigger\tcommand\tevents\tconsumers\toutcomes\tstatus\tdoc"
)

type Status string

const (
	StatusPlanned  Status = "planned"
	StatusBuilt    Status = "built"
	StatusVerified Status = "verified"
)

func (s Status) valid() bool {
	return s == StatusPlanned || s == StatusBuilt || s == StatusVerified
}

func (s Status) AtLeastBuilt() bool { return s == StatusBuilt || s == StatusVerified }

type Outcome string

const OutcomeOK Outcome = "ok"

const crashPrefix = "crash:"

func (o Outcome) CrashPoint() (string, bool) {
	return strings.CutPrefix(string(o), crashPrefix)
}

func (o Outcome) CodeName() (string, bool) {
	if _, crash := o.CrashPoint(); crash || o == OutcomeOK {
		return "", false
	}
	return string(o), true
}

type Flow struct {
	File      string
	Line      int
	ID        string
	Name      string
	Module    string
	Trigger   string
	Commands  []string
	Events    []string
	Consumers []string
	Outcomes  []Outcome
	Status    Status
	Doc       string
}

func TestName(f Flow, command string, o Outcome) string {
	suffix := string(o)
	if o == OutcomeOK {
		suffix = "OK"
	} else if point, ok := o.CrashPoint(); ok {
		var b strings.Builder
		b.WriteString("Crash")
		for part := range strings.SplitSeq(point, "-") {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
		suffix = b.String()
	}
	return "TestFlow" + f.ID + "_" + command + "_" + suffix
}

type Problem struct {
	File string
	Line int
	Msg  string
}

func (p Problem) String() string {
	file := cmp.Or(p.File, Dir)
	if p.Line == 0 {
		return file + ": " + p.Msg
	}
	return fmt.Sprintf("%s:%d: %s", file, p.Line, p.Msg)
}

func problemf(line int, format string, args ...any) Problem {
	return Problem{Line: line, Msg: fmt.Sprintf(format, args...)}
}

func fileProblemf(file string, line int, format string, args ...any) Problem {
	return Problem{File: file, Line: line, Msg: fmt.Sprintf(format, args...)}
}

func (f Flow) problemf(format string, args ...any) Problem {
	return Problem{File: f.File, Line: f.Line, Msg: fmt.Sprintf(format, args...)}
}

func ReadAll(repo fs.FS) ([]Flow, []Problem, error) {
	files, _ := fs.Glob(repo, Dir+"/*.tsv")
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("read %s: no flow files: %w", Dir, fs.ErrNotExist)
	}
	var (
		all      []Flow
		problems []Problem
	)
	for _, name := range files {
		body, err := fs.ReadFile(repo, name)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", name, err)
		}
		parsed, fileProblems := Parse(name, bytes.NewReader(body))
		if id := strings.TrimSuffix(path.Base(name), ".tsv"); len(fileProblems) == 0 {
			switch {
			case len(parsed) != 1:
				fileProblems = append(fileProblems, fileProblemf(name, 0, "has %d rows, want 1", len(parsed)))
			case parsed[0].ID != id:
				fileProblems = append(fileProblems, parsed[0].problemf(
					"id %s differs from the file name; rename the file to %s.tsv or set the id to %s",
					parsed[0].ID,
					parsed[0].ID,
					id,
				))
			}
		}
		problems = append(problems, fileProblems...)
		if len(fileProblems) == 0 {
			all = append(all, parsed[0])
		}
	}
	sort.Slice(all, func(i, j int) bool { return Less(all[i].ID, all[j].ID) })
	return all, problems, nil
}

func Less(a, b string) bool {
	numA, _ := strconv.Atoi(strings.TrimRight(a, "abcdefghijklmnopqrstuvwxyz"))
	numB, _ := strconv.Atoi(strings.TrimRight(b, "abcdefghijklmnopqrstuvwxyz"))
	if numA != numB {
		return numA < numB
	}
	return a < b
}

var (
	idPattern         = regexp.MustCompile(`^[0-9]+[a-z]?$`)
	identPattern      = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	crashPointPattern = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)
)

func Parse(file string, r io.Reader) ([]Flow, []Problem) {
	var (
		flows    []Flow
		problems []Problem
	)
	scanner := bufio.NewScanner(r)
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		if line == 1 {
			if text != Header {
				problems = append(problems, fileProblemf(file, line, "header must be %q", Header))
			}
			continue
		}
		if strings.TrimSpace(text) == "" {
			problems = append(problems, fileProblemf(file, line, "blank line"))
			continue
		}
		f, rowProblems := parseRow(file, line, text)
		problems = append(problems, rowProblems...)
		if len(rowProblems) == 0 {
			flows = append(flows, f)
		}
	}
	if err := scanner.Err(); err != nil {
		problems = append(problems, fileProblemf(file, line, "read: %v", err))
	}
	if line == 0 {
		problems = append(problems, fileProblemf(file, 1, "header must be %q", Header))
	}
	return flows, problems
}

func parseRow(file string, line int, text string) (Flow, []Problem) {
	cells := strings.Split(text, "\t")
	want := strings.Count(Header, "\t") + 1
	if len(cells) != want {
		return Flow{}, []Problem{fileProblemf(file, line, "has %d columns, want %d", len(cells), want)}
	}
	f := Flow{
		File:      file,
		Line:      line,
		ID:        cells[0],
		Name:      cells[1],
		Module:    cells[2],
		Trigger:   cells[3],
		Commands:  list(cells[4]),
		Events:    list(cells[5]),
		Consumers: list(cells[6]),
		Status:    Status(cells[8]),
		Doc:       cells[9],
	}
	for _, o := range list(cells[7]) {
		f.Outcomes = append(f.Outcomes, Outcome(o))
	}
	return f, rowShape(f)
}

func rowShape(f Flow) []Problem {
	var problems []Problem
	bad := func(format string, args ...any) { problems = append(problems, f.problemf(format, args...)) }
	if !idPattern.MatchString(f.ID) {
		bad("id %q must be digits with at most one lowercase letter after them", f.ID)
	}
	for _, c := range [...]struct{ column, value string }{{"flow", f.Name}, {"module", f.Module}, {"doc", f.Doc}} {
		if c.value == "" {
			bad("%s is empty", c.column)
		}
	}
	if !f.Status.valid() {
		bad("status %q is not planned, built or verified", f.Status)
	}
	for _, command := range f.Commands {
		if !identPattern.MatchString(command) {
			bad("command %q is not an exported Go identifier", command)
		}
	}
	if n := len(f.Triggers()); n > 1 && n != len(f.Commands) {
		bad("trigger lists %d triggers for %d commands; list one, or one per command in order", n, len(f.Commands))
	}
	if f.Status.AtLeastBuilt() && len(f.Commands) == 0 {
		bad("command is empty on a %s flow", f.Status)
	}
	return append(problems, outcomeShape(f)...)
}

func outcomeShape(f Flow) []Problem {
	if len(f.Outcomes) == 0 {
		return []Problem{f.problemf("outcomes is empty")}
	}
	var problems []Problem
	for _, o := range f.Outcomes {
		if point, ok := o.CrashPoint(); ok && !crashPointPattern.MatchString(point) {
			problems = append(problems, f.problemf("outcome %s must name a kebab-case crash point", o))
		}
	}
	return problems
}

func list(cell string) []string {
	var out []string
	for item := range strings.SplitSeq(cell, ";") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
