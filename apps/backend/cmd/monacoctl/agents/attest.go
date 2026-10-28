package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const stage0Context = "stage0/ok"

func attestCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	const use = "attest <pr>"
	if len(args) != 1 {
		return usageError(use)
	}
	n, err := positiveInt(args[0], use)
	if err != nil {
		return err
	}
	if !env.Config.LandOnStage0 {
		_, _ = fmt.Fprintln(stdout, "land on stage 0 is off; nothing attested")
		return nil
	}
	pr, err := env.GitHub.PR(ctx, n)
	if err != nil {
		return err
	}
	tree, err := env.headTree(ctx, pr.Head.SHA, pr.Head.Ref)
	if err != nil {
		return err
	}
	record, err := env.readCheckRecord(tree)
	if errors.Is(err, os.ErrNotExist) {
		return detailErr(errs.CodeInvalidInput, "monacoctl.agents.attest", fmt.Sprintf(
			"no stage 0 record for #%d head %s (tree %s); run monacoctl agents check first",
			n, pr.Head.SHA[:12], tree[:12],
		))
	}
	if err != nil {
		return err
	}
	auth, err := env.statusAuth(ctx, "")
	if err != nil {
		return err
	}
	body := map[string]string{"state": "success", "context": stage0Context, "description": record.description(tree)}
	if err := env.GitHub.call(
		ctx,
		http.MethodPost,
		env.GitHub.repo("/statuses/%s", pr.Head.SHA),
		auth,
		body,
		nil,
	); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "#%d %s attested: stage 0 passed on tree %s\n", n, pr.Head.SHA[:12], tree[:12])
	return nil
}

func (env *Env) headTree(ctx context.Context, sha, ref string) (string, error) {
	out, err := env.Run(ctx, env.Work, "", "git", "rev-parse", sha+"^{tree}")
	if err != nil {
		_, fetchErr := env.Run(ctx, env.Work, "", "git", "fetch", "--quiet", "origin", ref)
		if out, err = env.Run(ctx, env.Work, "", "git", "rev-parse", sha+"^{tree}"); err != nil {
			return "", fmt.Errorf("tree of %s: %w", sha[:12], errors.Join(err, fetchErr))
		}
	}
	return strings.TrimSpace(string(out)), nil
}
