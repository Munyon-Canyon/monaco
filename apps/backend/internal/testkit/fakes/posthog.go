package fakes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	posthogFixture = "/posthog/batch_ok"
	posthogSetKey  = "$set"
)

type PostHogCapture struct {
	APIKey     string
	UUID       uuid.UUID
	Event      string
	DistinctID string
	Timestamp  time.Time
	Properties map[string]any
	Set        map[string]any
}

type posthogRequest struct {
	APIKey string `json:"api_key"`
	Batch  []struct {
		UUID       uuid.UUID      `json:"uuid"`
		Event      string         `json:"event"`
		DistinctID string         `json:"distinct_id"`
		Timestamp  time.Time      `json:"timestamp"`
		Properties map[string]any `json:"properties"`
	} `json:"batch"`
}

func (r posthogRequest) captures() ([]PostHogCapture, error) {
	if len(r.Batch) == 0 {
		return nil, fieldError("batch")
	}
	out := make([]PostHogCapture, len(r.Batch))
	for i, e := range r.Batch {
		switch {
		case e.UUID == uuid.Nil:
			return nil, fieldError("uuid")
		case e.Event == "":
			return nil, fieldError("event")
		case e.DistinctID == "":
			return nil, fieldError("distinct_id")
		case e.Timestamp.IsZero():
			return nil, fieldError("timestamp")
		}
		raw, present := e.Properties[posthogSetKey]
		set, isObject := raw.(map[string]any)
		if present && !isObject {
			return nil, fieldError(posthogSetKey)
		}
		delete(e.Properties, posthogSetKey)
		out[i] = PostHogCapture{
			APIKey: r.APIKey, UUID: e.UUID, Event: e.Event, DistinctID: e.DistinctID, Timestamp: e.Timestamp,
			Properties: e.Properties, Set: set,
		}
	}
	return out, nil
}

func (s *Server) posthogBatch(w http.ResponseWriter, r *http.Request) {
	var req posthogRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.APIKey == "" {
		http.Error(w, fieldError("api_key").Error(), http.StatusUnauthorized)
		return
	}
	captures, err := req.captures()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.posthog = append(s.posthog, captures...)
	s.mu.Unlock()
	s.serveFixture(w, []string{posthogFixture})
}

func (s *Server) PostHogCaptures() []PostHogCapture {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[uuid.UUID]bool{}
	var out []PostHogCapture
	for _, c := range s.posthog {
		if !seen[c.UUID] {
			seen[c.UUID] = true
			out = append(out, c)
		}
	}
	return out
}

func (s *Server) PostHogReceived() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posthog)
}
