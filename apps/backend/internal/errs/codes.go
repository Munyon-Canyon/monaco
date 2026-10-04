package errs

import (
	"iter"
	"maps"
	"reflect"
	"slices"
)

type Code string

type Row struct {
	Name      string
	Kind      Kind
	Retryable bool
	Alert     bool
	Message   string
}

type codeFiles struct{}

func codeFileRows() iter.Seq[map[Code]Row] {
	return func(yield func(map[Code]Row) bool) {
		files := reflect.ValueOf(codeFiles{})
		for i := range files.NumMethod() {
			rows, ok := files.Method(i).Interface().(func() map[Code]Row)
			if ok && !yield(rows()) {
				return
			}
		}
	}
}

func table() map[Code]Row {
	all := map[Code]Row{}
	for rows := range codeFileRows() {
		maps.Copy(all, rows)
	}
	return all
}

func row(code Code) Row {
	platform := codeFiles{}.Platform()
	if r, ok := platform[code]; ok {
		return r
	}
	for rows := range codeFileRows() {
		if r, ok := rows[code]; ok {
			return r
		}
	}
	return platform[CodeInternal]
}

func Name(code Code) string { return row(code).Name }

func KindOf(code Code) Kind { return row(code).Kind }

func Retryable(code Code) bool { return row(code).Retryable }

func Alert(code Code) bool { return row(code).Alert }

func Message(code Code) string { return row(code).Message }

func All() []Code { return slices.Sorted(maps.Keys(table())) }
