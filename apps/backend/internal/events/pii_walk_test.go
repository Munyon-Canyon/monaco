package events_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type personalWalk struct {
	personal  []string
	pkgs      map[string]struct{}
	cross     bool
	reachable map[string]bool
	seenOpen  map[reflect.Type]bool
	seenShut  map[reflect.Type]bool
	seenLine  map[string]bool
	lines     []string
}

func personalTagViolations(root reflect.Type, personal []string, pkgPaths ...string) []string {
	w := &personalWalk{
		personal:  personal,
		pkgs:      make(map[string]struct{}, len(pkgPaths)),
		reachable: map[string]bool{},
		seenLine:  map[string]bool{},
	}
	for _, path := range pkgPaths {
		w.pkgs[path] = struct{}{}
	}
	w.walkPass(root, false)
	w.walkPass(root, true)
	return w.lines
}

func (w *personalWalk) walkPass(root reflect.Type, cross bool) {
	w.cross = cross
	w.seenOpen = map[reflect.Type]bool{}
	w.seenShut = map[reflect.Type]bool{}
	w.walk(root, true)
}

func (w *personalWalk) walk(typ reflect.Type, open bool) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch {
	case typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map:
		w.walkNested(typ.Elem(), open)
	case typ.Kind() == reflect.Interface:
		w.walkInterface(typ)
	case typ.Kind() == reflect.Struct:
		w.walkStruct(typ, open)
	}
}

func (w *personalWalk) walkNested(elem reflect.Type, open bool) {
	elem = derefWalk(elem)
	if elem.Kind() == reflect.Struct || elem.Kind() == reflect.Map ||
		elem.Kind() == reflect.Slice || elem.Kind() == reflect.Array {
		w.walk(elem, open)
	}
}

func (w *personalWalk) walkInterface(typ reflect.Type) {
	if !w.cross {
		return
	}
	for i := range typ.NumMethod() {
		m := typ.Method(i).Type
		for n := range m.NumIn() {
			w.walk(m.In(n), false)
		}
		for n := range m.NumOut() {
			w.walk(m.Out(n), false)
		}
	}
}

func (w *personalWalk) walkStruct(typ reflect.Type, open bool) {
	if _, ok := w.pkgs[typ.PkgPath()]; !ok || w.seen(typ, open) {
		return
	}
	fields := make([]reflect.StructField, typ.NumField())
	for i := range fields {
		fields[i] = typ.Field(i)
		w.note(typ, fields[i], open)
	}
	w.walkFields(fields, open, false)
	w.walkFields(fields, open, true)
}

func (w *personalWalk) seen(typ reflect.Type, open bool) bool {
	if open {
		if w.seenOpen[typ] {
			return true
		}
		w.seenOpen[typ] = true
		return false
	}
	if w.seenOpen[typ] || w.seenShut[typ] {
		return true
	}
	w.seenShut[typ] = true
	return false
}

func (w *personalWalk) walkFields(fields []reflect.StructField, open bool, interfaces bool) {
	for _, field := range fields {
		if interfaceType(field.Type) != interfaces {
			continue
		}
		w.walk(field.Type, open)
	}
}

func (w *personalWalk) note(typ reflect.Type, field reflect.StructField, open bool) {
	id := typ.PkgPath() + "." + typ.Name() + "." + field.Name
	if open {
		w.reachable[id] = true
	}
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	tag := field.Tag.Get("pii")
	where := typ.Name() + "." + field.Name
	if slices.Contains(w.personal, name) && tag != "true" {
		w.add(fmt.Sprintf("%s json %q lacks pii:\"true\"", where, name))
	}
	kind := opaqueKind(field.Type)
	if kind != "" && !allowedOpaque(kind, tag) {
		w.add(fmt.Sprintf("%s json %q has pii tag %q", where, name, tag))
	}
	if tag == "keys" && kind != "map" {
		w.add(fmt.Sprintf("%s json %q pii:\"keys\" is not a map", where, name))
	}
	if !open && !w.reachable[id] && (tag == "true" || tag == "keys") {
		w.add(fmt.Sprintf("%s json %q pii:%q is unreachable", where, name, tag))
	}
}

func (w *personalWalk) add(line string) {
	if w.seenLine[line] {
		return
	}
	w.seenLine[line] = true
	w.lines = append(w.lines, line)
}

func derefWalk(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

func interfaceType(typ reflect.Type) bool {
	return derefWalk(typ).Kind() == reflect.Interface
}

func opaqueKind(typ reflect.Type) string {
	typ = derefWalk(typ)
	switch {
	case typ.Kind() == reflect.Interface:
		return "interface"
	case typ.Kind() == reflect.Map:
		return "map"
	case typ == reflect.TypeFor[json.RawMessage]():
		return "raw"
	default:
		return ""
	}
}

func allowedOpaque(kind, tag string) bool {
	switch tag {
	case "true", "false":
		return true
	case "keys":
		return kind == "map"
	default:
		return false
	}
}

type plainAny struct {
	Body any `json:"body"`
}

type plainRaw struct {
	Raw json.RawMessage `json:"raw"`
}

type plainMap struct {
	Labels map[string]string `json:"labels"`
}

type keyedString struct {
	Label string `json:"label" pii:"keys"`
}

type badTagMap struct {
	Labels map[string]string `json:"labels" pii:"yes"`
}

type buriedPersonal struct {
	UserID string `json:"user_id" pii:"true"`
}

type anyBody interface {
	Read() buriedPersonal
}

type throughAny struct {
	Body anyBody `json:"body" pii:"false"`
}

type shapedMember struct {
	UserID string `json:"user_id" pii:"true"`
	Role   string `json:"role"`
}

type shapedEvent struct {
	UserID  string                             `json:"user_id"  pii:"true"`
	Note    string                             `json:"note"`
	Body    any                                `json:"body"     pii:"false"`
	Raw     json.RawMessage                    `json:"raw"      pii:"true"`
	Labels  map[string]string                  `json:"labels"   pii:"false"`
	Keyed   map[string]shapedMember            `json:"keyed"    pii:"keys"`
	Items   map[string]*shapedMember           `json:"items"    pii:"false"`
	Members []shapedMember                     `json:"members"`
	Ptrs    []*shapedMember                    `json:"ptrs"`
	Fixed   [1]shapedMember                    `json:"fixed"`
	Nested  [][]shapedMember                   `json:"nested"`
	Who     *shapedMember                      `json:"who"`
	BySlice map[string][]shapedMember          `json:"by_slice" pii:"false"`
	ByMap   map[string]map[string]shapedMember `json:"by_map"   pii:"false"`
	ByArray map[string][1]shapedMember         `json:"by_array" pii:"false"`
}

type bareVoter struct {
	UserID string `json:"user_id"`
}

type votersBySlice struct {
	Votes map[string][]bareVoter `json:"votes" pii:"false"`
}

type votersByMap struct {
	Votes map[string]map[string]bareVoter `json:"votes" pii:"false"`
}

type votersByArray struct {
	Votes map[string][2]bareVoter `json:"votes" pii:"false"`
}

func walkPkg() string {
	return reflect.TypeFor[plainAny]().PkgPath()
}

func oneLine(t *testing.T, root reflect.Type, personal []string, want string) {
	t.Helper()
	lines := personalTagViolations(root, personal, walkPkg())
	if len(lines) != 1 || !strings.Contains(lines[0], want) {
		t.Fatalf("lines = %#v, want one line containing %q", lines, want)
	}
}

func TestPersonalTagViolations_flagsUntaggedOpaqueFields(t *testing.T) {
	t.Parallel()
	oneLine(t, reflect.TypeFor[plainAny](), nil, `"body"`)
	oneLine(t, reflect.TypeFor[plainRaw](), nil, `"raw"`)
	oneLine(t, reflect.TypeFor[plainMap](), nil, `"labels"`)
}

func TestPersonalTagViolations_flagsPIIHiddenBehindAnInterface(t *testing.T) {
	t.Parallel()
	oneLine(t, reflect.TypeFor[throughAny](), []string{"user_id"}, `pii:"true" is unreachable`)
}

func TestPersonalTagViolations_flagsKeysOnAString(t *testing.T) {
	t.Parallel()
	oneLine(t, reflect.TypeFor[keyedString](), nil, `pii:"keys" is not a map`)
}

func TestPersonalTagViolations_flagsAnUnknownOpaqueTag(t *testing.T) {
	t.Parallel()
	oneLine(t, reflect.TypeFor[badTagMap](), nil, `has pii tag "yes"`)
}

func TestPersonalTagViolations_flagsAnUntaggedNameInsideAMapOfContainers(t *testing.T) {
	t.Parallel()
	personal := []string{"user_id"}
	t.Run("slice", func(t *testing.T) {
		t.Parallel()
		oneLine(t, reflect.TypeFor[votersBySlice](), personal, `json "user_id" lacks pii:"true"`)
	})
	t.Run("map", func(t *testing.T) {
		t.Parallel()
		oneLine(t, reflect.TypeFor[votersByMap](), personal, `json "user_id" lacks pii:"true"`)
	})
	t.Run("array", func(t *testing.T) {
		t.Parallel()
		oneLine(t, reflect.TypeFor[votersByArray](), personal, `json "user_id" lacks pii:"true"`)
	})
}

func TestPersonalTagViolations_acceptsEveryTaggedShape(t *testing.T) {
	t.Parallel()
	personal := []string{
		"user_id", "voter_ids", "handle", "email", "phone", "wallet_address", "address",
		"creator_id", "actor_id", "proposer_id",
	}
	lines := personalTagViolations(reflect.TypeFor[shapedEvent](), personal, walkPkg())
	if len(lines) != 0 {
		t.Fatalf("lines = %#v, want none", lines)
	}
}
