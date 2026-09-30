package verify

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

type Unit struct {
	Flow    tools.Flow
	Outcome tools.Outcome
	Script  flows.Script
}

func (u Unit) Name() string { return u.Flow.ID + " " + string(u.Outcome) }

func readFlows(dir string) ([]tools.Flow, error) {
	file, err := os.Open(filepath.Join(dir, tools.File))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", tools.File, err)
	}
	defer func() { _ = file.Close() }()
	parsed, problems := tools.Parse(file)
	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s", fs.ErrInvalid, problems[0])
	}
	return parsed, nil
}

func selectUnits(all []tools.Flow, target Target, scripts map[string]flows.Script) ([]Unit, error) {
	var units []Unit
	for _, f := range all {
		if target.Flow != "" && f.ID != target.Flow || !f.Status.AtLeastBuilt() {
			continue
		}
		for _, o := range f.Outcomes {
			if !wanted(o, target) {
				continue
			}
			script, ok := scripts[tools.ScriptName(f, o)]
			if !ok {
				return nil, fmt.Errorf("%w: flow %s outcome %s has no script %s in internal/testkit/flows",
					fs.ErrNotExist, f.ID, o, tools.ScriptName(f, o))
			}
			units = append(units, Unit{Flow: f, Outcome: o, Script: script})
		}
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("%w: no built flow outcome matches %+v", fs.ErrNotExist, target)
	}
	return units, nil
}

func workerEnv(units []Unit, env map[string][]string) ([]string, error) {
	type setting struct{ flow, value string }
	set := map[string]setting{}
	var out []string
	for _, u := range units {
		for _, kv := range env[u.Flow.ID] {
			key, value, _ := strings.Cut(kv, "=")
			prev, seen := set[key]
			switch {
			case !seen:
				set[key] = setting{u.Flow.ID, value}
				out = append(out, kv)
			case prev.value != value:
				return nil, fmt.Errorf("%w: flows %s and %s set %s to %q and %q",
					errWorkerEnv, prev.flow, u.Flow.ID, key, prev.value, value)
			}
		}
	}
	return out, nil
}

func wanted(o tools.Outcome, target Target) bool {
	point, crash := o.CrashPoint()
	switch {
	case target.Outcome != "":
		return string(o) == target.Outcome
	case target.CrashAt != "":
		return point == target.CrashAt
	default:
		return !crash
	}
}
