package marketing

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

type kind uint8

const (
	kindString kind = iota + 1
	kindNumber
	kindBool
	kindList
)

type value struct {
	kind kind
	text string
	list []string
	line int
}

type table struct {
	path string
	vals map[string]value
	used map[string]bool
	err  error
}

type doc struct {
	root   *table
	tables map[string]*table
	arrays map[string][]*table
	read   map[string]bool
}

type parser struct {
	src  string
	pos  int
	line int
}

func newTable(path string) *table {
	return &table{path: path, vals: map[string]value{}, used: map[string]bool{}}
}

func parseDoc(src string) (*doc, error) {
	d := &doc{
		root: newTable(""), tables: map[string]*table{}, arrays: map[string][]*table{}, read: map[string]bool{},
	}
	p := &parser{src: src, line: 1}
	cur := d.root
	for {
		p.skipBlank()
		if p.pos >= len(p.src) {
			return d, nil
		}
		var err error
		if p.peek() == '[' {
			cur, err = p.header(d)
		} else {
			err = p.pair(cur)
		}
		if err == nil {
			err = p.endLine()
		}
		if err != nil {
			return nil, err
		}
	}
}

func (p *parser) peek() byte {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *parser) fail(format string, args ...any) error {
	return invalid(fmt.Sprintf("line %d: %s", p.line, fmt.Sprintf(format, args...)))
}

func (p *parser) skipSpace() {
	for p.peek() == ' ' || p.peek() == '\t' || p.peek() == '\r' {
		p.pos++
	}
}

func (p *parser) skipBlank() {
	for p.pos < len(p.src) {
		switch p.peek() {
		case ' ', '\t', '\r':
			p.pos++
		case '\n':
			p.pos++
			p.line++
		case '#':
			for p.pos < len(p.src) && p.peek() != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *parser) endLine() error {
	p.skipSpace()
	switch p.peek() {
	case '#', '\n', 0:
		return nil
	}
	return p.fail("unexpected text after a value")
}

func isBare(c byte) bool {
	return c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func (p *parser) bare() string {
	start := p.pos
	for isBare(p.peek()) {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *parser) header(d *doc) (*table, error) {
	array := strings.HasPrefix(p.src[p.pos:], "[[")
	closing := "]"
	p.pos++
	if array {
		closing = "]]"
		p.pos++
	}
	name := p.bare()
	if name == "" || !strings.HasPrefix(p.src[p.pos:], closing) {
		return nil, p.fail("a table header is [name] or [[name]]")
	}
	p.pos += len(closing)
	t := newTable(name)
	if array {
		d.arrays[name] = append(d.arrays[name], t)
		return t, nil
	}
	if _, dup := d.tables[name]; dup {
		return nil, p.fail("table [%s] appears twice", name)
	}
	d.tables[name] = t
	return t, nil
}

func (p *parser) pair(t *table) error {
	line := p.line
	key := p.bare()
	if key == "" {
		return p.fail("expected a key")
	}
	p.skipSpace()
	if p.peek() != '=' {
		return p.fail("expected = after %s", key)
	}
	p.pos++
	p.skipSpace()
	v, err := p.value()
	if err != nil {
		return err
	}
	if _, dup := t.vals[key]; dup {
		return p.fail("key %s appears twice", key)
	}
	v.line = line
	t.vals[key] = v
	return nil
}

func (p *parser) value() (value, error) {
	switch c := p.peek(); {
	case c == '"':
		s, err := p.basic()
		return value{kind: kindString, text: s}, err
	case c == '\'':
		s, err := p.literal()
		return value{kind: kindString, text: s}, err
	case c == '[':
		list, err := p.array()
		return value{kind: kindList, list: list}, err
	case c == '+' || c == '-' || c >= '0' && c <= '9':
		return value{kind: kindNumber, text: p.word()}, nil
	case c >= 'a' && c <= 'z':
		w := p.word()
		if w != "true" && w != "false" {
			return value{}, p.fail("expected a value, got %s", w)
		}
		return value{kind: kindBool, text: w}, nil
	}
	return value{}, p.fail("expected a value")
}

func (p *parser) word() string {
	start := p.pos
	for p.pos < len(p.src) && !strings.ContainsRune(" \t\r\n,]#", rune(p.src[p.pos])) {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *parser) basic() (string, error) {
	start := p.pos
	p.pos++
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '\\':
			p.pos++
		case '\n':
			return "", p.fail("a string ends on its line")
		case '"':
			p.pos++
			s, err := strconv.Unquote(p.src[start:p.pos])
			if err != nil {
				return "", p.fail("a string has a bad escape")
			}
			return s, nil
		}
		p.pos++
	}
	return "", p.fail("unterminated string")
}

func (p *parser) literal() (string, error) {
	rest := p.src[p.pos+1:]
	end := strings.IndexAny(rest, "'\n")
	if end < 0 || rest[end] == '\n' {
		return "", p.fail("unterminated string")
	}
	p.pos += end + 2
	return rest[:end], nil
}

func (p *parser) array() ([]string, error) {
	p.pos++
	items := []string{}
	for {
		p.skipBlank()
		switch p.peek() {
		case ']':
			p.pos++
			return items, nil
		case 0:
			return nil, p.fail("unterminated array")
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if v.kind != kindString {
			return nil, p.fail("arrays hold strings only")
		}
		items = append(items, v.text)
		p.skipBlank()
		switch p.peek() {
		case ',':
			p.pos++
		case ']':
		case 0:
			return nil, p.fail("unterminated array")
		default:
			return nil, p.fail("expected , or ] in an array")
		}
	}
}

func (d *doc) table(name string) *table {
	d.read[name] = true
	if t, ok := d.tables[name]; ok {
		return t
	}
	return newTable(name)
}

func (d *doc) list(name string) []*table {
	d.read[name] = true
	return d.arrays[name]
}

func (d *doc) unread() error {
	names := slices.Sorted(maps.Keys(d.tables))
	names = append(names, slices.Sorted(maps.Keys(d.arrays))...)
	for _, name := range names {
		if !d.read[name] {
			return invalid(fmt.Sprintf("unknown table %q", name))
		}
	}
	return nil
}

func (t *table) take(key string) (value, bool) {
	t.used[key] = true
	v, ok := t.vals[key]
	return v, ok
}

func (t *table) fail(key string, v value, want string) {
	if t.err == nil {
		t.err = invalid(fmt.Sprintf("line %d: %s must be %s", v.line, t.qualify(key), want))
	}
}

func (t *table) qualify(key string) string {
	if t.path == "" {
		return key
	}
	return t.path + "." + key
}

func (t *table) done() error {
	if t.err != nil {
		return t.err
	}
	for _, key := range slices.Sorted(maps.Keys(t.vals)) {
		if !t.used[key] {
			return invalid(fmt.Sprintf("line %d: unknown key %q", t.vals[key].line, t.qualify(key)))
		}
	}
	return nil
}

func (t *table) str(key string, into *string) {
	v, ok := t.take(key)
	switch {
	case !ok:
	case v.kind != kindString:
		t.fail(key, v, "a string")
	default:
		*into = v.text
	}
}

func (t *table) strings(key string, into *[]string) {
	v, ok := t.take(key)
	switch {
	case !ok:
	case v.kind != kindList:
		t.fail(key, v, "an array of strings")
	default:
		*into = v.list
	}
}

func (t *table) boolean(key string, into *bool) {
	v, ok := t.take(key)
	switch {
	case !ok:
	case v.kind != kindBool:
		t.fail(key, v, "true or false")
	default:
		*into = v.text == "true"
	}
}

func (t *table) integer(key string, into *int) {
	v, ok := t.take(key)
	if !ok {
		return
	}
	n, err := strconv.Atoi(strings.ReplaceAll(v.text, "_", ""))
	if v.kind != kindNumber || err != nil {
		t.fail(key, v, "a whole number")
		return
	}
	*into = n
}

func (t *table) float(key string, into *float64) {
	v, ok := t.take(key)
	if !ok {
		return
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(v.text, "_", ""), 64)
	if v.kind != kindNumber || err != nil {
		t.fail(key, v, "a number")
		return
	}
	*into = n
}
