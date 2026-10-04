package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	liveSim        = "11111111-1111-1111-1111-111111111111"
	goneSim        = "22222222-2222-2222-2222-222222222222"
	liveJourneySim = "33333333-3333-3333-3333-333333333333"
)

// stopFixture is a primary checkout with two lanes, "live" and "gone" (its worktree
// deleted), each with a registered simulator, next to the fixture's two stock simulators.
func stopFixture(t *testing.T) (primary, state string) {
	t.Helper()
	primary = simCheckout(t, "")
	for _, lane := range []string{"live", "gone"} {
		cmd := exec.Command("git", "worktree", "add", "-q", filepath.Join(primary, ".worktrees", lane))
		cmd.Dir = primary
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("worktree add: %v\n%s", err, out)
		}
	}
	if err := os.RemoveAll(filepath.Join(primary, ".worktrees", "gone")); err != nil {
		t.Fatal(err)
	}
	state = t.TempDir()
	var created strings.Builder
	for _, d := range []struct{ udid, name string }{
		{liveSim, "Monaco live"}, {goneSim, "Monaco gone"}, {liveJourneySim, "Monaco Journeys live A"},
	} {
		b, _ := json.Marshal(map[string]string{"udid": d.udid, "name": d.name, "deviceType": "phone", "runtime": "com.apple.CoreSimulator.SimRuntime.iOS-18-5"})
		created.Write(append(b, '\n'))
	}
	writeTestFile(t, filepath.Join(state, "created.jsonl"), created.String())
	writeTestFile(t, filepath.Join(primary, ".git", "monaco-lane-sims.tsv"),
		liveSim+"\tlive\tMonaco live\n"+goneSim+"\tgone\tMonaco gone\n")
	return primary, state
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func simCalls(t *testing.T, state string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(state, "calls.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	slices.Sort(lines)
	return lines
}

func TestStopMobile_primarySparesLiveLanesAndDeletesOrphans(t *testing.T) {
	primary, state := stopFixture(t)
	r := runSim(t, primary, "stop-mobile.sh", "simctl", state)
	if r.err != nil {
		t.Fatalf("stop-mobile.sh: %v\n%s", r.err, r.out)
	}
	want := []string{
		"delete " + goneSim,
		"terminate AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA com.monaco.app",
		"terminate BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB com.monaco.app",
		"uninstall AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA com.monaco.app",
		"uninstall BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB com.monaco.app",
	}
	if got := simCalls(t, state); !slices.Equal(got, want) {
		t.Fatalf("simctl calls\n%s\nwant\n%s\noutput:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"), r.out)
	}
	if !strings.Contains(r.out, "deleted simulator Monaco gone ("+goneSim+"), its worktree gone is gone") {
		t.Fatalf("no deletion line:\n%s", r.out)
	}
	registry, err := os.ReadFile(filepath.Join(primary, ".git", "monaco-lane-sims.tsv"))
	if err != nil || string(registry) != liveSim+"\tlive\tMonaco live\n" {
		t.Fatalf("registry after cleanup %q (%v), want only the live lane", registry, err)
	}
}

func TestStopMobile_laneTouchesOnlyItsOwnSimulators(t *testing.T) {
	primary, state := stopFixture(t)
	r := runSim(t, filepath.Join(primary, ".worktrees", "live"), "stop-mobile.sh", "simctl", state)
	if r.err != nil {
		t.Fatalf("stop-mobile.sh: %v\n%s", r.err, r.out)
	}
	want := []string{"terminate " + liveSim + " com.monaco.app", "uninstall " + liveSim + " com.monaco.app"}
	if got := simCalls(t, state); !slices.Equal(got, want) {
		t.Fatalf("simctl calls %q, want %q\n%s", got, want, r.out)
	}
}

// Only xcodebuild processes building into this checkout's derived data are stopped.
func TestStopMobile_stopsOnlyThisCheckoutsXcodebuild(t *testing.T) {
	primary, state := stopFixture(t)
	// git prints the checkout with symlinks resolved (/private/var, not /var).
	primary, err := filepath.EvalSymlinks(primary)
	if err != nil {
		t.Fatal(err)
	}
	lane := filepath.Join(primary, ".worktrees", "live")
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "xcodebuild"), "#!/bin/sh\nsleep 30\n")
	start := func(checkout string) *exec.Cmd {
		cmd := exec.Command(filepath.Join(bin, "xcodebuild"), "-derivedDataPath", filepath.Join(checkout, ".build", "DerivedData"))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	mine, theirs := start(lane), start(primary)
	exited := make(chan struct{})
	go func() { _ = mine.Wait(); close(exited) }()

	if r := runSim(t, lane, "stop-mobile.sh", "simctl", state); r.err != nil {
		t.Fatalf("stop-mobile.sh: %v\n%s", r.err, r.out)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("this lane's xcodebuild is still running")
	}
	if err := theirs.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the primary checkout's xcodebuild was stopped: %v", err)
	}
}
