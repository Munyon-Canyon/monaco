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
)

func everyFlow() []string {
	return []string{
		AppRoot + "/Package.swift",
		"apps/backend/internal/tools/flows/",
		"apps/backend/internal/testkit/scenarios/",
		"apps/backend/internal/testkit/fakes/",
	}
}

func Affected(changed []string, flows []Flow, apps map[string]AppRow) []string {
	hit := map[string]bool{}
	for _, file := range changed {
		for _, f := range flows {
			if touches(file, f, apps[f.ID]) {
				hit[f.ID] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(hit))
}

func selected(ids []string, id string) bool { return len(ids) == 0 || slices.Contains(ids, id) }

func touches(file string, f Flow, app AppRow) bool {
	switch {
	case slices.ContainsFunc(everyFlow(), func(p string) bool {
		return file == p || strings.HasSuffix(p, "/") && strings.HasPrefix(file, p)
	}):
		return true
	case file == "apps/backend/api/openapi.yaml" || strings.HasPrefix(file, "apps/backend/api/spec/"):
		return app.Status.AtLeastBuilt()
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
