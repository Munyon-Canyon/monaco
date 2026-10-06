package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func EventuallyLog(msg observability.Msg, want map[string]string) Step {
	return EventuallyLogs(msg, want, 1)
}

func EventuallyLogs(msg observability.Msg, want map[string]string, n int) Step {
	return func(s *Scenario) {
		s.t.Helper()
		from, seen := 0, 0
		await(s.t, fmt.Sprintf("%d %s log lines with %v", n, msg.Name, want), func() (bool, <-chan struct{}) {
			lines, changed := s.app.lines(from)
			from += len(lines)
			seen += countLogs(s.t, lines, msg, want)
			return seen >= n, changed
		})
	}
}

func countLogs(t T, lines []string, msg observability.Msg, want map[string]string) int {
	t.Helper()
	n := 0
	for _, line := range lines {
		fields, ok := logFields(line)
		if !ok || fields["msg"] != msg.Name || !fieldsMatch(fields, want) {
			continue
		}
		for _, key := range msg.Required {
			if _, ok := fields[key]; !ok {
				t.Fatalf("scenario: %s log line lacks required attr %q: %s", msg.Name, key, line)
			}
		}
		n++
	}
	return n
}

func logFields(line string) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader([]byte(line)))
	dec.UseNumber()
	var fields map[string]any
	return fields, dec.Decode(&fields) == nil
}

func fieldsMatch(fields map[string]any, want map[string]string) bool {
	for path, v := range want {
		var got any = fields
		for key := range strings.SplitSeq(path, ".") {
			m, ok := got.(map[string]any)
			if !ok {
				return false
			}
			got = m[key]
		}
		if got == nil || fmt.Sprint(got) != v {
			return false
		}
	}
	return true
}
