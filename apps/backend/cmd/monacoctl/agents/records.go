package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

type Record struct {
	Ticket   int       `json:"ticket"`
	Model    string    `json:"model"`
	Worktree string    `json:"worktree"`
	Branch   string    `json:"branch,omitempty"`
	Base     string    `json:"base"`
	State    State     `json:"state"`
	AgentID  string    `json:"agent_id,omitempty"`
	Queued   *Queue    `json:"queued,omitempty"`
	Started  time.Time `json:"started"`
	Changed  time.Time `json:"changed"`
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

func (env *Env) saveRecord(r Record) error {
	data, _ := json.MarshalIndent(r, "", "  ")
	path := env.recordPath(r.Ticket)
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
