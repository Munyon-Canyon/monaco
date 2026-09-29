package agents

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

const handoffMarker = "<!-- monacoctl agents handoff -->"

func handoffCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return usageError("handoff")
	}
	all, err := env.comments(ctx)
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
	body := handoffBody(env, views, rs)
	if err := env.publishComment(ctx, env.Config.Tracking, all, handoffMarker, body); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "handoff posted to #%d\n", env.Config.Tracking)
	return nil
}

func handoffBody(env *Env, views []ticketView, rs []Record) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s\nHandoff at %s. Feature branch `%s`.\n\n**Batch**\n\n",
		handoffMarker, env.Now().UTC().Format(time.RFC3339), env.Config.FeatureBranch)
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
