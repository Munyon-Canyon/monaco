package gen

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func planFlow(root *os.Root, args []string) (Plan, error) {
	const op = "gen.planFlow"
	raw, err := root.ReadFile(flows.File)
	if err != nil {
		return Plan{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	all, problems := flows.Parse(bytes.NewReader(raw))
	i := slices.IndexFunc(all, func(f flows.Flow) bool { return f.ID == args[0] })
	if i < 0 {
		return Plan{}, invalid(op, "%s has no valid row with id %q (%d problems)", flows.File, args[0], len(problems))
	}
	f := all[i]
	if f.Command == "" {
		return Plan{}, invalid(op, "flow %s has no command to name its tests after", f.ID)
	}
	if err := requireModule(root, f.Module, f.Command, exportPattern); err != nil {
		return Plan{}, err
	}
	d, err := newData(root, f.Module, f.Command)
	if err != nil {
		return Plan{}, err
	}
	for _, o := range f.Outcomes {
		d.Tests = append(d.Tests, flows.TestName(f, o))
	}
	create, err := renderAll(d, map[string]string{
		filepath.Join(moduleDir(f.Module), "flow"+f.ID+"_test.go"): "flow/flow_test.go.tmpl",
	})
	return Plan{Create: create}, err
}
