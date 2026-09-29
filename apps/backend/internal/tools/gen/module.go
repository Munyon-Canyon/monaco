package gen

import (
	"os"
	"path/filepath"
	"strings"
)

const changelogAdded = "### Added\n\n"

func planModule(root *os.Root, args []string) (Plan, error) {
	name := args[0]
	if !modulePattern.MatchString(name) {
		return Plan{}, invalid("gen.planModule", "module %q must match %s", name, modulePattern)
	}
	d, err := newData(root, name, name)
	if err != nil {
		return Plan{}, err
	}
	dir := moduleDir(name)
	create, err := renderAll(d, map[string]string{
		filepath.Join(dir, "module.go"):               "module/module.go.tmpl",
		filepath.Join(dir, "main_test.go"):            "module/main_test.go.tmpl",
		filepath.Join(dir, "domain", "domain.go"):     "module/domain.go.tmpl",
		filepath.Join(dir, "app", "app.go"):           "module/app.go.tmpl",
		filepath.Join(dir, "adapters", "adapters.go"): "module/adapters.go.tmpl",
		filepath.Join("queries", name, ".gitkeep"):    "module/gitkeep.tmpl",
	})
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Create: create,
		Edit:   map[string]func(string) (string, error){"CHANGELOG.md": changelogStub(name)},
	}, nil
}

func changelogStub(name string) func(string) (string, error) {
	line := "- The `" + name + "` module.\n"
	const op = "gen.changelogStub"
	return func(old string) (string, error) {
		_, unreleased, ok := strings.Cut(old, "## [Unreleased]")
		if !ok {
			return "", invalid(op, "no ## [Unreleased] section")
		}
		at := strings.Index(unreleased, changelogAdded)
		if next := strings.Index(unreleased, "\n## "); at < 0 || (next >= 0 && next < at) {
			return "", invalid(op, "the Unreleased section has no ### Added list")
		}
		cut := len(old) - len(unreleased) + at + len(changelogAdded)
		return old[:cut] + line + old[cut:], nil
	}
}
