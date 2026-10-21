package main

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	pingTimeout  = 2 * time.Second
	staleTicks   = 3
	checkOK      = "ok"
	checkNoTick  = "no tick yet"
	checkNoRun   = "no success yet"
	checkOffline = "disconnected"
)

type lastTicker interface {
	LastTick(name string) time.Time
}

type lastSucceeder interface {
	LastSuccess(name string) time.Time
}

type health struct {
	connected func() bool
	pool      *pgxpool.Pool
	ticks     lastTicker
	successes lastSucceeder
	progress  progressReader
	pollers   []poller.Poller
	clock     clock.Clock
	pingLimit time.Duration
}

func (h health) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.serve)
	mux.HandleFunc("GET /readyz", h.ready)
	return mux
}

func (h health) serve(w http.ResponseWriter, r *http.Request) {
	var body strings.Builder
	healthy := true
	report := func(name, status string) {
		healthy = healthy && status == checkOK
		body.WriteString(name + " " + status + "\n")
	}
	report("nats", h.nats())
	report("db", h.db(r.Context()))
	for _, p := range h.pollers {
		report("poller:"+p.Name(), h.poller(p))
	}
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_, _ = io.WriteString(w, body.String())
}

func (h health) ready(w http.ResponseWriter, r *http.Request) {
	var body strings.Builder
	healthy := true
	report := func(name, status string) {
		healthy = healthy && status == checkOK
		body.WriteString(name + " " + status + "\n")
	}
	report("nats", h.nats())
	report("db", h.db(r.Context()))
	for _, p := range h.pollers {
		if poller.IsMoney(p.Name()) {
			report("poller:"+p.Name(), h.succeeded(p))
		}
	}
	lines, status := h.progressLines(r.Context())
	if status != checkOK {
		report("progress", status)
	}
	body.WriteString(lines)
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_, _ = io.WriteString(w, body.String())
}

func (h health) progressLines(ctx context.Context) (string, string) {
	if h.progress == nil {
		return "", checkOK
	}
	limit := pingTimeout
	if h.pingLimit > 0 {
		limit = h.pingLimit
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	p, err := h.progress.Progress(ctx)
	if err != nil {
		return "", err.Error()
	}
	seconds := func(d time.Duration) string { return strconv.FormatInt(int64(d/time.Second), 10) }
	var b strings.Builder
	b.WriteString("progress:money_intent_oldest_seconds " + seconds(p.MoneyIntentOldest) + "\n")
	b.WriteString("progress:outbox_unpublished " + strconv.FormatInt(p.OutboxUnpublished, 10) + "\n")
	b.WriteString("progress:outbox_oldest_seconds " + seconds(p.OutboxOldest) + "\n")
	b.WriteString("progress:consumer_pending " + strconv.FormatInt(p.ConsumerPending, 10) + "\n")
	b.WriteString("progress:consumer_oldest_seconds " + seconds(p.ConsumerOldest) + "\n")
	return b.String(), checkOK
}

func (h health) succeeded(p poller.Poller) string {
	last := h.successes.LastSuccess(p.Name())
	if last.IsZero() {
		return checkNoRun
	}
	if age := h.clock.Now().Sub(last); age >= staleTicks*poller.TickBudget(p) {
		return "stale: last success " + age.Round(time.Second).String() + " ago"
	}
	return checkOK
}

func (h health) nats() string {
	if !h.connected() {
		return checkOffline
	}
	return checkOK
}

func (h health) db(ctx context.Context) string {
	limit := pingTimeout
	if h.pingLimit > 0 {
		limit = h.pingLimit
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		return err.Error()
	}
	return checkOK
}

func (h health) poller(p poller.Poller) string {
	last := h.ticks.LastTick(p.Name())
	if last.IsZero() {
		return checkNoTick
	}
	if age := h.clock.Now().Sub(last); age >= staleTicks*poller.TickBudget(p) {
		return "stale: last tick " + age.Round(time.Second).String() + " ago"
	}
	return checkOK
}
