package agents

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func trunkRegistry(t *testing.T, f *fixture) {
	t.Helper()
	writeFile(t, filepath.Join(f.dir, flows.Dir, "00.tsv"), flows.Header+"\n"+
		"00\tPing\tsystem\tGET /p\tRecordPing\t\t\tok\tbuilt\tdocs/f.md\n")
	writeFile(t, filepath.Join(f.dir, flows.Dir, "01.tsv"), flows.Header+"\n"+
		"01\tSign in\tidentity\tGET /s\tSignIn\t\t\tok\tbuilt\tdocs/f.md\n")
	writeFile(t, filepath.Join(f.dir, flows.AppDir, "00.tsv"), flows.AppHeader+"\n00\tSystemPing\tbuilt\tdocs/f.md\n")
	git(t, f.dir, "add", "-A")
	git(t, f.dir, "commit", "-qm", "registry")
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "HEAD")
}

func TestForecast_listsFilesTouchedByTwoStacksNotTwoPRsOfOneStack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	trunkRegistry(t, f)
	f.hub.on(list("/pulls?state=open"), []PR{
		pr(10, "a1", "fb", ""),
		pr(11, "a2", "a1", ""),
		pr(20, "b1", "fb", "Part of #2"),
		pr(30, "c1", "main", ""),
		pr(31, "c2", "c1", ""),
	})
	f.hub.on(list("/pulls/10/files?"), []File{{Filename: "shared.go"}, {Filename: "stack-a.go"}})
	f.hub.on(list("/pulls/11/files?"), []File{{Filename: "stack-a.go"}, {Filename: "late.go"}})
	f.hub.on(list("/pulls/20/files?"), []File{{Filename: "shared.go"}, {Filename: "late.go"}})
	code, stdout, stderr := f.agents(t, "forecast")
	want := "2 files touched by more than one open stack into fb:\n  late.go  #10 #20\n  shared.go  #10 #20\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestForecast_capsTheListAtTwentyLines(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	trunkRegistry(t, f)
	f.hub.on(list("/pulls?state=open"), []PR{pr(1, "a", "fb", ""), pr(2, "b", "fb", "")})
	files := make([]File, 0, 25)
	for i := range 25 {
		files = append(files, File{Filename: fmt.Sprintf("f%02d.go", i)})
	}
	f.hub.on(list("/pulls/1/files?"), files)
	f.hub.on(list("/pulls/2/files?"), files)
	code, stdout, _ := f.agents(t, "forecast")
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if code != 0 || len(lines) != maxLines || lines[maxLines-1] != "  and 7 more" || lines[1] != "  f00.go  #1 #2" {
		t.Fatalf("code=%d lines=%d stdout=%q", code, len(lines), stdout)
	}
}

func TestForecast_failsWhenGitHubFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if code, _, stderr := f.agents(t, "forecast"); code != 1 || !strings.Contains(stderr, "pulls?state=open") {
		t.Fatalf("list: code=%d stderr=%q", code, stderr)
	}
	f.hub.on(list("/pulls?state=open"), []PR{pr(1, "a", "fb", "")})
	if code, _, stderr := f.agents(t, "forecast"); code != 1 || !strings.Contains(stderr, "pulls/1/files") {
		t.Fatalf("files: code=%d stderr=%q", code, stderr)
	}
}

func TestForecast_listsAFlowTwoStacksTouchThroughDifferentFiles(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	trunkRegistry(t, f)
	f.hub.on(list("/pulls?state=open"), []PR{pr(1701, "a", "fb", ""), pr(1705, "b", "fb", ""), pr(1709, "c", "fb", "")})
	f.hub.on(list("/pulls/1701/files?"), []File{{Filename: "packages/flows/app/00.tsv"}})
	f.hub.on(list("/pulls/1705/files?"), []File{{Filename: "apps/backend/internal/modules/system/http.go"}})
	f.hub.on(list("/pulls/1709/files?"), []File{{Filename: flows.Dir + "/01.tsv"}})
	code, stdout, stderr := f.agents(t, "forecast")
	want := "no file is touched by more than one open stack into fb\nflows: 00 #1701 #1705\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	f.hub.on(list("/pulls/1705/files?"), []File{{Filename: "apps/backend/internal/modules/cabal/http.go"}})
	if code, stdout, _ := f.agents(t, "forecast"); code != 0 || strings.Contains(stdout, "flows:") {
		t.Fatalf("disjoint stacks: code=%d stdout=%q", code, stdout)
	}
}

func specBranch(t *testing.T, f *fixture, branch, spec string) PR {
	t.Helper()
	git(t, f.dir, "checkout", "-q", "-b", branch, "origin/fb")
	writeFile(t, filepath.Join(f.dir, flows.SpecPath), spec)
	git(t, f.dir, "commit", "-qam", branch)
	sha, err := exec.CommandContext(t.Context(), "git", "-C", f.dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	git(t, f.dir, "checkout", "-q", "-")
	head := pr(len(branch), branch, "fb", "")
	head.Head.SHA = strings.TrimSpace(string(sha))
	return head
}

func TestForecast_aSpecChangeReachesOnlyTheFlowsWhoseRoutesChanged(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	const base = "paths:\n  /p:\n    get: {}\n  /s:\n    get: {}\n"
	writeFile(t, filepath.Join(f.dir, flows.SpecPath), base)
	trunkRegistry(t, f)
	git(t, f.dir, "remote", "add", "origin", f.dir)
	ping := specBranch(t, f, "a", strings.Replace(base, "/p:\n    get: {}", "/p:\n    get: {summary: Ping.}", 1))
	signIn := specBranch(t, f, "bb", strings.Replace(base, "/s:\n    get: {}", "/s:\n    get: {summary: In.}", 1))
	again := specBranch(t, f, "ccc", strings.Replace(base, "/p:\n    get: {}", "/p:\n    get: {summary: P.}", 1))
	f.hub.on(list("/pulls?state=open"), []PR{ping, signIn})
	for _, p := range []PR{ping, signIn, again} {
		f.hub.on(list(fmt.Sprintf("/pulls/%d/files?", p.Number)), []File{{Filename: flows.SpecPath}})
	}
	if code, stdout, stderr := f.agents(t, "forecast"); code != 0 || strings.Contains(stdout, "flows:") {
		t.Fatalf("stacks on different routes: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	f.hub.on(list("/pulls?state=open"), []PR{ping, signIn, again})
	want := "flows: 00 #1 #3\n"
	if code, stdout, stderr := f.agents(t, "forecast"); code != 0 || !strings.HasSuffix(stdout, want) {
		t.Fatalf("stacks on one route: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestForecast_saysSoWhenAStackHeadCannotBeFetched(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	writeFile(t, filepath.Join(f.dir, flows.SpecPath), "paths: {}\n")
	trunkRegistry(t, f)
	git(t, f.dir, "remote", "add", "origin", f.dir)
	f.hub.on(list("/pulls?state=open"), []PR{pr(1, "gone", "fb", ""), pr(2, "b", "fb", "")})
	f.hub.on(list("/pulls/1/files?"), []File{{Filename: flows.SpecPath}})
	f.hub.on(list("/pulls/2/files?"), []File{})
	if code, stdout, _ := f.agents(t, "forecast"); code != 0 || !strings.Contains(stdout, "flows: not checked: ") ||
		!strings.Contains(stdout, "gone") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
}

func TestForecast_saysSoWithoutTheTrunkRegistry(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(list("/pulls?state=open"), []PR{pr(1, "a", "fb", ""), pr(2, "b", "fb", "")})
	f.hub.on(list("/pulls/1/files?"), []File{})
	f.hub.on(list("/pulls/2/files?"), []File{})
	if code, stdout, _ := f.agents(t, "forecast"); code != 0 || !strings.Contains(stdout, "flows: not checked: ") ||
		!strings.Contains(stdout, "origin/fb") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
}

func TestStacks_stopsOnABaseCycle(t *testing.T) {
	t.Parallel()
	got := stacks([]PR{pr(1, "a", "b", ""), pr(2, "b", "a", "")}, "fb")
	if len(got) != 0 {
		t.Fatalf("stacks = %v", got)
	}
}
