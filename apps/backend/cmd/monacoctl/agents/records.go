package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	Base     string    `json:"base"`
	State    State     `json:"state"`
	AgentID  string    `json:"agent_id,omitempty"`
	Started  time.Time `json:"started"`
	Changed  time.Time `json:"changed"`
}

var errNoRecord = errors.New("no owner record")

func (env *Env) recordPath(ticket int) string {
	return filepath.Join(env.Root, recordsDir, strconv.Itoa(ticket)+".json")
}

func (env *Env) record(ticket int) (Record, error) {
	data, err := os.ReadFile(env.recordPath(ticket))
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, fmt.Errorf("%w for #%d in %s", errNoRecord, ticket, recordsDir)
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
	if err := os.MkdirAll(filepath.Dir(env.recordPath(r.Ticket)), 0o750); err != nil {
		return fmt.Errorf("write owner record: %w", err)
	}
	if err := os.WriteFile(env.recordPath(r.Ticket), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write owner record: %w", err)
	}
	return nil
}

func (env *Env) records() ([]Record, error) {
	entries, err := os.ReadDir(filepath.Join(env.Root, recordsDir))
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
		r, err := env.record(n)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
