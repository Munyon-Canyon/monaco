package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestMain_usageListsCommandsAndExitsTwo(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, args := range [][]string{nil, {"nope"}, {"--verbose"}} {
		code, stdout, stderr := f.agents(t, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "  forecast\n") {
			t.Fatalf("%q: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestMain_badArgumentsExitTwoWithTheCommandUsage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	code, _, stderr := f.agents(t, "forecast", "x")
	if code != 2 || !strings.HasPrefix(stderr, "monacoctl agents: usage: monacoctl agents forecast") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestFixture_seedsTheRepositoryLookupGitPrints(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	want, err := harnessGit(context.Background(), f.dir, "", strings.Fields(repoLookup)...)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.cached(nil)(context.Background(), f.dir, "", "git", strings.Fields(repoLookup)...)
	if err != nil || string(got) != string(want) {
		t.Fatalf("seeded %q, %v; git prints %q", got, err, want)
	}
}

func TestMain_outsideARepositoryFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.dir = t.TempDir()
	code, _, stderr := f.agents(t, "forecast")
	if code != 1 || !strings.Contains(stderr, "find the repository: git rev-parse") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestMain_configErrorsNameTheFileAndLine(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                         "read config: open",
		"lanes = 8\n":              ".monaco/agents.toml: missing repo",
		"# comment\n\nlanes\n":     ".monaco/agents.toml:3: want key = value",
		"lanes = eight\n":          `.monaco/agents.toml:1: int: strconv.Atoi: parsing "eight"`,
		"repo = o/r\n":             ".monaco/agents.toml:1: quote: invalid syntax",
		strings.Repeat("x", 70000): "read .monaco/agents.toml: bufio.Scanner: token too long",
	}
	for content, want := range cases {
		f := newFixture(t)
		path := filepath.Join(f.dir, configPath)
		if content == "" {
			_ = os.Remove(path)
		} else {
			writeFile(t, path, content)
		}
		code, _, stderr := f.agents(t, "forecast")
		if code != 1 || !strings.Contains(stderr, want) {
			t.Fatalf("%.40q: code=%d stderr=%.200q want %q", content, code, stderr, want)
		}
	}
	f := newFixture(t)
	writeFile(t, filepath.Join(f.dir, configPath), testConfig+"colour = \"blue\"\n")
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.noFailures()
	code, _, stderr := f.agents(t, "watch", "--once")
	if code != 0 || stderr != "monacoctl agents: warning: unknown key \"batch.colour\" in .monaco/agents.toml "+
		"(newer config, or a typo)\n" {
		t.Fatalf("an unknown key: code=%d stderr=%q", code, stderr)
	}
}

func TestGitHub_clientOwnsItsTransport(t *testing.T) {
	t.Parallel()
	tr := newGitHubClient().Transport
	if tr == nil || tr == http.DefaultTransport {
		t.Fatalf("Transport = %v, want one the client owns", tr)
	}
}

func TestGitHub_keptAliveConnectionSurvivesAnotherServerClosing(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(api.Close)
	client := newGitHubClient()
	reused := false
	get := func() {
		trace := &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) { reused = i.Reused }}
		ctx := httptrace.WithClientTrace(t.Context(), trace)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	get()
	httptest.NewServer(http.NotFoundHandler()).Close()
	get()
	if !reused {
		t.Fatal("closing another httptest.Server dropped the client's kept-alive connection")
	}
}

func TestGitHub_fallsBackToTheGhCLIForAToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.env = []string{f.env[0]}
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			return []byte("from-gh\n"), nil
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	f.hub.on(list("/pulls?state=open"), []PR{})
	if code, _, stderr := f.agents(t, "forecast"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got := f.hub.authOf(list("/pulls?state=open")); got != "token from-gh" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestGitHub_usesGitHubTokenWhenGHTokenIsUnset(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.env = []string{f.env[0], "GITHUB_TOKEN=other", "HOME=" + f.home}
	f.hub.on(list("/pulls?state=open"), []PR{})
	if code, _, stderr := f.agents(t, "forecast"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got := f.hub.authOf(list("/pulls?state=open")); got != "token other" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestGitHub_tokenFailureStopsTheCall(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.env = []string{f.env[0]}
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			return nil, errors.New("gh: not logged in")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	code, _, stderr := f.agents(t, "forecast")
	if code != 1 || !strings.Contains(stderr, "github token: gh: not logged in") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestGitHub_pagesUntilAShortPage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	full := make([]File, pageSize)
	for i := range full {
		full[i] = File{Filename: "a"}
	}
	f.hub.on(list("/pulls/1/files?"), full)
	f.hub.on(get("/pulls/1/files?&per_page=100&page=2"), []File{{Filename: "b"}})
	files, err := f.Env(t).GitHub.Files(context.Background(), 1)
	if err != nil || len(files) != pageSize+1 || files[pageSize].Filename != "b" {
		t.Fatalf("files=%d err=%v", len(files), err)
	}
}

func TestGitHub_reportsStatusDecodeAndTransportErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(get("/pulls/2"), "{")
	gh := f.Env(t).GitHub
	ctx := context.Background()
	if _, err := gh.PR(
		ctx,
		1,
	); err == nil ||
		!strings.Contains(err.Error(), `GET /repos/o/r/pulls/1: 404 Not Found: {"message":"Not Found"}`) {
		t.Fatalf("404: %v", err)
	}
	if _, err := gh.PR(ctx, 2); err == nil || !strings.Contains(err.Error(), "decode GET /repos/o/r/pulls/2") {
		t.Fatalf("decode: %v", err)
	}
	if got := f.hub.contentTypeOf(get("/pulls/2")); got != "" {
		t.Fatalf("a request without a body sent Content-Type %q", got)
	}
	if err := gh.call(
		ctx,
		http.MethodPost,
		"/x",
		"",
		make(chan int),
		nil,
	); err == nil ||
		!strings.Contains(err.Error(), "encode POST /x") {
		t.Fatalf("encode: %v", err)
	}
	if err := gh.call(
		ctx,
		"BAD METHOD",
		"/x",
		"",
		nil,
		nil,
	); err == nil ||
		!strings.Contains(err.Error(), "invalid method") {
		t.Fatalf("method: %v", err)
	}
	gh.API = "http://127.0.0.1:0"
	if _, err := gh.Issue(ctx, 1); err == nil || !strings.Contains(err.Error(), "GET /repos/o/r/issues/1") {
		t.Fatalf("transport: %v", err)
	}
}

func TestGitHub_callWithNoOutputIgnoresTheBody(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on("POST /x", "not json")
	if err := f.Env(
		t,
	).GitHub.call(
		context.Background(),
		http.MethodPost,
		"/x",
		"Bearer j",
		map[string]string{"a": "b"},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if f.hub.body("POST /x") != `{"a":"b"}` || f.hub.authOf("POST /x") != "Bearer j" {
		t.Fatalf("body=%q auth=%q", f.hub.body("POST /x"), f.hub.authOf("POST /x"))
	}
	if got := f.hub.contentTypeOf("POST /x"); got != "application/json" {
		t.Fatalf("a request with a body sent Content-Type %q", got)
	}
}

func TestExec_includesStderrInTheError(t *testing.T) {
	t.Parallel()
	out, err := Exec(context.Background(), "", "hi", "cat")
	if err != nil || string(out) != "hi" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	_, err = Exec(context.Background(), "", "", "git", "no-such-command")
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"git no-such-command: exit status 1: git: 'no-such-command' is not a git command",
		) {
		t.Fatalf("err=%v", err)
	}
}

func TestExec_reapsAGrandchildHoldingStdout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "pid")
	var out []byte
	var err error
	took := timed(func() {
		out, err = Exec(context.Background(), dir, "", "sh", "-c", "sleep 600 & echo $! > pid; echo hi")
	})
	if took > execWaitDelay()+time.Second {
		t.Fatalf("returned in %s", took)
	}
	if !bytes.Contains(out, []byte("hi")) || !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("out=%q err=%v", out, err)
	}
	assertGone(t, readPID(t, pidfile))
}

func TestExec_cancelKillsTheProcessGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "pid")
	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		poll(time.Minute, func() bool {
			select {
			case <-stop:
				return true
			default:
			}
			body, err := os.ReadFile(pidfile)
			if err != nil || strings.TrimSpace(string(body)) == "" {
				return false
			}
			cancel()
			return true
		})
	}()
	var err error
	took := timed(func() {
		_, err = Exec(ctx, dir, "", "sh", "-c", "sleep 600 & echo $! > pid; wait")
	})
	if took > 5*time.Second {
		t.Fatalf("cancel took %s err=%v", took, err)
	}
	if err == nil {
		t.Fatal("expected an error")
	}
	assertGone(t, readPID(t, pidfile))
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		t.Fatalf("pid %q: %v", body, err)
	}
	return pid
}

func assertGone(t *testing.T, pid int) {
	t.Helper()
	if poll(time.Second, func() bool { return exited(pid) }) {
		return
	}
	t.Fatalf("process %d still alive", pid)
}

func exited(pid int) bool {
	if zombie(pid) {
		return true
	}
	return exec.CommandContext(context.Background(), "kill", "-0", strconv.Itoa(pid)).Run() != nil
}

func zombie(pid int) bool {
	body, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	_, state, ok := strings.Cut(string(body), ") ")
	return ok && strings.HasPrefix(state, "Z")
}

func TestSpawn_startsAndReportsAMissingProgram(t *testing.T) {
	t.Parallel()
	if err := spawn("true"); err != nil {
		t.Fatal(err)
	}
	if err := spawn(
		"monacoctl-no-such-program",
	); err == nil ||
		!strings.Contains(err.Error(), "start monacoctl-no-such-program") {
		t.Fatalf("err=%v", err)
	}
}

func TestWriteLimited_capsPastTwentyLines(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	writeLimited(&b, "", false)
	writeLimited(&b, "a\n", true)
	writeLimited(&b, "a\n", false)
	writeLimited(&b, strings.Repeat("x\n", 25), false)
	got := b.String()
	if !strings.Contains(got, "a\na\n") || !strings.Contains(got, "and 6 more") {
		t.Fatalf("got %q", got)
	}
	b.Reset()
	writeLimited(&b, strings.Repeat("z\n", 20)+"tail", false)
	if !strings.Contains(b.String(), "and 2 more") || strings.Contains(b.String(), "tail") {
		t.Fatalf("no newline: %q", b.String())
	}
}

func TestExitCode_invalidInputExitsTwoAndADetailLessErrorIsSilent(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	invalid := errs.New(
		errs.CodeInvalidInput,
		"monacoctl.agents.forecast",
		slog.String("detail", "usage: monacoctl agents forecast"),
	)
	if code := exitCode(invalid, &stderr); code != 2 ||
		stderr.String() != "monacoctl agents: usage: monacoctl agents forecast\n" {
		t.Fatalf("invalid: code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	silent := errs.New(errs.CodeForbidden, "monacoctl.agents.watch")
	if code := exitCode(silent, &stderr); code != 1 || stderr.Len() != 0 {
		t.Fatalf("silent: code=%d stderr=%q", code, stderr.String())
	}
}

func TestCliText_readsDetailPastOtherAttrs(t *testing.T) {
	t.Parallel()
	err := errs.New(
		errs.CodeInvalidInput,
		"monacoctl.agents.test",
		slog.String("pr", "4"),
		slog.String("detail", "want this"),
	)
	if got := cliText(err); got != "want this" {
		t.Fatalf("cliText = %q", got)
	}
}

func TestExecWaitDelay_defaultsToTenSecondsWithoutAValidOverride(t *testing.T) {
	t.Setenv("MONACO_EXEC_WAIT_DELAY", "")
	if d := execWaitDelay(); d != 10*time.Second {
		t.Fatalf("execWaitDelay() = %s, want 10s", d)
	}
}

func TestPoll_returnsFalseOnceTheLimitElapses(t *testing.T) {
	t.Parallel()
	if poll(10*time.Millisecond, func() bool { return false }) {
		t.Fatal("poll() = true, want false once the limit elapses")
	}
}

func hangingGH(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	script := fstest.MapFS{"gh": {Data: []byte("#!/bin/sh\nsleep 600\n"), Mode: 0o700}}
	if err := os.CopyFS(bin, script); err != nil {
		t.Fatal(err)
	}
	return bin + string(os.PathListSeparator) + os.Getenv("PATH")
}

func TestExec_aHungGHCallFailsAtItsDeadline(t *testing.T) {
	t.Setenv("PATH", hangingGH(t))
	t.Setenv("MONACO_GH_TIMEOUT", "300ms")
	var err error
	took := timed(func() {
		_, err = Exec(t.Context(), "", "", "gh", "api", "graphql")
	})
	if took > ghTimeout()+execWaitDelay() {
		t.Fatalf("returned in %s", took)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestGHTimeout_defaultsToOneMinuteWithoutAValidOverride(t *testing.T) {
	t.Setenv("MONACO_GH_TIMEOUT", "")
	if d := ghTimeout(); d != time.Minute {
		t.Fatalf("ghTimeout() = %s, want 1m", d)
	}
}

func TestHarnessGit_waitsOutAGitSlowerThanTheExecWaitDelay(t *testing.T) {
	t.Parallel()
	sleep := execWaitDelay() + 2*time.Second
	script := fmt.Sprintf("sleep %.0f; echo done", sleep.Seconds())
	out, err := harnessRun(context.Background(), "sh", t.TempDir(), "", "-c", script)
	if err != nil || strings.TrimSpace(string(out)) != "done" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestHarnessGit_errorIncludesStderr(t *testing.T) {
	t.Parallel()
	_, err := harnessGit(context.Background(), "", "", "no-such-command")
	if err == nil || !strings.Contains(err.Error(), "is not a git command") {
		t.Fatalf("err=%v", err)
	}
}
