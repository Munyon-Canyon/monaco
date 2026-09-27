package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	errorCodeBegin = "# BEGIN GENERATED ErrorCode"
	errorCodeEnd   = "# END GENERATED ErrorCode"
)

func gen(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "errors" {
		_, _ = fmt.Fprintln(stderr, "usage: monacoctl gen errors <openapi.yaml>")
		return 2
	}
	if err := writeErrorCodes(args[1], errs.All()); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %d error codes to %s\n", len(errs.All()), args[1])
	return 0
}

func writeErrorCodes(path string, codes []errs.Code) error {
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, "monacoctl.writeErrorCodes")
	}
	defer func() { _ = dir.Close() }()
	spec, err := dir.ReadFile(filepath.Base(path))
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, "monacoctl.writeErrorCodes")
	}
	out, err := spliceErrorCodes(string(spec), codes)
	if err != nil {
		return err
	}
	if err := dir.WriteFile(filepath.Base(path), []byte(out), 0o600); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "monacoctl.writeErrorCodes")
	}
	return nil
}

func spliceErrorCodes(spec string, codes []errs.Code) (string, error) {
	lines := strings.Split(spec, "\n")
	begin, end := markerLine(lines, errorCodeBegin), markerLine(lines, errorCodeEnd)
	if begin < 0 || end < begin {
		return "", errs.New(errs.CodeInvalidInput, "monacoctl.spliceErrorCodes",
			slog.String("want", "one "+errorCodeBegin+" line followed by one "+errorCodeEnd+" line"))
	}
	indent := lines[begin][:len(lines[begin])-len(strings.TrimLeft(lines[begin], " "))]
	enum := make([]string, len(codes))
	for i, code := range codes {
		enum[i] = indent + "- " + string(code)
	}
	return strings.Join(slices.Concat(lines[:begin+1], enum, lines[end:]), "\n"), nil
}

func markerLine(lines []string, marker string) int {
	found := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != marker {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = i
	}
	return found
}
