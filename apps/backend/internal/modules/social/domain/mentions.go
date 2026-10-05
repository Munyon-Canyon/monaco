package domain

import (
	"regexp"
	"strings"
)

const MaxMentions = 20

var greedyMentionRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9_@])@([A-Za-z0-9_]+)`)

func ParseMentions(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range greedyMentionRE.FindAllStringSubmatchIndex(body, -1) {
		name := strings.ToLower(body[m[2]:m[3]])
		if len(name) < 3 || len(name) > 20 || seen[name] || inURL(body, m[2]) {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if len(out) == MaxMentions {
			break
		}
	}
	return out
}

func inURL(body string, at int) bool {
	start := strings.LastIndexAny(body[:at], " \t\r\n") + 1
	return strings.Contains(body[start:at], "://")
}
