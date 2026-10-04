package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func EventuallyLog(msg observability.Msg, want map[string]string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		from := 0
		await(s.t, fmt.Sprintf("a %s log line with %v", msg.Name, want), func() (bool, <-chan struct{}) {
			lines, changed := s.app.lines(from)
			from += len(lines)
			line, fields, ok := findLog(lines, msg.Name, want)
			if !ok {
				return false, changed
			}
			for _, key := range msg.Required {
				if _, ok := fields[key]; !ok {
					s.t.Fatalf("scenario: %s log line lacks required attr %q: %s", msg.Name, key, line)
				}
			}
			return true, changed
		})
	}
}

func findLog(lines []string, name string, want map[string]string) (string, map[string]any, bool) {
	for _, line := range lines {
		if fields, ok := logFields(line); ok && fields["msg"] == name && fieldsMatch(fields, want) {
			return line, fields, true
		}
	}
	return "", nil, false
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
