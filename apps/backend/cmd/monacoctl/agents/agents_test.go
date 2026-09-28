package agents

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain_usageListsCommandsAndExitsTwo(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, args := range [][]string{nil, {"nope"}} {
		code, stdout, stderr := f.agents(t, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "  forecast\n  verify-plan\n") {
			t.Fatalf("%q: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestMain_badArgumentsExitTwoWithTheCommandUsage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, args := range [][]string{{"forecast", "x"}, {"verify-plan"}, {"verify-plan", "abc"}, {"verify-plan", "0"}} {
		code, _, stderr := f.agents(t, args...)
		if code != 2 || !strings.HasPrefix(stderr, "monacoctl agents: usage: monacoctl agents "+args[0]) {
			t.Fatalf("%q: code=%d stderr=%q", args, code, stderr)
		}
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
		"":                                    "read config: open",
		"lanes = 8\n":                         ".monaco/agents.toml: missing repo",
		"# comment\n\nlanes\n":                ".monaco/agents.toml:3: want key = value",
		"lanes = eight\n":                     `.monaco/agents.toml:1: strconv.Atoi: parsing "eight"`,
		"repo = o/r\n":                        ".monaco/agents.toml:1: invalid syntax",
		"repo = \"o/r\"\ncolour = \"blue\"\n": `.monaco/agents.toml:2: unknown key "colour"`,
		strings.Repeat("x", 70000):            "read .monaco/agents.toml: bufio.Scanner: token too long",
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
}

func TestGitHub_fallsBackToTheGhCLIForAToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.env = f.env[:1]
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			return []byte("from-gh\n"), nil
		}
		return runCommand(ctx, dir, stdin, name, args...)
	}
	f.hub.on(list("/pulls?state=open"), []PR{})
	if code, _, stderr := f.agents(t, "forecast"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got := f.hub.authOf(list("/pulls?state=open")); got != "token from-gh" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestGitHub_tokenFailureStopsTheCall(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.env = f.env[:1]
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			return nil, failure("gh: not logged in")
		}
		return runCommand(ctx, dir, stdin, name, args...)
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
}

func TestRunCommand_includesStderrInTheError(t *testing.T) {
	t.Parallel()
	out, err := runCommand(context.Background(), "", "hi", "cat")
	if err != nil || string(out) != "hi" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	_, err = runCommand(context.Background(), "", "", "git", "no-such-command")
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"git no-such-command: exit status 1: git: 'no-such-command' is not a git command",
		) {
		t.Fatalf("err=%v", err)
	}
}

func TestTool_runsFromTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	if code := Tool(nil)([]string{"nope"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
