package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const recordsDir = ".monaco/agents"

type State string

const (
	Running State = "running"
	Done    State = "done"
	Exited  State = "exited"
)

type Outcome string

const (
	outcomeLanded  Outcome = "landed"
	outcomeEjected Outcome = "ejected"
)

type Settlement struct {
	Top     int       `json:"top"`
	PRs     []int     `json:"prs"`
	Outcome Outcome   `json:"outcome"`
	Detail  string    `json:"detail"`
	At      time.Time `json:"at"`
}

type Record struct {
	Ticket    int         `json:"ticket"`
	Model     string      `json:"model"`
	Worktree  string      `json:"worktree"`
	Branch    string      `json:"branch,omitempty"`
	Base      string      `json:"base"`
	State     State       `json:"state"`
	AgentID   string      `json:"agent_id,omitempty"`
	Queued    Queues      `json:"queues,omitempty"`
	Armed     Arms        `json:"arms,omitempty"`
	OldQueued *Queue      `json:"queued,omitempty"`
	OldArmed  *Arm        `json:"armed,omitempty"`
	Settled   *Settlement `json:"settled,omitempty"`
	Started   time.Time   `json:"started"`
	Changed   time.Time   `json:"changed"`

	staleTops []int
}

type (
	Queues []Queue
	Arms   = Queues
)

func (r Record) MarshalJSON() ([]byte, error) {
	type plain Record
	p := plain(r)
	p.OldQueued, p.OldArmed = r.Queued.first(), r.Armed.first()
	raw, _ := json.Marshal(p)
	return raw, nil
}

func (r *Record) UnmarshalJSON(data []byte) error {
	type plain Record
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("decode owner record: %w", err)
	}
	queues, lostQueues := reconcile(p.OldQueued, p.Queued)
	arms, lostArms := reconcile(p.OldArmed, p.Armed)
	p.Queued, p.Armed, p.OldQueued, p.OldArmed = queues, arms, nil, nil
	p.staleTops = slices.Concat(lostQueues, lostArms)
	*r = Record(p)
	return nil
}

func reconcile(one *Queue, list Queues) (Queues, []int) {
	if list == nil {
		if one == nil {
			return nil, nil
		}
		return Queues{*one}, nil
	}
	if one == nil && len(list) == 0 || one != nil && len(list) > 0 && sameStack(*one, list[0]) {
		return list, nil
	}
	return intersect(one, list)
}

func sameStack(a, b Queue) bool { return a.Top == b.Top && a.At.Equal(b.At) }

func intersect(one *Queue, list Queues) (Queues, []int) {
	var kept Queues
	var lost []int
	for _, e := range list {
		if one != nil && sameStack(*one, e) {
			kept = append(kept, e)
			continue
		}
		lost = append(lost, e.Top)
	}
	if one != nil && kept == nil {
		lost = append(lost, one.Top)
	}
	return kept, lost
}

func (r Record) staleLine() (string, bool) {
	if len(r.staleTops) == 0 {
		return "", false
	}
	return fmt.Sprintf("#%d record was rewritten by an older monacoctl; queued/armed lists may be stale; "+
		"rerun land-stack for %s with a current monacoctl", r.Ticket, prRefs(r.staleTops)), true
}

func (qs Queues) first() *Queue {
	if len(qs) == 0 {
		return nil
	}
	return &qs[0]
}

func (qs Queues) find(top int) *Queue {
	if i := slices.IndexFunc(qs, func(q Queue) bool { return q.Top == top }); i >= 0 {
		return &qs[i]
	}
	return nil
}

func (qs Queues) holding(pr int) *Queue {
	if i := slices.IndexFunc(qs, func(q Queue) bool { return slices.Contains(q.PRs, pr) }); i >= 0 {
		return &qs[i]
	}
	return nil
}

func (qs Queues) with(q Queue) Queues {
	return append(qs.without(q.Top), q)
}

func (qs Queues) without(top int) Queues {
	out := slices.DeleteFunc(slices.Clone(qs), func(q Queue) bool { return q.Top == top })
	if len(out) == 0 {
		return nil
	}
	return out
}

func noRecord(ticket int) error {
	return detailErr(
		errs.CodeNotFound,
		"monacoctl.agents.record",
		fmt.Sprintf("no owner record for #%d in %s", ticket, recordsDir),
	)
}

func (env *Env) recordPath(ticket int) string {
	return filepath.Join(env.Common, recordsDir, strconv.Itoa(ticket)+".json")
}

func (env *Env) record(ctx context.Context, ticket int) (Record, error) {
	r, err := env.localRecord(ticket)
	if errs.CodeOf(err) != errs.CodeNotFound {
		return r, err
	}
	return env.rebuildRecord(ctx, ticket)
}

func (env *Env) peekRecord(ctx context.Context, ticket int) (Record, error) {
	r, err := env.localRecord(ticket)
	if errs.CodeOf(err) != errs.CodeNotFound {
		return r, err
	}
	return env.recordFromComment(ctx, ticket)
}

func (env *Env) localRecord(ticket int) (Record, error) {
	data, err := os.ReadFile(env.recordPath(ticket))
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, noRecord(ticket)
	}
	if err != nil {
		return Record{}, fmt.Errorf("read owner record: %w", err)
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, fmt.Errorf("decode %s: %w", env.recordPath(ticket), err)
	}
	return r, nil
}

func jsonKeys(t reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		keys[name] = true
	}
	return keys
}

func jsonKeepingUnknownKeys(path string, r Record) []byte {
	out := map[string]json.RawMessage{}
	if old, err := os.ReadFile(path); err == nil && json.Unmarshal(old, &out) == nil {
		recordKeys := jsonKeys(reflect.TypeFor[Record]())
		for k := range out {
			if recordKeys[k] {
				delete(out, k)
			}
		}
	}
	known, _ := json.Marshal(r)
	_ = json.Unmarshal(known, &out)
	data, _ := json.MarshalIndent(out, "", "  ")
	return data
}

func (env *Env) withRecordLock(ticket int, apply func(Record) (Record, error)) (Record, error) {
	path := env.recordPath(ticket)
	f, err := lockWait(path + ".lock")
	if err != nil {
		return Record{}, err
	}
	defer unlock(f)
	cur, err := env.localRecord(ticket)
	if err != nil {
		return Record{}, err
	}
	next, err := apply(cur)
	if err != nil {
		return Record{}, err
	}
	return next, env.saveRecord(next)
}

func (env *Env) updateRecord(ctx context.Context, ticket int, apply func(*Record)) error {
	r, err := env.withRecordLock(ticket, func(r Record) (Record, error) {
		apply(&r)
		return r, nil
	})
	if err != nil {
		return err
	}
	return env.publishRecord(ctx, r)
}

func (env *Env) saveRecord(r Record) error {
	path := env.recordPath(r.Ticket)
	data := jsonKeepingUnknownKeys(path, r)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("write owner record: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write owner record: %w", err)
	}
	return nil
}

func (env *Env) records() ([]Record, error) {
	entries, err := os.ReadDir(filepath.Join(env.Common, recordsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list owner records: %w", err)
	}
	var out []Record
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r, err := env.localRecord(n)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func worktreeHere(r Record) bool {
	info, err := os.Stat(r.Worktree)
	return err == nil && info.IsDir()
}
