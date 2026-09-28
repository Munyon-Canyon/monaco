package agents

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_outsideARepositoryFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.dir = t.TempDir()
	_, err := load(context.Background(), f.env, f.dir, f.run)
	if err == nil || !strings.Contains(err.Error(), "find the repository: git rev-parse") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoad_configErrorsNameTheFileAndLine(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                                    "read config: open",
		"lanes = 8\n":                         ".monaco/agents.toml: missing repo",
		"# comment\n\nlanes\n":                ".monaco/agents.toml:3: want key = value",
		"lanes = eight\n":                     `.monaco/agents.toml:1: int: strconv.Atoi: parsing "eight"`,
		"repo = o/r\n":                        ".monaco/agents.toml:1: quote: invalid syntax",
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
		_, err := load(context.Background(), f.env, f.dir, f.run)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%.40q: err=%v want %q", content, err, want)
		}
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
	if _, err := f.Env(t).GitHub.PRs(context.Background(), "state=open"); err != nil {
		t.Fatal(err)
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
	if _, err := f.Env(t).GitHub.PRs(context.Background(), "state=open"); err != nil {
		t.Fatal(err)
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
			return nil, failure("gh: not logged in")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	_, err := f.Env(t).GitHub.PRs(context.Background(), "state=open")
	if err == nil || !strings.Contains(err.Error(), "github token: gh: not logged in") {
		t.Fatalf("err=%v", err)
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
