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
		for _, o := range f.Outcomes {
			name := TestName(f, o)
			owned[name] = true
			if !f.Status.AtLeastBuilt() {
				continue
			}
			passed, ran := results[name]
			switch {
			case !ran:
				problems = append(
					problems,
					problemf(f.Line, "outcome %s has no test %s in the go test -json input", o, name),
				)
			case !passed:
				problems = append(problems, problemf(f.Line, "outcome %s test %s failed", o, name))
			}
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

var flowTest = regexp.MustCompile(`^TestFlow[0-9]+[a-z]?_[^/]*$`)

func ScriptName(f Flow, o Outcome) string {
	return "F" + strings.ReplaceAll(strings.TrimPrefix(TestName(f, o), "TestFlow"), "_", "")
}

func CheckScripts(flows []Flow, env Env) []Problem {
	var problems []Problem
	for _, f := range flows {
		for _, o := range f.Outcomes {
			name := ScriptName(f, o)
			if env.Scripts(f, name) {
				continue
			}
			if problem, ok := missingScript(f, o, name); ok {
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
