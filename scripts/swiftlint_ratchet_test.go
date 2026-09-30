package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const ratchetCSVHeader = "file,line,character,severity,type,reason,rule_id\n"

type ratchetRepo struct {
	dir  string
	bin  string
	csv  string
	args string
}

func newRatchetRepo(t *testing.T, baseline string) ratchetRepo {
	t.Helper()
	root := repoRoot(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"scripts/swiftlint-ratchet.sh", "scripts/require-docker.sh"} {
		copyFile(t, filepath.Join(root, rel), filepath.Join(dir, rel))
	}
	writeRatchetFile(t, filepath.Join(dir, "apps/mobile/Monaco/A.swift"), "let a = [1].first!\nlet b = [2].first!\n// note\n")
	writeRatchetFile(t, filepath.Join(dir, "packages/mobile-core/Sources/B.swift"), "let c = [3].first!\n")
	writeRatchetFile(t, filepath.Join(dir, ".swiftlint-baseline.tsv"), baseline)
	r := ratchetRepo{dir: dir, bin: t.TempDir(), csv: filepath.Join(t.TempDir(), "lint.csv")}
	r.args = filepath.Join(filepath.Dir(r.csv), "args")
	writeRatchetFile(t, filepath.Join(r.bin, "swiftlint"), `#!/bin/sh
if [ "$1" = version ]; then echo "${STUB_SWIFTLINT_VERSION:-0.65.0}"; exit 0; fi
echo "$@" > "$STUB_ARGS"
cat "$STUB_CSV"
exit 2
`)
	writeRatchetFile(t, filepath.Join(r.bin, "docker"), `#!/bin/sh
[ "$1" = info ] && exit 0
echo "$@" > "$STUB_ARGS"
sed "s#^$STUB_ROOT/#/repo/#" "$STUB_CSV"
exit 2
`)
	return r
}

func writeRatchetFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (r ratchetRepo) lintFinds(t *testing.T, rows ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(ratchetCSVHeader)
	for _, row := range rows {
		b.WriteString(r.dir + "/" + row + "\n")
	}
	writeRatchetFile(t, r.csv, b.String())
}

func (r ratchetRepo) run(t *testing.T, env []string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"scripts/swiftlint-ratchet.sh"}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+":/usr/bin:/bin",
		"STUB_CSV="+r.csv,
		"STUB_ARGS="+r.args,
		"STUB_ROOT="+r.dir,
	)
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatalf("run ratchet: %v", err)
	}
	return 0, string(out)
}

func (r ratchetRepo) baseline(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, ".swiftlint-baseline.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	unwrapA1  = `apps/mobile/Monaco/A.swift,1,18,Warning,Force Unwrapping,Force unwrapping should be avoided,force_unwrapping`
	unwrapA2  = `apps/mobile/Monaco/A.swift,2,18,Warning,Force Unwrapping,Force unwrapping should be avoided,force_unwrapping`
	commentA3 = `apps/mobile/Monaco/A.swift,3,1,Error,No comments,"No comments in Swift. Use a better name, a type, a test, or an issue.",no_comments`
	unwrapB1  = `packages/mobile-core/Sources/B.swift,1,18,Warning,Force Unwrapping,Force unwrapping should be avoided,force_unwrapping`
)

const ratchetBaseline = "force_unwrapping\tapps/mobile/Monaco/A.swift\t2\n" +
	"force_unwrapping\tpackages/mobile-core/Sources/B.swift\t1\n" +
	"no_comments\tapps/mobile/Monaco/A.swift\t1\n"

func TestSwiftlintRatchet_passesAtTheBaselineAndLintsBothTreesByDefault(t *testing.T) {
	r := newRatchetRepo(t, ratchetBaseline)
	r.lintFinds(t, unwrapA1, unwrapA2, commentA3, unwrapB1, unwrapB1)
	code, out := r.run(t, nil)
	if code != 0 {
		t.Fatalf("expected a pass at the baseline, code=%d out=%s", code, out)
	}
	args, _ := os.ReadFile(r.args)
	if got := strings.TrimSpace(string(args)); got != "lint --quiet --force-exclude --reporter csv apps/mobile packages/mobile-core" {
		t.Fatalf("unexpected swiftlint args %q", got)
	}
}

func TestSwiftlintRatchet_failsWhenACountGrowsAndPrintsTheLines(t *testing.T) {
	r := newRatchetRepo(t, "force_unwrapping\tapps/mobile/Monaco/A.swift\t1\n")
	r.lintFinds(t, unwrapA1, unwrapA2, commentA3)
	code, out := r.run(t, nil, "apps/mobile/Monaco/A.swift")
	if code != 1 {
		t.Fatalf("expected exit 1, code=%d out=%s", code, out)
	}
	for _, want := range []string{
		"swiftlint grew: force_unwrapping apps/mobile/Monaco/A.swift 1 -> 2",
		"apps/mobile/Monaco/A.swift:2: force_unwrapping: Force unwrapping should be avoided: let b = [2].first!",
		"swiftlint grew: no_comments apps/mobile/Monaco/A.swift 0 -> 1",
		"apps/mobile/Monaco/A.swift:3: no_comments: No comments in Swift. Use a better name, a type, a test, or an issue.: // note",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSwiftlintRatchet_fullRunDemandsALowerBaselineButANamedPathDoesNot(t *testing.T) {
	r := newRatchetRepo(t, ratchetBaseline)
	r.lintFinds(t, unwrapA1, commentA3)
	code, out := r.run(t, nil)
	if code != 1 {
		t.Fatalf("expected the full run to fail, code=%d out=%s", code, out)
	}
	for _, want := range []string{
		"lower the baseline: force_unwrapping apps/mobile/Monaco/A.swift 2 -> 1",
		"lower the baseline: force_unwrapping packages/mobile-core/Sources/B.swift 1 -> 0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if code, out := r.run(t, nil, filepath.Join(r.dir, "apps/mobile/Monaco/A.swift")); code != 0 {
		t.Fatalf("a named-path run checks growth only, code=%d out=%s", code, out)
	}
}

func TestSwiftlintRatchet_updateLowersRowsAndRefusesToRaiseThem(t *testing.T) {
	r := newRatchetRepo(t, ratchetBaseline)
	r.lintFinds(t, unwrapA1, commentA3)
	if code, out := r.run(t, nil, "--update"); code != 0 {
		t.Fatalf("expected --update to lower the rows, code=%d out=%s", code, out)
	}
	want := "force_unwrapping\tapps/mobile/Monaco/A.swift\t1\nno_comments\tapps/mobile/Monaco/A.swift\t1\n"
	if got := r.baseline(t); got != want {
		t.Fatalf("baseline after --update:\n%q\nwant:\n%q", got, want)
	}
	r.lintFinds(t, unwrapA1, unwrapA2, commentA3)
	code, out := r.run(t, nil, "--update")
	if code != 1 || !strings.Contains(out, "swiftlint grew: force_unwrapping apps/mobile/Monaco/A.swift 1 -> 2") {
		t.Fatalf("expected --update to refuse a rise, code=%d out=%s", code, out)
	}
	if got := r.baseline(t); got != want {
		t.Fatalf("--update wrote on a rise:\n%q", got)
	}
}

func TestSwiftlintRatchet_fallsBackToThePinnedImageAndStripsItsMount(t *testing.T) {
	r := newRatchetRepo(t, ratchetBaseline)
	r.lintFinds(t, unwrapA1, unwrapA2, commentA3, unwrapB1, unwrapB1)
	code, out := r.run(t, []string{"STUB_SWIFTLINT_VERSION=0.1.0"})
	if code != 0 {
		t.Fatalf("expected a pass through docker, code=%d out=%s", code, out)
	}
	args, _ := os.ReadFile(r.args)
	if !strings.Contains(string(args), "--entrypoint swiftlint ghcr.io/realm/swiftlint:0.65.0 lint") {
		t.Fatalf("expected the pinned image, args=%s", args)
	}
}
