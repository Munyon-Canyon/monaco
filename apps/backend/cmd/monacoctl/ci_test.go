package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func ciModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                      "module example.com/m\n\ngo 1.25.0\n",
		"README.md":                   "m\n",
		"leaf/leaf.go":                "package leaf\n",
		"shared/shared.go":            "package shared\n\nfunc S() int { return 1 }\n",
		"mid/mid.go":                  "package mid\n\nimport \"example.com/m/shared\"\n\nfunc M() int { return shared.S() }\n",
		"top/top.go":                  "package top\n\nimport \"example.com/m/mid\"\n\nfunc T() int { return mid.M() }\n",
		"user/user.go":                "package user\n\nimport _ \"errors\"\n",
		"user/user_test.go":           "package user_test\n\nimport \"example.com/m/shared\"\n\nvar _ = shared.S\n",
		"internal/testkit/testkit.go": "package testkit\n",
	}
	for name, body := range files {
		writeCIFile(t, dir, name, body)
	}
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "tag", "base")
	return dir
}

func writeCIFile(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runCIAffected(env ciEnv, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := ciTool(env)(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestCIAffectedPrintsChangedPackagesAndTheirImporters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change map[string]string
		want   string
	}{
		{"leaf change", map[string]string{"leaf/leaf.go": "package leaf\n\nfunc L() {}\n"}, "./leaf\n"},
		{
			"shared-package change reaches importers and packages whose tests import it",
			map[string]string{"shared/shared.go": "package shared\n\nfunc S() int { return 2 }\n"},
			"./mid\n./shared\n./top\n./user\n",
		},
		{"go.mod change", map[string]string{"go.mod": "module example.com/m\n\ngo 1.25.1\n"}, "./...\n"},
		{"go.sum change", map[string]string{"go.sum": ""}, "./...\n"},
		{
			"testkit change",
			map[string]string{"internal/testkit/testkit.go": "package testkit\n\nvar X = 1\n"},
			"./...\n",
		},
		{"non-Go change", map[string]string{"README.md": "changed\n"}, "./...\n"},
		{"Go file outside any package", map[string]string{"leaf/testdata/x.go": "package x\n"}, "./...\n"},
		{"no change", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := ciModule(t)
			for name, body := range tc.change {
				writeCIFile(t, dir, name, body)
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")
			env := ciEnv{moduleDir: dir, goBin: "go", gitBin: "git", exec: runCommand}
			code, stdout, stderr := runCIAffected(env, "affected", "--base", "base")
			if code != 0 || stdout != tc.want {
				t.Fatalf("code=%d stdout=%q want %q stderr=%s", code, stdout, tc.want, stderr)
			}
		})
	}
}

func TestCIAffectedRejectsBadArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"unknown"}, {"affected"}, {"affected", "--base", "x", "extra"}, {"affected", "--nope"}} {
		code, stdout, stderr := runCIAffected(ciEnv{}, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, ciUsage) {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestCIAffectedFailsWhenGitOrGoListFails(t *testing.T) {
	t.Parallel()
	dir := ciModule(t)
	writeCIFile(t, dir, "leaf/leaf.go", "package leaf\n\nfunc L() {}\n")
	git(t, dir, "commit", "-q", "-am", "change")
	fakeGo := func(out string, err error) execFunc {
		return func(ctx context.Context, d string, env []string, name string, args ...string) ([]byte, error) {
			if name == "go" {
				return []byte(out), err
			}
			return runCommand(ctx, d, env, name, args...)
		}
	}
	cases := []struct {
		name, base, want string
		exec             execFunc
	}{
		{"unknown base", "nope", "bad revision", runCommand},
		{"go list fails", "base", "go list broke", fakeGo("", errs.New(errs.CodeInternal, "go list broke"))},
		{"go list prints bad JSON", "base", "decode_failed", fakeGo("{", nil)},
	}
	for _, tc := range cases {
		env := ciEnv{moduleDir: dir, goBin: "go", gitBin: "git", exec: tc.exec}
		code, stdout, stderr := runCIAffected(env, "affected", "--base", tc.base)
		if code != 1 || stdout != "" || !strings.Contains(stderr, tc.want) {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.name, code, stdout, stderr)
		}
	}
}

func TestToolCiIsRegistered(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	if code := run(nil, tools(nil), nil, []string{"ci"}, &bytes.Buffer{}, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), ciUsage) {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
