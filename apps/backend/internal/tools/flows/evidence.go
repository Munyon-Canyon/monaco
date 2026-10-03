package flows

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type TestResults map[string]bool

func ReadTestResults(r io.Reader) (TestResults, error) {
	results := TestResults{}
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		var event struct {
			Action string `json:"Action"`
			Test   string `json:"Test"`
		}
		if json.Unmarshal(line, &event) == nil && event.Test != "" {
			switch event.Action {
			case "pass":
				if _, seen := results[event.Test]; !seen {
					results[event.Test] = true
				}
			case "fail":
				results[event.Test] = false
			}
		}
		if errors.Is(err, io.EOF) {
			return results, nil
		}
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, "flows.ReadTestResults")
		}
	}
}

func CheckTests(flows []Flow, results TestResults) []Problem {
	var problems []Problem
	owned := map[string]bool{}
	for _, f := range flows {
		for _, name := range testNames(f) {
			owned[name] = true
		}
		if f.Status.AtLeastBuilt() {
			problems = append(problems, flowTestProblems(f, results)...)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(results)) {
		if flowTest.MatchString(name) && !owned[name] {
			problems = append(
				problems,
				problemf(0, "test %s matches no flow outcome; delete the test or add its row", name),
			)
		}
	}
	return problems
}

func testNames(f Flow) []string {
	names := make([]string, 0, len(f.Commands)*len(f.Outcomes))
	for _, o := range f.Outcomes {
		for _, command := range f.Commands {
			names = append(names, TestName(f, command, o))
		}
	}
	return names
}

func flowTestProblems(f Flow, results TestResults) []Problem {
	var problems []Problem
	tested := map[string]bool{}
	for _, o := range f.Outcomes {
		var names []string
		ran := false
		for _, command := range f.Commands {
			name := TestName(f, command, o)
			names = append(names, name)
			passed, seen := results[name]
			if !seen {
				continue
			}
			ran, tested[command] = true, true
			if !passed {
				problems = append(problems, problemf(f.Line, "outcome %s test %s failed", o, name))
			}
		}
		if !ran {
			problems = append(problems, problemf(f.Line,
				"outcome %s has no test %s in the go test -json input", o, strings.Join(names, " or ")))
		}
	}
	if len(f.Commands) < 2 {
		return problems
	}
	for _, command := range f.Commands {
		if !tested[command] {
			problems = append(
				problems,
				problemf(f.Line, "command %s has no flow test in the go test -json input", command),
			)
		}
	}
	return problems
}

var flowTest = regexp.MustCompile(`^TestFlow[0-9]+[a-z]?_[^/]*$`)

func ScriptName(f Flow, command string, o Outcome) string {
	return "F" + strings.ReplaceAll(strings.TrimPrefix(TestName(f, command, o), "TestFlow"), "_", "")
}

func CheckScripts(flows []Flow, env Env) []Problem {
	var problems []Problem
	for _, f := range flows {
		for _, o := range f.Outcomes {
			var names []string
			found := false
			for _, command := range f.Commands {
				name := ScriptName(f, command, o)
				names = append(names, name)
				found = found || env.Scripts(f, name)
			}
			if found {
				continue
			}
			if problem, ok := missingScript(f, o, strings.Join(names, " or ")); ok {
				problems = append(problems, problem)
			}
		}
	}
	return problems
}

func missingScript(f Flow, o Outcome, name string) (Problem, bool) {
	_, crash := o.CrashPoint()
	switch {
	case f.Status == StatusBuilt && !crash:
		return problemf(
			f.Line,
			"built flow outcome %s has no script %s in internal/testkit/flows; monacoctl verify all fails without it",
			o,
			name,
		), true
	case f.Status == StatusVerified:
		return problemf(
			f.Line,
			"verified flow outcome %s has no script %s in internal/testkit/flows for monacoctl verify all",
			o,
			name,
		), true
	default:
		return Problem{}, false
	}
}
