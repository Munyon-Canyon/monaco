package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const swiftLineWidth = 120

func isSwiftKeyword(name string) bool {
	return slices.Contains(strings.Fields(`associatedtype class deinit enum extension fileprivate func import init
		inout internal let open operator private precedencegroup protocol public rethrows static struct subscript
		typealias var break case catch continue default defer do else fallthrough for guard if in repeat return
		throw switch where while as await false is nil self super throws true try`), name)
}

func swiftCaseName(code errs.Code) string {
	words := strings.Split(string(code), "_")
	for i := 1; i < len(words); i++ {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	name := strings.Join(words, "")
	if isSwiftKeyword(name) {
		return "_" + name
	}
	return name
}

func renderSwiftCases(codes []errs.Code) string {
	var b strings.Builder
	b.WriteString("@testable import MonacoAPI\n\n")
	b.WriteString("extension Components.Schemas.ErrorCode {\n")
	b.WriteString("    var isListed: Bool {\n        switch self {\n")
	line := "        case"
	for i, code := range codes {
		item := " ." + swiftCaseName(code)
		if i < len(codes)-1 {
			item += ","
		} else {
			item += ":"
		}
		if len(line)+len(item) > swiftLineWidth {
			b.WriteString(line + "\n")
			line = "            "
			item = strings.TrimPrefix(item, " ")
		}
		line += item
	}
	b.WriteString(line + "\n            true\n        }\n    }\n}\n")
	return b.String()
}

func writeSwiftCases(path string, codes []errs.Code) error {
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, "monacoctl.writeSwiftCases")
	}
	defer func() { _ = dir.Close() }()
	if err := dir.WriteFile(filepath.Base(path), []byte(renderSwiftCases(codes)), 0o600); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "monacoctl.writeSwiftCases")
	}
	return nil
}
