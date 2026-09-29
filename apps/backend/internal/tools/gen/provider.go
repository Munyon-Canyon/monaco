package gen

import (
	"os"
	"path/filepath"
)

func planProvider(root *os.Root, args []string) (Plan, error) {
	name := args[0]
	if !modulePattern.MatchString(name) {
		return Plan{}, invalid("gen.planProvider", "provider %q must match %s", name, modulePattern)
	}
	d, err := newData(root, name, name)
	if err != nil {
		return Plan{}, err
	}
	dir := filepath.Join("internal", "providers", name)
	create, err := renderAll(d, map[string]string{
		filepath.Join(dir, "client.go"):                         "provider/client.go.tmpl",
		filepath.Join(dir, "client_test.go"):                    "provider/client_test.go.tmpl",
		filepath.Join(dir, "main_test.go"):                      "provider/main_test.go.tmpl",
		filepath.Join(dir, name+"fake", "fake.go"):              "provider/fake.go.tmpl",
		filepath.Join(fakesFixtures, name, "_health.json"):      "provider/health.json.tmpl",
		filepath.Join(fakesFixtures, name, "things", "t1.json"): "provider/thing.json.tmpl",
	})
	return Plan{Create: create}, err
}

const fakesFixtures = "internal/testkit/fakes/testdata/fakes"
