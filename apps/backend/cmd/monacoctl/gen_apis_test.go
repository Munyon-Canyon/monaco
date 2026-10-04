package main

import (
	"bytes"
	"go/format"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	apisBaseImport = "example.com/out"
	apisBase       = `openapi: 3.1.0
info:
  title: T
  version: "1"
paths: {}
components:
  responses:
    Problem:
      description: A problem.
      content:
        application/problem+json:
          schema:
            $ref: "#/components/schemas/Problem"
  schemas:
    Problem:
      type: object
      required: [code]
      properties:
        code:
          $ref: "#/components/schemas/ErrorCode"
`
	apisFunding = `paths:
  /v1/me/balance:
    get:
      operationId: getMyBalance
      tags: [funding]
      responses:
        "200":
          description: The balance.
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Balance"}
        default: {$ref: "#/components/responses/Problem"}
components:
  schemas:
    Balance:
      type: object
      properties:
        micros: {type: string}
`
	apisNotify = `paths:
  /v1/devices:
    delete:
      operationId: deleteDevice
      tags: [notify]
      responses:
        "204": {description: Removed.}
        default: {$ref: "#/components/responses/Problem"}
`
	apisMarket = `components:
  schemas:
    Asset:
      type: object
      properties:
        symbol: {type: string}
`
	apisCabal = `paths:
  /v1/cabals:
    get:
      operationId: getCabals
      tags: [cabal]
      responses:
        "200":
          description: The cabals.
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Cabal"}
components:
  schemas:
    Cabal:
      type: object
      properties:
        name: {type: string}
`
)

func genAPIsLocked(specDir, outDir, baseImport string, codes ...errs.Code) error {
	oapiCodegenState.Lock()
	defer oapiCodegenState.Unlock()
	if codes == nil {
		codes = []errs.Code{errs.CodeNotFound}
	}
	return genAPIs(specDir, outDir, baseImport, codes)
}

func apiSpecDir(t *testing.T, modules map[string]string) string {
	t.Helper()
	files := map[string]string{"base.yaml": apisBase}
	for name, body := range modules {
		files[name] = body
	}
	return writeSpecDir(t, files)
}

func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func requireText(t *testing.T, path string, want, omit []string) string {
	t.Helper()
	text := readText(t, path)
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Fatalf("%s lacks %q:\n%s", path, w, text)
		}
	}
	for _, o := range omit {
		if strings.Contains(text, o) {
			t.Fatalf("%s holds %q:\n%s", path, o, text)
		}
	}
	return text
}

func requireGofmt(t *testing.T, src string) {
	t.Helper()
	if formatted, err := format.Source([]byte(src)); err != nil || !bytes.Equal(formatted, []byte(src)) {
		t.Fatalf("rendered file is not gofmt clean (err %v):\n%s", err, src)
	}
}

func TestGenAPIs_writesOnePackagePerSpecFile(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	specs := apiSpecDir(t, map[string]string{
		"funding.yaml": apisFunding, "cabal.yaml": apisCabal, "notify.yaml": apisNotify, "market.yaml": apisMarket,
	})
	if err := genAPIsLocked(specs, out, apisBaseImport); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"api.gen.go", "apiall/apiall.gen.go",
		"cabalapi/api.gen.go", "cabalapi/mount.gen.go",
		"fundingapi/api.gen.go", "fundingapi/mount.gen.go",
		"marketapi/api.gen.go", "marketapi/mount.gen.go",
		"notifyapi/api.gen.go", "notifyapi/mount.gen.go",
	}
	if got := filesUnder(t, out); !slices.Equal(got, want) {
		t.Fatalf("generated files = %v, want %v", got, want)
	}
	requireText(t, filepath.Join(out, "cabalapi", "api.gen.go"), []string{"package cabalapi", "GetCabals("},
		[]string{"GetMyBalance"})
	requireText(t, filepath.Join(out, "marketapi", "api.gen.go"), []string{"type Asset struct"}, nil)
	requireText(t, filepath.Join(out, "notifyapi", "api.gen.go"), []string{"DeleteDevice("}, nil)
	requireGofmt(t, requireText(t, filepath.Join(out, "fundingapi", "mount.gen.go"), []string{
		"func Mount(ssi StrictServerInterface, m api.Mount, strict ...StrictMiddlewareFunc)",
		`import "` + apisBaseImport + `"`,
	}, nil))
	mounts := make([]string, 0, 8)
	for _, pkg := range []string{"cabalapi", "fundingapi", "marketapi", "notifyapi"} {
		mounts = append(mounts, `"`+apisBaseImport+"/"+pkg+`"`, pkg+".Mount(")
	}
	requireGofmt(t, requireText(t, filepath.Join(out, "apiall", "apiall.gen.go"), mounts, nil))
}

func TestGenAPIs_mapsBaseTypesToTheBasePackage(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	if err := genAPIsLocked(apiSpecDir(t, map[string]string{"funding.yaml": apisFunding}), out,
		apisBaseImport); err != nil {
		t.Fatal(err)
	}
	funding := readText(t, filepath.Join(out, "fundingapi", "api.gen.go"))
	if !strings.Contains(funding, `externalRef0 "`+apisBaseImport+`"`) ||
		!strings.Contains(funding, "externalRef0.Problem") || strings.Contains(funding, "type Problem ") {
		t.Fatalf("fundingapi does not take Problem from the base package:\n%s", funding)
	}
	base := readText(t, filepath.Join(out, "api.gen.go"))
	if !strings.Contains(base, "package api") || !strings.Contains(base, "type Problem struct") ||
		!strings.Contains(base, "type ErrorCode string") || strings.Contains(base, "StrictServerInterface") ||
		strings.Contains(base, "Balance") {
		t.Fatalf("the base package is not models of base.yaml and the error codes only:\n%s", base)
	}
}

func TestGenAPIs_removesThePackageOfADeletedSpecFile(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	specs := apiSpecDir(t, map[string]string{"funding.yaml": apisFunding, "cabal.yaml": apisCabal})
	if err := genAPIsLocked(specs, out, apisBaseImport); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(specs, "funding.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := genAPIsLocked(specs, out, apisBaseImport); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "fundingapi")); !os.IsNotExist(err) {
		t.Fatalf("fundingapi survives its spec file: stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "cabalapi", "api.gen.go")); err != nil {
		t.Fatalf("cabalapi was removed with funding: %v", err)
	}
	if all := readText(t, filepath.Join(out, "apiall", "apiall.gen.go")); strings.Contains(all, "fundingapi") {
		t.Fatalf("apiall still mounts fundingapi:\n%s", all)
	}
}

func TestGenAPIs_refusesACrossModuleRef(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	cabal := strings.Replace(apisCabal, "#/components/schemas/Cabal", "#/components/schemas/Balance", 1)
	err := genAPIsLocked(apiSpecDir(t, map[string]string{"funding.yaml": apisFunding, "cabal.yaml": cabal}),
		out, apisBaseImport)
	if err == nil || !strings.Contains(err.Error(), "cabal.yaml refs schema Balance, which funding.yaml defines") {
		t.Fatalf("genAPIs with a cross-module ref = %v, want the ref named", err)
	}
	if files := filesUnder(t, out); len(files) != 0 {
		t.Fatalf("genAPIs wrote %v before refusing", files)
	}
}

func TestCommittedAPIPackagesMatchTheSpecSources(t *testing.T) {
	t.Parallel()
	const committed = "../../internal/platform/httpx/api"
	out := t.TempDir()
	if err := genAPIsLocked(
		"../../api/spec",
		out,
		backendModule+"/internal/platform/httpx/api",
		errs.All()...); err != nil {
		t.Fatal(err)
	}
	for _, rel := range filesUnder(t, out) {
		if readText(t, filepath.Join(out, rel)) != readText(t, filepath.Join(committed, rel)) {
			t.Errorf("%s is stale: run go generate ./... in apps/backend", rel)
		}
	}
	entries, err := os.ReadDir(committed)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(out, e.Name())); e.IsDir() && os.IsNotExist(err) {
			t.Errorf("%s has no spec file: run go generate ./... in apps/backend", e.Name())
		}
	}
}

type genAPIsFailure struct {
	name     string
	modules  map[string]string
	noBase   bool
	specDirs []string
	outDirs  []string
	outFiles []string
	missing  string
	want     string
}

func (tc genAPIsFailure) arrange(t *testing.T) (specs, out string) {
	t.Helper()
	files := map[string]string{"base.yaml": apisBase, "cabal.yaml": apisCabal}
	if tc.noBase {
		delete(files, "base.yaml")
	}
	maps.Copy(files, tc.modules)
	specs, out = writeSpecDir(t, files), t.TempDir()
	for _, dir := range tc.specDirs {
		mustMkdir(t, filepath.Join(specs, dir))
	}
	for _, dir := range tc.outDirs {
		mustMkdir(t, filepath.Join(out, dir))
	}
	for _, file := range tc.outFiles {
		if err := os.WriteFile(filepath.Join(out, file), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	switch tc.missing {
	case "specs":
		specs = filepath.Join(specs, "nope")
	case "out":
		out = filepath.Join(out, "gone")
	}
	return specs, out
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
}

func TestGenAPIs_reportsWhatItCannotReadOrWrite(t *testing.T) {
	t.Parallel()
	ref := func(to string) string { return strings.Replace(apisCabal, "#/components/schemas/Cabal", to, 1) }
	cases := []genAPIsFailure{
		{name: "a missing spec dir", missing: "specs", want: "nope"},
		{name: "no base.yaml", noBase: true, want: "has no base.yaml"},
		{name: "an unreadable spec file", specDirs: []string{"market.yaml"}, want: "market.yaml"},
		{
			name:    "a spec file that is not YAML",
			modules: map[string]string{"market.yaml": "paths: ["},
			want:    "market.yaml",
		},
		{
			name:    "a base ref nobody defines",
			modules: map[string]string{"base.yaml": strings.Replace(apisBase, "schemas/ErrorCode", "schemas/Gone", 1)},
			want:    "api: ",
		},
		{
			name:    "a module ref to a kind nobody defines",
			modules: map[string]string{"cabal.yaml": ref("#/components/examples/Gone")},
			want:    "cabalapi: ",
		},
		{
			name:    "a ref to a file other than base.yaml",
			modules: map[string]string{"cabal.yaml": ref("other.yaml#/components/schemas/Cabal")},
			want:    "refs other.yaml",
		},
		{
			name:    "an operation oapi-codegen refuses",
			modules: map[string]string{"cabal.yaml": strings.Replace(apisCabal, "/v1/cabals:", "/v1/cabals/{id}:", 1)},
			want:    "oapi-codegen cabalapi",
		},
		{
			name: "a leftover error_codes.yaml next to the injected codes",
			modules: map[string]string{
				"error_codes.yaml": "components:\n  schemas:\n    ErrorCode:\n      type: string\n",
			},
			want: "schema ErrorCode is defined in both",
		},
		{name: "a missing out dir", missing: "out", want: "gone"},
		{name: "a file where a package dir goes", outFiles: []string{"cabalapi"}, want: "cabalapi"},
		{name: "a dir where a generated file goes", outDirs: []string{"api.gen.go"}, want: "api.gen.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			specs, out := tc.arrange(t)
			if err := genAPIsLocked(specs, out, apisBaseImport); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("genAPIs = %v, want an error naming %q", err, tc.want)
			}
		})
	}
}

func TestGenAPIs_commandWritesThePackagesOrNamesTheProblem(t *testing.T) {
	t.Parallel()
	specs, out := apiSpecDir(t, map[string]string{"cabal.yaml": apisCabal}), t.TempDir()
	var stdout, stderr bytes.Buffer
	oapiCodegenState.Lock()
	code := gen([]string{"apis", specs, out}, &stdout, &stderr)
	missing := gen([]string{"apis", filepath.Join(specs, "nope"), out}, &bytes.Buffer{}, &stderr)
	oapiCodegenState.Unlock()
	if code != 0 || !strings.Contains(stdout.String(), "wrote the API packages under "+out) {
		t.Fatalf("gen apis = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "cabalapi", mountGenFile)); err != nil {
		t.Fatalf("gen apis wrote no cabalapi: %v", err)
	}
	if missing != 1 || !strings.Contains(stderr.String(), "monacoctl: ") {
		t.Fatalf("gen apis on a missing spec dir = %d, stderr %q", missing, stderr.String())
	}
}
