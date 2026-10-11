package multichain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

//go:embed testdata/*.json
var recordings embed.FS

type Reply[T any] struct {
	Value T         `json:"value"`
	Code  errs.Code `json:"error,omitempty"`
}

func (r Reply[T]) err(op string) error {
	if r.Code == "" {
		return nil
	}
	return errs.New(r.Code, op)
}

type Script[T any] struct {
	mu       sync.Mutex
	queue    []Reply[T]
	recorded Reply[T]
	calls    int
}

func recorded[T any](name string) *Script[T] {
	raw, err := recordings.ReadFile("testdata/" + name + ".json")
	if err != nil {
		panic(fmt.Sprintf("multichain fakes: recording %s: %v", name, err))
	}
	s := &Script[T]{}
	if err := json.Unmarshal(raw, &s.recorded); err != nil {
		panic(fmt.Sprintf("multichain fakes: recording %s: %v", name, err))
	}
	return s
}

func (s *Script[T]) Push(replies ...Reply[T]) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, replies...)
}

func (s *Script[T]) PushJSON(raw []byte) error {
	var r Reply[T]
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("multichain fakes: decode reply: %w", err)
	}
	s.Push(r)
	return nil
}

func (s *Script[T]) NextJSON() ([]byte, error) {
	raw, err := json.Marshal(s.next())
	if err != nil {
		return nil, fmt.Errorf("multichain fakes: encode reply: %w", err)
	}
	return raw, nil
}

func (s *Script[T]) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *Script[T]) next() Reply[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if len(s.queue) == 0 {
		return s.recorded
	}
	r := s.queue[0]
	s.queue = s.queue[1:]
	return r
}
