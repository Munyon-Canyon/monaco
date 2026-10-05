package flows

import (
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
)

var (
	idFile     = regexp.MustCompile(`^([0-9]+[a-z]?)\.tsv$`)
	flowPrefix = regexp.MustCompile(`^Flow([0-9]+[a-z]?)(?:[^a-z0-9]|$)`)
	testPrefix = regexp.MustCompile(`^F(?:low)?([0-9]+[a-z]?)(?:[^a-z0-9]|$)`)
	scriptFile = regexp.MustCompile(`^f([0-9]+[a-z]?)\.go$`)
	namedFile  = regexp.MustCompile(`^(?:[Ff](?:low)?)?([0-9]+[a-z]?)(?:[^a-z0-9]|$)`)
)

func everyFlow(file string) bool {
	return file == AppRoot+"/Package.swift" || strings.HasPrefix(file, "apps/backend/internal/tools/flows/")
}

func harness(file string) bool {
	return strings.HasPrefix(file, "apps/backend/internal/testkit/scenarios/") ||
		strings.HasPrefix(file, "apps/backend/internal/testkit/fakes/")
}

func TestOnly(file string) bool {
	switch {
	case strings.HasSuffix(file, "_test.go"), strings.Contains("/"+file, "/testdata/"):
		return true
	case strings.HasPrefix(file, "apps/backend/internal/testkit/"):
		return ownerID(file) == "" && (!harness(file) || match(namedFile, path.Base(file)) == "")
	}
	return false
}

func Affected(changed, ops []string, flows []Flow) []string {
	hit := map[string]bool{}
	for _, file := range changed {
		for _, f := range flows {
			if touches(file, f, ops) {
				hit[f.ID] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(hit))
}

func selected(ids []string, id string) bool { return len(ids) == 0 || slices.Contains(ids, id) }

func touches(file string, f Flow, ops []string) bool {
	switch {
	case everyFlow(file):
		return true
	case SpecFile(file):
		return slices.ContainsFunc(f.Triggers(), func(t string) bool { return slices.Contains(ops, t) })
	case harness(file):
		if id := match(namedFile, path.Base(file)); id != "" {
			return id == f.ID
		}
		return strings.HasSuffix(file, ".go") && !strings.HasSuffix(file, "_test.go")
	case strings.HasPrefix(file, "apps/backend/internal/modules/"):
		module, _, _ := strings.Cut(strings.TrimPrefix(file, "apps/backend/internal/modules/"), "/")
		return module == f.Module
	}
	return ownerID(file) == f.ID
}

func ownerID(file string) string {
	dir, base := path.Split(file)
	switch {
	case dir == Dir+"/" || dir == AppDir+"/":
		return match(idFile, base)
	case dir == AppRoot+"/Sources/MonacoFlows/" || strings.HasPrefix(dir, ModelRoot+"/"):
		return match(flowPrefix, base)
	case strings.HasPrefix(dir, "packages/mobile-core/Tests/"):
		return match(testPrefix, base)
	case dir == "apps/backend/internal/testkit/flows/":
		return match(scriptFile, base)
	}
	return ""
}

func match(re *regexp.Regexp, name string) string {
	if m := re.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}
