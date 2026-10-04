package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Verdict struct {
	PR          int    `json:"pr"`
	SHA         string `json:"sha"`
	PatchID     string `json:"patch_id"`
	State       string `json:"state"`
	Kind        string `json:"kind"`
	Model       string `json:"model"`
	Description string `json:"description"`
}

type verdictIn struct {
	state, sha, kind, model, report, key string
	pr                                   int
}

func verdictCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	if len(args) > 0 && args[0] == "carry" {
		return carryCmd(ctx, env, args[1:], stdout)
	}
	return postVerdict(ctx, env, args, stdout)
}

func postVerdict(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	in, err := parseVerdict(args)
	if err != nil {
		return err
	}
	pr, err := env.GitHub.PR(ctx, in.pr)
	if err != nil {
		return err
	}
	p, err := env.plan(ctx, pr)
	if err != nil {
		return err
	}
	if err := checkVerdict(in, pr, p); err != nil {
		return err
	}
	text, err := os.ReadFile(in.report)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}
	in.model = strings.TrimSpace(in.model)
	desc := describe(Kind(in.kind), in.model, string(text))
	auth, err := env.statusAuth(ctx, in.key)
	if err != nil {
		return err
	}
	patch, err := env.stablePatch(ctx, pr.Base, in.pr)
	if err != nil {
		return err
	}
	if err := env.postStatus(ctx, auth, in.sha, in.state, desc); err != nil {
		return err
	}
	if err := env.saveVerdict(
		Verdict{
			PR:          in.pr,
			SHA:         in.sha,
			PatchID:     patch,
			State:       in.state,
			Kind:        in.kind,
			Model:       in.model,
			Description: desc,
		},
	); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "#%d %s %s\n", in.pr, in.state, desc)
	return env.recordBranch(ctx, p.Ticket, pr.Head.Ref)
}

func (env *Env) recordBranch(ctx context.Context, ticket int, branch string) error {
	r, err := env.record(ctx, ticket)
	if err != nil {
		return err
	}
	if r.Branch == "" {
		r.Branch = branch
		if err := env.saveRecord(r); err != nil {
			return err
		}
	}
	return env.publishRecord(ctx, r)
}

func parseVerdict(args []string) (verdictIn, error) {
	use := "verdict pass|fail <pr> <sha> --kind light|full --model <name> --report <file>"
	if len(args) < 3 || (args[0] != "pass" && args[0] != "fail") {
		return verdictIn{}, usageError(use)
	}
	n, err := positiveInt(args[1], use)
	if err != nil {
		return verdictIn{}, err
	}
	in := verdictIn{pr: n, sha: args[2], state: "success"}
	if args[0] == "fail" {
		in.state = "failure"
	}
	flags, err := flagPairs(args[3:], use)
	if err != nil {
		return verdictIn{}, err
	}
	in.kind, in.model, in.report, in.key = flags["--kind"], flags["--model"], flags["--report"], flags["--key"]
	if invalidVerdict(in) {
		return verdictIn{}, usageError(use)
	}
	if in.model == "fable" {
		return verdictIn{}, detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.verdict",
			"fable is not a verifier model",
		)
	}
	return in, nil
}

func invalidVerdict(in verdictIn) bool {
	badKind := in.kind != string(Light) && in.kind != string(Full)
	return badKind || in.model == "" || in.report == "" || in.sha == ""
}

func flagPairs(args []string, use string) (map[string]string, error) {
	if len(args)%2 != 0 {
		return nil, usageError(use)
	}
	out := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		switch args[i] {
		case "--kind", "--model", "--report", "--key":
			out[args[i]] = args[i+1]
		default:
			return nil, usageError(use)
		}
	}
	return out, nil
}

func checkVerdict(in verdictIn, pr PR, p Plan) error {
	switch {
	case p.NoOwner != "":
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.verdict",
			"owner record: "+p.NoOwner,
		)
	case in.model == p.Owner:
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.verdict",
			fmt.Sprintf("verifier model %s equals the owner model %s on #%d", in.model, p.Owner, p.Ticket),
		)
	case in.sha != pr.Head.SHA:
		return detailErr(errs.CodeInvalidInput, "monacoctl.agents.verdict", "sha is not the pull request head")
	case Kind(in.kind) == Light && p.Kind == Full:
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.verdict",
			fmt.Sprintf("--kind %s is weaker than %s", in.kind, p.Kind),
		)
	default:
		return nil
	}
}

func describe(kind Kind, model, report string) string {
	line := ""
	for _, l := range strings.Split(report, "\n") {
		if strings.TrimSpace(l) != "" {
			line = strings.TrimSpace(l)
			break
		}
	}
	s := fmt.Sprintf("%s by %s: %s", kind, model, line)
	r := []rune(s)
	if len(r) >= 141 {
		return string(r[:140])
	}
	return s
}

func (env *Env) postStatus(ctx context.Context, auth, sha, state, desc string) error {
	body := map[string]string{"state": state, "context": "verify", "description": desc}
	return env.GitHub.call(ctx, "POST", env.GitHub.repo("/statuses/%s", sha), auth, body, nil)
}

func (env *Env) verdictPath(pr int) string {
	return filepath.Join(env.Common, recordsDir, "verdicts", strconv.Itoa(pr)+".json")
}

func (env *Env) saveVerdict(v Verdict) error {
	data, _ := json.MarshalIndent(v, "", "  ")
	path := env.verdictPath(v.PR)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("write verdict: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write verdict: %w", err)
	}
	return nil
}

func (env *Env) loadVerdict(pr int) (Verdict, error) {
	data, err := os.ReadFile(env.verdictPath(pr))
	if err != nil {
		return Verdict{}, fmt.Errorf("read verdict: %w", err)
	}
	var v Verdict
	if err := json.Unmarshal(data, &v); err != nil {
		return Verdict{}, fmt.Errorf("decode verdict: %w", err)
	}
	return v, nil
}

func carryCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	n, err := prArg(args, "verdict carry <pr>")
	if err != nil {
		return err
	}
	rec, err := env.loadVerdict(n)
	if err != nil {
		return err
	}
	pr, err := env.GitHub.PR(ctx, n)
	if err != nil {
		return err
	}
	if pr.Head.SHA == rec.SHA {
		_, _ = fmt.Fprintf(stdout, "#%d head unchanged\n", n)
		return nil
	}
	id, err := env.stablePatch(ctx, pr.Base, n)
	if err != nil {
		return err
	}
	if id != rec.PatchID {
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.verdict",
			"patch-id differs from the recorded verdict; verify again",
		)
	}
	old := rec.SHA
	desc := carried(old, rec.Description)
	auth, err := env.statusAuth(ctx, "")
	if err != nil {
		return err
	}
	if err := env.postStatus(ctx, auth, pr.Head.SHA, rec.State, desc); err != nil {
		return err
	}
	rec.SHA, rec.Description = pr.Head.SHA, desc
	if err := env.saveVerdict(rec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "#%d carried from %s\n", n, shortSHA(old))
	return nil
}

func carried(old, desc string) string {
	s := "carried from " + shortSHA(old) + ": " + desc
	r := []rune(s)
	if len(r) >= 141 {
		return string(r[:140])
	}
	return s
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (env *Env) stablePatch(ctx context.Context, base Ref, pr int) (id string, err error) {
	prefix := fmt.Sprintf("refs/monaco/verdict/%d/%d/", pr, os.Getpid())
	baseRef, headRef := prefix+"base", prefix+"pr"
	defer func() { err = errors.Join(err, env.deleteVerdictRefs(ctx, baseRef, headRef)) }()
	if err = env.fetchVerdictRefs(ctx, base, pr, baseRef, headRef); err != nil {
		return "", err
	}
	fork, err := env.Run(ctx, env.Work, "", "git", "merge-base", baseRef, headRef)
	if err != nil {
		return "", err
	}
	diff, err := env.Run(ctx, env.Work, "", "git", "diff", strings.TrimSpace(string(fork)), headRef)
	if err != nil {
		return "", err
	}
	out, err := env.Run(ctx, env.Work, string(diff), "git", "patch-id", "--stable")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", detailErr(errs.CodeDecodeFailed, "monacoctl.agents.verdict", "patch-id returned nothing")
	}
	return fields[0], nil
}

func (env *Env) fetchVerdictRefs(ctx context.Context, base Ref, pr int, baseRef, headRef string) error {
	fetch := func(baseSource string) error {
		_, err := env.Run(
			ctx,
			env.Work,
			"",
			"git",
			"fetch",
			"--no-tags",
			"origin",
			"+"+baseSource+":"+baseRef,
			fmt.Sprintf("+refs/pull/%d/head:%s", pr, headRef),
		)
		return err
	}
	byBranch := fetch("refs/heads/" + base.Ref)
	if byBranch == nil {
		return nil
	}
	if bySHA := fetch(base.SHA); bySHA != nil {
		return errors.Join(byBranch, bySHA)
	}
	return nil
}

func (env *Env) deleteVerdictRefs(ctx context.Context, baseRef, headRef string) error {
	script := fmt.Sprintf("delete %s\ndelete %s\n", baseRef, headRef)
	_, err := env.Run(context.WithoutCancel(ctx), env.Work, script, "git", "update-ref", "--stdin")
	return err
}
