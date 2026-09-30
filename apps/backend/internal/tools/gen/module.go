package gen

import (
	"os"
	"path/filepath"
	"strings"
)

const changelogAdded = "### Added\n\n"

func planModule(_ *os.Root, modPath string, args []string) (Plan, error) {
	name := args[0]
	if !modulePattern.MatchString(name) {
		return Plan{}, invalid("gen.planModule", "module %q must match %s", name, modulePattern)
	}
	d := newData(modPath, name, name)
	dir := moduleDir(name)
	return Plan{
		Create: renderAll(d, map[string]string{
			filepath.Join(dir, "module.go"):               "module/module.go.tmpl",
			filepath.Join(dir, "main_test.go"):            "module/main_test.go.tmpl",
			filepath.Join(dir, "domain", "domain.go"):     "module/domain.go.tmpl",
			filepath.Join(dir, "app", "app.go"):           "module/app.go.tmpl",
			filepath.Join(dir, "adapters", "adapters.go"): "module/adapters.go.tmpl",
			filepath.Join("queries", name, ".gitkeep"):    "module/gitkeep.tmpl",
		}),
		Edit: map[string]func(string) (string, error){"CHANGELOG.md": changelogStub(name)},
	}, nil
}

func changelogStub(name string) func(string) (string, error) {
	line := "- The `" + name + "` module.\n"
	const op = "gen.changelogStub"
	return func(old string) (string, error) {
		heading := strings.Index("\n"+old, "\n## [Unreleased]\n")
		if heading < 0 {
			return "", invalid(op, "no ## [Unreleased] section")
		}
		unreleased := old[heading+len("## [Unreleased]"):]
		at := strings.Index(unreleased, changelogAdded)
		if next := strings.Index(unreleased, "\n## "); at < 0 || (next >= 0 && next < at) {
			return "", invalid(op, "the Unreleased section has no ### Added list")
		}
		cut := len(old) - len(unreleased) + at + len(changelogAdded)
		entry := line
		if strings.HasPrefix(old[cut:], "#") {
			entry += "\n"
		}
		return old[:cut] + entry + old[cut:], nil
	}
}
