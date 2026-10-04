package scenario

func SubscribeCore(subject string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		if s.app.coreSubscribe == nil {
			return
		}
		s.core[subject] = s.app.coreSubscribe(s.t, subject)
	}
}

func ExpectCore(subject string, check func([]byte) error) Step {
	return func(s *Scenario) {
		s.t.Helper()
		messages, ok := s.core[subject]
		if !ok {
			if s.app.coreSubscribe == nil {
				return
			}
			s.t.Fatalf("scenario: ExpectCore(%s) needs SubscribeCore(%s) first", subject, subject)
		}
		select {
		case data, ok := <-messages:
			if !ok {
				s.t.Fatalf("scenario: core subscription for %s closed before a message arrived", subject)
			}
			if err := check(data); err != nil {
				s.t.Fatalf("scenario: core %s: %v", subject, err)
			}
		case <-s.t.Context().Done():
			s.t.Fatalf("scenario: core %s: %v", subject, s.t.Context().Err())
		}
	}
}
