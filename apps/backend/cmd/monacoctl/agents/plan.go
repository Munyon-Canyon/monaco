package agents

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Kind string

const (
	Light Kind = "light"
	Full  Kind = "full"

	smallDiff     = 50
	verifierBrief = "docs/agents/verifier.md"
	opus          = "opus"
	sonnet        = "sonnet"
)

type Plan struct {
	Kind    Kind
	Model   string
	Reason  string
	Lines   int
	Files   int
	Ticket  int
	Owner   string
	NoOwner string
}

func ticketRef() *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b(?:part of|closes)\s+#(\d+)\b`)
}

func concurrent() *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\+.*(\bgo func\b|\bsync\.|\batomic\.|\bchan\b|\bselect \{)`)
}

func sensitiveAreas() []string {
	return []string{
		"apps/backend/internal/platform/db/",
		"apps/backend/internal/platform/bus/",
		"apps/backend/internal/platform/money/",
		"apps/backend/internal/platform/concurrency/",
		"apps/backend/internal/modules/trading/",
		"apps/backend/internal/modules/treasury/",
		"apps/backend/internal/modules/funding/",
	}
}

func verifyPlanCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	n, err := prArg(args, "verify-plan <pr>")
	if err != nil {
		return err
	}
	pr, err := env.GitHub.PR(ctx, n)
	if err != nil {
		return err
	}
	p, err := env.plan(ctx, pr)
	if err != nil {
		return err
	}
	writePlan(stdout, n, p)
	return nil
}

func writePlan(stdout io.Writer, n int, p Plan) {
	_, _ = fmt.Fprintf(
		stdout,
		"#%d: %s, verifier %s\nreason: %s\nnon-test lines: %d in %d files\n",
		n,
		p.Kind,
		p.Model,
		p.Reason,
		p.Lines,
		p.Files,
	)
	ticket := "unknown"
	if p.NoOwner != "" {
		_, _ = fmt.Fprintf(stdout, "owner: unknown (%s)\n", p.NoOwner)
	} else {
		ticket = strconv.Itoa(p.Ticket)
		_, _ = fmt.Fprintf(stdout, "owner: #%d %s\n", p.Ticket, p.Owner)
	}
	writeSpawn(stdout, p.Model, fmt.Sprintf("pr: %d\nticket: %s\nbrief: %s\n", n, ticket, verifierBrief))
}

func prArg(args []string, use string) (int, error) {
	if len(args) != 1 {
		return 0, usageError(use)
	}
	return positiveInt(args[0], use)
}

func positiveInt(raw, use string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(raw, "#"))
	if err != nil || n <= 0 {
		return 0, usageError(use)
	}
	return n, nil
}

func (env *Env) plan(ctx context.Context, pr PR) (Plan, error) {
	files, err := env.GitHub.Files(ctx, pr.Number)
	if err != nil {
		return Plan{}, err
	}
	p := classify(files)
	ticket, ok := pr.Ticket()
	if !ok {
		p.NoOwner = "the PR body names no Part of or Closes ticket"
		return p, nil
	}
	r, err := env.record(ticket)
	switch {
	case errs.CodeOf(err) == errs.CodeNotFound:
		p.NoOwner = cliText(err)
	case err != nil:
		return Plan{}, err
	default:
		p.Ticket, p.Owner = ticket, r.Model
		if p.Model == r.Model {
			p.Model = map[string]string{opus: sonnet, sonnet: opus}[p.Model]
		}
	}
	return p, nil
}

func classify(files []File) Plan {
	p := Plan{Kind: Light, Model: sonnet, Reason: "test-only"}
	hygiene := true
	for _, f := range files {
		if testFile(f.Filename) {
			continue
		}
		hygiene = noteFile(&p, f) && hygiene
	}
	return finishPlan(p, hygiene)
}

func noteFile(p *Plan, f File) bool {
	p.Lines += f.Additions + f.Deletions
	p.Files++
	if p.Model != opus {
		if area := sensitiveArea(f); area != "" {
			p.Kind, p.Model, p.Reason = Full, opus, area
		}
	}
	return hygieneFile(f.Filename)
}

func finishPlan(p Plan, hygiene bool) Plan {
	switch {
	case p.Model == opus:
	case p.Files > 0 && hygiene:
		p.Reason = "ci, pr hygiene, or size"
		if p.Lines >= smallDiff {
			p.Kind = Full
		}
	case p.Lines >= smallDiff:
		p.Kind, p.Reason = Full, fmt.Sprintf("%d non-test lines", p.Lines)
	case p.Files > 0:
		p.Reason = fmt.Sprintf("under %d non-test lines", smallDiff)
	}
	return p
}

func hygieneFile(name string) bool {
	switch {
	case strings.HasPrefix(name, ".github/workflows/"):
		return true
	case strings.HasPrefix(name, "scripts/check-pr-") && strings.HasSuffix(name, ".py"):
		return true
	case name == "scripts/pr-body.sh" || name == ".github/pull_request_template.md":
		return true
	default:
		return false
	}
}

func testFile(name string) bool {
	return strings.HasSuffix(name, "_test.go") || strings.Contains(name, "/testdata/") ||
		strings.HasPrefix(name, "testdata/")
}

func sensitiveArea(f File) string {
	for _, area := range sensitiveAreas() {
		if strings.HasPrefix(f.Filename, area) {
			return f.Filename + " is in " + strings.TrimPrefix(area, "apps/backend/internal/")
		}
	}
	if m := concurrent().FindStringSubmatch(f.Patch); m != nil {
		return fmt.Sprintf("%s adds concurrency code (%s)", f.Filename, m[1])
	}
	return ""
}

func (p PR) Ticket() (int, bool) {
	m := ticketRef().FindStringSubmatch(p.Body)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	return n, true
}
