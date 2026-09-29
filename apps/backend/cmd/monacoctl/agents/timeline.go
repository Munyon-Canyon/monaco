package agents

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const timelineUse = "timeline [--batch <file>]"

func timelineCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	path := env.batchPath()
	switch {
	case len(args) == 2 && args[0] == "--batch":
		path = args[1]
	case len(args) != 0:
		return usageError(timelineUse)
	}
	b, ok, err := loadBatch(path)
	if err != nil {
		return err
	}
	if !ok {
		return detailErr(errs.CodeNotFound, "monacoctl.agents.timeline", "no batch at "+path)
	}
	if b, err = env.withDispatch(b, Batch{}); err != nil {
		return err
	}
	views, err := env.views(ctx, b)
	if err != nil {
		return err
	}
	writeTimeline(stdout, views)
	return nil
}

func writeTimeline(w io.Writer, views []ticketView) {
	_, _ = fmt.Fprintf(w, "%-7s %-11s  %-11s  %-11s  %-11s  %-11s  %-11s  %s\n",
		"ticket", "dispatch", "push", "stage 1", "verdict", "queued", "merged", "total")
	var start, end time.Time
	merged := 0
	for _, v := range views {
		marks := v.marks()
		_, _ = fmt.Fprintf(w, "%-7s", fmt.Sprintf("#%d", v.Ticket))
		for _, t := range marks {
			_, _ = fmt.Fprintf(w, " %-11s ", stamp(t))
		}
		_, _ = fmt.Fprintf(w, " %s\n", between(marks[0], marks[5]))
		start = earliest(start, marks[0])
		if !marks[5].IsZero() {
			merged++
			end = latest(end, marks[5])
		}
	}
	_, _ = fmt.Fprintf(w, "batch: %d of %d merged, %s from first dispatch to last merge\n",
		merged, len(views), between(start, end))
}

func (v ticketView) marks() [6]time.Time {
	return [6]time.Time{
		v.Dispatched,
		v.first(func(p ticketPR) []time.Time { return []time.Time{p.Opened} }),
		v.lastOfAll(func(p ticketPR) (time.Time, bool) { return p.Stage1At, p.Stage1 == "success" }),
		v.lastOfAll(func(p ticketPR) (time.Time, bool) { return p.VerifyAt, p.Verify == "success" }),
		v.first(func(p ticketPR) []time.Time {
			var added []time.Time
			for _, e := range p.Queued {
				if e.Added {
					added = append(added, e.At)
				}
			}
			return added
		}),
		v.merged(),
	}
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("01-02 15:04")
}

func between(from, to time.Time) string {
	if from.IsZero() || to.IsZero() {
		return "-"
	}
	return span(to.Sub(from))
}
