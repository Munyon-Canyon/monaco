package agents

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerdict_refusesAWeakerKindTheOwnerModelOrTheWrongSHA(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := strings.Repeat("a", 40)
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	f.hub.on(get("/pulls/5"), pr(5, "h", "fb", "Part of #40"))
	head := pr(5, "h", "fb", "Part of #40")
	head.Head.SHA = sha
	f.hub.on(get("/pulls/5"), head)
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 60}})
	args := []string{"verdict", "pass", "5", sha, "--kind", "full", "--model", "sonnet", "--report", f.report(t, "ok")}
	if code, _, stderr := f.agents(t, "verdict"); code != 2 || !strings.Contains(stderr, "verdict pass|fail") {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "verdict", "pass", "5", sha, "--kind"); code != 2 {
		t.Fatalf("flag: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "verdict", "nope", "5", sha); code != 2 {
		t.Fatalf("verb: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "verdict", "pass", "5", sha, "--bogus", "x"); code != 2 {
		t.Fatalf("bogus: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "verdict", "pass", "nope", sha); code != 2 {
		t.Fatalf("pr: %d %q", code, stderr)
	}
	fable := append([]string{}, args...)
	fable[7] = "fable"
	if code, _, stderr := f.agents(t, fable...); code != 1 || !strings.Contains(stderr, "fable") {
		t.Fatalf("fable: %d %q", code, stderr)
	}
	owned := append([]string{}, args...)
	owned[7] = opus
	if code, _, stderr := f.agents(t, owned...); code != 1 || !strings.Contains(stderr, "equals the owner") {
		t.Fatalf("owner: %d %q", code, stderr)
	}
	wrong := append([]string{}, args...)
	wrong[3] = strings.Repeat("b", 40)
	if code, _, stderr := f.agents(t, wrong...); code != 1 || !strings.Contains(stderr, "not the pull request head") {
		t.Fatalf("sha: %d %q", code, stderr)
	}
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "apps/backend/internal/platform/db/u.go", Additions: 1}})
	weak := append([]string{}, args...)
	weak[5] = string(RootCheck)
	if code, _, stderr := f.agents(t, weak...); code != 1 || !strings.Contains(stderr, "weaker than full") {
		t.Fatalf("kind: %d %q", code, stderr)
	}
	f.hub.on(get("/pulls/8"), pr(8, "h", "fb", "no ticket"))
	f.hub.on(list("/pulls/8/files?"), []File{})
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"fail",
		"8",
		pr(8, "h", "fb", "").Head.SHA,
		"--kind",
		"root-check",
		"--model",
		"sonnet",
		"--report",
		f.report(t, "x"),
	); code != 1 ||
		!strings.Contains(stderr, "owner record") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
}

func TestVerdict_postsWithTheGhTokenWhenTheKeyIsAbsent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := strings.Repeat("c", 40)
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	head := pr(5, "h", "fb", "Part of #40")
	head.Head.SHA = sha
	f.hub.on(get("/pulls/5"), head)
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	f.scriptGit(
		map[string]string{"fetch": "", "merge-base": "abc\n", "diff": "diff\n", "patch-id": "deadbeef other\n"},
		nil,
	)
	report := f.report(t, "\n  looked at the diff\n")
	code, stdout, stderr := f.agents(
		t,
		"verdict",
		"pass",
		"5",
		sha,
		"--kind",
		"full",
		"--model",
		"sonnet",
		"--report",
		report,
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "success full by sonnet: looked at the diff") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	route := "POST /repos/o/r/statuses/" + sha
	if f.hub.authOf(route) != "token tok" || !strings.Contains(f.hub.body(route), `"context":"verify"`) {
		t.Fatalf("auth=%q body=%q", f.hub.authOf(route), f.hub.body(route))
	}
	if strings.Contains(stderr, "tok") {
		t.Fatal(stderr)
	}
}

func TestVerdict_signsAnInstallationToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	key := f.key(t, "RSA PRIVATE KEY", func() ([]byte, error) {
		k, err := rsa.GenerateKey(rand.Reader, 1024)
		if err != nil {
			return nil, err
		}
		return x509.MarshalPKCS1PrivateKey(k), nil
	})
	sha := strings.Repeat("d", 40)
	f.owner(t, Record{Ticket: 40, Model: sonnet, State: Running})
	head := pr(5, "h", "fb", "Part of #40")
	head.Head.SHA = sha
	f.hub.on(get("/pulls/5"), head)
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.hub.on("POST /app/installations/100/access_tokens", `{"token":"ghs_test"}`)
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	f.scriptGit(
		map[string]string{"fetch": "", "merge-base": "abc\n", "diff": "diff\n", "patch-id": "abc123 other\n"},
		nil,
	)
	long := strings.Repeat("å", 200)
	code, stdout, stderr := f.agents(
		t,
		"verdict",
		"fail",
		"5",
		sha,
		"--kind",
		"root-check",
		"--model",
		"opus",
		"--report",
		f.report(t, long),
		"--key",
		key,
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "failure") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	auth := f.hub.authOf("POST /app/installations/100/access_tokens")
	if !strings.HasPrefix(auth, "Bearer ") || strings.Count(auth, ".") != 2 {
		t.Fatalf("jwt auth=%q", auth)
	}
	if f.hub.authOf("POST /repos/o/r/statuses/"+sha) != "token ghs_test" {
		t.Fatalf("status auth=%q", f.hub.authOf("POST /repos/o/r/statuses/"+sha))
	}
	if _, err := f.Env(t).loadVerdict(5); err != nil {
		t.Fatal(err)
	}
}

func TestVerdict_keyAndTokenFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := strings.Repeat("e", 40)
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	head := pr(5, "h", "fb", "Part of #40")
	head.Head.SHA = sha
	f.hub.on(get("/pulls/5"), head)
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.scriptGit(map[string]string{"patch-id": "abc other\n", "fetch": "", "merge-base": "a\n", "diff": "d\n"}, nil)
	base := []string{
		"verdict",
		"pass",
		"5",
		sha,
		"--kind",
		"root-check",
		"--model",
		"sonnet",
		"--report",
		f.report(t, "ok"),
	}
	f.env = []string{f.env[0], "GH_TOKEN=tok"}
	if code, _, stderr := f.agents(t, base...); code != 1 || !strings.Contains(stderr, "HOME is unset") {
		t.Fatalf("home: %d %q", code, stderr)
	}
	f = newFixture(t)
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	f.hub.on(get("/pulls/5"), head)
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.scriptGit(map[string]string{"patch-id": "abc other\n", "fetch": "", "merge-base": "a\n", "diff": "d\n"}, nil)
	nested := filepath.Join(f.home, "not-a-dir")
	writeFile(t, nested, "x")
	if code, _, stderr := f.agents(
		t,
		append(base, "--key", filepath.Join(nested, "k.pem"))...); code != 1 ||
		!strings.Contains(stderr, "verifier key") {
		t.Fatalf("stat: %d %q", code, stderr)
	}
	writeFile(t, filepath.Join(f.home, "bad.pem"), "hello")
	if code, _, stderr := f.agents(
		t,
		append(base, "--key", filepath.Join(f.home, "bad.pem"))...); code != 1 ||
		!strings.Contains(stderr, "not PEM") {
		t.Fatalf("pem: %d %q", code, stderr)
	}
	writeFile(t, filepath.Join(f.home, "junk.pem"), pemBlock("RSA PRIVATE KEY", []byte("nope")))
	if code, _, stderr := f.agents(
		t,
		append(base, "--key", filepath.Join(f.home, "junk.pem"))...); code != 1 ||
		!strings.Contains(stderr, "not PKCS1 or PKCS8") {
		t.Fatalf("pkcs: %d %q", code, stderr)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.home, "ec.pem"), pemBlock("PRIVATE KEY", der))
	if code, _, stderr := f.agents(
		t,
		append(base, "--key", filepath.Join(f.home, "ec.pem"))...); code != 1 ||
		!strings.Contains(stderr, "not RSA") {
		t.Fatalf("ec: %d %q", code, stderr)
	}
	pkcs8 := f.key(t, "PRIVATE KEY", func() ([]byte, error) {
		k, err := rsa.GenerateKey(rand.Reader, 1024)
		if err != nil {
			return nil, err
		}
		return x509.MarshalPKCS8PrivateKey(k)
	})
	f.hub.on("POST /app/installations/100/access_tokens", `{"token":""}`)
	f.scriptGit(map[string]string{"patch-id": "abc other\n", "fetch": "", "merge-base": "a\n", "diff": "d\n"}, nil)
	if code, _, stderr := f.agents(
		t,
		append(base, "--key", pkcs8)...); code != 1 ||
		!strings.Contains(stderr, "empty") {
		t.Fatalf("empty: %d %q", code, stderr)
	}
	f.hub.on("POST /app/installations/100/access_tokens", `nope`)
	if code, _, stderr := f.agents(
		t,
		append(base, "--key", pkcs8)...); code != 1 ||
		!strings.Contains(stderr, "decode POST /app/installations/100") {
		t.Fatalf("decode: %d %q", code, stderr)
	}
	dir := filepath.Join(f.home, "report-dir")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	f.scriptGit(map[string]string{}, map[string]string{})
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"pass",
		"5",
		sha,
		"--kind",
		"root-check",
		"--model",
		"sonnet",
		"--report",
		dir,
	); code != 1 ||
		!strings.Contains(stderr, "read report") {
		t.Fatalf("report: %d %q", code, stderr)
	}
}

func TestStablePatch_reportsEachGitFailure(t *testing.T) {
	t.Parallel()
	steps := []string{"fetch", "merge-base", "diff", "patch-id"}
	for _, step := range steps {
		f := newFixture(t)
		env := f.Env(t)
		fail := map[string]string{step: step + " down"}
		env.Run = scripted(
			fail,
			map[string]string{"fetch": "", "merge-base": "a\n", "diff": "d\n", "patch-id": "id other\n"},
		)
		if _, err := env.stablePatch(
			context.Background(),
			"fb",
			5,
		); err == nil ||
			!strings.Contains(err.Error(), step+" down") {
			t.Fatalf("%s: %v", step, err)
		}
	}
	f := newFixture(t)
	env := f.Env(t)
	env.Run = scripted(nil, map[string]string{"fetch": "", "merge-base": "a\n", "diff": "d\n", "patch-id": "\n"})
	if _, err := env.stablePatch(
		context.Background(),
		"fb",
		5,
	); err == nil ||
		!strings.Contains(cliText(err), "nothing") {
		t.Fatal(err)
	}
}

func TestVerdict_carryRepostsOrRefuses(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	base, head, other := commits(t, f.dir)
	sha := head
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	f.hub.on(get("/pulls/5"), headed(5, sha))
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	f.point(t, base, head)
	f.skipFetch()
	report := f.report(t, "ok")
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"pass",
		"5",
		sha,
		"--kind",
		"root-check",
		"--model",
		"sonnet",
		"--report",
		report,
	); code != 0 {
		t.Fatalf("post: %d %q", code, stderr)
	}
	if code, stdout, stderr := f.agents(
		t,
		"verdict",
		"carry",
		"5",
	); code != 0 || stdout != "#5 head unchanged\n" ||
		stderr != "" {
		t.Fatalf("same: %d %q %q", code, stdout, stderr)
	}
	f.hub.on(get("/pulls/5"), headed(5, other))
	f.point(t, base, other)
	f.hub.on("POST /repos/o/r/statuses/"+other, "ok")
	if code, stdout, stderr := f.agents(
		t,
		"verdict",
		"carry",
		"5",
	); code != 0 ||
		!strings.Contains(stdout, "carried from "+sha[:7]) {
		t.Fatalf("carry: %d %q %q", code, stdout, stderr)
	}
	if f.hub.authOf("POST /repos/o/r/statuses/"+other) != "token tok" {
		t.Fatal(f.hub.authOf("POST /repos/o/r/statuses/" + other))
	}
	moved := commitFile(t, f.dir, "h.go", "changed\n")
	f.hub.on(get("/pulls/5"), headed(5, moved))
	f.point(t, base, moved)
	if code, _, stderr := f.agents(t, "verdict", "carry", "5"); code != 1 || !strings.Contains(stderr, "verify again") {
		t.Fatalf("differ: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "verdict", "carry"); code != 2 {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "verdict", "carry", "9"); code != 1 || !strings.Contains(stderr, "read verdict") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
	writeFile(t, f.Env(t).verdictPath(4), "{")
	f.hub.on(get("/pulls/4"), headed(4, sha))
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"carry",
		"4",
	); code != 1 ||
		!strings.Contains(stderr, "decode verdict") {
		t.Fatalf("decode: %d %q", code, stderr)
	}
	if shortSHA("abcd") != "abcd" || len([]rune(carried(strings.Repeat("a", 40), strings.Repeat("b", 200)))) != 140 {
		t.Fatal("short or carried")
	}
}

func TestVerdict_reportsDownstreamFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := strings.Repeat("a", 40)
	args := []string{
		"verdict",
		"pass",
		"5",
		sha,
		"--kind",
		"root-check",
		"--model",
		"sonnet",
		"--report",
		f.report(t, "ok"),
	}
	if code, _, stderr := f.agents(t, args...); code != 1 || !strings.Contains(stderr, "pulls/5") {
		t.Fatalf("pr: %d %q", code, stderr)
	}
	f.hub.on(get("/pulls/5"), headed(5, sha))
	if code, _, stderr := f.agents(t, args...); code != 1 || !strings.Contains(stderr, "files") {
		t.Fatalf("files: %d %q", code, stderr)
	}
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"pass",
		"5",
		"",
		"--kind",
		"nope",
		"--model",
		"sonnet",
		"--report",
		f.report(t, "ok"),
	); code != 2 {
		t.Fatalf("sha: %d %q", code, stderr)
	}
	f.env = []string{f.env[0], "HOME=" + f.home}
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			return nil, fmt.Errorf("no gh")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	if code, _, stderr := f.agents(t, args...); code != 1 || !strings.Contains(stderr, "github token") {
		t.Fatalf("token: %d %q", code, stderr)
	}
	f = newFixture(t)
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	f.hub.on(get("/pulls/5"), headed(5, sha))
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	dir := filepath.Join(f.home, "keydir")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.agents(
		t,
		append(args, "--key", dir)...); code != 1 ||
		!strings.Contains(stderr, "read verifier key") {
		t.Fatalf("read key: %d %q", code, stderr)
	}
	f.scriptGit(nil, map[string]string{"fetch": "fetch down"})
	if code, _, stderr := f.agents(t, args...); code != 1 || !strings.Contains(stderr, "fetch down") {
		t.Fatalf("fetch: %d %q", code, stderr)
	}
	f.scriptGit(map[string]string{"fetch": "", "merge-base": "a\n", "diff": "d\n", "patch-id": "id other\n"}, nil)
	if code, _, stderr := f.agents(t, args...); code != 1 || !strings.Contains(stderr, "statuses/"+sha) {
		t.Fatalf("status: %d %q", code, stderr)
	}
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	writeFile(t, filepath.Join(f.Env(t).Common, recordsDir, "verdicts"), "x")
	if code, _, stderr := f.agents(t, args...); code != 1 || !strings.Contains(stderr, "write verdict") {
		t.Fatalf("save: %d %q", code, stderr)
	}
	rec := f.Env(t)
	_ = os.Remove(filepath.Join(rec.Common, recordsDir, "verdicts"))
	if err := rec.saveVerdict(Verdict{PR: 7, SHA: "old", PatchID: "same"}); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.agents(t, "verdict", "carry", "7"); code != 1 || !strings.Contains(stderr, "pulls/7") {
		t.Fatalf("carry pr: %d %q", code, stderr)
	}
	f.hub.on(get("/pulls/7"), headed(7, "newsha"))
	f.scriptGit(nil, map[string]string{"fetch": "fetch down"})
	if code, _, stderr := f.agents(t, "verdict", "carry", "7"); code != 1 || !strings.Contains(stderr, "fetch down") {
		t.Fatalf("carry fetch: %d %q", code, stderr)
	}
	f.scriptGit(map[string]string{"fetch": "", "merge-base": "a\n", "diff": "d\n", "patch-id": "same other\n"}, nil)
	f.env = []string{f.env[0]}
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 &&
			(args[0] == "fetch" || args[0] == "merge-base" || args[0] == "diff" || args[0] == "patch-id") {
			return []byte(
				map[string]string{"fetch": "", "merge-base": "a\n", "diff": "d\n", "patch-id": "same other\n"}[args[0]],
			), nil
		}
		if name == "gh" {
			return nil, fmt.Errorf("no gh")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	if code, _, stderr := f.agents(t, "verdict", "carry", "7"); code != 1 || !strings.Contains(stderr, "github token") {
		t.Fatalf("carry token: %d %q", code, stderr)
	}
	f.env = []string{f.env[0], "GH_TOKEN=tok", "HOME=" + f.home}
	f.scriptGit(map[string]string{"fetch": "", "merge-base": "a\n", "diff": "d\n", "patch-id": "same other\n"}, nil)
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"carry",
		"7",
	); code != 1 ||
		!strings.Contains(stderr, "statuses/newsha") {
		t.Fatalf("carry status: %d %q", code, stderr)
	}
	f.hub.on("POST /repos/o/r/statuses/newsha", "ok")
	if err := os.Chmod(f.Env(t).verdictPath(7), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.Env(t).verdictPath(7), 0o600) })
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"carry",
		"7",
	); code != 1 ||
		!strings.Contains(stderr, "write verdict") {
		t.Fatalf("carry save: %d %q", code, stderr)
	}
}

func TestStatusAuth_returnsTheTokenError(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	env.Home = f.home
	env.GitHub.Token = func(context.Context) (string, error) { return "", fmt.Errorf("no gh") }
	if _, err := env.statusAuth(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "no gh") {
		t.Fatal(err)
	}
	if err := env.saveVerdict(Verdict{PR: 3, SHA: "a", PatchID: "p"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	env.GitHub.Token = func(context.Context) (string, error) {
		calls++
		if calls > 1 {
			return "", fmt.Errorf("no gh")
		}
		return "tok", nil
	}
	env.Run = scripted(nil, map[string]string{"fetch": "", "merge-base": "a\n", "diff": "d\n", "patch-id": "p x\n"})
	f.hub.on(get("/pulls/3"), headed(3, "bbbb"))
	err := carryCmd(context.Background(), env, []string{"3"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no gh") {
		t.Fatal(err)
	}
}

func TestVerdict_saveFailsWhenTheVerdictPathIsAFile(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	writeFile(t, filepath.Join(env.Common, recordsDir, "verdicts"), "x")
	if err := env.saveVerdict(Verdict{PR: 1}); err == nil || !strings.Contains(err.Error(), "write verdict") {
		t.Fatal(err)
	}
	env = newFixture(t).Env(t)
	writeFile(t, env.verdictPath(3)+"/x", "")
	if err := env.saveVerdict(Verdict{PR: 3}); err == nil || !strings.Contains(err.Error(), "write verdict") {
		t.Fatal(err)
	}
}

func (f *fixture) report(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.txt")
	writeFile(t, path, text)
	return path
}

func (f *fixture) key(t *testing.T, typ string, der func() ([]byte, error)) string {
	t.Helper()
	b, err := der()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.home, typ+".pem")
	writeFile(t, path, pemBlock(typ, b))
	return path
}

func (f *fixture) scriptGit(ok, fail map[string]string) {
	f.run = scripted(fail, ok)
}

func scripted(fail, ok map[string]string) Runner {
	return func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 {
			if msg, bad := fail[args[0]]; bad {
				return nil, fmt.Errorf("%s", msg)
			}
			if s, good := ok[args[0]]; good {
				return []byte(s), nil
			}
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
}

func (f *fixture) skipFetch() {
	prev := f.run
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "fetch" {
			return nil, nil
		}
		return prev(ctx, dir, stdin, name, args...)
	}
}

func (f *fixture) point(t *testing.T, base, head string) {
	t.Helper()
	git(t, f.dir, "update-ref", "refs/monaco/verdict/base", base)
	git(t, f.dir, "update-ref", "refs/monaco/verdict/pr", head)
}

func headed(n int, sha string) PR {
	p := pr(n, "h", "fb", "Part of #40")
	p.Head.SHA = sha
	return p
}

func commits(t *testing.T, dir string) (base, same, retarget string) {
	t.Helper()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "root")
	base = commitFile(t, dir, "keep.go", "package keep\n")
	head := commitFile(t, dir, "a.go", "package a\n")
	git(t, dir, "checkout", "-q", "-b", "other", base)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	git(t, dir, "add", "a.go")
	git(t, dir, "commit", "-q", "-m", "retarget")
	out, err := Exec(context.Background(), dir, "", "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	retarget = strings.TrimSpace(string(out))
	git(t, dir, "checkout", "-q", "main")
	return base, head, retarget
}

func commitFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	writeFile(t, filepath.Join(dir, name), body)
	git(t, dir, "add", name)
	git(t, dir, "commit", "-q", "-m", name)
	cmdOut, err := Exec(context.Background(), dir, "", "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(cmdOut))
}
