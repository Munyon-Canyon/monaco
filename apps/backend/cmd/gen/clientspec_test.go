package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientSpecDropsOnlyAdditionalPropertiesFalse(t *testing.T) {
	t.Parallel()
	in := "openapi: 3.1.0\nschemas:\n  A:\n    type: object\n    additionalProperties: false\n    required: [x]\n" +
		"  B: {type: object, additionalProperties: false, required: [y]}\n" +
		"  C: {type: object, required: [z], additionalProperties: false}\n" +
		"  D:\n    additionalProperties: true\n"
	want := clientSpecHeader + "openapi: 3.1.0\nschemas:\n  A:\n    type: object\n    required: [x]\n" +
		"  B: {type: object, required: [y]}\n" +
		"  C: {type: object, required: [z]}\n" +
		"  D:\n    additionalProperties: true\n"
	if got := string(clientSpec([]byte(in))); got != want {
		t.Fatalf("clientSpec =\n%s\nwant\n%s", got, want)
	}
}

func TestCommittedClientSpecIsTheBundleWithoutStrictSchemas(t *testing.T) {
	t.Parallel()
	bundle, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../" + clientSpecOut)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "additionalProperties: false") {
		t.Fatal("client openapi.yaml still has additionalProperties: false")
	}
	if string(got) != string(clientSpec(bundle)) {
		t.Fatal("client openapi.yaml differs from api/openapi.yaml; run go generate ./...")
	}
}

func TestGenClientSpecWritesARegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	in, out := dir+"/in.yaml", dir+"/out.yaml"
	if err := os.WriteFile(in, []byte("a:\n  additionalProperties: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := genClientSpec(in, out); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Clean(out)); string(got) != clientSpecHeader+"a:\n" {
		t.Fatalf("out = %q", got)
	}
	for name, args := range map[string][2]string{
		"missing input":   {dir + "/nope", out},
		"missing out dir": {in, dir + "/nope/out.yaml"},
		"out is a dir":    {in, dir},
	} {
		if genClientSpec(args[0], args[1]) == nil {
			t.Fatalf("%s should fail", name)
		}
	}
}
