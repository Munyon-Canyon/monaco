package gen

import (
	"os"
	"path/filepath"
)

const fakesFixtures = "internal/testkit/fakes/testdata/fakes"

func planProvider(_ *os.Root, modPath string, args []string) (Plan, error) {
	name := args[0]
	if !modulePattern.MatchString(name) {
		return Plan{}, invalid("gen.planProvider", "provider %q must match %s", name, modulePattern)
	}
	d := newData(modPath, name, name)
	dir := filepath.Join("internal", "providers", name)
	return Plan{Create: renderAll(d, map[string]string{
		filepath.Join(dir, "client.go"):                         "provider/client.go.tmpl",
		filepath.Join(dir, "client_test.go"):                    "provider/client_test.go.tmpl",
		filepath.Join(dir, "main_test.go"):                      "provider/main_test.go.tmpl",
		filepath.Join(dir, name+"fake", "fake.go"):              "provider/fake.go.tmpl",
		filepath.Join(fakesFixtures, name, "_health.json"):      "provider/health.json.tmpl",
		filepath.Join(fakesFixtures, name, "things", "t1.json"): "provider/thing.json.tmpl",
	})}, nil
}
