package errs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func declaredCodes(t *testing.T, path string) map[string]Code {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	found := map[string]Code{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			if value, ok := spec.(*ast.ValueSpec); ok {
				collectCode(t, value, found)
			}
		}
	}
	return found
}

func collectCode(t *testing.T, spec *ast.ValueSpec, found map[string]Code) {
	t.Helper()
	typ, ok := spec.Type.(*ast.Ident)
	if !ok || typ.Name != "Code" {
		return
	}
	for i, name := range spec.Names {
		lit, ok := spec.Values[i].(*ast.BasicLit)
		if !ok {
			t.Fatalf("%s is not a string literal", name.Name)
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", name.Name, err)
		}
		found[name.Name] = Code(value)
	}
}

func snakeCase(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && startsWord(runes, i) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func startsWord(runes []rune, i int) bool {
	afterLower := unicode.IsLower(runes[i-1])
	endsAcronym := i+1 < len(runes) && unicode.IsLower(runes[i+1])
	return afterLower || endsAcronym
}

func TestEveryDeclaredCodeHasATableRowAndEveryRowHasACode(t *testing.T) {
	t.Parallel()
	declared := declaredCodes(t, "codes.go")
	if len(declared) == 0 {
		t.Fatal("no Code constants found in codes.go")
	}
	rows := table()
	for ident, code := range declared {
		if _, ok := rows[code]; !ok {
			t.Errorf("%s (%q) has no table row", ident, code)
		}
	}
	values := slices.Collect(maps.Values(declared))
	for code := range rows {
		if !slices.Contains(values, code) {
			t.Errorf("table row %q has no Code constant in codes.go", code)
		}
	}
}

func TestRowNameMatchesIdentifierAndWireValueIsItsSnakeCase(t *testing.T) {
	t.Parallel()
	for ident, code := range declaredCodes(t, "codes.go") {
		name := strings.TrimPrefix(ident, "Code")
		if got := Name(code); got != name {
			t.Errorf("%s: Name = %q, want %q", ident, got, name)
		}
		if want := snakeCase(name); string(code) != want {
			t.Errorf("%s: wire value = %q, want %q", ident, code, want)
		}
	}
}

func TestEveryRowHasAKindAndAMessage(t *testing.T) {
	t.Parallel()
	for code, row := range table() {
		if row.Kind < KindInvalid || row.Kind > KindInternal {
			t.Errorf("%q: Kind %d is not a declared Kind", code, row.Kind)
		}
		if row.Message == "" {
			t.Errorf("%q: empty Message", code)
		}
	}
}

func TestAllIsSortedAndListsEveryRow(t *testing.T) {
	t.Parallel()
	all := All()
	want := slices.Sorted(maps.Keys(table()))
	if !slices.Equal(all, want) {
		t.Fatalf("All() = %v, want %v", all, want)
	}
}

func TestAccessorsReadTheRow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code      Code
		kind      Kind
		retryable bool
		alert     bool
	}{
		{CodeInvalidInput, KindInvalid, false, false},
		{CodeClientClosed, KindInvalid, false, false},
		{CodeUnauthorized, KindUnauthorized, false, false},
		{CodeForbidden, KindForbidden, false, false},
		{CodeNotFound, KindNotFound, false, false},
		{CodeIdempotencyMismatch, KindConflict, false, false},
		{CodeIdempotencyInFlight, KindConflict, false, false},
		{CodeVersionConflict, KindConflict, false, false},
		{CodeRateLimited, KindRateLimited, true, false},
		{CodeUpstreamUnavailable, KindUnavailable, true, false},
		{CodeUpstreamTimeout, KindUnavailable, true, false},
		{CodePrivyUnavailable, KindUnavailable, true, false},
		{CodeRPCUnavailable, KindUnavailable, true, false},
		{CodeRelayerUnderfunded, KindUnavailable, false, true},
		{CodeInvalidAddress, KindInvalid, false, false},
		{CodeDBUnavailable, KindUnavailable, true, false},
		{CodeInvalidConfig, KindInternal, false, true},
		{CodeDecodeFailed, KindInternal, false, true},
		{CodeInternal, KindInternal, false, true},
		{CodePanic, KindInternal, false, true},
		{CodeHandleTaken, KindBlocked, false, false},
		{CodeAuthStateTransition, KindInternal, true, false},
		{CodeWalletMismatch, KindInternal, false, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			t.Parallel()
			if got := KindOf(tt.code); got != tt.kind {
				t.Errorf("KindOf = %d, want %d", got, tt.kind)
			}
			if got := Retryable(tt.code); got != tt.retryable {
				t.Errorf("Retryable = %v, want %v", got, tt.retryable)
			}
			if got := Alert(tt.code); got != tt.alert {
				t.Errorf("Alert = %v, want %v", got, tt.alert)
			}
			if got := Message(tt.code); got != table()[tt.code].Message {
				t.Errorf("Message = %q, want the row's message", got)
			}
		})
	}
}

func TestUnknownCodeReadsAsInternal(t *testing.T) {
	t.Parallel()
	unknown := Code("not_in_the_table")
	if got := KindOf(unknown); got != KindInternal {
		t.Errorf("KindOf = %d, want KindInternal", got)
	}
	if !Alert(unknown) {
		t.Error("Alert = false, want true")
	}
	if Retryable(unknown) {
		t.Error("Retryable = true, want false")
	}
	if got, want := Message(unknown), Message(CodeInternal); got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
}
