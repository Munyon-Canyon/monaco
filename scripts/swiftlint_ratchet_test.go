package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const ratchetCSVHeader = "file,line,character,severity,type,reason,rule_id\n"

const ratchetBaseTree = ".build/swiftlint-base."

const ratchetBaseDir = `base="$(printf '%s\n' "$@" | sed -n 's#^\(\.build/swiftlint-base\.[^/]*\)/.*#\1#p' | head -n 1)"`

type ratchetRepo struct {
	dir     string
	bin     string
	headCSV string
	baseCSV string
	args    string
}

func newRatchetRepo(t *testing.T) ratchetRepo {
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
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "base")
	git(t, dir, "update-ref", "refs/remotes/origin/staging", "HEAD")
	stubs := t.TempDir()
	r := ratchetRepo{
		dir:     dir,
		bin:     t.TempDir(),
		headCSV: filepath.Join(stubs, "head.csv"),
		baseCSV: filepath.Join(stubs, "base.csv"),
		args:    filepath.Join(stubs, "args"),
	}
	writeRatchetFile(t, r.baseCSV, ratchetCSVHeader)
	writeRatchetFile(t, filepath.Join(r.bin, "swiftlint"), `#!/bin/sh
if [ "$1" = version ]; then echo "${STUB_SWIFTLINT_VERSION:-0.65.0}"; exit 0; fi
echo "$@" >> "$STUB_ARGS"
`+ratchetBaseDir+`
case "$*" in
  *.build/swiftlint-base*) sed "s#@BASE@#$base#" "$STUB_BASE_CSV" ;;
  *) cat "$STUB_CSV" ;;
esac
exit 2
`)
	writeRatchetFile(t, filepath.Join(r.bin, "docker"), `#!/bin/sh
[ "$1" = info ] && exit 0
echo "$@" >> "$STUB_ARGS"
`+ratchetBaseDir+`
case "$*" in
  *.build/swiftlint-base*) csv="$STUB_BASE_CSV" ;;
  *) csv="$STUB_CSV" ;;
esac
sed -e "s#@BASE@#$base#" -e "s#^$STUB_ROOT/#/repo/#" "$csv"
exit 2
`)
	return r
}

func writeRatchetFile(t *testing.T, path, body string) {
	t.Helper()
	writeExecutable(t, path, body)
}

func (r ratchetRepo) edit(t *testing.T, rel, body string) {
	t.Helper()
	writeRatchetFile(t, filepath.Join(r.dir, rel), body)
}

func (r ratchetRepo) writeCSV(t *testing.T, path, prefix string, rows []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(ratchetCSVHeader)
	for _, row := range rows {
		b.WriteString(r.dir + "/" + prefix + row + "\n")
	}
	writeRatchetFile(t, path, b.String())
}

func (r ratchetRepo) headFinds(t *testing.T, rows ...string) {
	t.Helper()
	r.writeCSV(t, r.headCSV, "", rows)
}

func (r ratchetRepo) baseFinds(t *testing.T, rows ...string) {
	t.Helper()
	r.writeCSV(t, r.baseCSV, "@BASE@/", rows)
}

func (r ratchetRepo) run(t *testing.T, env []string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"scripts/swiftlint-ratchet.sh"}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+":/usr/bin:/bin",
		"STUB_CSV="+r.headCSV,
		"STUB_BASE_CSV="+r.baseCSV,
		"STUB_ARGS="+r.args,
		"STUB_ROOT="+r.dir,
		"PR_LABELS=",
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

func (r ratchetRepo) lintCalls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(r.args)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

const (
	aSwift    = "apps/mobile/Monaco/A.swift"
	unwrapA1  = aSwift + `,1,18,Warning,Force Unwrapping,Force unwrapping should be avoided,force_unwrapping`
	unwrapA2  = aSwift + `,2,18,Warning,Force Unwrapping,Force unwrapping should be avoided,force_unwrapping`
	commentA3 = aSwift + `,3,1,Error,No comments,"No comments in Swift. Use a better name, a type, a test, or an issue.",no_comments`
)

func TestSwiftlintRatchet_passesWhenNoChangedFileGrowsAndLintsOnlyChangedFiles(t *testing.T) {
	r := newRatchetRepo(t)
	r.edit(t, aSwift, "let a = [1].first!\nlet b = [2].first!\n// note\nlet d = 4\n")
	r.baseFinds(t, unwrapA1, unwrapA2, commentA3)
	r.headFinds(t, unwrapA1, unwrapA2, commentA3)
	code, out := r.run(t, nil)
	if code != 0 {
		t.Fatalf("expected a pass, code=%d out=%s", code, out)
	}
	calls := r.lintCalls(t)
	head, base := "lint --quiet --force-exclude --reporter csv "+aSwift, "lint --quiet --force-exclude --reporter csv "+ratchetBaseTree
	if len(calls) != 2 || calls[0] != head || !strings.HasPrefix(calls[1], base) || !strings.HasSuffix(calls[1], "/"+aSwift) {
		t.Fatalf("swiftlint calls %q, want the changed file at head and its base copy", calls)
	}
}

func TestSwiftlintRatchet_failsWhenACountGrowsAndPrintsTheLines(t *testing.T) {
	r := newRatchetRepo(t)
	r.edit(t, aSwift, "let a = [1].first!\nlet b = [2].first!\n// note\nlet d = 4\n")
	r.baseFinds(t, unwrapA1)
	r.headFinds(t, unwrapA1, unwrapA2, commentA3)
	code, out := r.run(t, nil, aSwift)
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

func TestSwiftlintRatchet_aShrinkPassesWithNoStoredFile(t *testing.T) {
	r := newRatchetRepo(t)
	r.edit(t, aSwift, "let a = [1].first!\n")
	r.baseFinds(t, unwrapA1, unwrapA2, commentA3)
	r.headFinds(t, unwrapA1)
	if code, out := r.run(t, nil); code != 0 {
		t.Fatalf("a shrink must pass, code=%d out=%s", code, out)
	}
	status, err := exec.Command("git", "-C", r.dir, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(status)); got != "M "+aSwift {
		t.Fatalf("the ratchet must leave only the edit behind, status=%q", got)
	}
}

func TestSwiftlintRatchet_gateChangeApprovedWaivesGrowth(t *testing.T) {
	r := newRatchetRepo(t)
	r.edit(t, aSwift, "let a = [1].first!\nlet b = [2].first!\n// note\nlet d = 4\n")
	r.baseFinds(t, unwrapA1)
	r.headFinds(t, unwrapA1, unwrapA2)
	if code, out := r.run(t, []string{`PR_LABELS=["fast-track"]`}); code != 1 {
		t.Fatalf("another label must not waive growth, code=%d out=%s", code, out)
	}
	code, out := r.run(t, []string{`PR_LABELS=["fast-track","gate-change-approved"]`})
	if code != 0 || !strings.Contains(out, "swiftlint growth approved by gate-change-approved") ||
		!strings.Contains(out, "swiftlint grew: force_unwrapping apps/mobile/Monaco/A.swift 1 -> 2") {
		t.Fatalf("expected the label to waive the growth and still report it, code=%d out=%s", code, out)
	}
}

func TestSwiftlintRatchet_fallsBackToThePinnedImageAndStripsItsMount(t *testing.T) {
	r := newRatchetRepo(t)
	r.edit(t, aSwift, "let a = [1].first!\nlet b = [2].first!\n// note\nlet d = 4\n")
	r.baseFinds(t, unwrapA1, unwrapA2, commentA3)
	r.headFinds(t, unwrapA1, unwrapA2, commentA3)
	code, out := r.run(t, []string{"STUB_SWIFTLINT_VERSION=0.1.0"})
	if code != 0 {
		t.Fatalf("expected a pass through docker with /repo and the base tree stripped, code=%d out=%s", code, out)
	}
	for _, call := range r.lintCalls(t) {
		if !strings.Contains(call, "-v "+r.dir+":/repo -w /repo --entrypoint swiftlint ghcr.io/realm/swiftlint:0.65.0 lint") {
			t.Fatalf("expected the pinned image, call=%s", call)
		}
	}
}

func TestSwiftlintRatchet_aRenamedFileComparesAgainstItsOldPath(t *testing.T) {
	r := newRatchetRepo(t)
	const renamed = "apps/mobile/Monaco/Renamed.swift"
	git(t, r.dir, "mv", aSwift, renamed)
	r.baseFinds(t, strings.Replace(unwrapA1, aSwift, renamed, 1))
	r.headFinds(t, strings.Replace(unwrapA1, aSwift, renamed, 1))
	code, out := r.run(t, nil)
	if code != 0 {
		t.Fatalf("a rename with an unchanged violation must pass, code=%d out=%s", code, out)
	}
	calls := r.lintCalls(t)
	if len(calls) != 2 || !strings.Contains(calls[1], ratchetBaseTree) || !strings.HasSuffix(calls[1], "/"+renamed) {
		t.Fatalf("expected the old blob linted at the new path, calls=%q", calls)
	}
}

func TestSwiftlintRatchet_aNewFileCountsFromZero(t *testing.T) {
	r := newRatchetRepo(t)
	const added = "apps/mobile/Monaco/C.swift"
	r.edit(t, added, "let e = [5].first!\n")
	r.headFinds(t, added+`,1,18,Warning,Force Unwrapping,Force unwrapping should be avoided,force_unwrapping`)
	code, out := r.run(t, nil)
	if code != 1 || !strings.Contains(out, "swiftlint grew: force_unwrapping "+added+" 0 -> 1") {
		t.Fatalf("expected a new file to fail 0 -> 1, code=%d out=%s", code, out)
	}
	if calls := r.lintCalls(t); len(calls) != 1 {
		t.Fatalf("a new file has no base copy to lint, calls=%q", calls)
	}
}
