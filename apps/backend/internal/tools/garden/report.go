package garden

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func (r Report) Markdown() string {
	var b strings.Builder
	b.WriteString("# Garden report\n\n| Kind | Findings |\n| --- | --- |\n")
	for _, s := range r.Sections {
		_, _ = fmt.Fprintf(&b, "| %s | %s |\n", s.Kind, s.count())
	}
	for _, s := range r.Sections {
		s.write(&b)
	}
	return b.String()
}

func (s Section) count() string {
	switch {
	case s.Skipped:
		return "skipped"
	case s.Err != nil:
		return "failed"
	default:
		return strconv.Itoa(len(s.Findings))
	}
}

func (s Section) write(b *strings.Builder) {
	_, _ = fmt.Fprintf(b, "\n## %s (%s)\n\n", s.Kind, s.count())
	switch {
	case s.Skipped:
		b.WriteString("Not run.\n")
		return
	case s.Err != nil:
		_, _ = fmt.Fprintf(b, "The check could not run:\n\n```\n%s\n```\n", s.Err)
		return
	case len(s.Findings) == 0:
		b.WriteString("None.\n")
		return
	}
	findings := slices.SortedFunc(slices.Values(s.Findings), func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Rule, b.Rule), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	sep := ""
	for _, group := range byRule(findings) {
		if rule := group[0].Rule; rule != "" {
			_, _ = fmt.Fprintf(b, "%s### %s (%d)\n\n", sep, rule, len(group))
			sep = "\n"
		}
		for _, f := range group {
			_, _ = fmt.Fprintf(b, "- `%s:%d` %s\n", f.File, f.Line, strings.Join(strings.Fields(f.Text), " "))
		}
	}
}

func byRule(sorted []Finding) [][]Finding {
	var groups [][]Finding
	for start := 0; start < len(sorted); {
		end := start + 1
		for end < len(sorted) && sorted[end].Rule == sorted[start].Rule {
			end++
		}
		groups = append(groups, sorted[start:end])
		start = end
	}
	return groups
}
