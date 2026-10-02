package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

type LedgerCheck struct {
	Name  string
	Check func(ctx context.Context, pool *pgxpool.Pool) error
}

func LedgerChecks() []LedgerCheck { return fromReplay(replay.LedgerChecks()) }

type LedgerDiffsError []string

func (d LedgerDiffsError) Error() string { return strings.Join(d, "; ") }

func fromReplay(checks []replay.LedgerCheck) []LedgerCheck {
	out := make([]LedgerCheck, 0, len(checks))
	for _, c := range checks {
		out = append(out, LedgerCheck{Name: c.Name, Check: func(ctx context.Context, pool *pgxpool.Pool) error {
			diffs, err := c.Check(ctx, pool)
			if err != nil || len(diffs) == 0 {
				return err
			}
			return LedgerDiffsError(diffs)
		}})
	}
	return out
}

type InvariantError struct {
	Flow, Msg string
}

func (e *InvariantError) Error() string {
	if e.Flow == "" {
		return "invariant: " + e.Msg
	}
	return "flow " + e.Flow + " invariant: " + e.Msg
}

func (d *driver) settle(ctx context.Context, res *Result) error {
	query, cancel := detached(ctx)
	defer cancel()
	ids, err := d.flowEvents(query, res.Users)
	res.Events = ids
	if err != nil {
		return err
	}
	if err := d.converge(ctx, res.Unit, ids); err != nil {
		return err
	}
	kind, _ := res.Unit.Flow.TriggerKind()
	if kind == tools.TriggerRoute {
		if msg := d.outcomeMismatch(res); msg != "" {
			return &InvariantError{Flow: res.Unit.Name(), Msg: msg}
		}
	}
	found, msg := d.awaitLogs(ctx, res.logFrom, d.requiredLogs(res.Unit))
	res.logLines = found
	if kind != tools.TriggerRoute {
		if mismatch := d.outcomeMismatch(res); mismatch != "" {
			return &InvariantError{Flow: res.Unit.Name(), Msg: mismatch}
		}
	}
	if msg != "" {
		return &InvariantError{Flow: res.Unit.Name(), Msg: msg}
	}
	return nil
}

func (d *driver) awaitLogs(ctx context.Context, from int, needs []logNeed) ([]string, string) {
	tick := time.NewTicker(logPollEvery)
	defer tick.Stop()
	for {
		found, msg := logsMissing(d.env.Logs.Lines()[from:], needs)
		if msg == "" || ctx.Err() != nil {
			return found, msg
		}
		select {
		case <-ctx.Done():
		case <-tick.C:
		}
	}
}

func (d *driver) outcomeMismatch(res *Result) string {
	kind, name := res.Unit.Flow.TriggerKind()
	if kind == tools.TriggerRoute {
		return routeMismatch(res, name)
	}
	need, missing := d.triggerLine(res.Unit, kind, name)
	if slices.ContainsFunc(d.env.Logs.Lines()[res.logFrom:], func(l Line) bool { return lineMatches(l, need) }) {
		return ""
	}
	return missing
}

func routeMismatch(res *Result, route string) string {
	method, path, _ := strings.Cut(route, " ")
	i := slices.IndexFunc(res.Exchanges, func(e scenario.Exchange) bool {
		return e.Method == method && pathMatches(path, e.Path)
	})
	if i < 0 {
		return fmt.Sprintf("no %s request was sent", route)
	}
	got := res.Exchanges[i]
	name, isCode := res.Unit.Outcome.CodeName()
	if !isCode {
		if got.Status >= http.StatusBadRequest {
			return fmt.Sprintf("%s answered %d, want a 2xx for outcome %s", route, got.Status, res.Unit.Outcome)
		}
		return ""
	}
	code := codeNamed(name)
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(got.Response, &body)
	if want := errs.HTTPStatus(errs.KindOf(code)); got.Status != want || body.Code != string(code) {
		return fmt.Sprintf("%s answered %d code %q, want %d code %q", route, got.Status, body.Code, want, code)
	}
	return ""
}

func (d *driver) triggerLine(u Unit, kind tools.TriggerKind, name string) (logNeed, string) {
	codeName, isCode := u.Outcome.CodeName()
	code := string(codeNamed(codeName))
	if kind == tools.TriggerPoller {
		if isCode {
			return logNeed{observability.PollerFailed, map[string]string{"poller": name, "code": code}},
				fmt.Sprintf("no poller.tick.failed for %s with code %s after the script started", name, code)
		}
		return logNeed{observability.PollerTick, map[string]string{"poller": name}},
			fmt.Sprintf("no poller.tick for %s after the script started", name)
	}
	subject := d.env.Subject(events.Type(name).Subject())
	if isCode {
		return logNeed{observability.BusDispatched, map[string]string{"subject": subject, "code": code}},
			fmt.Sprintf("no bus.dispatched for %s with code %s after the script started", name, code)
	}
	ack := string(bus.OutcomeAck)
	return logNeed{observability.BusDispatched, map[string]string{"subject": subject, "outcome": ack}},
		fmt.Sprintf("no bus.dispatched for %s with outcome %s after the script started", name, ack)
}

func pathMatches(pattern, path string) bool {
	want, got := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != got[i] && !strings.HasPrefix(want[i], "{") {
			return false
		}
	}
	return true
}

func codeNamed(name string) errs.Code {
	for _, c := range errs.All() {
		if errs.Name(c) == name {
			return c
		}
	}
	return errs.Code(name)
}

type logNeed struct {
	msg   observability.Msg
	attrs map[string]string
}

func (d *driver) requiredLogs(u Unit) []logNeed {
	needs := d.triggerLogs(u)
	if _, isCode := u.Outcome.CodeName(); isCode {
		return needs
	}
	if len(u.Flow.Events) > 0 {
		needs = append(needs, logNeed{msg: observability.BusRelayTick})
	}
	for _, w := range d.watchedBy(u) {
		needs = append(needs, logNeed{observability.BusDispatched, map[string]string{"handler": w.handler}})
	}
	return needs
}

func (d *driver) triggerLogs(u Unit) []logNeed {
	kind, name := u.Flow.TriggerKind()
	if kind != tools.TriggerRoute {
		need, _ := d.triggerLine(u, kind, name)
		return []logNeed{need}
	}
	method, route, _ := strings.Cut(name, " ")
	needs := []logNeed{{observability.HTTPRequest, map[string]string{"method": method, "route": route}}}
	if code, isCode := u.Outcome.CodeName(); isCode {
		needs = append(needs, logNeed{observability.HTTPProblem, map[string]string{"code": string(codeNamed(code))}})
	}
	return needs
}

func logsMissing(lines []Line, needs []logNeed) ([]string, string) {
	var found []string
	for _, need := range needs {
		i := slices.IndexFunc(lines, func(l Line) bool { return lineMatches(l, need) })
		if i < 0 {
			return found, fmt.Sprintf("no %s log line with %v", need.msg.Name, need.attrs)
		}
		fields := map[string]any{}
		_ = json.Unmarshal([]byte(lines[i].Text), &fields)
		for _, key := range need.msg.Required {
			if _, ok := fields[key]; !ok {
				return found, fmt.Sprintf("%s log line from %s lacks required attr %q: %s",
					need.msg.Name, lines[i].Process, key, lines[i].Text)
			}
		}
		found = append(found, lines[i].Process+": "+lines[i].Text)
	}
	return found, ""
}

func lineMatches(line Line, need logNeed) bool {
	fields := map[string]any{}
	return json.Unmarshal([]byte(line.Text), &fields) == nil && fields["msg"] == need.msg.Name &&
		matches(fields, need.attrs)
}

func matches(fields map[string]any, attrs map[string]string) bool {
	for k, v := range attrs {
		if fmt.Sprint(fields[k]) != v {
			return false
		}
	}
	return true
}

func (d *driver) global(ctx context.Context, ledger []LedgerCheck, rep *report) []error {
	var failures []error
	stream, err := d.env.JS.Stream(ctx, d.env.DeadLetter)
	if err != nil {
		failures = append(failures, fmt.Errorf("read %s: %w", d.env.DeadLetter, err))
	} else if rep.deadLetters = stream.CachedInfo().State.Msgs; rep.deadLetters > 0 {
		failures = append(failures, &InvariantError{Msg: fmt.Sprintf("%d dead letters in %s", rep.deadLetters,
			d.env.DeadLetter)})
	}
	internal := map[string]bool{}
	for _, c := range errs.All() {
		internal[string(c)] = errs.KindOf(c) == errs.KindInternal
	}
	for _, line := range d.env.Logs.Lines() {
		var fields struct {
			Code any `json:"code"`
		}
		if json.Unmarshal([]byte(line.Text), &fields) == nil && internal[fmt.Sprint(fields.Code)] {
			failures = append(failures, &InvariantError{Msg: fmt.Sprintf("%s logged an internal error: %s",
				line.Process, line.Text)})
		}
	}
	for _, l := range ledger {
		if err := l.Check(ctx, d.env.Pool); err != nil {
			failures = append(failures, &InvariantError{Msg: "ledger " + l.Name + ": " + err.Error()})
		}
	}
	return failures
}
