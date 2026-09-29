package gen

import (
	"os"
	"path/filepath"
)

func planCommand(root *os.Root, args []string) (Plan, error) {
	module, name := args[0], args[1]
	if err := requireModule(root, module, name, exportPattern); err != nil {
		return Plan{}, err
	}
	d, err := newData(root, module, name)
	if err != nil {
		return Plan{}, err
	}
	dir := moduleDir(module)
	create, err := renderAll(d, map[string]string{
		filepath.Join(dir, "app", d.File+".go"): "command/command.go.tmpl",
		filepath.Join(dir, d.File+"_test.go"):   "command/command_test.go.tmpl",
	})
	return Plan{Create: create}, err
}
