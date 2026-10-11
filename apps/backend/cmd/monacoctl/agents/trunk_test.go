package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type trunkRepo struct {
	t       *testing.T
	primary string
	pusher  string
	env     *Env
}

func trunkGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := harnessGit(t.Context(), dir, "", gitArgs(args...)...)
	if err != nil {
		t.Fatalf("git %q in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newTrunkRepo(t *testing.T) *trunkRepo {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	origin := filepath.Join(root, "origin.git")
	primary := filepath.Join(root, "primary")
	pusher := filepath.Join(root, "pusher")
	trunkGit(t, root, "init", "-q", "--bare", "-b", "staging", origin)
	trunkGit(t, root, "clone", "-q", origin, primary)
	trunkGit(t, primary, "switch", "-q", "-c", "staging")
	writeFile(t, filepath.Join(primary, "tracked.txt"), "one\n")
	trunkGit(t, primary, "add", ".")
	trunkGit(t, primary, "commit", "-q", "-m", "one")
	trunkGit(t, primary, "push", "-q", "-u", "origin", "staging")
	trunkGit(t, root, "clone", "-q", origin, pusher)
	r := &trunkRepo{t: t, primary: primary, pusher: pusher}
	r.env = &Env{
		Work: primary, Common: filepath.Join(primary, ".git"), Run: Exec,
		Config: Config{FeatureBranch: "staging"},
	}
	return r
}

func (r *trunkRepo) land(file string) string {
	r.t.Helper()
	writeFile(r.t, filepath.Join(r.pusher, file), "landed\n")
	trunkGit(r.t, r.pusher, "add", ".")
	trunkGit(r.t, r.pusher, "commit", "-q", "-m", "land "+file)
	trunkGit(r.t, r.pusher, "push", "-q", "origin", "staging")
	return trunkGit(r.t, r.pusher, "rev-parse", "HEAD")
}

func (r *trunkRepo) head() string {
	r.t.Helper()
	return trunkGit(r.t, r.primary, "rev-parse", "refs/heads/staging")
}

func TestFastForwardTrunk(t *testing.T) {
	t.Parallel()
	const moved = "staging fast-forwarded to "
	cases := []struct {
		name      string
		setup     func(r *trunkRepo)
		wantMoved bool
		wantMsg   string
	}{
		{name: "behind and clean moves", wantMoved: true, wantMsg: moved},
		{
			name:    "tracked change skips",
			wantMsg: "staging not fast-forwarded: primary checkout has uncommitted changes",
			setup:   func(r *trunkRepo) { writeFile(r.t, filepath.Join(r.primary, "tracked.txt"), "edited\n") },
		},
		{
			name:      "untracked file still moves",
			wantMoved: true,
			wantMsg:   moved,
			setup:     func(r *trunkRepo) { writeFile(r.t, filepath.Join(r.primary, "scratch.txt"), "x\n") },
		},
		{
			name: "diverged local staging is left alone",
			setup: func(r *trunkRepo) {
				writeFile(r.t, filepath.Join(r.primary, "local.txt"), "x\n")
				trunkGit(r.t, r.primary, "add", ".")
				trunkGit(r.t, r.primary, "commit", "-q", "-m", "local only")
			},
		},
		{
			name:      "not checked out moves via fetch",
			wantMoved: true,
			wantMsg:   moved,
			setup:     func(r *trunkRepo) { trunkGit(r.t, r.primary, "switch", "-q", "--detach") },
		},
		{
			name:    "checked out in another worktree skips",
			wantMsg: "staging not fast-forwarded: checked out in worktree ",
			setup: func(r *trunkRepo) {
				trunkGit(r.t, r.primary, "switch", "-q", "--detach")
				lane := filepath.Join(filepath.Dir(r.primary), "lane")
				trunkGit(r.t, r.primary, "worktree", "add", "-q", lane, "staging")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newTrunkRepo(t)
			if tc.setup != nil {
				tc.setup(r)
			}
			want := r.land("landed.txt")
			before := r.head()
			msg, err := r.env.fastForwardTrunk(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(msg, tc.wantMsg) {
				t.Fatalf("message %q, want prefix %q", msg, tc.wantMsg)
			}
			if tc.wantMsg == "" && msg != "" {
				t.Fatalf("message %q, want none", msg)
			}
			after := r.head()
			switch {
			case tc.wantMoved && after != want:
				t.Fatalf("staging at %s, want origin %s", after, want)
			case !tc.wantMoved && after != before:
				t.Fatalf("staging moved from %s to %s", before, after)
			}
			short := strings.TrimPrefix(msg, tc.wantMsg)
			if tc.wantMoved && (short == "" || !strings.HasPrefix(want, short)) {
				t.Fatalf("message %q does not end in the short sha of %s", msg, want)
			}
			if tc.name == "tracked change skips" {
				if got, _ := os.ReadFile(filepath.Join(r.primary, "tracked.txt")); string(got) != "edited\n" {
					t.Fatalf("tracked edit lost: %q", got)
				}
			}
		})
	}
}

func TestFastForwardTrunk_saysNothingWhenCurrent(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	msg, err := r.env.fastForwardTrunk(context.Background())
	if err != nil || msg != "" {
		t.Fatalf("got %q, %v", msg, err)
	}
}

func TestFastForwardTrunk_returnsAGitFailureInsteadOfMovingOn(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"fetch", "worktree", "status", "merge"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			r := newTrunkRepo(t)
			r.land("landed.txt")
			before := r.head()
			r.env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
				if args[0] == step {
					return nil, errors.New(step + " broke")
				}
				return Exec(ctx, dir, stdin, name, args...)
			}
			msg, err := r.env.fastForwardTrunk(context.Background())
			if err == nil || !strings.Contains(err.Error(), step+" broke") || msg != "" {
				t.Fatalf("got %q, %v", msg, err)
			}
			if after := r.head(); after != before {
				t.Fatalf("staging moved from %s to %s", before, after)
			}
		})
	}
}

func TestFastForwardTrunk_leavesAMissingLocalBranchAlone(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	trunkGit(t, r.primary, "switch", "-q", "--detach")
	trunkGit(t, r.primary, "branch", "-q", "-D", "staging")
	r.land("landed.txt")
	msg, err := r.env.fastForwardTrunk(context.Background())
	if err != nil || msg != "" {
		t.Fatalf("got %q, %v", msg, err)
	}
	if got := trunkGit(t, r.primary, "branch", "--list", "staging"); got != "" {
		t.Fatalf("staging recreated: %q", got)
	}
}
