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
	Command string
	Outcome tools.Outcome
	Script  flows.Script
}

func (u Unit) Name() string {
	if len(u.Flow.Commands) > 1 {
		return u.Flow.ID + " " + u.Command + " " + string(u.Outcome)
	}
	return u.Flow.ID + " " + string(u.Outcome)
}

func readFlows(dir string) ([]tools.Flow, error) {
	parsed, problems, err := tools.ReadAll(os.DirFS(filepath.Join(dir, "..", "..")))
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s", fs.ErrInvalid, problems[0])
	}
	return parsed, nil
}

func selectUnits(all []tools.Flow, target Target, scripts map[string]flows.Script) ([]Unit, error) {
	var units []Unit
	for _, f := range all {
		if !selectFlow(f, target) {
			continue
		}
		for _, o := range f.Outcomes {
			if !wanted(o, target) {
				continue
			}
			found, err := outcomeUnits(f, o, target, scripts)
			if err != nil {
				return nil, err
			}
			units = append(units, found...)
		}
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("%w: no built flow outcome matches %+v", fs.ErrNotExist, target)
	}
	return units, nil
}

func selectFlow(f tools.Flow, target Target) bool {
	return f.Status.AtLeastBuilt() && (target.Flow == "" || f.ID == target.Flow)
}

func outcomeUnits(
	f tools.Flow, o tools.Outcome, target Target, scripts map[string]flows.Script,
) ([]Unit, error) {
	var units []Unit
	names := make([]string, 0, len(f.Commands))
	for _, command := range f.Commands {
		name := tools.ScriptName(f, command, o)
		names = append(names, name)
		if script, ok := scripts[name]; ok {
			units = append(units, Unit{Flow: f, Command: command, Outcome: o, Script: script})
		}
	}
	if len(units) > 0 || (target.Flow == "" && target.CrashAt != "") {
		return units, nil
	}
	return nil, fmt.Errorf("%w: flow %s outcome %s has no script %s in internal/testkit/flows",
		fs.ErrNotExist, f.ID, o, strings.Join(names, " or "))
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
