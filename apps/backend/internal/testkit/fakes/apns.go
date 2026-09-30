package fakes

import "net/http"

func (s *Server) apnsPush(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("apns-topic") == "" {
		s.serveFixture(w, []string{"/apns/missing_topic_400"})
		return
	}
	s.serveFixture(w, []string{"/apns/ok"})
}
