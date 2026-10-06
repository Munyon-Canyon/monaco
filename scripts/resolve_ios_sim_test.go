package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// simScripts are the scripts a scratch checkout carries; they find the checkout from
// their own path, so each test runs real copies outside this repo's own worktree.
var simScripts = []string{"lane-sim-udid.sh", "simslim-ensure.sh", "resolve-ios-sim.sh", "gold-sim-udid.sh", "stop-mobile.sh"}

// simCheckout is a scratch git repo with the sim scripts committed. lane is "" for the
// primary checkout, or the name of a linked worktree added under it.
func simCheckout(t *testing.T, lane string) string {
	t.Helper()
	root := repoRoot(t)
	primary := filepath.Join(t.TempDir(), "monaco")
	if err := os.MkdirAll(filepath.Join(primary, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range simScripts {
		src, err := os.ReadFile(filepath.Join(root, "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		writeExecutable(t, filepath.Join(primary, "scripts", name), string(src))
	}
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(primary, "init", "-q")
	git(primary, "add", ".")
	git(primary, "commit", "-q", "--no-verify", "-m", "scripts")
	if lane == "" {
		return primary
	}
	wt := filepath.Join(primary, ".worktrees", lane)
	git(primary, "worktree", "add", "-q", wt)
	return wt
}

type simRun struct {
	out   string
	udid  string // last stdout line
	err   error
	state string
}

// runSim runs script in checkout against fixture, with a fake xcrun whose created
// devices live in state.
func runSim(t *testing.T, checkout, script, fixture, state string, env ...string) simRun {
	t.Helper()
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(checkout, "scripts", script))
	cmd.Dir = checkout
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "scripts", "testdata", "fakebin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_SIMCTL_DIR="+filepath.Join(root, "scripts", "testdata", fixture),
		"FAKE_SIMCTL_STATE="+state,
		"SIMSLIM_UDID=",
		"MONACO_SIM_UDID=",
	)
	cmd.Env = append(cmd.Env, env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	return simRun{out: stdout.String() + stderr.String(), udid: strings.TrimSpace(lines[len(lines)-1]), err: err, state: state}
}

func createLog(t *testing.T, state string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(state, "create.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestResolveIOSSim_picksBootedIPhoneWhenSlimUnset(t *testing.T) {
	r := runSim(t, simCheckout(t, ""), "resolve-ios-sim.sh", "simctl", t.TempDir())
	if r.err != nil {
		t.Fatalf("resolve-ios-sim.sh: %v\n%s", r.err, r.out)
	}
	if r.udid != "BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB" {
		t.Fatalf("expected booted iPhone UDID, got %q\nfull output:\n%s", r.udid, r.out)
	}
	if !strings.Contains(r.out, "SimSlim not installed") && !strings.Contains(r.out, "SIMSLIM_UDID unset") {
		t.Fatalf("expected a fallback warning, got:\n%s", r.out)
	}
}

// A fresh CI runner has several runtimes and nothing booted; the oldest can sit below the
// app's deployment target, so the pick must come from the newest runtime.
func TestResolveIOSSim_prefersNewestRuntimeWhenNothingBooted(t *testing.T) {
	r := runSim(t, simCheckout(t, ""), "resolve-ios-sim.sh", "simctl-shutdown", t.TempDir())
	if r.err != nil {
		t.Fatalf("resolve-ios-sim.sh: %v\n%s", r.err, r.out)
	}
	if r.udid != "DDDDDDDD-DDDD-DDDD-DDDD-DDDDDDDDDDDD" {
		t.Fatalf("expected the iOS 26.0 iPhone, got %q\n%s", r.udid, r.out)
	}
}

func TestResolveIOSSim_usesSlimUdidWhenDeviceExists(t *testing.T) {
	r := runSim(t, simCheckout(t, ""), "resolve-ios-sim.sh", "simctl", t.TempDir(),
		"SIMSLIM_UDID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA")
	if r.err != nil {
		t.Fatalf("resolve-ios-sim.sh: %v\n%s", r.err, r.out)
	}
	if r.udid != "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA" {
		t.Fatalf("expected SIMSLIM_UDID, got %q\n%s", r.udid, r.out)
	}
	if !strings.Contains(r.out, "not installed or not ready") {
		t.Fatalf("expected slim-not-ready warning, got:\n%s", r.out)
	}
}

func TestResolveIOSSim_noCreateWhenEmpty(t *testing.T) {
	state := t.TempDir()
	r := runSim(t, simCheckout(t, ""), "resolve-ios-sim.sh", "simctl-empty", state)
	if r.err == nil {
		t.Fatalf("expected failure with no simulators, got:\n%s", r.out)
	}
	if !strings.Contains(r.out, "no iOS Simulator available") {
		t.Fatalf("expected no-sim error, got:\n%s", r.out)
	}
	if got := createLog(t, state); got != nil {
		t.Fatalf("the primary checkout created a simulator: %v", got)
	}
}

func TestGoldSimUdid_failsWhenUnset(t *testing.T) {
	r := runSim(t, simCheckout(t, ""), "gold-sim-udid.sh", "simctl", t.TempDir())
	if r.err == nil {
		t.Fatalf("expected fail when SIMSLIM_UDID unset, got:\n%s", r.out)
	}
	if !strings.Contains(r.out, "SIMSLIM_UDID is unset") {
		t.Fatalf("unexpected output:\n%s", r.out)
	}
}

func TestGoldSimUdid_printsWhenDeviceExists(t *testing.T) {
	r := runSim(t, simCheckout(t, ""), "gold-sim-udid.sh", "simctl", t.TempDir(),
		"SIMSLIM_UDID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA")
	if r.err != nil {
		t.Fatalf("gold-sim-udid.sh: %v\n%s", r.err, r.out)
	}
	if strings.TrimSpace(r.out) != "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA" {
		t.Fatalf("got %q", r.out)
	}
}

// In a linked worktree, resolve-ios-sim.sh and gold-sim-udid.sh both create the lane's own
// simulator once, from the gold device's type and runtime, and return it from then on.
func TestResolveIOSSim_laneCreatesItsOwnSimulatorOnce(t *testing.T) {
	lane := simCheckout(t, "agent-7")
	state := t.TempDir()
	first := runSim(t, lane, "resolve-ios-sim.sh", "simctl", state, "SIMSLIM_UDID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA")
	if first.err != nil {
		t.Fatalf("resolve-ios-sim.sh: %v\n%s", first.err, first.out)
	}
	if first.udid == "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA" || first.udid == "" {
		t.Fatalf("the lane got the gold simulator %q:\n%s", first.udid, first.out)
	}
	want := []string{"create Monaco agent-7 com.apple.CoreSimulator.SimDeviceType.iPhone-16 com.apple.CoreSimulator.SimRuntime.iOS-18-5"}
	if got := createLog(t, state); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("creates %q, want %q", got, want)
	}

	again := runSim(t, lane, "resolve-ios-sim.sh", "simctl", state, "SIMSLIM_UDID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA")
	gold := runSim(t, lane, "gold-sim-udid.sh", "simctl", state)
	if again.udid != first.udid || gold.udid != first.udid || gold.err != nil {
		t.Fatalf("second resolve %q and gold-sim-udid %q (%v), want %q:\n%s", again.udid, gold.udid, gold.err, first.udid, gold.out)
	}
	if n := len(createLog(t, state)); n != 1 {
		t.Fatalf("%d creates, want 1", n)
	}
	registry, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(lane)), ".git", "monaco-lane-sims.tsv"))
	if err != nil || string(registry) != first.udid+"\tagent-7\tMonaco agent-7\n" {
		t.Fatalf("lane registry %q (%v), want one row for %s", registry, err, first.udid)
	}

	// The primary checkout of the same repo keeps the gold simulator.
	primary := filepath.Dir(filepath.Dir(lane))
	if r := runSim(t, primary, "resolve-ios-sim.sh", "simctl", state, "SIMSLIM_UDID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"); r.udid != "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA" {
		t.Fatalf("primary resolved %q, want the gold UDID:\n%s", r.udid, r.out)
	}
}

// Without SIMSLIM_UDID the gold template is the primary checkout's newest-runtime pick.
func TestResolveIOSSim_laneCopiesTheNewestRuntimeWithoutSlim(t *testing.T) {
	state := t.TempDir()
	r := runSim(t, simCheckout(t, "lane-b"), "resolve-ios-sim.sh", "simctl-shutdown", state)
	if r.err != nil {
		t.Fatalf("resolve-ios-sim.sh: %v\n%s", r.err, r.out)
	}
	want := "create Monaco lane-b com.apple.CoreSimulator.SimDeviceType.iPhone-17 com.apple.CoreSimulator.SimRuntime.iOS-26-0"
	if got := createLog(t, state); len(got) != 1 || got[0] != want {
		t.Fatalf("creates %q, want %q", got, want)
	}
}

func TestResolveIOSSim_laneCallsAtOnceCreateOneSimulator(t *testing.T) {
	lane := simCheckout(t, "racer")
	state := t.TempDir()
	var wg sync.WaitGroup
	runs := make([]simRun, 4)
	for i := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runs[i] = runSim(t, lane, "resolve-ios-sim.sh", "simctl", state, "FAKE_SIMCTL_CREATE_DELAY=0.3")
		}()
	}
	wg.Wait()
	for _, r := range runs {
		if r.err != nil || r.udid != runs[0].udid {
			t.Fatalf("concurrent resolves disagree: %q vs %q (%v)\n%s", r.udid, runs[0].udid, r.err, r.out)
		}
	}
	if n := len(createLog(t, state)); n != 1 {
		t.Fatalf("%d creates from 4 concurrent calls, want 1", n)
	}
}

func TestResolveIOSSim_monacoSimUdidOverridesLaneAndPrimary(t *testing.T) {
	for _, lane := range []string{"", "pinned"} {
		state := t.TempDir()
		for _, script := range []string{"resolve-ios-sim.sh", "gold-sim-udid.sh"} {
			r := runSim(t, simCheckout(t, lane), script, "simctl", state,
				"MONACO_SIM_UDID=FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF", "SIMSLIM_UDID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA")
			if r.err != nil || r.udid != "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF" {
				t.Fatalf("lane %q %s printed %q (%v), want MONACO_SIM_UDID:\n%s", lane, script, r.udid, r.err, r.out)
			}
		}
		if got := createLog(t, state); got != nil {
			t.Fatalf("lane %q created %v despite MONACO_SIM_UDID", lane, got)
		}
	}
}

// After creating the lane simulator, simslim slims it once with the checkout's profile.
func TestResolveIOSSim_laneSlimsANewSimulator(t *testing.T) {
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "simslim.calls")
	script := "#!/bin/sh\necho \"$*\" >> " + calls + "\n[ \"$1\" = on ]\n"
	writeExecutable(t, filepath.Join(bin, "simslim"), script)
	r := runSim(t, simCheckout(t, "slim"), "resolve-ios-sim.sh", "simctl", t.TempDir(),
		"HOME="+t.TempDir(), "PATH="+filepath.Join(repoRoot(t), "scripts", "testdata", "fakebin")+string(os.PathListSeparator)+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if r.err != nil {
		t.Fatalf("resolve-ios-sim.sh: %v\n%s", r.err, r.out)
	}
	got, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if want := "on " + r.udid + " --profile "; !strings.Contains(string(got), want) || !strings.Contains(string(got), "/ci/profiles/base-slim.json --preserve-boot-state\n") {
		t.Fatalf("simslim calls %q, want %q with the checkout's ci/profiles/base-slim.json", got, want)
	}
}

// simslimBin is a PATH holding a mocked simslim that logs its calls. verify exits with
// verifyRC; on always succeeds.
func simslimBin(t *testing.T, verifyRC int) (path, calls string) {
	t.Helper()
	bin := t.TempDir()
	calls = filepath.Join(t.TempDir(), "simslim.calls")
	script := "#!/bin/sh\necho \"$*\" >> " + calls + "\n[ \"$1\" = verify ] && exit " + strconv.Itoa(verifyRC) + "\nexit 0\n"
	writeExecutable(t, filepath.Join(bin, "simslim"), script)
	fake := filepath.Join(repoRoot(t), "scripts", "testdata", "fakebin")
	return fake + string(os.PathListSeparator) + bin + string(os.PathListSeparator) + os.Getenv("PATH"), calls
}

func ensure(t *testing.T, path, mode, udid string, env ...string) (calls string, out string) {
	t.Helper()
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "simslim-ensure.sh"), mode, udid)
	cmd.Env = append(os.Environ(), append([]string{"PATH=" + path, "HOME=/nonexistent"}, env...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("simslim-ensure.sh %s: %v\n%s", mode, err, b)
	}
	return "", string(b)
}

func readCalls(t *testing.T, calls string) string {
	t.Helper()
	b, _ := os.ReadFile(calls)
	return string(b)
}

func TestSimslimEnsure_createCallsOnWithTheProfile(t *testing.T) {
	path, calls := simslimBin(t, 0)
	ensure(t, path, "create", "UDID1", "SIMSLIM_PROFILE=/p.json")
	if got, want := readCalls(t, calls), "on UDID1 --profile /p.json --preserve-boot-state\n"; got != want {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

func TestSimslimEnsure_defaultProfileIsTheRepos(t *testing.T) {
	path, calls := simslimBin(t, 0)
	ensure(t, path, "create", "UDID1", "HOME=/h")
	profile := filepath.Join(repoRoot(t), "ci", "profiles", "base-slim.json")
	if got, want := readCalls(t, calls), "on UDID1 --profile "+profile+" --preserve-boot-state\n"; got != want {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

func TestSimslimEnsure_checkLeavesASlimSimulatorAlone(t *testing.T) {
	path, calls := simslimBin(t, 0)
	ensure(t, path, "check", "UDID1", "SIMSLIM_PROFILE=/p.json")
	if got, want := readCalls(t, calls), "verify UDID1 --profile /p.json\n"; got != want {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

func TestSimslimEnsure_checkRepairsOnceWhenNotSlim(t *testing.T) {
	path, calls := simslimBin(t, 1)
	ensure(t, path, "check", "UDID1", "SIMSLIM_PROFILE=/p.json")
	want := "verify UDID1 --profile /p.json\non UDID1 --profile /p.json --preserve-boot-state\n"
	if got := readCalls(t, calls); got != want {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

func TestSimslimEnsure_missingSimslimWarnsAndContinues(t *testing.T) {
	_, out := ensure(t, "/usr/bin:/bin", "create", "UDID1")
	if !strings.Contains(out, "warning: SimSlim not installed") {
		t.Fatalf("output %q, want a missing simslim warning", out)
	}
}

func TestSimslimEnsure_optOutSkipsSimslim(t *testing.T) {
	path, calls := simslimBin(t, 1)
	_, out := ensure(t, path, "check", "UDID1", "MONACO_NO_SIMSLIM=1")
	if got := readCalls(t, calls); got != "" || out != "" {
		t.Fatalf("calls %q output %q, want none", got, out)
	}
}

func TestResolveIOSSim_laneChecksAnExistingSimulator(t *testing.T) {
	path, calls := simslimBin(t, 1)
	lane := simCheckout(t, "again")
	state := t.TempDir()
	env := []string{"HOME=/h", "PATH=" + path}
	first := runSim(t, lane, "resolve-ios-sim.sh", "simctl", state, env...)
	second := runSim(t, lane, "resolve-ios-sim.sh", "simctl", state, env...)
	if first.err != nil || second.err != nil || first.udid != second.udid {
		t.Fatalf("runs %v %v: %q %q", first.err, second.err, first.out, second.out)
	}
	profile := filepath.Join(lane, "ci", "profiles", "base-slim.json")
	want := "on " + first.udid + " --profile " + profile + " --preserve-boot-state\n" +
		"verify " + first.udid + " --profile " + profile + "\n" +
		"on " + first.udid + " --profile " + profile + " --preserve-boot-state\n"
	if got := readCalls(t, calls); !strings.Contains(got, want) {
		t.Fatalf("calls %q, want to contain %q", got, want)
	}
}
