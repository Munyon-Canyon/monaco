package fakes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

const (
	ablyFixture     = "/ably/channels/message_ok"
	AblyMessagesRun = "/ably/channels/messages"
)

type AblyPublish struct {
	Channel string
	Name    string
	Data    any
}

type ablyMessage struct {
	Name     string `json:"name"`
	Data     any    `json:"data"`
	Encoding string `json:"encoding"`
}

func (m ablyMessage) decoded() (any, error) {
	if m.Encoding != "json" {
		return m.Data, nil
	}
	raw, ok := m.Data.(string)
	if !ok {
		return nil, fieldError("data")
	}
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fieldError("data")
	}
	return data, nil
}

func foldedAblyRoute(upstream string, r *http.Request) (string, []string, bool) {
	folded := strings.HasPrefix(r.URL.Path, "/channels/") && strings.HasSuffix(r.URL.Path, "/messages")
	if upstream != "ably" || !folded {
		return "", nil, false
	}
	return AblyMessagesRun, []string{AblyMessagesRun}, true
}

func (s *Server) ablyPublish(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
		http.Error(w, fieldError("authorization").Error(), http.StatusUnauthorized)
		return
	}
	var body json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	messages := []ablyMessage{}
	if err := json.Unmarshal(body, &messages); err != nil {
		var one ablyMessage
		if err := json.Unmarshal(body, &one); err != nil {
			http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
			return
		}
		messages = append(messages, one)
	}
	published := make([]AblyPublish, len(messages))
	for i, m := range messages {
		data, err := m.decoded()
		if err != nil || m.Name == "" {
			http.Error(w, fieldError("message").Error(), http.StatusBadRequest)
			return
		}
		published[i] = AblyPublish{Channel: r.PathValue("name"), Name: m.Name, Data: data}
	}
	s.mu.Lock()
	s.ably = append(s.ably, published...)
	s.mu.Unlock()
	s.serveFixture(w, []string{ablyFixture})
}

type AblyExpect struct {
	Channel string `json:"channel"`
	Event   string `json:"event"`
	Count   int    `json:"count"`
}

func (s *Server) ablyExpect(w http.ResponseWriter, r *http.Request) {
	var want AblyExpect
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&want); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	got := 0
	for _, p := range s.AblyPublishes() {
		if p.Channel == want.Channel && p.Name == want.Event {
			got++
		}
	}
	if got != want.Count {
		msg := "ably saw " + strconv.Itoa(got) + " " + want.Event + " on " + want.Channel
		http.Error(w, msg+", want "+strconv.Itoa(want.Count), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) AblyPublishes() []AblyPublish {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AblyPublish(nil), s.ably...)
}

func foldedRoute(upstream string, r *http.Request) (string, []string, bool) {
	for _, fold := range []func(string, *http.Request) (string, []string, bool){
		foldedStorageRoute, foldedCoinGeckoRoute, foldedAblyRoute,
	} {
		if route, keys, ok := fold(upstream, r); ok {
			return route, keys, true
		}
	}
	return "", nil, false
}
