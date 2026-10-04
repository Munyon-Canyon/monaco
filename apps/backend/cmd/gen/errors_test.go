package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func writeSwift(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ErrorCodeCases.gen.swift")
	if err := os.WriteFile(path, []byte("@testable import MonacoAPI\n// stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readSpec(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	return dir.ReadFile(filepath.Base(path))
}

func TestGenErrors_replacesTheSwiftCaseListWithOneLinePerCode(t *testing.T) {
	t.Parallel()
	path := writeSwift(t)
	if err := genErrors(path); err != nil {
		t.Fatal(err)
	}
	got, err := readSpec(t, path)
	if err != nil {
		t.Fatal(err)
	}
	var cases strings.Builder
	for _, code := range errs.All() {
		cases.WriteString("        case ." + swiftCaseName(code) + ": true\n")
	}
	want := "@testable import MonacoAPI\n\nextension Components.Schemas.ErrorCode {\n    var isListed: Bool {\n" +
		"        switch self {\n" + cases.String() + "        }\n    }\n}\n"
	if string(got) != want {
		t.Fatalf("swift =\n%s\nwant\n%s", got, want)
	}
	for _, want := range []string{
		"\n        case ._internal: true\n", "\n        case .idempotencyInFlight: true\n", "\n        case .xNotLinked: true\n",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("swift list lacks %q", want)
		}
	}
	for _, line := range strings.Split(string(got), "\n") {
		if len(line) > 120 {
			t.Errorf("line over 120 columns: %q", line)
		}
	}
}

func TestGenErrors_isIdempotent(t *testing.T) {
	t.Parallel()
	path := writeSwift(t)
	var lists [2][]byte
	for i := range lists {
		if err := genErrors(path); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		var err error
		if lists[i], err = readSpec(t, path); err != nil {
			t.Fatal(err)
		}
	}
	if first, second := lists[0], lists[1]; !bytes.Equal(first, second) {
		t.Fatalf("second run changed the swift list:\n%s\nvs\n%s", first, second)
	}
}

func TestGenErrors_reportsWhatItCannotWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := genErrors(filepath.Join(dir, "gone", "c.swift")); err == nil ||
		!strings.Contains(err.Error(), "gen.writeSwiftCases: invalid_input") {
		t.Errorf("missing dir: genErrors = %v", err)
	}
	if err := genErrors(dir); err == nil || !strings.Contains(err.Error(), "gen.writeSwiftCases: internal") {
		t.Errorf("path is a directory: genErrors = %v", err)
	}
}

func TestBundleOpenAPI_injectsTheErrorCodeSchema(t *testing.T) {
	t.Parallel()
	dir := writeSpecDir(t, map[string]string{"base.yaml": bundleBase})
	bundle, err := bundleOpenAPI(dir, errs.All())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundle), "\n  # api/spec/error_codes.yaml\n    ErrorCode:\n      type: string\n") ||
		!strings.HasSuffix(string(bundle), "\n        - x_not_linked\n") ||
		strings.Contains(string(bundle), "cmd/gen errors from") {
		t.Fatalf("bundle =\n%s", bundle)
	}
	stale := writeSpecDir(t, map[string]string{
		"base.yaml": bundleBase, "error_codes.yaml": renderErrorCodes(errs.All()),
	})
	if _, err := bundleOpenAPI(stale, errs.All()); err == nil ||
		!strings.Contains(err.Error(), "schema ErrorCode is defined in both error_codes.yaml and error_codes.yaml") {
		t.Fatalf("a spec dir with its own ErrorCode: err = %v", err)
	}
}

func TestCommittedSpecListsEveryErrorCode(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas struct {
				ErrorCode struct {
					Enum []errs.Code `yaml:"enum"`
				} `yaml:"ErrorCode"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	if got := spec.Components.Schemas.ErrorCode.Enum; !slices.Equal(got, errs.All()) {
		t.Fatalf("api/openapi.yaml ErrorCode enum = %v, want errs.All() = %v; run go generate ./...", got, errs.All())
	}
}

func TestCommittedSwiftCaseListMatchesErrs(t *testing.T) {
	t.Parallel()
	got, err := os.ReadFile("../../../../packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != renderSwiftCases(errs.All()) {
		t.Fatal("ErrorCodeCases.gen.swift differs from errs.All(); run go generate ./...")
	}
}
