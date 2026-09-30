package flows

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	File   = "flows.tsv"
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
	Line      int
	ID        string
	Name      string
	Module    string
	Trigger   string
	Command   string
	Events    []string
	Consumers []string
	Outcomes  []Outcome
	Status    Status
	Doc       string
}

func TestName(f Flow, o Outcome) string {
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
	return "TestFlow" + f.ID + "_" + f.Command + "_" + suffix
}

type Problem struct {
	Line int
	Msg  string
}

func (p Problem) String() string {
	if p.Line == 0 {
		return File + ": " + p.Msg
	}
	return fmt.Sprintf("%s:%d: %s", File, p.Line, p.Msg)
}

func problemf(line int, format string, args ...any) Problem {
	return Problem{Line: line, Msg: fmt.Sprintf(format, args...)}
}

var (
	idPattern         = regexp.MustCompile(`^[0-9]+[a-z]?$`)
	identPattern      = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	crashPointPattern = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)
)

func Parse(r io.Reader) ([]Flow, []Problem) {
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
				problems = append(problems, problemf(line, "header must be %q", Header))
			}
			continue
		}
		if strings.TrimSpace(text) == "" {
			problems = append(problems, problemf(line, "blank line"))
			continue
		}
		f, rowProblems := parseRow(line, text)
		problems = append(problems, rowProblems...)
		if len(rowProblems) == 0 {
			flows = append(flows, f)
		}
	}
	if err := scanner.Err(); err != nil {
		problems = append(problems, problemf(line, "read: %v", err))
	}
	if line == 0 {
		problems = append(problems, problemf(1, "header must be %q", Header))
	}
	return flows, problems
}

func parseRow(line int, text string) (Flow, []Problem) {
	cells := strings.Split(text, "\t")
	want := strings.Count(Header, "\t") + 1
	if len(cells) != want {
		return Flow{}, []Problem{problemf(line, "has %d columns, want %d", len(cells), want)}
	}
	f := Flow{
		Line:      line,
		ID:        cells[0],
		Name:      cells[1],
		Module:    cells[2],
		Trigger:   cells[3],
		Command:   cells[4],
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
	bad := func(format string, args ...any) { problems = append(problems, problemf(f.Line, format, args...)) }
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
	if f.Command != "" && !identPattern.MatchString(f.Command) {
		bad("command %q is not an exported Go identifier", f.Command)
	}
	if f.Status.AtLeastBuilt() && f.Command == "" {
		bad("command is empty on a %s flow", f.Status)
	}
	return append(problems, outcomeShape(f)...)
}

func outcomeShape(f Flow) []Problem {
	if len(f.Outcomes) == 0 {
		return []Problem{problemf(f.Line, "outcomes is empty")}
	}
	var problems []Problem
	for _, o := range f.Outcomes {
		if point, ok := o.CrashPoint(); ok && !crashPointPattern.MatchString(point) {
			problems = append(problems, problemf(f.Line, "outcome %s must name a kebab-case crash point", o))
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
