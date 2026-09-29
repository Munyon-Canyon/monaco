package flows

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
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

func ScriptName(f Flow, o Outcome) string {
	return "F" + strings.ReplaceAll(strings.TrimPrefix(TestName(f, o), "TestFlow"), "_", "")
}

var (
	flowTest = regexp.MustCompile(`^TestFlow[0-9]+_[^/]*$`)
	objectID = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
)

type Fresh func(module, sha string) (bool, error)

func EvidencePath(f Flow) string { return path.Join("test/evidence", f.ID+".json") }

func CheckEvidence(flows []Flow, env Env) []Problem {
	var problems []Problem
	for _, f := range flows {
		if f.Status != StatusVerified {
			continue
		}
		if msg := evidence(f, env); msg != "" {
			problems = append(problems, Problem{Line: f.Line, Msg: msg})
		}
	}
	return problems
}

func evidence(f Flow, env Env) string {
	file := EvidencePath(f)
	body, err := fs.ReadFile(env.Repo, path.Join(env.BackendDir, file))
	if err != nil {
		return "verified flow has no " + file
	}
	var stamp struct {
		SHA string `json:"sha"`
	}
	if json.Unmarshal(body, &stamp) != nil || stamp.SHA == "" {
		return file + " has no sha stamp"
	}
	if !objectID.MatchString(stamp.SHA) {
		return fmt.Sprintf("%s sha stamp %q is not a full git object id", file, stamp.SHA)
	}
	fresh, err := env.Fresh(f.Module, stamp.SHA)
	switch {
	case err != nil:
		return fmt.Sprintf("%s stamp %s: %v", file, stamp.SHA, err)
	case !fresh:
		return fmt.Sprintf(
			"%s stamp %s is older than the newest commit in internal/modules/%s",
			file,
			stamp.SHA,
			f.Module,
		)
	}
	return ""
}
