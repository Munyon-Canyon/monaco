package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGenLinksNoGeneratedCode(t *testing.T) {
	t.Parallel()
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		rel, ok := strings.CutPrefix(pkg, backendModule+"/")
		if !ok {
			continue
		}
		if rel == apiDir || strings.HasPrefix(rel, apiDir+"/") || strings.HasSuffix(rel, "/sqlc") ||
			strings.HasPrefix(rel, "internal/modules/") {
			t.Errorf("cmd/gen links %s; a generator that imports generated code cannot rebuild it after a restack", pkg)
		}
	}
}
