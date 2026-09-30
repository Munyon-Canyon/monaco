package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestToolManifest_everyInvokedBinaryIsInstalled(t *testing.T) {
	root := repoRoot(t)
	dev := toolArray(t, filepath.Join(root, "scripts", "install-dev.sh"), "dev_tools")
	ci := toolArray(t, filepath.Join(root, ".github", "actions", "backend-test-env", "action.yml"), "ci_tools")
	if !sameSet(dev, ci) {
		t.Fatalf("just install and backend-test-env list different tools\ninstall: %s\naction: %s", strings.Join(dev, " "), strings.Join(ci, " "))
	}
	have := map[string]bool{}
	for _, name := range dev {
		have[name] = true
	}
	var missing []string
	for _, bin := range invokedBinaries(t, root) {
		if !have[bin] {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("binaries missing from just install and backend-test-env: %s", strings.Join(missing, " "))
	}
}

func TestShellBins_skipsCasePatterns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "spaced alternation",
			src:  "case $x in\nJustfile | foo) ;;\nesac\n",
		},
		{
			name: "command after the pattern",
			src:  "case $x in\na|b) run-x ;;\nesac\n",
			want: []string{"run-x"},
		},
		{
			name: "command outside any case",
			src:  "run-x\n",
			want: []string{"run-x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shellBins(tc.src, nil)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("shellBins() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestToolManifest_plantedUnknownBinaryFails(t *testing.T) {
	have := map[string]bool{"go": true, "jq": true}
	missing := absent([]string{"not-a-real-monaco-tool"}, have)
	if len(missing) != 1 || missing[0] != "not-a-real-monaco-tool" {
		t.Fatalf("planted tool was accepted: %v", missing)
	}
	if got := absent([]string{"go"}, have); len(got) != 0 {
		t.Fatalf("known tool was rejected: %v", got)
	}
}

func absent(bins []string, have map[string]bool) []string {
	var missing []string
	for _, bin := range bins {
		if !have[bin] {
			missing = append(missing, bin)
		}
	}
	return missing
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func toolArray(t *testing.T, path, name string) []string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?s)` + name + `\s*=\s*\((.*?)\)`)
	m := re.FindStringSubmatch(string(text))
	if m == nil {
		t.Fatalf("%s has no %s array", path, name)
	}
	var tools []string
	for _, field := range strings.Fields(m[1]) {
		tools = append(tools, strings.Trim(field, `"'`))
	}
	sort.Strings(tools)
	return tools
}

func invokedBinaries(t *testing.T, root string) []string {
	t.Helper()
	var srcs []string
	sh, err := filepath.Glob(filepath.Join(root, "scripts", "*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	nested, err := filepath.Glob(filepath.Join(root, "scripts", "*", "*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range append(sh, nested...) {
		// cloud-setup.sh runs only in the Claude Code on the web container and installs every tool it uses.
		if filepath.Base(path) == "cloud-setup.sh" {
			continue
		}
		srcs = append(srcs, mustRead(t, path))
	}
	srcs = append(srcs, justfileShell(mustRead(t, filepath.Join(root, "Justfile"))))
	yml, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range yml {
		srcs = append(srcs, workflowRuns(mustRead(t, path)))
	}
	funcs := map[string]bool{}
	fnRe := regexp.MustCompile(`(?m)(?:^|\s)([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*\)`)
	for _, src := range srcs {
		for _, m := range fnRe.FindAllStringSubmatch(src, -1) {
			funcs[m[1]] = true
		}
	}
	seen := map[string]bool{}
	var bins []string
	for _, src := range srcs {
		for _, bin := range shellBins(src, funcs) {
			if seen[bin] {
				continue
			}
			seen[bin] = true
			bins = append(bins, bin)
		}
	}
	sort.Strings(bins)
	return bins
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func workflowRuns(yml string) string {
	var b strings.Builder
	lines := strings.Split(yml, "\n")
	for i := 0; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "run: |") || trim == "run: |" {
			indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
			for i++; i < len(lines); i++ {
				if strings.TrimSpace(lines[i]) == "" {
					b.WriteByte('\n')
					continue
				}
				got := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
				if got <= indent {
					i--
					break
				}
				b.WriteString(lines[i])
				b.WriteByte('\n')
			}
			continue
		}
		if strings.HasPrefix(trim, "run:") {
			b.WriteString(strings.TrimSpace(strings.TrimPrefix(trim, "run:")))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func justfileShell(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func shellBins(src string, funcs map[string]bool) []string {
	builtin := map[string]bool{}
	for _, w := range strings.Fields(`if then else elif fi for do done while until case esac function select in time ! echo printf cd exit return set export local readonly unset true false test source break continue read trap shift eval wait umask type hash getopts mapfile readarray pushd popd dirs shopt ulimit alias unalias times pwd jobs fg bg fc history caller enable help let declare typeset builtin bash sh`) {
		builtin[w] = true
	}
	var bins []string
	delim := ""
	inSingle := false
	caseDepth := 0
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		if delim != "" {
			if trim == delim {
				delim = ""
			}
			continue
		}
		if inSingle {
			if strings.Contains(line, "'") {
				inSingle = false
			}
			continue
		}
		if i := strings.Index(line, "<<"); i >= 0 && !strings.HasPrefix(strings.TrimSpace(line[i:]), "<<<") {
			rest := strings.Trim(strings.TrimLeft(strings.TrimSpace(line[i+2:]), "-"), `"'`)
			if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
				rest = rest[:sp]
			}
			if rest != "" {
				delim = rest
			}
		}
		if strings.Contains(line, "python3 -c '") && !strings.Contains(line, "'") == false && strings.Count(line, "'") < 2 {
			inSingle = true
		}
		if strings.Contains(line, "-c '") && strings.Count(line, "'") == 1 {
			inSingle = true
		}
		scan := line
		if i := indexComment(scan); i >= 0 {
			scan = scan[:i]
		}
		delta := caseDepthDelta(scan)
		if caseDepth > 0 {
			if cmd, arm := caseArmCommand(scan); arm {
				scan = cmd
			}
		}
		if caseDepth+delta < 0 {
			caseDepth = 0
		} else {
			caseDepth += delta
		}
		scan = strings.ReplaceAll(scan, "&&", " ; ")
		scan = strings.ReplaceAll(scan, "||", " ; ")
		for _, seg := range strings.Split(scan, ";") {
			if i := strings.Index(seg, " | "); i >= 0 {
				for _, part := range strings.Split(seg, " | ") {
					bins = append(bins, commandWord(part, builtin, funcs)...)
				}
				continue
			}
			bins = append(bins, commandWord(seg, builtin, funcs)...)
		}
		if sub := regexp.MustCompile(`\$\(([A-Za-z][A-Za-z0-9+_-]*)`).FindAllStringSubmatch(scan, -1); sub != nil {
			for _, m := range sub {
				if keepBin(m[1], builtin, funcs) {
					bins = append(bins, m[1])
				}
			}
		}
	}
	return bins
}

func caseDepthDelta(line string) int {
	words := shellWords(line)
	delta := 0
	for i := 0; i < len(words); i++ {
		switch words[i] {
		case "esac":
			delta--
		case "case":
			for _, next := range words[i+1:] {
				if next == "in" {
					delta++
					break
				}
			}
		}
	}
	return delta
}

func caseArmCommand(line string) (string, bool) {
	inSingle, inDouble := false, false
	paren := 0
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			if c == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
		case c == '\\' && i+1 < len(line):
			i++
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '(':
			paren++
		case c == ')' && paren > 0:
			paren--
		case c == ')':
			rest := strings.TrimSpace(line[i+1:])
			rest = strings.TrimSpace(strings.TrimSuffix(rest, ";;"))
			return rest, true
		}
	}
	return "", false
}

func shellWords(line string) []string {
	var words []string
	var b strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if b.Len() == 0 {
			return
		}
		words = append(words, b.String())
		b.Reset()
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			if c == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == ' ' || c == '\t' || c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			flush()
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return words
}

func commandWord(seg string, builtin, funcs map[string]bool) []string {
	fields := strings.Fields(seg)
	i := 0
	for i < len(fields) && (isAssign(fields[i]) || strings.HasPrefix(fields[i], "-") || fields[i] == "sudo" || fields[i] == "command" || fields[i] == "exec" || fields[i] == "env" || fields[i] == "time" || fields[i] == "nohup" || fields[i] == "nice" || fields[i] == "timeout" || fields[i] == "xargs") {
		i++
	}
	if i >= len(fields) {
		return nil
	}
	word := strings.Trim(fields[i], `"'`+"`")
	if strings.Contains(word, "/") || strings.HasPrefix(word, ".") {
		return nil
	}
	if !keepBin(word, builtin, funcs) {
		return nil
	}
	return []string{word}
}

func keepBin(word string, builtin, funcs map[string]bool) bool {
	if !binName(word) || builtin[word] || funcs[word] || allCaps(word) || strings.Contains(word, ".") || len(word) < 2 {
		return false
	}
	switch word {
	case "api", "bad", "build-for-testing", "cannot", "clean", "closes", "command", "compose", "continuing", "create", "delta", "depth", "error", "full", "get", "has", "import", "inside", "issue", "lines", "list", "must", "next", "no", "not", "number", "or", "pr", "print", "re-slimming", "rev-parse", "run", "scripts", "see", "skipping", "test-without-building", "the", "ubuntu-latest", "version", "qa-report", "ios-sim-clipboard-bridge", "Log", "Nightly", "Print", "Result", "Seconds", "Step", "Ctrl+C":
		return false
	}
	return true
}

func binName(word string) bool {
	return regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+._-]*$`).MatchString(word)
}

func allCaps(word string) bool {
	return regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(word)
}

func isAssign(w string) bool {
	if strings.HasPrefix(w, "-") {
		return false
	}
	i := strings.IndexByte(w, '=')
	if i <= 0 {
		return false
	}
	for _, c := range w[:i] {
		if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func indexComment(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return i
		}
	}
	return -1
}
