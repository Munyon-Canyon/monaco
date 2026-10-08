package agents

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/sync/errgroup"
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
	if code, _, stderr := f.agents(t, owned...); code != 1 ||
		!strings.Contains(stderr, "verifier model opus equals the owner model opus on #40") {
		t.Fatalf("owner: %d %q", code, stderr)
	}
	wrong := append([]string{}, args...)
	wrong[3] = strings.Repeat("b", 40)
	if code, _, stderr := f.agents(t, wrong...); code != 1 || !strings.Contains(stderr, "not the pull request head") {
		t.Fatalf("sha: %d %q", code, stderr)
	}
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "apps/backend/internal/platform/db/u.go", Additions: 1}})
	weak := append([]string{}, args...)
	weak[5] = string(Light)
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
		"light",
		"--model",
		"sonnet",
		"--report",
		f.report(t, "x"),
	); code != 1 ||
		!strings.Contains(stderr, "owner record") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
}

func TestVerdict_checksTheOwnerModelOfAnotherRootsTicketWithoutRebuildingItsRecord(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := strings.Repeat("c", 40)
	f.ownerComments(40, ownerComment(2, Record{Ticket: 40, Model: sonnet, State: Running}))
	f.hub.on(get("/pulls/5"), headed(5, sha))
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	status := "POST /repos/o/r/statuses/" + sha
	f.hub.on(status, "ok")
	f.scriptGit(map[string]string{"fetch": "", "merge-base": "abc\n", "diff": "d\n", "patch-id": "id x\n"}, nil)
	report := f.report(t, "ok")
	verdict := func(model string) (int, string, string) {
		return f.agents(t, "verdict", "pass", "5", sha, "--kind", "full", "--model", model, "--report", report)
	}

	code, _, stderr := verdict(sonnet)
	if code != 1 || !strings.Contains(stderr, "verifier model sonnet equals the owner model sonnet on #40") ||
		f.hub.body(status) != "" {
		t.Fatalf("same model: code=%d stderr=%q status body %q", code, stderr, f.hub.body(status))
	}
	if saved := f.agentsDirEntries(t); len(saved) != 0 {
		t.Errorf("the refused verdict saved %v for a ticket this clone has no record of", saved)
	}

	code, stdout, stderr := verdict(opus)
	if code != 0 || stderr != "" || stdout != "#5 success full by opus: ok\n" ||
		!strings.Contains(f.hub.body(status), `"state":"success"`) {
		t.Fatalf("other model: code=%d stdout=%q stderr=%q status body %q", code, stdout, stderr, f.hub.body(status))
	}
	if saved := f.agentsDirEntries(t); !slices.Equal(saved, []string{"verdicts"}) {
		t.Errorf("the posted verdict saved %v, want only its own verdict under verdicts", saved)
	}
	for _, call := range f.hub.callsContaining("/issues/") {
		if !strings.HasPrefix(call, "GET ") {
			t.Errorf("the posted verdict wrote %q to a ticket this clone has no record of", call)
		}
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
		"light",
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
		"light",
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
		"light",
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
			Ref{Ref: "fb"},
			5,
			0,
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
		Ref{Ref: "fb"},
		5,
		0,
	); err == nil ||
		!strings.Contains(cliText(err), "nothing") {
		t.Fatal(err)
	}
}

func TestStablePatch_eachConcurrentCallRecordsItsOwnPatchID(t *testing.T) {
	t.Parallel()
	const calls = 2
	f := newFixture(t)
	remote := f.remote(t)
	base, one, retarget := commits(t, remote)
	git(t, remote, "checkout", "-q", "--detach", retarget)
	two := commitFile(t, remote, "b.go", "package b\n")
	git(t, remote, "update-ref", "refs/heads/fb", base)
	git(t, remote, "update-ref", "refs/pull/1/head", one)
	git(t, remote, "update-ref", "refs/pull/2/head", two)
	want := [calls]string{patchID(t, remote, base, one, 0), patchID(t, remote, base, two, 0)}
	if want[0] == want[1] {
		t.Fatal("the two pull requests need different patch-ids")
	}
	env := f.Env(t)
	var arrived atomic.Int32
	fetched := make(chan struct{})
	env.Run = atMergeBase(env.Run, func(ctx context.Context) error {
		if arrived.Add(1) == calls {
			close(fetched)
		}
		select {
		case <-fetched:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	var got [calls]string
	g, ctx := errgroup.WithContext(t.Context())
	for i := range calls {
		g.Go(func() (err error) {
			got[i], err = env.stablePatch(ctx, Ref{Ref: "fb"}, i+1, 0)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("patch-ids %v, want %v", got, want)
	}
}

func TestStablePatch_leavesNoVerdictRefsBehind(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	remote := f.remote(t)
	base, head, _ := commits(t, remote)
	git(t, remote, "update-ref", "refs/heads/fb", base)
	git(t, remote, "update-ref", "refs/pull/5/head", head)
	env := f.Env(t)
	want := patchID(t, remote, base, head, 0)
	if id, err := env.stablePatch(t.Context(), Ref{Ref: "fb"}, 5, 0); err != nil || id != want {
		t.Fatalf("success: %q %v", id, err)
	}
	noVerdictRefs(t, f.dir)

	env.Run = scripted(map[string]string{"merge-base": "merge-base down"}, nil)
	if _, err := env.stablePatch(
		t.Context(),
		Ref{Ref: "fb"},
		5,
		0,
	); err == nil ||
		!strings.Contains(err.Error(), "merge-base down") {
		t.Fatalf("failure: %v", err)
	}
	noVerdictRefs(t, f.dir)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	env.Run = atMergeBase(hostless, func(context.Context) error {
		cancel()
		return nil
	})
	if _, err := env.stablePatch(ctx, Ref{Ref: "fb"}, 5, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	noVerdictRefs(t, f.dir)
}

func TestVerdict_postsTheSamePatchIDWithOrWithoutTheBaseBranch(t *testing.T) {
	t.Parallel()
	for name, deleted := range map[string]bool{"branch present": false, "branch deleted": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			remote := f.remote(t)
			git(t, remote, "config", "uploadpack.allowReachableSHA1InWant", "true")
			base, head, _ := commits(t, remote)
			git(t, remote, "update-ref", "refs/heads/fb", base)
			git(t, remote, "update-ref", "refs/pull/5/head", head)
			if deleted {
				git(t, remote, "update-ref", "-d", "refs/heads/fb")
			}
			f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
			pull := headed(5, head)
			pull.Base.SHA = base
			f.hub.on(get("/pulls/5"), pull)
			f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
			status := "POST /repos/o/r/statuses/" + head
			f.hub.on(status, "ok")
			code, _, stderr := f.agents(
				t, "verdict", "pass", "5", head, "--kind", "light", "--model", "sonnet", "--report", f.report(t, "ok"),
			)
			if code != 0 {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
			if posted := f.hub.callsContaining(status); len(posted) != 1 ||
				!strings.Contains(f.hub.body(status), `"state":"success"`) {
				t.Fatalf("status calls %v body %q", posted, f.hub.body(status))
			}
			recorded, err := f.Env(t).loadVerdict(5)
			if err != nil || recorded.PatchIDU0 != patchID(t, remote, base, head, 0) || recorded.PatchID != "" {
				t.Fatalf("recorded %+v, %v", recorded, err)
			}
			noVerdictRefs(t, f.dir)
		})
	}
}

func TestVerdict_carryRepostsOrRefuses(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	remote := f.remote(t)
	base, head, other := commits(t, remote)
	sha := head
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	f.hub.on(get("/pulls/5"), headed(5, sha))
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	git(t, remote, "update-ref", "refs/heads/fb", base)
	git(t, remote, "update-ref", "refs/pull/5/head", head)
	report := f.report(t, "ok")
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"pass",
		"5",
		sha,
		"--kind",
		"light",
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
	git(t, remote, "update-ref", "refs/pull/5/head", other)
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
	moved := commitFile(t, remote, "h.go", "changed\n")
	f.hub.on(get("/pulls/5"), headed(5, moved))
	git(t, remote, "update-ref", "refs/pull/5/head", moved)
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

func TestVerdict_carryJudgesAPullRequestByItsOwnAddedAndRemovedLines(t *testing.T) {
	t.Parallel()
	const (
		text          = "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
		legacyVerdict = `{"pr":5,"sha":%q,"patch_id":%q,"state":"success","kind":"light","model":"sonnet",` +
			`"description":"light by sonnet: ok"}` + "\n"
	)
	withAdded := func(base string, added ...string) string {
		return strings.Replace(base, "five\n", "five\n"+strings.Join(added, "\n")+"\n", 1)
	}
	beside := strings.Replace(text, "four", "FOUR", 1)
	far := strings.Replace(text, "one", "ONE", 1)
	both := []string{"added one", "added two"}
	cases := []struct {
		name           string
		trunk          string
		added          []string
		legacy         bool
		sameU0, sameU3 bool
		carries        bool
	}{
		{
			name:  "a restack that changes a context line beside the hunk",
			trunk: beside, added: both, sameU0: true, carries: true,
		},
		{
			name:  "a restack after which an added line differs",
			trunk: far, added: []string{"added one", "added 2"},
		},
		{
			name:  "a patch that drops one of its added lines",
			trunk: far, added: []string{"added one"},
		},
		{
			name:  "a verdict without patch_id_u0 whose three-line patch-id matches",
			trunk: far, added: both, legacy: true, sameU0: true, sameU3: true, carries: true,
		},
		{
			name:  "a verdict without patch_id_u0 after a context line changed",
			trunk: beside, added: both, legacy: true, sameU0: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			remote := f.remote(t)
			base := commitFile(t, remote, "a.txt", text)
			head := commitFile(t, remote, "a.txt", withAdded(text, both...))
			git(t, remote, "checkout", "-q", "--detach", base)
			trunk := commitFile(t, remote, "a.txt", c.trunk)
			restacked := commitFile(t, remote, "a.txt", withAdded(c.trunk, c.added...))
			for width, same := range map[int]bool{0: c.sameU0, 3: c.sameU3} {
				was := patchID(t, remote, base, head, width)
				now := patchID(t, remote, trunk, restacked, width)
				if (was == now) != same {
					t.Fatalf("patch-ids at -U%d equal %v, want %v", width, was == now, same)
				}
			}

			f.pullAt(t, remote, base, head)
			if c.legacy {
				id := patchID(t, remote, base, head, 3)
				writeFile(t, f.Env(t).verdictPath(5), fmt.Sprintf(legacyVerdict, head, id))
			} else {
				f.postPass(t, head)
			}
			before, err := f.Env(t).loadVerdict(5)
			if err != nil {
				t.Fatal(err)
			}
			if u0 := patchID(t, remote, base, head, 0); !c.legacy {
				if before.PatchIDU0 != u0 || before.PatchID != "" {
					t.Fatalf("posted %+v, want only the patch-id %s without context lines", before, u0)
				}
			}

			f.pullAt(t, remote, trunk, restacked)
			status := "POST /repos/o/r/statuses/" + restacked
			code, stdout, stderr := f.agents(t, "verdict", "carry", "5")
			after, err := f.Env(t).loadVerdict(5)
			if err != nil {
				t.Fatal(err)
			}
			posted := f.hub.callsContaining(status)
			if !c.carries {
				if code != 1 || !strings.Contains(stderr, "patch-id differs from the recorded verdict; verify again") {
					t.Fatalf("refused: code=%d stderr=%q", code, stderr)
				}
				if after != before || len(posted) != 0 {
					t.Fatalf("a refused carry saved %+v over %+v and posted %v", after, before, posted)
				}
				return
			}
			want := before
			want.SHA, want.Description = restacked, "carried from "+head[:7]+": "+before.Description
			if code != 0 || stdout != "#5 carried from "+head[:7]+"\n" || stderr != "" {
				t.Fatalf("carried: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if after != want || len(posted) != 1 || !strings.Contains(f.hub.body(status), `"state":"success"`) {
				t.Fatalf("a carry saved %+v, want %+v, and posted %v", after, want, posted)
			}
		})
	}
}

func (f *fixture) postPass(t *testing.T, head string) {
	t.Helper()
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 2}})
	report := f.report(t, "ok")
	args := []string{"verdict", "pass", "5", head, "--kind", "light", "--model", "sonnet", "--report", report}
	if code, _, stderr := f.agents(t, args...); code != 0 {
		t.Fatalf("post: %d %q", code, stderr)
	}
}

func (f *fixture) pullAt(t *testing.T, remote, base, head string) {
	t.Helper()
	git(t, remote, "update-ref", "refs/heads/fb", base)
	git(t, remote, "update-ref", "refs/pull/5/head", head)
	f.hub.on(get("/pulls/5"), headed(5, head))
	f.hub.on("POST /repos/o/r/statuses/"+head, "ok")
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
		"light",
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
			return nil, errors.New("no gh")
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
			return nil, errors.New("no gh")
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
	freeze(t, f.Env(t).verdictPath(7))
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
	env.GitHub.Token = func(context.Context) (string, error) { return "", errors.New("no gh") }
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
			return "", errors.New("no gh")
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

func (f *fixture) remote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "--template=", "-b", "main")
	git(t, f.dir, "remote", "add", "origin", dir)
	return dir
}

func atMergeBase(next Runner, hook func(context.Context) error) Runner {
	return func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "merge-base" {
			if err := hook(ctx); err != nil {
				return nil, err
			}
		}
		return next(ctx, dir, stdin, name, args...)
	}
}

func patchID(t *testing.T, dir, base, head string, width int) string {
	t.Helper()
	diff, err := harnessGit(context.Background(), dir, "", "diff", fmt.Sprintf("-U%d", width), base, head)
	if err != nil {
		t.Fatal(err)
	}
	out, err := harnessGit(context.Background(), dir, string(diff), "patch-id", "--stable")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(out))[0]
}

func noVerdictRefs(t *testing.T, dir string) {
	t.Helper()
	out, err := harnessGit(context.Background(), dir, "", "for-each-ref", "--format=%(refname)", "refs/monaco/verdict/")
	if err != nil {
		t.Fatal(err)
	}
	if left := strings.TrimSpace(string(out)); left != "" {
		t.Fatalf("verdict refs left behind:\n%s", left)
	}
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
	out, err := harnessGit(context.Background(), dir, "", "rev-parse", "HEAD")
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
	cmdOut, err := harnessGit(context.Background(), dir, "", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(cmdOut))
}
