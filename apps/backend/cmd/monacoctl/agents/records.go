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
	Ticket   int         `json:"ticket"`
	Model    string      `json:"model"`
	Worktree string      `json:"worktree"`
	Branch   string      `json:"branch,omitempty"`
	Base     string      `json:"base"`
	State    State       `json:"state"`
	AgentID  string      `json:"agent_id,omitempty"`
	Queued   *Queue      `json:"queued,omitempty"`
	Armed    *Arm        `json:"armed,omitempty"`
	Settled  *Settlement `json:"settled,omitempty"`
	Started  time.Time   `json:"started"`
	Changed  time.Time   `json:"changed"`
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
	f, err := lockWait(env.recordPath(ticket) + ".lock")
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
