package marketing

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	dataDir    = ".build/marketing"
	statePath  = dataDir + "/state.json"
	alertsPath = dataDir + "/alerts.jsonl"
	logPath    = dataDir + "/watch.log"
	seenTTL    = 7 * 24 * time.Hour
)

type State struct {
	Cursors  map[string]string    `json:"cursors"`
	Seen     map[string]time.Time `json:"seen"`
	Day      string               `json:"day"`
	Sent     int                  `json:"sent"`
	Spent    int                  `json:"spent_micros"`
	Failures int                  `json:"failures"`
}

func (s *State) Roll(now time.Time) {
	if s.Cursors == nil {
		s.Cursors = map[string]string{}
	}
	if s.Seen == nil {
		s.Seen = map[string]time.Time{}
	}
	if day := now.UTC().Format(time.DateOnly); s.Day != day {
		s.Day, s.Sent, s.Spent = day, 0, 0
	}
	for id, at := range s.Seen {
		if now.Sub(at) > seenTTL {
			delete(s.Seen, id)
		}
	}
}

type Store struct{ root *os.Root }

func OpenStore(repo string) (*Store, error) {
	const op = "marketing.OpenStore"
	root, err := os.OpenRoot(repo)
	if err != nil {
		return nil, storage(err, op)
	}
	if err := root.MkdirAll(dataDir, 0o750); err != nil {
		_ = root.Close()
		return nil, storage(err, op)
	}
	return &Store{root: root}, nil
}

func (s *Store) Close() { _ = s.root.Close() }

func (s *Store) Load(now time.Time) (State, error) {
	const op = "marketing.Store.Load"
	raw, err := s.root.ReadFile(statePath)
	if errors.Is(err, fs.ErrNotExist) {
		raw, err = []byte("{}"), nil
	}
	if err != nil {
		return State{}, storage(err, op, slog.String("path", statePath))
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("path", statePath))
	}
	st.Roll(now)
	return st, nil
}

func (s *Store) Save(st State) error {
	const op = "marketing.Store.Save"
	raw, _ := json.Marshal(st)
	tmp := statePath + ".tmp"
	if err := s.root.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return storage(err, op, slog.String("path", tmp))
	}
	return storage(s.root.Rename(tmp, statePath), op, slog.String("path", statePath))
}

func (s *Store) Append(path string, line []byte) error {
	const op = "marketing.Store.Append"
	f, err := s.root.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return storage(err, op, slog.String("path", path))
	}
	_, werr := f.Write(line)
	return storage(errors.Join(werr, f.Close()), op, slog.String("path", path))
}

func storage(err error, op string, attrs ...slog.Attr) error {
	if err == nil {
		return nil
	}
	return errs.Wrap(err, errs.CodeStorageUnavailable, op, attrs...)
}
