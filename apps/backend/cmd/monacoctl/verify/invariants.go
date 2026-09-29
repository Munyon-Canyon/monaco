package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type LedgerCheck struct {
	Name  string
	Check func(ctx context.Context, pool *pgxpool.Pool) error
}

func LedgerChecks() []LedgerCheck { return nil }

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
	if msg := d.outcomeMismatch(res); msg != "" {
		return &InvariantError{Flow: res.Unit.Name(), Msg: msg}
	}
	found, msg := logsMissing(d.env.Logs.Lines(), d.requiredLogs(res.Unit))
	res.logLines = found
	if msg != "" {
		return &InvariantError{Flow: res.Unit.Name(), Msg: msg}
	}
	return nil
}

func (d *driver) outcomeMismatch(res *Result) string {
	method, path, _ := strings.Cut(res.Unit.Flow.Trigger, " ")
	i := slices.IndexFunc(res.Exchanges, func(e scenario.Exchange) bool {
		return e.Method == method && pathMatches(path, e.Path)
	})
	if i < 0 {
		return fmt.Sprintf("no %s request was sent", res.Unit.Flow.Trigger)
	}
	got := res.Exchanges[i]
	name, isCode := res.Unit.Outcome.CodeName()
	if !isCode {
		if got.Status >= http.StatusBadRequest {
			return fmt.Sprintf("%s answered %d, want a 2xx for outcome %s", res.Unit.Flow.Trigger, got.Status,
				res.Unit.Outcome)
		}
		return ""
	}
	code := codeNamed(name)
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(got.Response, &body)
	if want := errs.HTTPStatus(errs.KindOf(code)); got.Status != want || body.Code != string(code) {
		return fmt.Sprintf("%s answered %d code %q, want %d code %q", res.Unit.Flow.Trigger, got.Status,
			body.Code, want, code)
	}
	return ""
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
	var needs []logNeed
	if method, route, ok := strings.Cut(u.Flow.Trigger, " "); ok {
		needs = append(needs, logNeed{observability.HTTPRequest, map[string]string{"method": method, "route": route}})
	}
	if name, isCode := u.Outcome.CodeName(); isCode {
		return append(needs, logNeed{observability.HTTPProblem, map[string]string{"code": string(codeNamed(name))}})
	}
	if len(u.Flow.Events) > 0 {
		needs = append(needs, logNeed{msg: observability.BusRelayTick})
	}
	for _, w := range d.watchedBy(u) {
		needs = append(needs, logNeed{observability.BusDispatched, map[string]string{"handler": w.handler}})
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
