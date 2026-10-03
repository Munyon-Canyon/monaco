package gen

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func planFlow(root *os.Root, modPath string, args []string) (Plan, error) {
	const op = "gen.planFlow"
	all, problems, err := flows.ReadAll(os.DirFS(filepath.Join(root.Name(), "..", "..")))
	if err != nil {
		return Plan{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	i := slices.IndexFunc(all, func(f flows.Flow) bool { return f.ID == args[0] })
	if i < 0 {
		return Plan{}, invalid(op, "%s/%s.tsv has no valid row with id %q (%d problems)",
			flows.Dir, args[0], args[0], len(problems))
	}
	f := all[i]
	if len(f.Commands) == 0 {
		return Plan{}, invalid(op, "flow %s has no command to name its tests after", f.ID)
	}
	for _, command := range f.Commands {
		if err := requireModule(root, f.Module, command, exportPattern); err != nil {
			return Plan{}, err
		}
	}
	d := newData(modPath, f.Module, f.Commands[0])
	for _, command := range f.Commands {
		for _, o := range f.Outcomes {
			d.Tests = append(d.Tests, flows.TestName(f, command, o))
		}
	}
	return Plan{Create: renderAll(d, map[string]string{
		filepath.Join(moduleDir(f.Module), "flow"+f.ID+"_test.go"): "flow/flow_test.go.tmpl",
	})}, nil
}
