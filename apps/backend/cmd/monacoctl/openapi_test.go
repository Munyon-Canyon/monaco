package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAPILint_RequiresAdminRole(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	if err := os.Mkdir(filepath.Join(wd, "api"), 0o750); err != nil {
		t.Fatal(err)
	}
	spec := []byte("openapi: 3.1.0\ninfo: {title: test, version: '1'}\npaths:\n" +
		"  /v1/admin/planted:\n    get:\n      responses: {'200': {description: ok}}\n")
	if err := os.WriteFile(filepath.Join(wd, "api", "openapi.yaml"), spec, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := toolOpenapi(toolEnv{wd: wd})([]string{"lint"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "GET /v1/admin/planted") {
		t.Fatalf("lint = %d, %q", code, stderr.String())
	}
}

func TestOpenAPILint_UsageReadAndValidSpec(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	lint := toolOpenapi(toolEnv{wd: wd})
	var stdout, stderr bytes.Buffer
	if code := lint(nil, &stdout, &stderr); code != 2 || stderr.String() != "usage: monacoctl openapi lint\n" {
		t.Fatalf("usage = %d, %q", code, stderr.String())
	}
	stderr.Reset()
	if code := lint(
		[]string{"lint"},
		&stdout,
		&stderr,
	); code != 1 ||
		!strings.Contains(stderr.String(), "openapi lint:") {
		t.Fatalf("read = %d, %q", code, stderr.String())
	}
	if err := os.Mkdir(filepath.Join(wd, "api"), 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(wd, "api", "openapi.yaml")
	if err := os.WriteFile(path, []byte("not: [yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := lint(
		[]string{"lint"},
		&stdout,
		&stderr,
	); code != 1 ||
		!strings.Contains(stderr.String(), "openapi lint:") {
		t.Fatalf("malformed = %d, %q", code, stderr.String())
	}
	if err := os.WriteFile(
		path,
		[]byte("openapi: 3.1.0\ninfo: {title: test, version: '1'}\npaths:\n"+
			"  /v1/me:\n    get:\n      responses: {'200': {description: ok}}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := lint([]string{"lint"}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("valid = %d, %q", code, stderr.String())
	}
}
