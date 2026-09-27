package flows

import "strings"

func Markdown(flows []Flow) string {
	var b strings.Builder
	b.WriteString("| # | Flow | Command / trigger | Events | Consumers |\n| --- | --- | --- | --- | --- |\n")
	for _, f := range flows {
		trigger := code(f.Trigger)
		if f.Command != "" {
			trigger = code(f.Command) + " on " + trigger
		}
		cells := []string{f.ID, cell(f.Name), trigger, codes(f.Events), cell(orNone(strings.Join(f.Consumers, ", ")))}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	return b.String()
}

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func code(s string) string { return "`" + cell(s) + "`" }

func codes(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, code(item))
	}
	return orNone(strings.Join(quoted, ", "))
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
