package gen

import (
	"os"
	"path/filepath"
)

func planCommand(root *os.Root, modPath string, args []string) (Plan, error) {
	module, name := args[0], args[1]
	if err := requireModule(root, module, name, exportPattern); err != nil {
		return Plan{}, err
	}
	d := newData(modPath, module, name)
	dir := moduleDir(module)
	return Plan{Create: renderAll(d, map[string]string{
		filepath.Join(dir, "app", d.File+".go"): "command/command.go.tmpl",
		filepath.Join(dir, d.File+"_test.go"):   "command/command_test.go.tmpl",
	})}, nil
}
