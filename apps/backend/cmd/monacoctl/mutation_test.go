package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
		"tonly/x_test.go":  "package tonly_test\n",
	}
	var list strings.Builder
	list.WriteString("MODDIR\tMODDIR/a\t1\n" +
		"MODDIR\tMODDIR/b\t1\n" +
		"MODDIR\tMODDIR/c\t1\n" +
		"MODDIR\tMODDIR/kit\t1\n" +
		"MODDIR\tMODDIR/tonly\t0\n")
	for _, p := range extraPkgs {
		files[p+"/x.go"] = "package " + p + "\n\nimport \"example.com/m/a\"\n\nfunc X() int { return a.A() }\n"
		list.WriteString("MODDIR\tMODDIR/" + p + "\t1\n")
	}
	files["golist.txt"] = list.String()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return mutationEnv{
		moduleDir: dir,
		goBin:     "go",
		gitBin:    "git",
		gremlins:  "gremlins",
		exec:      fakeExec,
		tmpDir:    t.TempDir(),
	}
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
	if _, err := os.Stat(filepath.Join(env.moduleDir, ".git")); err == nil {
		git(t, env.moduleDir, "add", ".")
		git(t, env.moduleDir, "commit", "-q", "-m", "change")
		return
	}
	changed, err := os.OpenFile(filepath.Join(env.moduleDir, "changed.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = changed.Close() }()
	if _, err := changed.WriteString(name + "\n"); err != nil {
		t.Fatal(err)
	}
}

func fakeExec(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	if name == "go" && args[0] == "test" {
		if args[len(args)-1] == "./flaky" {
			return []byte("ok  \texample.com/m/flaky\t0.1s\n"), nil
		}
		return []byte("=== RUN   TestX\ntestkit.Main: build template: atlas not found\n"),
			errs.New(errs.CodeInternal, "exit status 1")
	}
	switch name {
	case "go":
		list, err := os.ReadFile(filepath.Join(dir, "golist.txt"))
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, "fake go list")
		}
		return []byte(strings.ReplaceAll(string(list), "MODDIR", dir)), nil
	case "git":
		if strings.HasPrefix(args[3], "nope") {
			return nil, errs.New(errs.CodeInternal, "fake git: bad revision")
		}
		changed, _ := os.ReadFile(filepath.Join(dir, "changed.txt"))
		return changed, nil
	default:
		if err := logGremlins(dir, env, args); err != nil {
			return nil, err
		}
		return nil, fakeGremlins("./"+filepath.Base(dir), args[slices.Index(args, "--output")+1])
	}
}

func logGremlins(pkgDir string, env, args []string) error {
	log, err := os.OpenFile(
		filepath.Join(filepath.Dir(pkgDir), "gremlins.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "fake gremlins log")
	}
	defer func() { _ = log.Close() }()
	_, err = fmt.Fprintf(log, "%s: %s | %s\n", filepath.Base(pkgDir), strings.Join(args, " "), strings.Join(env, " "))
	return err
}

func gremlinsCalls(t *testing.T, env mutationEnv) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(env.moduleDir, "gremlins.log"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func fakeGremlins(pkg, out string) error {
	mutant := func(mutator, status string, line int) string {
		return fmt.Sprintf(`{"type":%q,"status":%q,"line":%d,"column":5}`, mutator, status, line)
	}
	statuses := map[string][]string{
		"./a":          {"LIVED"},
		"./slow":       {"TIMED OUT"},
		"./mostlyslow": {"KILLED", "TIMED OUT", "TIMED OUT"},
		"./halfslow":   {"KILLED", "TIMED OUT"},
	}
	switch pkg {
	case "./broken", "./flaky":
		return errs.New(errs.CodeInternal, "gremlins exploded")
	case "./garbled":
		return os.WriteFile(out, []byte("not json"), 0o600)
	case "./silent":
		return nil
	}
	list, ok := statuses[pkg]
	if !ok {
		list = []string{"KILLED"}
	}
	mutants := []string{mutant("ARITHMETIC_BASE", "NOT COVERED", 9)}
	for i, st := range list {
		mutants = append(
			mutants,
			mutant([]string{"CONDITIONALS_NEGATION", "ARITHMETIC_BASE", "INCREMENT_DECREMENT"}[i], st, 3+i),
		)
	}
	return os.WriteFile(
		out,
		[]byte(`{"files":[{"file_name":"x.go","mutations":[`+strings.Join(mutants, ",")+`]}]}`),
		0o600,
	)
}

func TestRunCommandReturnsStdoutAndCarriesStderrOnFailure(t *testing.T) {
	t.Parallel()
	out, err := runCommand(t.Context(), t.TempDir(), []string{"X=out"}, "sh", "-c", "echo $X; echo noise >&2")
	if err != nil || string(out) != "out\n" {
		t.Fatalf("runCommand = %q, %v; want stdout only", out, err)
	}
	if _, err := runCommand(t.Context(), t.TempDir(), nil, "sh", "-c", "echo boom >&2; exit 3"); err == nil ||
		err.Error() != "exit status 3: boom" {
		t.Fatalf("runCommand failure = %v, want the exit status and stderr", err)
	}
}

func TestMutationFailsOnASurvivorOnTheChangedLinesOfAChangedPackage(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	commitFile(t, env, "a/x_test.go", "package a\n")
	var stdout, stderr bytes.Buffer
	code := mutationTool(env)([]string{"--base", "origin/backend-rewrite"}, &stdout, &stderr)
	wantErr := "monacoctl mutation: a/x.go:3:5 CONDITIONALS_NEGATION survived; kill it with a test or list it in mutants.allow with a reason\n"
	if code != 1 || stdout.String() != "mutating 1 packages: a\n" || stderr.String() != wantErr {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	want := "a: unleash --silent --timeout-coefficient 50 --output OUT --exclude-files \\.gen\\.go$ --diff origin/backend-rewrite" +
		" | GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=diff.relative GIT_CONFIG_VALUE_0=true"
	if calls := gremlinsCalls(
		t,
		env,
	); len(calls) != 1 ||
		outputPath.ReplaceAllString(calls[0], "--output OUT") != want {
		t.Fatalf("gremlins calls = %q, want [%q]", calls, want)
	}
}

var outputPath = regexp.MustCompile(`--output \S+`)

func TestMutationAllMutatesEveryLineOfEveryPackage(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)([]string{"--all"}, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	calls := gremlinsCalls(t, env)
	if len(calls) != 3 || slices.ContainsFunc(calls, func(c string) bool { return strings.Contains(c, "--diff") }) {
		t.Fatalf("gremlins calls = %q, want a, b and c without --diff", calls)
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
		{name: "go list fails", remove: "golist.txt", want: "monacoctl.goList: internal"},
		{name: "unknown base", args: []string{"--base", "nope"}, want: "monacoctl.changedFiles: internal"},
		{name: "gremlins fails", pkg: "broken", want: "gremlins exploded: internal\ngo test ./broken for context:\n=== RUN   TestX\ntestkit.Main: build template: atlas not found\nexit status 1: internal"},
		{name: "gremlins fails but go test passes", pkg: "flaky", want: "gremlins exploded: internal\ngo test ./flaky for context:\nok  \texample.com/m/flaky\t0.1s\n"},
		{name: "every mutant timed out", pkg: "slow", want: "monacoctl.mutation: internal: more mutants in slow timed out than were tested; rerun on a quieter machine"},
		{name: "more mutants timed out than were killed", pkg: "mostlyslow", want: "monacoctl.mutation: internal: more mutants in mostlyslow timed out than were tested"},
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

func TestMutationRunsOnARealModuleWithTheGoAndGitCommands(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and git on a scratch module; CI runs it without -short, outside the 10 s package budget")
	}
	env := mutationModule(t, "")
	env.exec = func(ctx context.Context, dir string, environ []string, name string, args ...string) ([]byte, error) {
		if name != env.gremlins {
			return runCommand(ctx, dir, environ, name, args...)
		}
		ref := args[slices.Index(args, "--diff")+1]
		diff, err := runCommand(ctx, dir, environ, "git", "diff", "--name-only", "--merge-base", ref)
		if err != nil {
			return nil, err
		}
		if string(diff) != "x_test.go\n" {
			return nil, fmt.Errorf("gremlins would read git diff as %q; want paths relative to %s", diff, dir)
		}
		return fakeExec(ctx, dir, environ, name, args...)
	}
	git(t, env.moduleDir, "init", "-q", "-b", "main")
	git(t, env.moduleDir, "add", ".")
	git(t, env.moduleDir, "commit", "-q", "-m", "base")
	git(t, env.moduleDir, "checkout", "-q", "-b", "feature")
	commitFile(t, env, "a/x_test.go", "package a\n")
	commitFile(t, env, "tonly/x_test.go", "package tonly_test\n\nvar _ = 1\n")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)(nil, &stdout, &stderr); code != 1 || stdout.String() != "mutating 1 packages: a\n" ||
		!strings.HasPrefix(stderr.String(), "monacoctl mutation: a/x.go:3:5 CONDITIONALS_NEGATION survived") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var nope bytes.Buffer
	if code := mutationTool(env)([]string{"--base", "nope"}, &bytes.Buffer{}, &nope); code != 1 ||
		!strings.Contains(nope.String(), "monacoctl.changedFiles: internal") {
		t.Fatalf("unknown base with real git: code=%d stderr=%q", code, nope.String())
	}
}

func TestMutationAcceptsAsManyTimeoutsAsTestedMutants(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "", "halfslow")
	commitFile(t, env, "halfslow/x_test.go", "package halfslow\n")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func removeFile(t *testing.T, env mutationEnv, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(env.moduleDir, name)); err != nil {
		t.Fatal(err)
	}
}

func TestMutationUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"extra"}, {"--bogus"}, {"--list", "--pkg", "a"}, {"--all", "--report", "r.json"}} {
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

func TestMutationListPrintsTheChangedPackagesAsJSONWithoutMutating(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)([]string{"--list"}, &stdout, &stderr); code != 0 || stdout.String() != "[]\n" {
		t.Fatalf("no changes: code=%d stdout=%q stderr=%q, want []", code, stdout.String(), stderr.String())
	}
	commitFile(t, env, "c/x.go", "package c\n\nfunc C() int { return 4 }\n")
	commitFile(t, env, "a/x_test.go", "package a\n")
	stdout.Reset()
	if code := mutationTool(env)([]string{"--base", "main", "--list"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "[\"a\",\"c\"]\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(env.moduleDir, "gremlins.log")); !os.IsNotExist(err) {
		t.Fatalf("--list ran gremlins: %v", err)
	}
}

func TestMutationPkgMutatesOnlyThatChangedPackageOnItsChangedLines(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	commitFile(t, env, "c/x.go", "package c\n\nfunc C() int { return 4 }\n")
	commitFile(t, env, "a/x_test.go", "package a\n")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)([]string{"--base", "main", "--pkg", "c"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "mutating 1 packages: c\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	calls := gremlinsCalls(t, env)
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "c: ") || !strings.Contains(calls[0], "--diff main |") {
		t.Fatalf("gremlins calls = %q, want one call in c with --diff main", calls)
	}
}

func TestMutationPkgRejectsAPackageWithNoChangedGoFiles(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	commitFile(t, env, "c/x.go", "package c\n\nfunc C() int { return 4 }\n")
	var stdout, stderr bytes.Buffer
	want := "monacoctl mutation: monacoctl.mutation: invalid_input: b has no changed Go files to mutate; pick one from --list\n"
	if code := mutationTool(env)([]string{"--pkg", "b"}, &stdout, &stderr); code != 1 || stderr.String() != want {
		t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, stdout.String(), stderr.String(), want)
	}
	if _, err := os.Stat(filepath.Join(env.moduleDir, "gremlins.log")); !os.IsNotExist(err) {
		t.Fatalf("--pkg b ran gremlins: %v", err)
	}
}

func TestMutationListLeavesOutPackagesWithOnlyTestFiles(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	commitFile(t, env, "tonly/x_test.go", "package tonly_test\n\nvar _ = 1\n")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)([]string{"--list"}, &stdout, &stderr); code != 0 || stdout.String() != "[]\n" {
		t.Fatalf(
			"changed test-only package: code=%d stdout=%q stderr=%q, want []",
			code,
			stdout.String(),
			stderr.String(),
		)
	}
	stdout.Reset()
	if code := mutationTool(env)([]string{"--all", "--list"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "[\"a\",\"b\",\"c\"]\n" {
		t.Fatalf("--all --list: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestMutationAllPkgMutatesEveryLineOfOnePackage(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)([]string{"--all", "--pkg", "b"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "mutating 1 packages: b\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if calls := gremlinsCalls(
		t,
		env,
	); len(calls) != 1 || !strings.HasPrefix(calls[0], "b: ") ||
		strings.Contains(calls[0], "--diff") {
		t.Fatalf("gremlins calls = %q, want one call in b without --diff", calls)
	}
}

func TestMutationReportKeepsTheGremlinsReportAtThatPath(t *testing.T) {
	t.Parallel()
	env := mutationModule(t, "")
	report := filepath.Join(t.TempDir(), "gremlins-a.json")
	var stdout, stderr bytes.Buffer
	if code := mutationTool(env)([]string{"--all", "--pkg", "a", "--report", report}, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(report)
	if err != nil || !strings.Contains(string(data), `"status":"LIVED"`) {
		t.Fatalf("report = %q, %v; want the gremlins JSON with the survivor", data, err)
	}
}
