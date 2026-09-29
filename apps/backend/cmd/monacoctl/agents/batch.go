package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Batch struct {
	Created time.Time     `json:"created"`
	Tickets []BatchTicket `json:"tickets"`
}

type BatchTicket struct {
	Ticket  int      `json:"ticket"`
	Touches []string `json:"touches"`
}

type deferral struct {
	ticket int
	reason string
}

func batchCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	asked, err := parseBatch(args)
	if err != nil {
		return err
	}
	b := Batch{Created: env.Now()}
	var deferred []deferral
	for _, n := range asked {
		touches, reason, err := env.admit(ctx, n, asked, b.Tickets)
		if err != nil {
			return err
		}
		if reason != "" {
			deferred = append(deferred, deferral{n, reason})
			continue
		}
		b.Tickets = append(b.Tickets, BatchTicket{Ticket: n, Touches: touches})
	}
	if len(b.Tickets) > 0 {
		if err := env.saveBatch(b); err != nil {
			return err
		}
		nums := make([]int, len(b.Tickets))
		for i, t := range b.Tickets {
			nums[i] = t.Ticket
		}
		_, _ = fmt.Fprintf(
			stdout,
			"batch: %s (%d of %d) in %s\n",
			prList(nums),
			len(nums),
			env.Config.Batch,
			env.batchPath(),
		)
	}
	for _, d := range deferred {
		_, _ = fmt.Fprintf(stdout, "deferred #%d: %s\n", d.ticket, d.reason)
	}
	if len(b.Tickets) == 0 {
		return detailErr(errs.CodeInvalidInput, "monacoctl.agents.batch", "no ticket is ready; batch.json unchanged")
	}
	return nil
}

func parseBatch(args []string) ([]int, error) {
	const use = "batch <issue>..."
	if len(args) == 0 {
		return nil, usageError(use)
	}
	out := make([]int, 0, len(args))
	for _, a := range args {
		n, err := positiveInt(a, use)
		if err != nil {
			return nil, err
		}
		if slices.Contains(out, n) {
			detail := fmt.Sprintf("#%d is listed twice", n)
			return nil, detailErr(errs.CodeInvalidInput, "monacoctl.agents.batch", detail)
		}
		out = append(out, n)
	}
	return out, nil
}

func (env *Env) admit(ctx context.Context, n int, asked []int, accepted []BatchTicket) ([]string, string, error) {
	is, err := env.GitHub.Issue(ctx, n)
	if err != nil {
		return nil, "", err
	}
	touches := touchGlobs(is.Body)
	if len(touches) == 0 {
		return nil, "no Touches line", nil
	}
	reason, err := env.blockedReason(ctx, is.Body, asked)
	if err != nil || reason != "" {
		return nil, reason, err
	}
	for _, a := range accepted {
		if mine, theirs, ok := overlap(touches, a.Touches); ok {
			return nil, fmt.Sprintf("Touches %s overlaps #%d %s", mine, a.Ticket, theirs), nil
		}
	}
	if len(accepted) >= env.Config.Batch {
		return nil, fmt.Sprintf("batch is full at %d", env.Config.Batch), nil
	}
	return touches, "", nil
}

func (env *Env) blockedReason(ctx context.Context, body string, asked []int) (string, error) {
	value, ok := headerField(body, "Blocked by")
	lead := strings.ToLower(strings.TrimSpace(value))
	if !ok || strings.HasPrefix(lead, "none") || strings.HasPrefix(lead, "nothing") {
		return "", nil
	}
	ids := issueNums().FindAllStringSubmatch(value, -1)
	if len(ids) == 0 {
		return "Blocked by line has no issue numbers", nil
	}
	for _, id := range ids {
		b, _ := strconv.Atoi(id[1])
		if slices.Contains(asked, b) {
			return fmt.Sprintf("blocked by #%d in the same batch", b), nil
		}
		err := env.blockerMerged(ctx, b)
		if err != nil && errs.CodeOf(err) == errs.CodeInvalidInput {
			return cliText(err), nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

func headerField(body, name string) (string, bool) {
	field := `(?:\*\*)?` + regexp.QuoteMeta(name) + `(?:\*\*)?:(?:\*\*)?`
	re := regexp.MustCompile(`(?im)(?:^|·)[ \t]*` + field + `[ \t]*([^·\n]*)`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func touchGlobs(body string) []string {
	value, _ := headerField(body, "Touches")
	var out []string
	for _, g := range strings.Split(value, ",") {
		if g = strings.Trim(g, "` \t\r"); g != "" {
			out = append(out, g)
		}
	}
	return out
}

func overlap(mine, theirs []string) (string, string, bool) {
	for _, a := range mine {
		for _, b := range theirs {
			if globsOverlap(strings.Split(a, "/"), strings.Split(b, "/")) {
				return a, b, true
			}
		}
	}
	return "", "", false
}

func globsOverlap(a, b []string) bool {
	switch {
	case len(a) > 0 && a[0] == "**":
		return globsOverlap(a[1:], b) || (len(b) > 0 && globsOverlap(a, b[1:]))
	case len(b) > 0 && b[0] == "**":
		return globsOverlap(b, a)
	case len(a) == 0 || len(b) == 0:
		return len(a) == len(b)
	}
	return segmentsOverlap(a[0], b[0]) && globsOverlap(a[1:], b[1:])
}

func segmentsOverlap(a, b string) bool {
	const meta = "*?["
	if !strings.ContainsAny(a, meta) {
		ok, _ := path.Match(b, a)
		return ok
	}
	if !strings.ContainsAny(b, meta) {
		ok, _ := path.Match(a, b)
		return ok
	}
	pa, pb := a[:strings.IndexAny(a, meta)], b[:strings.IndexAny(b, meta)]
	sa, sb := a[strings.LastIndexAny(a, "*?]")+1:], b[strings.LastIndexAny(b, "*?]")+1:]
	return (strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa)) &&
		(strings.HasSuffix(sa, sb) || strings.HasSuffix(sb, sa))
}

func (env *Env) batchPath() string {
	return filepath.Join(env.Common, "pstack", env.Config.Milestone, "batch.json")
}

func (env *Env) saveBatch(b Batch) error {
	data, _ := json.MarshalIndent(b, "", "  ")
	if err := os.MkdirAll(filepath.Dir(env.batchPath()), 0o750); err != nil {
		return fmt.Errorf("write batch: %w", err)
	}
	if err := os.WriteFile(env.batchPath(), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write batch: %w", err)
	}
	return nil
}

func (env *Env) inBatch(ticket int) error {
	data, err := os.ReadFile(env.batchPath())
	if errors.Is(err, fs.ErrNotExist) {
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			fmt.Sprintf("no batch at %s; run monacoctl agents batch, or pass --urgent", env.batchPath()),
		)
	}
	if err != nil {
		return fmt.Errorf("read batch: %w", err)
	}
	var b Batch
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("decode %s: %w", env.batchPath(), err)
	}
	for _, t := range b.Tickets {
		if t.Ticket == ticket {
			return nil
		}
	}
	return detailErr(
		errs.CodeInvalidInput,
		"monacoctl.agents.dispatch",
		fmt.Sprintf("#%d is not in %s; pass --urgent to dispatch it outside the batch", ticket, env.batchPath()),
	)
}
