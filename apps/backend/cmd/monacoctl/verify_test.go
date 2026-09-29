package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func scratchModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := "module " + backendModule + "\n\ngo 1.25\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, flows.File), []byte(flows.Header+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVerifyTool_exitsTwoOnBadArgumentsAndOneWhenItCannotRun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, wd string
		args     []string
		code     int
		want     string
	}{
		{"usage", backendRoot(t), []string{"flow"}, 2, "bad arguments: flow needs a value\nusage: monacoctl verify"},
		{"no module", "/", nil, 1, "cannot find module"},
		{"run", scratchModule(t), []string{"all"}, 1, "monacoctl verify: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := verifyTool(nil, tc.wd, "/nonexistent/go")(tc.args, &stdout, &stderr)
			if code != tc.code || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("verify %q = %d, stderr %q, want %d and %q", tc.args, code, stderr.String(), tc.code, tc.want)
			}
		})
	}
}
