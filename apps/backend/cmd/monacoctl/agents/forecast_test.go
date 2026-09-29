package agents

import (
	"fmt"
	"strings"
	"testing"
)

func TestForecast_listsFilesTouchedByTwoStacksNotTwoPRsOfOneStack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
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

func TestStacks_stopsOnABaseCycle(t *testing.T) {
	t.Parallel()
	got := stacks([]PR{pr(1, "a", "b", ""), pr(2, "b", "a", "")}, "fb")
	if len(got) != 0 {
		t.Fatalf("stacks = %v", got)
	}
}

func retargetedStack(t *testing.T, landsLine string, fileOf20 ...File) *fixture {
	t.Helper()
	f := newFixture(t)
	f.hub.on(list("/pulls?state=open"), []PR{
		pr(1, "s1", "fb", ""),
		pr(2, "s2", "fb", ""),
		pr(3, "s3", "fb", landsLine+"\n\n## TLDR"),
		pr(20, "other", "fb", ""),
	})
	f.hub.on(list("/pulls/1/files?"), []File{{Filename: "a.go"}})
	f.hub.on(list("/pulls/2/files?"), []File{{Filename: "a.go"}, {Filename: "b.go"}})
	f.hub.on(list("/pulls/3/files?"), []File{{Filename: "a.go"}, {Filename: "b.go"}})
	f.hub.on(list("/pulls/20/files?"), append([]File{{Filename: "c.go"}}, fileOf20...))
	return f
}

func TestForecast_countsARetargetedStackOnce(t *testing.T) {
	t.Parallel()
	f := retargetedStack(t, "Lands stack: #1 #2 #3")
	code, stdout, stderr := f.agents(t, "forecast")
	if code != 0 || stdout != "no file is touched by more than one open stack into fb\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestForecast_reportsARealOverlapWithARetargetedStack(t *testing.T) {
	t.Parallel()
	f := retargetedStack(t, "Lands stack: #1 #2 #3", File{Filename: "a.go"})
	code, stdout, stderr := f.agents(t, "forecast")
	want := "1 files touched by more than one open stack into fb:\n  a.go  #1 #20\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestForecast_ignoresAMalformedLandsStackLine(t *testing.T) {
	t.Parallel()
	f := retargetedStack(t, "Lands stack: #x")
	code, stdout, stderr := f.agents(t, "forecast")
	want := "2 files touched by more than one open stack into fb:\n  a.go  #1 #2 #3\n  b.go  #2 #3\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
