package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func mutationModule(t *testing.T, allow string, extraPkgs ...string) mutationEnv {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":           "module example.com/m\n\ngo 1.25.0\n",
		coverageExclude:    "*.gen.go\nkit/\n",
		mutantsAllow:       allow,
		"a/x.go":           "package a\n\nfunc A() int { return 1 }\n",
		"b/x.go":           "package b\n\nimport \"example.com/m/a\"\n\nfunc B() int { return a.A() }\n",
		"c/x.go":           "package c\n\nfunc C() int { return 3 }\n",
		"kit/x.go":         "package kit\n\nimport \"example.com/m/a\"\n\nfunc K() int { return a.A() }\n",
		"a/testdata/x.txt": "fixture\n",
	}
	for _, p := range extraPkgs {
		files[p+"/x.go"] = "package " + filepath.Base(
			p,
		) + "\n\nimport \"example.com/m/a\"\n\nfunc X() int { return a.A() }\n"
	}
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "checkout", "-q", "-b", "feature")
	gremlins, err := filepath.Abs(filepath.Join("testdata", "gremlins"))
	if err != nil {
		t.Fatal(err)
	}
	return mutationEnv{moduleDir: dir, gremlins: gremlins, tmpDir: t.TempDir()}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(
		context.Background(),
		"git",
		append(
			[]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"},
			args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, env mutationEnv, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(env.moduleDir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, env.moduleDir, "add", ".")
	git(t, env.moduleDir, "commit", "-q", "-m", "change")
}

func TestMutationFailsOnASurvivorInAChangedPackageAndItsDependents(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	commitFile(t, env, "a/x_test.go", "package a\n")
	var stdout, stderr bytes.Buffer
	code := mutationTool(env)(nil, &stdout, &stderr)
	wantErr := "monacoctl mutation: a/x.go:3:5 CONDITIONALS_NEGATION survived; kill it with a test or list it in mutants.allow with a reason\n"
	if code != 1 || stdout.String() != "mutating 2 packages: a b\n" || stderr.String() != wantErr {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestMutationPassesWhenTheSurvivorIsAllowedWithAReason(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "\na/x.go:3:5\tCONDITIONALS_NEGATION\tequivalent: both branches return 1\n")
	commitFile(t, env, "a/x.go", "package a\n\nfunc A() int { return 2 }\n")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)([]string{"--base", "main"}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestMutationSkipsUnchangedPackagesNonGoFilesAndExcludedPaths(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	commitFile(t, env, "a/testdata/x.txt", "changed\n")
	commitFile(t, env, "c/x.go", "package c\n\nfunc C() int { return 4 }\n")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)(nil, &stdout, &stderr); code != 0 || stdout.String() != "mutating 1 packages: c\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := mutationTool(
		env,
	)(
		[]string{"--all"},
		&stdout,
		&stderr,
	); code != 1 ||
		stdout.String() != "mutating 3 packages: a b c\n" {
		t.Fatalf("--all: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestMutationReportsBrokenInputs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, allow, pkg, change, body, remove string
		noTmp                                  bool
		args                                   []string
		want                                   string
	}{
		{
			name: "allow line without a reason", allow: "a/x.go:3:5\tCONDITIONALS_NEGATION\t \n",
			want: "monacoctl.readAllow: decode_failed: mutants.allow:1: want file:line:col<TAB>MUTATOR<TAB>reason",
		},
		{name: "no allow file", remove: mutantsAllow, want: "monacoctl.readAllow: internal: open"},
		{name: "no exclude file", remove: coverageExclude, want: "monacoctl.mutation: internal: open"},
		{name: "go list fails", change: "c/x.go", body: "package c\n\nimport \"example.com/m/missing\"\n", want: "monacoctl.goList: internal"},
		{name: "unknown base", args: []string{"--base", "nope"}, want: "monacoctl.changedFiles: internal"},
		{name: "gremlins fails", pkg: "broken", want: "gremlins exploded"},
		{name: "no temp dir for the report", pkg: "broken", noTmp: true, want: "monacoctl.unleash: internal: open"},
		{name: "gremlins writes garbage", pkg: "garbled", want: "monacoctl.unleash: decode_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := brokenModule(t, tc.allow, tc.pkg, tc.change, tc.body, tc.remove, tc.noTmp)
			var stdout, stderr bytes.Buffer
			code := mutationTool(env)(tc.args, &stdout, &stderr)
			if code != 1 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("code=%d stderr=%q, want %q", code, stderr.String(), tc.want)
			}
		})
	}
}

func TestMutationTreatsAMissingReportAsNoMutants(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "", "silent")
	commitFile(t, env, "silent/x_test.go", "package silent\n")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(
		env,
	)(
		nil,
		&stdout,
		&stderr,
	); code != 0 ||
		stdout.String() != "mutating 1 packages: silent\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func brokenModule(t *testing.T, allow, pkg, change, body, remove string, noTmp bool) mutationEnv {
	t.Helper()
	var env mutationEnv
	if pkg != "" {
		env = mutationModule(t, allow, pkg)
		commitFile(t, env, pkg+"/x_test.go", "package "+pkg+"\n")
	} else {
		env = mutationModule(t, allow)
	}
	if change != "" {
		commitFile(t, env, change, body)
	}
	if remove != "" {
		removeFile(t, env, remove)
	}
	if noTmp {
		env.tmpDir = filepath.Join(env.tmpDir, "missing")
	}
	return env
}

func removeFile(t *testing.T, env mutationEnv, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(env.moduleDir, name)); err != nil {
		t.Fatal(err)
	}
}

func TestMutationUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"extra"}, {"--bogus"}} {
		var stdout, stderr bytes.Buffer
		if code := mutationTool(
			mutationEnv{},
		)(
			args,
			&stdout,
			&stderr,
		); code != 2 ||
			stderr.String() != mutationUsage+"\n" {
			t.Fatalf("%v: code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestReadAllowRejectsATwoFieldLineAndAnOverlongLine(t *testing.T) {
	t.Parallel()
	if _, err := readAllow(
		strings.NewReader("a.go:1:1\tARITHMETIC_BASE\n"),
	); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want decode_failed", err)
	}
	if _, err := readAllow(strings.NewReader(strings.Repeat("x", 70_000))); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal from the scanner", err)
	}
}
