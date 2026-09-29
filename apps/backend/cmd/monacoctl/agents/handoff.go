package agents

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const handoffMarker = "<!-- monacoctl agents handoff -->"

func handoffCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return usageError("handoff")
	}
	local, _, err := loadBatch(env.batchPath())
	if err != nil {
		return err
	}
	branch, err := env.handoffBranch(ctx, local)
	if err != nil {
		return err
	}
	tickets := make([]int, len(local.Tickets))
	for i, t := range local.Tickets {
		tickets[i] = t.Ticket
	}
	issue, err := env.trackingIssue(ctx, branch, tickets...)
	if err != nil {
		return err
	}
	all, err := env.issueComments(ctx, issue)
	if err != nil {
		return err
	}
	status, _ := newestTrusted(all, statusMarker)
	b, err := env.currentBatch(status.Body)
	if err != nil {
		return err
	}
	views, err := env.views(ctx, b)
	if err != nil {
		return err
	}
	rs, err := env.records()
	if err != nil {
		return err
	}
	if err := env.publishComment(ctx, issue, all, handoffMarker, handoffBody(env, branch, views, rs)); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "handoff posted to #%d\n", issue)
	return nil
}

func (env *Env) handoffBranch(ctx context.Context, local Batch) (string, error) {
	if branch := cmp.Or(env.Branch, local.Branch); branch != "" {
		return branch, nil
	}
	trunks, err := env.trunks(ctx)
	switch {
	case err != nil:
		return "", err
	case len(trunks) == 1:
		return trunks[0], nil
	}
	return "", detailErr(errs.CodeInvalidInput, "monacoctl.agents.handoff", fmt.Sprintf(
		"the batch names no feature branch and %s are live; pass --branch <feature>-checkpoint-<N>",
		strings.Join(trunks, ", ")))
}

func handoffBody(env *Env, branch string, views []ticketView, rs []Record) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s\nHandoff at %s. Feature branch `%s`.\n\n**Batch**\n\n",
		handoffMarker, env.Now().UTC().Format(time.RFC3339), branch)
	if len(views) == 0 {
		b.WriteString("No batch.\n")
	} else {
		b.WriteString(boardRows(views, env.Now()))
	}
	b.WriteString("\n**Running agents**\n\n")
	running := 0
	for _, r := range rs {
		if r.State == Exited {
			continue
		}
		running++
		_, _ = fmt.Fprintf(&b, "- #%d %s %s in `%s`, agent `%s`\n", r.Ticket, r.Model, r.State, r.Worktree, r.AgentID)
	}
	if running == 0 {
		b.WriteString("None.\n")
	}
	_, _ = fmt.Fprintf(&b, "\n**Next**\n\n`%s`\n", nextCommand(views))
	return b.String()
}

func nextCommand(views []ticketView) string {
	for _, v := range views {
		if v.Dispatched.IsZero() {
			return fmt.Sprintf("monacoctl agents dispatch %d --model opus", v.Ticket)
		}
	}
	for _, v := range views {
		if v.state() != "merged" {
			return "monacoctl agents watch"
		}
	}
	return "monacoctl agents batch <issue>..."
}
