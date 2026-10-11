package agents

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func (env *Env) fastForwardTrunk(ctx context.Context) (string, error) {
	trunk, root := env.Config.FeatureBranch, filepath.Dir(env.Common)
	if _, err := env.gitIn(ctx, root, "fetch", "--no-tags", "origin", trunk); err != nil {
		return "", err
	}
	behind, err := env.behindOrigin(ctx, root, trunk)
	if err != nil || !behind {
		return "", err
	}
	where, err := env.checkedOutAt(ctx, root, "refs/heads/"+trunk)
	if err != nil {
		return "", err
	}
	switch {
	case where == "":
		_, err = env.gitIn(ctx, root, "fetch", "--no-tags", "origin", trunk+":"+trunk)
	case filepath.Clean(where) != filepath.Clean(root):
		return fmt.Sprintf("%s not fast-forwarded: checked out in worktree %s", trunk, where), nil
	default:
		var skip string
		if skip, err = env.mergeInPrimary(ctx, root, trunk); skip != "" {
			return skip, nil
		}
	}
	if err != nil {
		return "", err
	}
	short, err := env.gitIn(ctx, root, "rev-parse", "--short", "refs/heads/"+trunk)
	return fmt.Sprintf("%s fast-forwarded to %s", trunk, short), err
}

func (env *Env) behindOrigin(ctx context.Context, root, trunk string) (bool, error) {
	local, err := env.gitIn(ctx, root, "for-each-ref", "--format=%(objectname)", "refs/heads/"+trunk)
	if err != nil || local == "" {
		return false, err
	}
	remote := "refs/remotes/origin/" + trunk
	want, err := env.revParse(ctx, root, remote)
	if err != nil || local == want {
		return false, err
	}
	ahead, err := env.gitIn(ctx, root, "rev-list", "--count", remote+"..refs/heads/"+trunk)
	return ahead == "0", err
}

func (env *Env) mergeInPrimary(ctx context.Context, root, trunk string) (string, error) {
	dirty, err := env.gitIn(ctx, root, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return "", err
	}
	if dirty != "" {
		return trunk + " not fast-forwarded: primary checkout has uncommitted changes", nil
	}
	_, err = env.gitIn(ctx, root, "merge", "--ff-only", "refs/remotes/origin/"+trunk)
	return "", err
}

func (env *Env) gitIn(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := env.Run(ctx, dir, "", "git", args...)
	return strings.TrimSpace(string(out)), err
}

func (env *Env) revParse(ctx context.Context, dir, ref string) (string, error) {
	return env.gitIn(ctx, dir, "rev-parse", "--verify", "--quiet", ref)
}

func (env *Env) checkedOutAt(ctx context.Context, dir, ref string) (string, error) {
	out, err := env.gitIn(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	path := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case line == "branch "+ref:
			return path, nil
		}
	}
	return "", nil
}

func (s *stream) trunkLines(ctx context.Context) []string {
	msg, err := s.env.fastForwardTrunk(ctx)
	key := msg
	switch {
	case err != nil:
		msg = watchErr("trunk: ", err)
		key = msg
	case msg != "" && !strings.Contains(msg, "fast-forwarded to"):
		sha, _ := s.env.revParse(ctx, filepath.Dir(s.env.Common), "refs/remotes/origin/"+s.env.Config.FeatureBranch)
		key = msg + "@" + sha
	}
	if msg == "" {
		s.trunkSaid = ""
		return nil
	}
	if key == s.trunkSaid {
		return nil
	}
	s.trunkSaid = key
	return []string{msg}
}
