package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	patch, err := env.stablePatch(ctx, pr.Base.Ref, in.pr)
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
	return nil
}

func parseVerdict(args []string) (verdictIn, error) {
	use := "verdict pass|fail <pr> <sha> --kind root-check|full --model <name> --report <file>"
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
		return verdictIn{}, failure("fable is not a verifier model")
	}
	return in, nil
}

func invalidVerdict(in verdictIn) bool {
	badKind := in.kind != string(RootCheck) && in.kind != string(Full)
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
		return failf("owner record: %s", p.NoOwner)
	case in.model == p.Owner:
		return failure("verifier model equals the owner")
	case in.sha != pr.Head.SHA:
		return failure("sha is not the pull request head")
	case Kind(in.kind) == RootCheck && p.Kind == Full:
		return failf("--kind %s is weaker than %s", in.kind, p.Kind)
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

func (env *Env) stablePatch(ctx context.Context, baseRef string, pr int) (string, error) {
	spec := fmt.Sprintf("+refs/pull/%d/head:refs/monaco/verdict/pr", pr)
	if _, err := env.Run(
		ctx,
		env.Work,
		"",
		"git",
		"fetch",
		"--no-tags",
		"origin",
		"+refs/heads/"+baseRef+":refs/monaco/verdict/base",
		spec,
	); err != nil {
		return "", err
	}
	base, err := env.Run(ctx, env.Work, "", "git", "merge-base", "refs/monaco/verdict/base", "refs/monaco/verdict/pr")
	if err != nil {
		return "", err
	}
	diff, err := env.Run(ctx, env.Work, "", "git", "diff", strings.TrimSpace(string(base)), "refs/monaco/verdict/pr")
	if err != nil {
		return "", err
	}
	out, err := env.Run(ctx, env.Work, string(diff), "git", "patch-id", "--stable")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", failure("patch-id returned nothing")
	}
	return fields[0], nil
}
