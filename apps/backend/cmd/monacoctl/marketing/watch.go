package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/providers/x"
)

const (
	pollEvery  = 5 * time.Minute
	failAfter  = 3
	postMicros = 5_000
	userMicros = 10_000
)

type Notifier interface {
	Send(ctx context.Context, text string) error
}

type Watcher struct {
	Config Config
	Source Source
	Notify Notifier
	Store  *Store
	Clock  clock.Clock
	Log    *slog.Logger
}

type searched struct {
	hits    map[string]*Hit
	cursors map[string]string
	failed  int
	last    error
}

func (w *Watcher) Run(ctx context.Context) {
	w.pollAndLog(ctx)
	ticker := w.Clock.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			w.pollAndLog(ctx)
		}
	}
}

func (w *Watcher) pollAndLog(ctx context.Context) {
	if err := w.Poll(ctx); err != nil && ctx.Err() == nil {
		w.Log.LogAttrs(ctx, slog.LevelError, "poll failed", errAttrs(err)...)
	}
}

func (w *Watcher) Poll(ctx context.Context) error {
	const op = "marketing.Watcher.Poll"
	now := w.Clock.Now()
	st, err := w.Store.Load(now)
	if err != nil {
		return err
	}
	if st.Spent >= w.Config.Budget() {
		w.Log.LogAttrs(ctx, slog.LevelInfo, "daily spend budget used up", slog.Int("spent_micros", st.Spent))
		return nil
	}
	got := w.search(ctx, &st)
	if ctx.Err() != nil {
		return errs.Wrap(context.Cause(ctx), errs.CodeUpstreamTimeout, op)
	}
	if got.failed == len(w.Config.Rules) {
		return w.outage(ctx, &st, got)
	}
	st.Failures = 0
	sent := st.Sent
	deliverErr := w.deliver(ctx, &st, now, got.hits)
	if deliverErr == nil {
		maps.Copy(st.Cursors, got.cursors)
	}
	w.Log.LogAttrs(ctx, slog.LevelInfo, "poll done", slog.Int("posts", len(got.hits)),
		slog.Int("alerts", st.Sent-sent), slog.Int("sent_today", st.Sent), slog.Int("spent_micros", st.Spent),
		slog.Int("failed_searches", got.failed))
	return errors.Join(deliverErr, w.Store.Save(st), partial(op, got.failed))
}

func (w *Watcher) search(ctx context.Context, st *State) searched {
	got := searched{hits: map[string]*Hit{}, cursors: map[string]string{}}
	for _, r := range w.Config.Rules {
		page, err := w.Source.Fetch(ctx, r, st.Cursors[r.Name])
		if err != nil {
			got.failed++
			got.last = err
			w.Log.LogAttrs(ctx, slog.LevelWarn, "search failed", append(errAttrs(err), slog.String("rule", r.Name))...)
			continue
		}
		st.Spent += cost(page)
		if page.Newest != "" {
			got.cursors[r.Name] = page.Newest
		}
		addPage(got.hits, r, page)
	}
	return got
}

func (w *Watcher) outage(ctx context.Context, st *State, got searched) error {
	st.Failures++
	if st.Failures == failAfter {
		text := fmt.Sprintf("Reply-finder: X search has failed %d polls in a row (%s). No alerts until it recovers. "+
			"The details are in .build/marketing/watch.log.", failAfter, errs.CodeOf(got.last))
		if err := w.Notify.Send(ctx, text); err != nil {
			w.Log.LogAttrs(ctx, slog.LevelWarn, "outage notice not sent", errAttrs(err)...)
		}
	}
	return errors.Join(partial("marketing.Watcher.Poll", got.failed), w.Store.Save(*st))
}

func (w *Watcher) deliver(ctx context.Context, st *State, now time.Time, hits map[string]*Hit) error {
	var candidates []Scored
	for _, id := range slices.Sorted(maps.Keys(hits)) {
		if _, done := st.Seen[id]; done {
			continue
		}
		if w.Config.Skipped(hits[id].Post, now) != "" {
			st.Seen[id] = now
			continue
		}
		candidates = append(candidates, w.Config.Score(*hits[id], now))
	}
	chosen, rest := w.Config.Select(candidates, w.Config.DailyCap-st.Sent)
	for _, s := range rest {
		st.Seen[s.Post.ID] = now
	}
	for _, s := range chosen {
		if err := w.alert(ctx, st, now, s); err != nil {
			return err
		}
	}
	return nil
}

func (w *Watcher) alert(ctx context.Context, st *State, now time.Time, s Scored) error {
	const op = "marketing.Watcher.alert"
	if err := w.Notify.Send(ctx, Message(s)); err != nil {
		return errs.Wrap(err, errs.CodeOf(err), op, slog.String("post", s.Post.ID))
	}
	st.Seen[s.Post.ID] = now
	st.Sent++
	line, _ := json.Marshal(alertLine{
		PostID: s.Post.ID, Score: s.Score, Angle: s.Angle, Rules: s.Rules, At: now.UTC(),
	})
	return errors.Join(w.Store.Append(alertsPath, append(line, '\n')), w.Store.Save(*st))
}

func cost(page x.Page) int {
	authors := make(map[string]bool, len(page.Posts))
	for _, p := range page.Posts {
		authors[p.Handle] = true
	}
	return len(page.Posts)*postMicros + len(authors)*userMicros
}

func addPage(hits map[string]*Hit, r Rule, page x.Page) {
	for _, p := range page.Posts {
		h, ok := hits[p.ID]
		if !ok {
			h = &Hit{Post: p}
			hits[p.ID] = h
		}
		h.Rules = append(h.Rules, r)
	}
}

func partial(op string, failed int) error {
	if failed == 0 {
		return nil
	}
	return errs.New(errs.CodeUpstreamUnavailable, op, slog.Int("failed_searches", failed))
}

func errAttrs(err error) []slog.Attr {
	return []slog.Attr{
		slog.String("code", string(errs.CodeOf(err))), slog.String("err", err.Error()),
		slog.GroupAttrs("detail", errs.Detail(err)...),
	}
}
