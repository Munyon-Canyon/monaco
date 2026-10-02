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
	Ticket     int       `json:"ticket"`
	Touches    []string  `json:"touches"`
	Dispatched time.Time `json:"dispatched,omitzero"`
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
	var (
		deferred []deferral
		shared   []string
	)
	for _, n := range asked {
		touches, waived, reason, err := env.admit(ctx, n, asked, b.Tickets)
		if err != nil {
			return err
		}
		if reason != "" {
			deferred = append(deferred, deferral{n, reason})
			continue
		}
		b.Tickets = append(b.Tickets, BatchTicket{Ticket: n, Touches: touches})
		shared = append(shared, waived...)
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
	for _, line := range shared {
		_, _ = fmt.Fprintf(stdout, "shared: %s\n", line)
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

func (env *Env) admit(
	ctx context.Context, n int, asked []int, accepted []BatchTicket,
) (touches, waived []string, reason string, err error) {
	is, err := env.GitHub.Issue(ctx, n)
	if err != nil {
		return nil, nil, "", err
	}
	touches = touchGlobs(is.Body)
	if len(touches) == 0 {
		return nil, nil, "no Touches line", nil
	}
	reason, err = env.blockedReason(ctx, is.Body, asked)
	if err != nil || reason != "" {
		return nil, nil, reason, err
	}
	for _, a := range accepted {
		o := overlap(touches, a.Touches, env.Config.Shared)
		if o.blocked {
			return nil, nil, fmt.Sprintf("Touches %s overlaps #%d %s", o.mine, a.Ticket, o.theirs), nil
		}
		if len(o.shared) > 0 {
			waived = append(waived, fmt.Sprintf("#%d and #%d both touch %s", n, a.Ticket, strings.Join(o.shared, ", ")))
		}
	}
	if len(accepted) >= env.Config.Batch {
		return nil, nil, fmt.Sprintf("batch is full at %d", env.Config.Batch), nil
	}
	return touches, waived, "", nil
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

type overlapResult struct {
	blocked      bool
	mine, theirs string
	shared       []string
}

func overlap(mine, theirs, shared []string) overlapResult {
	var o overlapResult
	for _, a := range mine {
		for _, b := range theirs {
			if !globsOverlap(strings.Split(a, "/"), strings.Split(b, "/")) {
				continue
			}
			if !inShared(a, shared) || !inShared(b, shared) {
				return overlapResult{blocked: true, mine: a, theirs: b}
			}
			if !slices.Contains(o.shared, a) {
				o.shared = append(o.shared, a)
			}
		}
	}
	return o
}

func inShared(glob string, shared []string) bool {
	return slices.ContainsFunc(shared, func(s string) bool {
		return globWithin(strings.Split(glob, "/"), strings.Split(s, "/"))
	})
}

func globWithin(a, s []string) bool {
	switch {
	case len(s) == 0:
		return len(a) == 0
	case s[0] == "**":
		return globWithin(a, s[1:]) || (len(a) > 0 && globWithin(a[1:], s))
	case len(a) == 0 || a[0] == "**":
		return false
	}
	return segmentWithin(a[0], s[0]) && globWithin(a[1:], s[1:])
}

func segmentWithin(a, s string) bool {
	if !strings.ContainsAny(a, "*?[") {
		ok, _ := path.Match(s, a)
		return ok
	}
	return s == "*" || a == s
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

func loadBatch(path string) (Batch, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Batch{}, false, nil
	}
	if err != nil {
		return Batch{}, false, fmt.Errorf("read batch: %w", err)
	}
	var b Batch
	if err := json.Unmarshal(data, &b); err != nil {
		return Batch{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	return b, true, nil
}

func (env *Env) inBatch(ticket int) error {
	b, ok, err := loadBatch(env.batchPath())
	if err != nil {
		return err
	}
	if !ok {
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			fmt.Sprintf("no batch at %s; run monacoctl agents batch, or pass --urgent", env.batchPath()),
		)
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
