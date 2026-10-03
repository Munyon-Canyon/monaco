package flows

import (
	"maps"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const SpecPath = "apps/backend/api/openapi.yaml"

func SpecFile(file string) bool {
	return file == SpecPath || strings.HasPrefix(file, "apps/backend/api/spec/")
}

type spec struct {
	root       map[string]any
	ops        map[string]any
	components map[string]any
}

func parseSpec(body []byte) spec {
	s := spec{root: map[string]any{}, ops: map[string]any{}, components: map[string]any{}}
	_ = yaml.Unmarshal(body, &s.root)
	paths, _ := s.root["paths"].(map[string]any)
	for p, v := range paths {
		item, _ := v.(map[string]any)
		for _, m := range []string{"get", "put", "post", "delete", "patch", "head", "options", "trace"} {
			if op, ok := item[m]; ok {
				s.ops[strings.ToUpper(m)+" "+p] = []any{item["parameters"], op}
			}
		}
	}
	kinds, _ := s.root["components"].(map[string]any)
	for kind, v := range kinds {
		named, _ := v.(map[string]any)
		for name, c := range named {
			s.components["#/components/"+kind+"/"+name] = c
		}
	}
	return s
}

func ChangedOperations(base, head []byte) []string {
	before, after := parseSpec(base), parseSpec(head)
	every := !reflect.DeepEqual(before.global(), after.global())
	stale := map[string]bool{}
	for _, ref := range union(before.components, after.components) {
		stale[ref] = !reflect.DeepEqual(before.components[ref], after.components[ref])
	}
	var changed []string
	for _, op := range union(before.ops, after.ops) {
		if every || !reflect.DeepEqual(before.ops[op], after.ops[op]) ||
			before.reaches(before.ops[op], stale, map[string]bool{}) ||
			after.reaches(after.ops[op], stale, map[string]bool{}) {
			changed = append(changed, op)
		}
	}
	return changed
}

func (s spec) global() map[string]any {
	g := maps.Clone(s.root)
	for _, k := range []string{"paths", "components", "info", "tags"} {
		delete(g, k)
	}
	return g
}

func union(a, b map[string]any) []string {
	return slices.Compact(slices.Sorted(maps.Keys(maps.Collect(func(yield func(string, any) bool) {
		for k, v := range a {
			_ = yield(k, v)
		}
		for k, v := range b {
			_ = yield(k, v)
		}
	}))))
}

func (s spec) reaches(node any, stale, seen map[string]bool) bool {
	var children []any
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok && !seen[ref] {
			seen[ref] = true
			if stale[ref] {
				return true
			}
			children = append(children, s.components[ref])
		}
		children = slices.AppendSeq(children, maps.Values(n))
	case []any:
		children = n
	}
	return slices.ContainsFunc(children, func(c any) bool { return s.reaches(c, stale, seen) })
}
