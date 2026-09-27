package flows

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Lookup func(f Flow, value string) bool

func Unchecked(Flow, string) bool { return true }

type Env struct {
	Repo        fs.FS
	BackendDir  string
	Events      Lookup
	Codes       Lookup
	Triggers    Lookup
	Commands    Lookup
	Consumers   Lookup
	Faultpoints Lookup
}

func CheckColumns(flows []Flow, env Env) []Problem {
	var problems []Problem
	firstLine := map[string]int{}
	docs := docAnchors{repo: env.Repo, cache: map[string]map[string]bool{}}
	for _, f := range flows {
		var msgs []string
		if first, dup := firstLine[f.ID]; dup {
			msgs = append(msgs, fmt.Sprintf("id %s already used on line %d", f.ID, first))
		} else {
			firstLine[f.ID] = f.Line
		}
		msgs = append(msgs, lookups(f, env)...)
		if msg := docs.check(f.Doc); msg != "" {
			msgs = append(msgs, msg)
		}
		for _, msg := range msgs {
			problems = append(problems, Problem{Line: f.Line, Msg: msg})
		}
	}
	return problems
}

func lookups(f Flow, env Env) []string {
	var codes, points, commands []string
	for _, o := range f.Outcomes {
		if name, ok := o.CodeName(); ok {
			codes = append(codes, name)
		}
		if point, ok := o.CrashPoint(); ok {
			points = append(points, point)
		}
	}
	if f.Command != "" {
		commands = []string{f.Command}
	}
	moduleDir := func(_ Flow, module string) bool {
		return isDir(env.Repo, path.Join(env.BackendDir, "internal/modules", module))
	}
	var msgs []string
	for _, c := range []struct {
		lookup Lookup
		values []string
		format string
	}{
		{moduleDir, []string{f.Module}, "module %s has no directory under internal/modules"},
		{env.Triggers, []string{f.Trigger}, "trigger %s is not a route, subject or poller"},
		{env.Commands, commands, "command %s is not a type in internal/modules/" + f.Module + "/app"},
		{env.Events, f.Events, "event %s is not in the events registry"},
		{env.Consumers, f.Consumers, "consumer %s is not a registered durable"},
		{env.Codes, codes, "outcome %s is not an errs code name"},
		{env.Faultpoints, points, "outcome crash:%s is not a registered faultpoint"},
	} {
		for _, v := range c.values {
			if !c.lookup(f, v) {
				msgs = append(msgs, fmt.Sprintf(c.format, v))
			}
		}
	}
	return msgs
}

func isDir(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	return err == nil && info.IsDir()
}

type docAnchors struct {
	repo  fs.FS
	cache map[string]map[string]bool
}

func (d docAnchors) check(doc string) string {
	file, anchor, _ := strings.Cut(doc, "#")
	anchors, ok := d.cache[file]
	if !ok {
		body, err := fs.ReadFile(d.repo, file)
		if err != nil {
			return "doc " + file + " does not exist"
		}
		anchors = Anchors(string(body))
		d.cache[file] = anchors
	}
	if anchor != "" && !anchors[anchor] {
		return "doc " + file + " has no heading with anchor #" + anchor
	}
	return ""
}

var (
	atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$`)
	inlineLink = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	htmlTag    = regexp.MustCompile(`<[^>]+>`)
)

func Anchors(markdown string) map[string]bool {
	anchors := map[string]bool{}
	counts := map[string]int{}
	fence := ""
	for line := range strings.SplitSeq(markdown, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			continue
		}
		m := atxHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		slug := Slug(m[1])
		if n := counts[slug]; n > 0 {
			anchors[slug+"-"+strconv.Itoa(n)] = true
		} else {
			anchors[slug] = true
		}
		counts[slug]++
	}
	return anchors
}

func Slug(heading string) string {
	text := htmlTag.ReplaceAllString(inlineLink.ReplaceAllString(heading, "$1"), "")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Mn, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}
