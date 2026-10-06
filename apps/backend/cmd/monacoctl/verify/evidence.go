package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	resultPass       = "pass"
	resultFail       = "fail"
	resultOverBudget = "over_budget"
	redacted         = "***"
)

type Evidence struct {
	Flow        string             `json:"flow"`
	Result      string             `json:"result"`
	Phase       Phase              `json:"phase,omitempty"`
	Error       string             `json:"error,omitempty"`
	Commit      string             `json:"commit"`
	Dirty       bool               `json:"dirty"`
	CrashAt     string             `json:"crash_at,omitempty"`
	Crashes     int                `json:"crashes"`
	Outcomes    []OutcomeEvidence  `json:"outcomes"`
	Consumers   []ConsumerEvidence `json:"consumers"`
	DeadLetters uint64             `json:"dead_letters"`
	RunFailures []string           `json:"run_failures,omitempty"`
	LatencyMS   Latency            `json:"latency_ms"`
	PhasesMS    map[Phase]int64    `json:"phases_ms"`
	Host        Host               `json:"host"`
}

type OutcomeEvidence struct {
	Outcome   string             `json:"outcome"`
	Result    string             `json:"result"`
	Failure   string             `json:"failure,omitempty"`
	PhasesMS  map[Phase]int64    `json:"phases_ms"`
	Exchanges []ExchangeEvidence `json:"exchanges"`
	Events    []EventEvidence    `json:"events"`
	LogLines  []string           `json:"log_lines"`
}

type ExchangeEvidence struct {
	Method   string          `json:"method"`
	Path     string          `json:"path"`
	Status   int             `json:"status"`
	Request  json.RawMessage `json:"request,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
	TookMS   int64           `json:"took_ms"`
}

type EventEvidence struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Published  bool     `json:"published"`
	Deliveries []string `json:"deliveries"`
}

type ConsumerEvidence struct {
	Durable     string `json:"durable"`
	Delivered   uint64 `json:"delivered"`
	AckFloor    uint64 `json:"ack_floor"`
	AckPending  int    `json:"ack_pending"`
	Redelivered int    `json:"redelivered"`
}

type Latency struct {
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
}

type Host struct {
	GOOS    string `json:"goos"`
	CPUs    int    `json:"cpus"`
	LoadAvg string `json:"load_avg"`
}

type report struct {
	target      Target
	units       []Unit
	results     []*Result
	phases      map[Phase]time.Duration
	consumers   []ConsumerEvidence
	deadLetters uint64
	global      []string
	crashes     int
	host        Host
}

func newReport(ctx context.Context, target Target, units []Unit) *report {
	uptime, _ := exec.CommandContext(ctx, "uptime").Output()
	return &report{
		target: target, units: units, phases: map[Phase]time.Duration{},
		host: Host{GOOS: runtime.GOOS, CPUs: runtime.NumCPU(), LoadAvg: strings.TrimSpace(string(uptime))},
	}
}

func (d *driver) collect(ctx context.Context, rep *report) error {
	durables := map[string]bool{}
	for _, res := range rep.results {
		rows, err := d.deliveries(ctx, res.Events)
		if err != nil {
			return err
		}
		for _, row := range rows {
			res.rows = append(res.rows, EventEvidence{
				ID: row.ID, Type: row.Type, Published: row.Published, Deliveries: row.Handlers,
			})
		}
		for _, w := range d.watchedBy(res.Unit) {
			durables[w.durable] = true
		}
	}
	for _, durable := range slices.Sorted(maps.Keys(durables)) {
		c, err := d.env.JS.Consumer(ctx, d.env.Events, durable)
		if err != nil {
			return fmt.Errorf("consumer %s: %w", durable, err)
		}
		info := c.CachedInfo()
		rep.consumers = append(rep.consumers, ConsumerEvidence{
			Durable: durable, Delivered: info.Delivered.Stream, AckFloor: info.AckFloor.Stream,
			AckPending: info.NumAckPending, Redelivered: info.NumRedelivered,
		})
	}
	return nil
}

func (rep *report) write(ctx context.Context, dir string, runErr error) error {
	commit, dirty := gitState(ctx, dir)
	byFlow := map[string]*Evidence{}
	var order []string
	for _, u := range rep.units {
		if byFlow[u.Flow.ID] == nil {
			order = append(order, u.Flow.ID)
			byFlow[u.Flow.ID] = &Evidence{
				Flow: u.Flow.ID, Result: resultPass, Commit: commit, Dirty: dirty, CrashAt: rep.target.CrashAt,
				Crashes: rep.crashes, Consumers: rep.consumers, DeadLetters: rep.lettersIn(u.Flow.ID),
				RunFailures: rep.global, PhasesMS: millis(rep.phases), Host: rep.host,
				Outcomes: []OutcomeEvidence{},
			}
		}
	}
	var took []time.Duration
	for _, res := range rep.results {
		ev := byFlow[res.Unit.Flow.ID]
		ev.Outcomes = append(ev.Outcomes, outcomeEvidence(res))
		for _, x := range res.Exchanges {
			took = append(took, x.Took)
		}
		ev.mark(res.Failure, res.Over)
	}
	var over *OverBudgetError
	errors.As(runErr, &over)
	err := os.MkdirAll(filepath.Join(dir, ".verify"), 0o750)
	for _, id := range order {
		ev := byFlow[id]
		ev.LatencyMS = latency(took)
		if runErr != nil {
			ev.mark(runErr.Error(), over)
		}
		err = errors.Join(err, writeEvidence(filepath.Join(dir, ".verify", evidenceName(id, rep.target))+".json", ev))
	}
	return err
}

func (ev *Evidence) mark(failure string, over *OverBudgetError) {
	switch {
	case over != nil && ev.Result != resultOverBudget:
		ev.Result, ev.Phase, ev.Error = resultOverBudget, over.Phase, over.Error()
	case failure != "" && ev.Result == resultPass:
		ev.Result, ev.Error = resultFail, failure
	}
}

func evidenceName(id string, t Target) string {
	if t.CrashAt == "" {
		return id
	}
	return id + "-crash-" + t.CrashAt
}

func writeEvidence(path string, ev *Evidence) error {
	body, err := json.MarshalIndent(ev, "", "  ")
	if err == nil {
		err = os.WriteFile(path, append(body, '\n'), 0o600)
	}
	if err != nil {
		return fmt.Errorf("write evidence: %w", err)
	}
	return nil
}

func outcomeEvidence(res *Result) OutcomeEvidence {
	result := resultPass
	switch {
	case res.Over != nil:
		result = resultOverBudget
	case !res.Pass():
		result = resultFail
	}
	out := OutcomeEvidence{
		Outcome: string(res.Unit.Outcome), Result: result, Failure: res.Failure, PhasesMS: millis(res.Phases),
		Exchanges: []ExchangeEvidence{}, Events: append([]EventEvidence{}, res.rows...),
		LogLines: append([]string{}, res.logLines...),
	}
	for _, x := range res.Exchanges {
		out.Exchanges = append(out.Exchanges, exchangeEvidence(x))
	}
	return out
}

func exchangeEvidence(x scenario.Exchange) ExchangeEvidence {
	return ExchangeEvidence{
		Method: x.Method, Path: x.Path, Status: x.Status, Request: redact(x.Request), Response: redact(x.Response),
		TookMS: x.Took.Milliseconds(),
	}
}

func redact(body []byte) json.RawMessage {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	out, _ := json.Marshal(scrub(v))
	return out
}

func scrub(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			if sensitive(k) {
				t[k] = redacted
				continue
			}
			t[k] = scrub(inner)
		}
	case []any:
		for i, inner := range t {
			t[i] = scrub(inner)
		}
	}
	return v
}

func sensitive(key string) bool {
	key = strings.ToLower(key)
	if slices.Contains([]string{"phone", "phone_number", "phone_hash", "x_user_id"}, key) {
		return true
	}
	for _, word := range []string{"token", "secret", "password", "authorization", "private", "signature"} {
		if strings.Contains(key, word) {
			return true
		}
	}
	return false
}

func latency(took []time.Duration) Latency {
	if len(took) == 0 {
		return Latency{}
	}
	sorted := slices.Sorted(slices.Values(took))
	at := func(q float64) int64 { return sorted[int(q*float64(len(sorted)-1))].Milliseconds() }
	return Latency{P50: at(0.5), P95: at(0.95)}
}

func millis(phases map[Phase]time.Duration) map[Phase]int64 {
	out := make(map[Phase]int64, len(phases))
	for p, d := range phases {
		out[p] = d.Milliseconds()
	}
	return out
}

func gitState(ctx context.Context, dir string) (string, bool) {
	head, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", true
	}
	status, _ := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain").Output()
	return strings.TrimSpace(string(head)), len(status) > 0
}
