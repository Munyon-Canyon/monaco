package main

import (
	"context"
	"io"
	"net/http"
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
	checkOffline = "disconnected"
)

type lastTicker interface {
	LastTick(name string) time.Time
}

type health struct {
	connected func() bool
	pool      *pgxpool.Pool
	ticks     lastTicker
	pollers   []poller.Poller
	clock     clock.Clock
}

func (h health) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.serve)
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

func (h health) nats() string {
	if !h.connected() {
		return checkOffline
	}
	return checkOK
}

func (h health) db(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
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
